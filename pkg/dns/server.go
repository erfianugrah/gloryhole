package dns

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/pattern"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/ratelimit"
	"glory-hole/pkg/storage"
	"glory-hole/pkg/telemetry"

	"github.com/miekg/dns"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// msgPool provides object pooling for dns.Msg to reduce allocations
var msgPool = sync.Pool{
	New: func() interface{} {
		return new(dns.Msg)
	},
}

// Handler is a DNS handler
// KillSwitchChecker defines the interface for checking temporary disable state
type KillSwitchChecker interface {
	IsBlocklistDisabled() (disabled bool, until time.Time)
	IsPoliciesDisabled() (disabled bool, until time.Time)
}

type Handler struct {
	Storage           storage.Storage
	BlocklistManager  *blocklist.Manager
	Blocklist         map[string]struct{}
	Whitelist         map[string]struct{}
	WhitelistPatterns atomic.Pointer[pattern.Matcher] // Pattern-based whitelist (wildcard/regex)
	Overrides         map[string]net.IP
	CNAMEOverrides    map[string]string
	LocalRecords      *localrecords.Manager
	PolicyEngine      *policy.Engine
	RuleEvaluator     *forwarder.RuleEvaluator
	Forwarder         *forwarder.Forwarder
	Cache             *cache.Cache
	ConfigWatcher     *config.Watcher   // For kill-switch feature (hot-reload config access)
	KillSwitch        KillSwitchChecker // For duration-based temporary disabling
	RateLimiter       *ratelimit.Manager
	Metrics           *telemetry.Metrics
	Logger            *logging.Logger
	lookupMu          sync.RWMutex
}

// NewHandler creates a new DNS handler
func NewHandler() *Handler {
	return &Handler{
		Blocklist:      make(map[string]struct{}),
		Whitelist:      make(map[string]struct{}),
		Overrides:      make(map[string]net.IP),
		CNAMEOverrides: make(map[string]string),
	}
}

// SetForwarder sets the upstream DNS forwarder
func (h *Handler) SetForwarder(f *forwarder.Forwarder) {
	h.Forwarder = f
}

// SetCache sets the DNS response cache
func (h *Handler) SetCache(c *cache.Cache) {
	h.Cache = c
}

// SetBlocklistManager sets the blocklist manager (lock-free, high performance)
func (h *Handler) SetBlocklistManager(m *blocklist.Manager) {
	h.BlocklistManager = m
}

// SetStorage sets the query logging storage
func (h *Handler) SetStorage(s storage.Storage) {
	h.Storage = s
}

// SetLocalRecords sets the local DNS records manager
func (h *Handler) SetLocalRecords(l *localrecords.Manager) {
	h.LocalRecords = l
}

// SetPolicyEngine sets the policy engine
func (h *Handler) SetPolicyEngine(e *policy.Engine) {
	h.PolicyEngine = e
}

// SetMetrics sets the metrics collector
func (h *Handler) SetMetrics(m *telemetry.Metrics) {
	h.Metrics = m
}

// SetLogger sets the logger
func (h *Handler) SetLogger(l *logging.Logger) {
	h.Logger = l
}

// SetKillSwitch sets the kill-switch manager for duration-based temporary disabling
func (h *Handler) SetKillSwitch(ks KillSwitchChecker) {
	h.KillSwitch = ks
}

// SetRateLimiter wires a rate limiter implementation.
func (h *Handler) SetRateLimiter(rl *ratelimit.Manager) {
	h.RateLimiter = rl
}

// writeMsg writes a DNS message to the response writer with error handling
// If the write fails (e.g., client disconnected), the error is silently ignored
// as there's no way to notify the client at that point
func (h *Handler) writeMsg(w dns.ResponseWriter, msg *dns.Msg) {
	if err := w.WriteMsg(msg); err != nil {
		// Client likely disconnected - nothing we can do
		// Telemetry will track the overall error rate if needed
		_ = err
	}
}

// ServeDNS implements the dns.Handler interface
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
	// Track query start time and details for logging
	startTime := time.Now()
	var blocked, cached bool
	var upstream string
	var responseCode int
	clientIP := getClientIP(w)

	// Async logging at the end (non-blocking, <10µs overhead)
	defer func() {
		if h.Storage != nil {
			// Extract domain and query type
			domain := ""
			queryType := ""
			if len(r.Question) > 0 {
				domain = strings.TrimSuffix(r.Question[0].Name, ".")
				queryType = dnsTypeLabel(r.Question[0].Qtype)
			}

			// Log query asynchronously (fire and forget)
			go func() {
				logCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()

				queryLog := &storage.QueryLog{
					Timestamp:      startTime,
					ClientIP:       clientIP,
					Domain:         domain,
					QueryType:      queryType,
					ResponseCode:   responseCode,
					Blocked:        blocked,
					Cached:         cached,
					ResponseTimeMs: time.Since(startTime).Milliseconds(),
					Upstream:       upstream,
				}

				// Log query to storage (fire and forget, but log errors)
				if err := h.Storage.LogQuery(logCtx, queryLog); err != nil {
					// Don't let logging errors affect DNS service, but log them
					if h.Logger != nil {
						h.Logger.Error("Failed to log query to storage",
							"domain", domain,
							"client_ip", clientIP,
							"error", err)
					}
				}
			}()
		}
	}()

	// Create response message
	// Note: We don't pool these because ResponseWriter may hold references
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true
	msg.RecursionAvailable = true

	// Handle EDNS0 (RFC 6891) - must be done early to apply to all responses
	HandleEDNS0(r, msg)

	// Validate request
	if len(r.Question) == 0 {
		msg.SetRcode(r, dns.RcodeFormatError)
		responseCode = dns.RcodeFormatError
		h.writeMsg(w, msg)
		return
	}

	question := r.Question[0]
	domain := question.Name
	qtype := question.Qtype
	qtypeLabel := dnsTypeLabel(qtype)

	// Enforce optional rate limiting before expensive work
	if h.RateLimiter != nil {
		if allowed, limited, action := h.RateLimiter.Allow(clientIP); !allowed && limited {
			dropped := action == config.RateLimitActionDrop
			h.recordRateLimit(ctx, clientIP, qtypeLabel, string(action), dropped)
			if h.RateLimiter.LogViolations() && h.Logger != nil {
				h.Logger.Warn("Rate limit exceeded",
					"client_ip", clientIP,
					"domain", domain,
					"action", action,
					"query_type", qtypeLabel,
				)
			}

			if dropped {
				responseCode = dns.RcodeRefused
				return
			}

			responseCode = dns.RcodeNameError
			msg.SetRcode(r, dns.RcodeNameError)
			h.writeMsg(w, msg)
			return
		}
	}

	// Check cache first (before any lookups)
	// Cache lookups are fast (~100ns) and can save upstream roundtrip (~10ms)
	if h.Cache != nil {
		if cachedResp := h.Cache.Get(ctx, r); cachedResp != nil {
			// Important: Update the message ID to match the query
			// Cached responses have the original query's ID, but we need this query's ID
			cachedResp.Id = r.Id

			// Handle EDNS0 for cached response
			HandleEDNS0(r, cachedResp)

			cached = true
			responseCode = cachedResp.Rcode
			h.writeMsg(w, cachedResp)
			return
		}
	}

	// Check local records (highest priority for authoritative answers)
	// Local records are for custom domain resolution (e.g., nas.local -> 192.168.1.100)
	if h.LocalRecords != nil {
		switch qtype {
		case dns.TypeA:
			// Check for direct A record
			if ips, ttl, found := h.LocalRecords.LookupA(domain); found {
				for _, ip := range ips {
					if ip.To4() != nil {
						rr := &dns.A{
							Hdr: dns.RR_Header{
								Name:   domain,
								Rrtype: dns.TypeA,
								Class:  dns.ClassINET,
								Ttl:    ttl,
							},
							A: ip.To4(),
						}
						msg.Answer = append(msg.Answer, rr)
					}
				}
				if len(msg.Answer) > 0 {
					responseCode = dns.RcodeSuccess
					h.writeMsg(w, msg)
					return
				}
			}

			// Check for CNAME and resolve it
			if ips, ttl, found := h.LocalRecords.ResolveCNAME(domain, 10); found {
				for _, ip := range ips {
					if ip.To4() != nil {
						rr := &dns.A{
							Hdr: dns.RR_Header{
								Name:   domain,
								Rrtype: dns.TypeA,
								Class:  dns.ClassINET,
								Ttl:    ttl,
							},
							A: ip.To4(),
						}
						msg.Answer = append(msg.Answer, rr)
					}
				}
				if len(msg.Answer) > 0 {
					responseCode = dns.RcodeSuccess
					h.writeMsg(w, msg)
					return
				}
			}

		case dns.TypeAAAA:
			// Check for direct AAAA record
			if ips, ttl, found := h.LocalRecords.LookupAAAA(domain); found {
				for _, ip := range ips {
					if ip.To4() == nil {
						rr := &dns.AAAA{
							Hdr: dns.RR_Header{
								Name:   domain,
								Rrtype: dns.TypeAAAA,
								Class:  dns.ClassINET,
								Ttl:    ttl,
							},
							AAAA: ip.To16(),
						}
						msg.Answer = append(msg.Answer, rr)
					}
				}
				if len(msg.Answer) > 0 {
					responseCode = dns.RcodeSuccess
					h.writeMsg(w, msg)
					return
				}
			}

			// Check for CNAME and resolve it (may return IPv6)
			if ips, ttl, found := h.LocalRecords.ResolveCNAME(domain, 10); found {
				for _, ip := range ips {
					if ip.To4() == nil {
						rr := &dns.AAAA{
							Hdr: dns.RR_Header{
								Name:   domain,
								Rrtype: dns.TypeAAAA,
								Class:  dns.ClassINET,
								Ttl:    ttl,
							},
							AAAA: ip.To16(),
						}
						msg.Answer = append(msg.Answer, rr)
					}
				}
				if len(msg.Answer) > 0 {
					responseCode = dns.RcodeSuccess
					h.writeMsg(w, msg)
					return
				}
			}

		case dns.TypeCNAME:
			// Check for CNAME record
			if target, ttl, found := h.LocalRecords.LookupCNAME(domain); found {
				rr := &dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeCNAME,
						Class:  dns.ClassINET,
						Ttl:    ttl,
					},
					Target: target,
				}
				msg.Answer = append(msg.Answer, rr)
				responseCode = dns.RcodeSuccess
				h.writeMsg(w, msg)
				return
			}

		case dns.TypeTXT:
			// Check for TXT records
			records := h.LocalRecords.LookupTXT(domain)
			if len(records) > 0 {
				for _, rec := range records {
					rr := &dns.TXT{
						Hdr: dns.RR_Header{
							Name:   domain,
							Rrtype: dns.TypeTXT,
							Class:  dns.ClassINET,
							Ttl:    rec.TTL,
						},
						Txt: rec.TxtRecords,
					}
					msg.Answer = append(msg.Answer, rr)
				}
				responseCode = dns.RcodeSuccess
				h.writeMsg(w, msg)
				return
			}

		case dns.TypeMX:
			// Check for MX records
			records := h.LocalRecords.LookupMX(domain)
			if len(records) > 0 {
				for _, rec := range records {
					rr := &dns.MX{
						Hdr: dns.RR_Header{
							Name:   domain,
							Rrtype: dns.TypeMX,
							Class:  dns.ClassINET,
							Ttl:    rec.TTL,
						},
						Preference: rec.Priority,
						Mx:         rec.Target,
					}
					msg.Answer = append(msg.Answer, rr)
				}
				responseCode = dns.RcodeSuccess
				h.writeMsg(w, msg)
				return
			}

		case dns.TypePTR:
			// Check for PTR records (reverse DNS)
			records := h.LocalRecords.LookupPTR(domain)
			if len(records) > 0 {
				for _, rec := range records {
					rr := &dns.PTR{
						Hdr: dns.RR_Header{
							Name:   domain,
							Rrtype: dns.TypePTR,
							Class:  dns.ClassINET,
							Ttl:    rec.TTL,
						},
						Ptr: rec.Target,
					}
					msg.Answer = append(msg.Answer, rr)
				}
				responseCode = dns.RcodeSuccess
				h.writeMsg(w, msg)
				return
			}

		case dns.TypeSRV:
			// Check for SRV records (service discovery)
			records := h.LocalRecords.LookupSRV(domain)
			if len(records) > 0 {
				for _, rec := range records {
					rr := &dns.SRV{
						Hdr: dns.RR_Header{
							Name:   domain,
							Rrtype: dns.TypeSRV,
							Class:  dns.ClassINET,
							Ttl:    rec.TTL,
						},
						Priority: rec.Priority,
						Weight:   rec.Weight,
						Port:     rec.Port,
						Target:   rec.Target,
					}
					msg.Answer = append(msg.Answer, rr)
				}
				responseCode = dns.RcodeSuccess
				h.writeMsg(w, msg)
				return
			}

		case dns.TypeNS:
			// Check for NS records (nameserver records)
			records := h.LocalRecords.LookupNS(domain)
			if len(records) > 0 {
				for _, rec := range records {
					rr := &dns.NS{
						Hdr: dns.RR_Header{
							Name:   domain,
							Rrtype: dns.TypeNS,
							Class:  dns.ClassINET,
							Ttl:    rec.TTL,
						},
						Ns: rec.Target,
					}
					msg.Answer = append(msg.Answer, rr)
				}
				responseCode = dns.RcodeSuccess
				h.writeMsg(w, msg)
				return
			}

		case dns.TypeSOA:
			// Check for SOA records (Start of Authority)
			records := h.LocalRecords.LookupSOA(domain)
			if len(records) > 0 {
				// Typically only one SOA per zone
				rec := records[0]
				rr := &dns.SOA{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeSOA,
						Class:  dns.ClassINET,
						Ttl:    rec.TTL,
					},
					Ns:      rec.Ns,
					Mbox:    rec.Mbox,
					Serial:  rec.Serial,
					Refresh: rec.Refresh,
					Retry:   rec.Retry,
					Expire:  rec.Expire,
					Minttl:  rec.Minttl,
				}
				msg.Answer = append(msg.Answer, rr)
				responseCode = dns.RcodeSuccess
				h.writeMsg(w, msg)
				return
			}

		case dns.TypeCAA:
			// Check for CAA records (Certificate Authority Authorization)
			records := h.LocalRecords.LookupCAA(domain)
			if len(records) > 0 {
				for _, rec := range records {
					rr := &dns.CAA{
						Hdr: dns.RR_Header{
							Name:   domain,
							Rrtype: dns.TypeCAA,
							Class:  dns.ClassINET,
							Ttl:    rec.TTL,
						},
						Flag:  rec.CaaFlag,
						Tag:   rec.CaaTag,
						Value: rec.CaaValue,
					}
					msg.Answer = append(msg.Answer, rr)
				}
				responseCode = dns.RcodeSuccess
				h.writeMsg(w, msg)
				return
			}
		}
	}

	// Evaluate policy engine rules (if configured and enabled via kill-switch)
	// Policy engine allows complex filtering rules with expressions
	// Kill-switch check: Only evaluate if enable_policies is true in config
	// If ConfigWatcher is nil (in tests), assume policies are enabled
	//
	// Priority: Temporary disable (duration-based) > Permanent enable (config)
	enablePolicies := true
	enableBlocklist := true
	if h.ConfigWatcher != nil {
		cfg := h.ConfigWatcher.Config()
		enablePolicies = cfg.Server.EnablePolicies
		enableBlocklist = cfg.Server.EnableBlocklist
	}

	// Check temporary disable state (takes precedence over permanent enable)
	if h.KillSwitch != nil {
		if disabled, _ := h.KillSwitch.IsBlocklistDisabled(); disabled {
			enableBlocklist = false
		}
		if disabled, _ := h.KillSwitch.IsPoliciesDisabled(); disabled {
			enablePolicies = false
		}
	}

	if enablePolicies && h.PolicyEngine != nil && h.PolicyEngine.Count() > 0 {

		// Create policy context
		policyCtx := policy.NewContext(
			strings.TrimSuffix(domain, "."),
			clientIP,
			qtypeLabel,
		)

		// Evaluate rules
		matched, rule := h.PolicyEngine.Evaluate(policyCtx)
		if matched && rule != nil {
			// Log policy match
			if h.Logger != nil {
				h.Logger.Info("Policy rule matched",
					"rule", rule.Name,
					"action", rule.Action,
					"domain", domain,
					"client_ip", clientIP,
					"query_type", qtypeLabel)
			}

			switch rule.Action {
			case policy.ActionBlock:
				// Block the request
				blocked = true
				responseCode = dns.RcodeNameError
				msg.SetRcode(r, dns.RcodeNameError)

				// Record blocked query metric
				h.recordBlockedQuery(ctx, "policy_block", qtypeLabel)

				// Log block action
				if h.Logger != nil {
					h.Logger.Debug("Policy blocked query",
						"rule", rule.Name,
						"domain", domain,
						"client_ip", clientIP)
				}

				// Cache blocked response to avoid repeated policy evaluation
				if h.Cache != nil {
					h.Cache.SetBlocked(ctx, r, msg)
				}

				h.writeMsg(w, msg)
				return

			case policy.ActionAllow:
				// Allow the request - skip blocklist check and forward directly
				if h.Forwarder != nil {
					// Log allow action with warning that blocklist checks are bypassed
					if h.Logger != nil {
						h.Logger.Warn("Policy ALLOW action bypasses blocklist checks - forwarding directly to upstream",
							"rule", rule.Name,
							"domain", domain,
							"client_ip", clientIP,
							"bypasses_blocklist", true)
					}

					resp, err := h.Forwarder.Forward(ctx, r)
					if err != nil {
						// Log forward error
						if h.Logger != nil {
							h.Logger.Error("Failed to forward allowed query",
								"rule", rule.Name,
								"domain", domain,
								"error", err)
						}
						responseCode = dns.RcodeServerFailure
						msg.SetRcode(r, dns.RcodeServerFailure)
						h.writeMsg(w, msg)
						return
					}

					// Track upstream server
					upstreams := h.Forwarder.Upstreams()
					if len(upstreams) > 0 {
						upstream = upstreams[0]
					}

					// Record forwarded query metric
					h.recordForwardedQuery(ctx, "policy_allow", qtypeLabel, upstream)

					// Cache the response
					if h.Cache != nil {
						h.Cache.Set(ctx, r, resp)
					}

					responseCode = resp.Rcode
					h.writeMsg(w, resp)
					return
				}
				// No forwarder, return NXDOMAIN
				if h.Logger != nil {
					h.Logger.Warn("Policy allow action but no forwarder configured",
						"rule", rule.Name,
						"domain", domain)
				}
				responseCode = dns.RcodeNameError
				msg.SetRcode(r, dns.RcodeNameError)
				h.writeMsg(w, msg)
				return

			case policy.ActionRedirect:
				// Redirect to specified IP address
				redirectIP := net.ParseIP(rule.ActionData)
				if redirectIP == nil {
					// Invalid redirect IP, log and treat as block
					if h.Logger != nil {
						h.Logger.Error("Policy redirect has invalid IP address",
							"rule", rule.Name,
							"domain", domain,
							"action_data", rule.ActionData)
					}
					responseCode = dns.RcodeNameError
					msg.SetRcode(r, dns.RcodeNameError)
					h.writeMsg(w, msg)
					return
				}

				// Log redirect action
				if h.Logger != nil {
					h.Logger.Debug("Policy redirecting query",
						"rule", rule.Name,
						"domain", domain,
						"redirect_ip", redirectIP.String(),
						"query_type", qtypeLabel)
				}

				// Create response based on query type and IP version
				if qtype == dns.TypeA && redirectIP.To4() != nil {
					// IPv4 A record
					rr := &dns.A{
						Hdr: dns.RR_Header{
							Name:   domain,
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    300, // 5 minutes TTL for redirects
						},
						A: redirectIP.To4(),
					}
					msg.Answer = append(msg.Answer, rr)
					responseCode = dns.RcodeSuccess
				} else if qtype == dns.TypeAAAA && redirectIP.To4() == nil {
					// IPv6 AAAA record
					rr := &dns.AAAA{
						Hdr: dns.RR_Header{
							Name:   domain,
							Rrtype: dns.TypeAAAA,
							Class:  dns.ClassINET,
							Ttl:    300,
						},
						AAAA: redirectIP,
					}
					msg.Answer = append(msg.Answer, rr)
					responseCode = dns.RcodeSuccess
				} else {
					// Query type doesn't match redirect IP version, return NODATA
					responseCode = dns.RcodeSuccess
				}

				// Cache the redirect response
				if h.Cache != nil {
					h.Cache.Set(ctx, r, msg)
				}

				h.writeMsg(w, msg)
				return

			case policy.ActionForward:
				// Forward to specific upstream servers from policy rule
				upstreams := rule.GetUpstreams()
				if len(upstreams) == 0 || h.Forwarder == nil {
					// No upstreams configured or no forwarder available
					if h.Logger != nil {
						h.Logger.Error("Policy forward action has no upstreams configured",
							"rule", rule.Name,
							"domain", domain)
					}
					responseCode = dns.RcodeServerFailure
					msg.SetRcode(r, dns.RcodeServerFailure)
					h.writeMsg(w, msg)
					return
				}

				// Log forward action
				if h.Logger != nil {
					h.Logger.Debug("Policy forwarding query to specific upstreams",
						"rule", rule.Name,
						"domain", domain,
						"upstreams", upstreams)
				}

				// Forward to conditional upstreams
				resp, err := h.Forwarder.ForwardWithUpstreams(ctx, r, upstreams)
				if err != nil {
					if h.Logger != nil {
						h.Logger.Error("Failed to forward query to policy upstreams",
							"rule", rule.Name,
							"domain", domain,
							"upstreams", upstreams,
							"error", err)
					}
					responseCode = dns.RcodeServerFailure
					msg.SetRcode(r, dns.RcodeServerFailure)
					h.writeMsg(w, msg)
					return
				}

				// Track upstream server
				if len(upstreams) > 0 {
					upstream = upstreams[0]
				}

				// Record forwarded query metric
				h.recordForwardedQuery(ctx, "conditional_rule", qtypeLabel, upstream)

				// Cache the response
				if h.Cache != nil {
					h.Cache.Set(ctx, r, resp)
				}

				responseCode = resp.Rcode
				h.writeMsg(w, resp)
				return
			}
		}
	}

	// FAST PATH: Use lock-free blocklist manager if available
	// This path is ~10x faster than the locked path below (~10ns vs ~110ns)
	// Kill-switch check: Only check blocklist if enable_blocklist is true in config
	var whitelisted bool

	if enableBlocklist && h.BlocklistManager != nil {
		// Lock-free atomic pointer read - blazing fast!
		blocked = h.BlocklistManager.IsBlocked(domain)

		// Check whitelist/overrides with read lock
		// Note: Could be optimized with atomic pointer in future if needed
		h.lookupMu.RLock()
		_, whitelisted = h.Whitelist[domain]
		h.lookupMu.RUnlock()

		// Check whitelist patterns if not exact match
		if !whitelisted {
			patterns := h.WhitelistPatterns.Load()
			if patterns != nil {
				whitelisted = patterns.Match(domain)
			}
		}

		// Override blocklist if whitelisted
		if whitelisted {
			blocked = false
		}

		h.lookupMu.RLock() // Re-acquire lock for subsequent map reads

		// Check local overrides for A/AAAA records
		var overrideIP net.IP
		var hasOverride bool
		if !blocked && (qtype == dns.TypeA || qtype == dns.TypeAAAA) {
			overrideIP, hasOverride = h.Overrides[domain]
		}

		// Check CNAME overrides
		var cnameTarget string
		var hasCNAME bool
		if !blocked && !hasOverride && (qtype == dns.TypeCNAME || qtype == dns.TypeA || qtype == dns.TypeAAAA) {
			cnameTarget, hasCNAME = h.CNAMEOverrides[domain]
		}

		h.lookupMu.RUnlock()

		// Handle results (shared with slow path below)
		if blocked {
			responseCode = dns.RcodeNameError
			msg.SetRcode(r, dns.RcodeNameError)

			// Record blocked query metric
			h.recordBlockedQuery(ctx, "blocklist_manager", qtypeLabel)

			// Cache blocked response to avoid repeated blocklist lookups
			if h.Cache != nil {
				h.Cache.SetBlocked(ctx, r, msg)
			}

			h.writeMsg(w, msg)
			return
		}

		if hasOverride {
			if qtype == dns.TypeA && overrideIP.To4() != nil {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: overrideIP.To4(),
				}
				msg.Answer = append(msg.Answer, rr)
			} else if qtype == dns.TypeAAAA && overrideIP.To16() != nil && overrideIP.To4() == nil {
				rr := &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					AAAA: overrideIP.To16(),
				}
				msg.Answer = append(msg.Answer, rr)
			}
			h.writeMsg(w, msg)
			return
		}

		if hasCNAME {
			rr := &dns.CNAME{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeCNAME,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				Target: cnameTarget,
			}
			msg.Answer = append(msg.Answer, rr)
			h.writeMsg(w, msg)
			return
		}
	} else if enableBlocklist {
		// SLOW PATH: Use legacy locked map lookups (backward compatibility)
		// Only used if BlocklistManager is nil but kill-switch is enabled
		// Single lock for all map lookups (performance optimization)
		h.lookupMu.RLock()

		// Check whitelist first (always allow)
		_, whitelisted = h.Whitelist[domain]
		h.lookupMu.RUnlock()

		// Check whitelist patterns if not exact match
		if !whitelisted {
			patterns := h.WhitelistPatterns.Load()
			if patterns != nil {
				whitelisted = patterns.Match(domain)
			}
		}

		// Check blocklist (if not whitelisted)
		if !whitelisted {
			h.lookupMu.RLock()
			_, blocked = h.Blocklist[domain]
			h.lookupMu.RUnlock()
		}

		h.lookupMu.RLock() // Re-acquire lock for subsequent map reads

		// Check local overrides for A/AAAA records
		var overrideIP net.IP
		var hasOverride bool
		if !blocked && (qtype == dns.TypeA || qtype == dns.TypeAAAA) {
			overrideIP, hasOverride = h.Overrides[domain]
		}

		// Check CNAME overrides
		var cnameTarget string
		var hasCNAME bool
		if !blocked && !hasOverride && (qtype == dns.TypeCNAME || qtype == dns.TypeA || qtype == dns.TypeAAAA) {
			cnameTarget, hasCNAME = h.CNAMEOverrides[domain]
		}

		h.lookupMu.RUnlock()
		// All lookups done - lock released

		// Handle results (duplicate code for simplicity)
		if blocked {
			responseCode = dns.RcodeNameError
			msg.SetRcode(r, dns.RcodeNameError)

			// Record blocked query metric
			h.recordBlockedQuery(ctx, "blocklist_legacy", qtypeLabel)

			// Cache blocked response to avoid repeated blocklist lookups
			if h.Cache != nil {
				h.Cache.SetBlocked(ctx, r, msg)
			}

			h.writeMsg(w, msg)
			return
		}

		if hasOverride {
			if qtype == dns.TypeA && overrideIP.To4() != nil {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: overrideIP.To4(),
				}
				msg.Answer = append(msg.Answer, rr)
			} else if qtype == dns.TypeAAAA && overrideIP.To16() != nil && overrideIP.To4() == nil {
				rr := &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					AAAA: overrideIP.To16(),
				}
				msg.Answer = append(msg.Answer, rr)
			}
			h.writeMsg(w, msg)
			return
		}

		if hasCNAME {
			rr := &dns.CNAME{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeCNAME,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				Target: cnameTarget,
			}
			msg.Answer = append(msg.Answer, rr)
			h.writeMsg(w, msg)
			return
		}
	}

	// If we get here, we don't have a local answer (not blocked, no overrides)

	// Check conditional forwarding rules
	if h.RuleEvaluator != nil && !h.RuleEvaluator.IsEmpty() && h.Forwarder != nil {
		// Evaluate conditional forwarding rules
		upstreams := h.RuleEvaluator.Evaluate(
			strings.TrimSuffix(domain, "."),
			clientIP,
			qtypeLabel,
		)

		if len(upstreams) > 0 {
			// Log conditional forwarding match
			if h.Logger != nil {
				h.Logger.Debug("Conditional forwarding rule matched",
					"domain", domain,
					"client_ip", clientIP,
					"upstreams", upstreams,
					"query_type", qtypeLabel)
			}

			// Rule matched - forward to conditional upstreams
			resp, err := h.Forwarder.ForwardWithUpstreams(ctx, r, upstreams)
			if err != nil {
				// Forwarding failed, return SERVFAIL
				if h.Logger != nil {
					h.Logger.Error("Conditional forwarding failed",
						"domain", domain,
						"upstreams", upstreams,
						"error", err)
				}
				responseCode = dns.RcodeServerFailure
				msg.SetRcode(r, dns.RcodeServerFailure)
				h.writeMsg(w, msg)
				return
			}

			// Track upstream server
			if len(upstreams) > 0 {
				upstream = upstreams[0]
			}

			// Record forwarded query metric
			h.recordForwardedQuery(ctx, "policy_forward", qtypeLabel, upstream)

			// Cache the successful response
			if h.Cache != nil {
				h.Cache.Set(ctx, r, resp)
			}

			responseCode = resp.Rcode
			h.writeMsg(w, resp)
			return
		}
	}

	// Forward to upstream DNS
	if h.Forwarder != nil {
		resp, err := h.Forwarder.Forward(ctx, r)
		if err != nil {
			// Forwarding failed, return SERVFAIL
			responseCode = dns.RcodeServerFailure
			msg.SetRcode(r, dns.RcodeServerFailure)
			h.writeMsg(w, msg)
			return
		}

		// Track upstream server used (approximation - before the query)
		upstreams := h.Forwarder.Upstreams()
		if len(upstreams) > 0 {
			// Note: This is an approximation since we don't know which exact upstream was used
			// The forwarder uses round-robin and retries, so this may not be 100% accurate
			upstream = upstreams[0] // Just use the first upstream as a placeholder
		}

		// Record forwarded query metric
		h.recordForwardedQuery(ctx, "default_forward", qtypeLabel, upstream)

		// Cache the upstream response (if cache is enabled)
		// This includes both successful responses and negative responses (NXDOMAIN)
		if h.Cache != nil {
			h.Cache.Set(ctx, r, resp)
		}

		// Return the upstream response
		responseCode = resp.Rcode
		h.writeMsg(w, resp)
		return
	}

	// No forwarder configured, return NXDOMAIN
	responseCode = dns.RcodeNameError
	msg.SetRcode(r, dns.RcodeNameError)
	h.writeMsg(w, msg)
}

// dnsTypeLabel returns a human-readable string for the query type, falling back to TYPE#### per RFC 3597 when unknown.
func dnsTypeLabel(qtype uint16) string {
	if label := dns.TypeToString[qtype]; label != "" {
		return label
	}
	return "TYPE" + strconv.FormatUint(uint64(qtype), 10)
}

// recordRateLimit captures rate limit violations and drops with consistent attributes.
func (h *Handler) recordRateLimit(ctx context.Context, clientIP, qtypeLabel, action string, dropped bool) {
	if h.Metrics == nil {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 3)
	if clientIP != "" {
		attrs = append(attrs, attribute.String("client", clientIP))
	}
	if qtypeLabel != "" {
		attrs = append(attrs, attribute.String("type", qtypeLabel))
	}
	if action != "" {
		attrs = append(attrs, attribute.String("action", action))
	}
	h.Metrics.RateLimitViolations.Add(ctx, 1, metric.WithAttributes(attrs...))
	if dropped {
		h.Metrics.RateLimitDropped.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

// recordBlockedQuery increments the blocked-query counter with contextual attributes for better observability.
func (h *Handler) recordBlockedQuery(ctx context.Context, reason, qtypeLabel string) {
	if h.Metrics == nil {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 2)
	if reason != "" {
		attrs = append(attrs, attribute.String("reason", reason))
	}
	if qtypeLabel != "" {
		attrs = append(attrs, attribute.String("type", qtypeLabel))
	}
	if len(attrs) == 0 {
		h.Metrics.DNSBlockedQueries.Add(ctx, 1)
		return
	}
	h.Metrics.DNSBlockedQueries.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// recordForwardedQuery increments the forwarded-query counter tagged with path/upstream metadata.
func (h *Handler) recordForwardedQuery(ctx context.Context, path, qtypeLabel, upstream string) {
	if h.Metrics == nil {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 3)
	if path != "" {
		attrs = append(attrs, attribute.String("path", path))
	}
	if qtypeLabel != "" {
		attrs = append(attrs, attribute.String("type", qtypeLabel))
	}
	if upstream != "" {
		attrs = append(attrs, attribute.String("upstream", upstream))
	}
	if len(attrs) == 0 {
		h.Metrics.DNSForwardedQueries.Add(ctx, 1)
		return
	}
	h.Metrics.DNSForwardedQueries.Add(ctx, 1, metric.WithAttributes(attrs...))
}
