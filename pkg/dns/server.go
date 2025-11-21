package dns

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/cache"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/storage"

	"github.com/miekg/dns"
)

// msgPool provides object pooling for dns.Msg to reduce allocations
var msgPool = sync.Pool{
	New: func() interface{} {
		return new(dns.Msg)
	},
}

// Handler is a DNS handler
type Handler struct {
	// Single lock for all lookup maps (performance optimization)
	// Using one lock instead of 4 separate locks reduces overhead from ~2-4μs to ~500ns
	// Note: BlocklistManager uses lock-free atomic.Pointer, so no lock needed for blocklist lookups
	lookupMu sync.RWMutex

	// Blocklist manager with lock-free updates (FAST PATH - preferred)
	BlocklistManager *blocklist.Manager

	// Legacy static maps (SLOW PATH - backward compatibility)
	Blocklist      map[string]struct{}
	Whitelist      map[string]struct{}
	Overrides      map[string]net.IP
	CNAMEOverrides map[string]string

	// Local DNS records manager (e.g., nas.local -> 192.168.1.100)
	LocalRecords *localrecords.Manager // Optional local DNS records

	// Policy engine for advanced rule-based filtering
	PolicyEngine *policy.Engine // Optional policy engine

	Forwarder *forwarder.Forwarder
	Cache     *cache.Cache    // Optional DNS response cache
	Storage   storage.Storage // Optional query logging storage
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

// ServeDNS implements the dns.Handler interface
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
	// Track query start time and details for logging
	startTime := time.Now()
	var blocked, cached bool
	var upstream string
	var responseCode int

	// Async logging at the end (non-blocking, <10µs overhead)
	defer func() {
		if h.Storage != nil {
			// Extract domain and query type
			domain := ""
			queryType := ""
			if len(r.Question) > 0 {
				domain = strings.TrimSuffix(r.Question[0].Name, ".")
				queryType = dns.TypeToString[r.Question[0].Qtype]
			}

			// Get client IP
			clientIP := ""
			if addr, ok := w.RemoteAddr().(*net.UDPAddr); ok {
				clientIP = addr.IP.String()
			} else if addr, ok := w.RemoteAddr().(*net.TCPAddr); ok {
				clientIP = addr.IP.String()
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

				if err := h.Storage.LogQuery(logCtx, queryLog); err != nil {
					// Silently fail - don't let logging errors affect DNS service
					// In production, this would go to a separate error log
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

	// Validate request
	if len(r.Question) == 0 {
		msg.SetRcode(r, dns.RcodeFormatError)
		responseCode = dns.RcodeFormatError
		w.WriteMsg(msg)
		return
	}

	question := r.Question[0]
	domain := question.Name
	qtype := question.Qtype

	// Check cache first (before any lookups)
	// Cache lookups are fast (~100ns) and can save upstream roundtrip (~10ms)
	if h.Cache != nil {
		if cachedResp := h.Cache.Get(ctx, r); cachedResp != nil {
			// Important: Update the message ID to match the query
			// Cached responses have the original query's ID, but we need this query's ID
			cachedResp.Id = r.Id
			cached = true
			responseCode = cachedResp.Rcode
			w.WriteMsg(cachedResp)
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
					w.WriteMsg(msg)
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
					w.WriteMsg(msg)
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
					w.WriteMsg(msg)
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
					w.WriteMsg(msg)
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
				w.WriteMsg(msg)
				return
			}
		}
	}

	// Evaluate policy engine rules (if configured)
	// Policy engine allows complex filtering rules with expressions
	if h.PolicyEngine != nil && h.PolicyEngine.Count() > 0 {
		// Get client IP for policy evaluation
		clientIP := ""
		if addr, ok := w.RemoteAddr().(*net.UDPAddr); ok {
			clientIP = addr.IP.String()
		} else if addr, ok := w.RemoteAddr().(*net.TCPAddr); ok {
			clientIP = addr.IP.String()
		}

		// Create policy context
		policyCtx := policy.NewContext(
			strings.TrimSuffix(domain, "."),
			clientIP,
			dns.TypeToString[qtype],
		)

		// Evaluate rules
		matched, rule := h.PolicyEngine.Evaluate(policyCtx)
		if matched && rule != nil {
			switch rule.Action {
			case policy.ActionBlock:
				// Block the request
				blocked = true
				responseCode = dns.RcodeNameError
				msg.SetRcode(r, dns.RcodeNameError)
				w.WriteMsg(msg)
				return

			case policy.ActionAllow:
				// Allow the request - skip blocklist check and forward directly
				if h.Forwarder != nil {
					resp, err := h.Forwarder.Forward(ctx, r)
					if err != nil {
						responseCode = dns.RcodeServerFailure
						msg.SetRcode(r, dns.RcodeServerFailure)
						w.WriteMsg(msg)
						return
					}

					// Track upstream server
					upstreams := h.Forwarder.Upstreams()
					if len(upstreams) > 0 {
						upstream = upstreams[0]
					}

					// Cache the response
					if h.Cache != nil {
						h.Cache.Set(ctx, r, resp)
					}

					responseCode = resp.Rcode
					w.WriteMsg(resp)
					return
				}
				// No forwarder, return NXDOMAIN
				responseCode = dns.RcodeNameError
				msg.SetRcode(r, dns.RcodeNameError)
				w.WriteMsg(msg)
				return

			case policy.ActionRedirect:
				// Redirect to specified IP address
				redirectIP := net.ParseIP(rule.ActionData)
				if redirectIP == nil {
					// Invalid redirect IP, log and treat as block
					responseCode = dns.RcodeNameError
					msg.SetRcode(r, dns.RcodeNameError)
					w.WriteMsg(msg)
					return
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

				w.WriteMsg(msg)
				return
			}
		}
	}

	// FAST PATH: Use lock-free blocklist manager if available
	// This path is ~10x faster than the locked path below (~10ns vs ~110ns)
	var whitelisted bool

	if h.BlocklistManager != nil {
		// Lock-free atomic pointer read - blazing fast!
		blocked = h.BlocklistManager.IsBlocked(domain)

		// Still need to check whitelist/overrides with lock (for now)
		// TODO: Move whitelist to atomic pointer for full lock-free operation
		h.lookupMu.RLock()
		_, whitelisted = h.Whitelist[domain]

		// Override blocklist if whitelisted
		if whitelisted {
			blocked = false
		}

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
			w.WriteMsg(msg)
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
			w.WriteMsg(msg)
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
			w.WriteMsg(msg)
			return
		}
	} else {
		// SLOW PATH: Use legacy locked map lookups (backward compatibility)
		// Single lock for all map lookups (performance optimization)
		h.lookupMu.RLock()

		// Check whitelist first (always allow)
		_, whitelisted = h.Whitelist[domain]

		// Check blocklist (if not whitelisted)
		if !whitelisted {
			_, blocked = h.Blocklist[domain]
		}

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
			w.WriteMsg(msg)
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
			w.WriteMsg(msg)
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
			w.WriteMsg(msg)
			return
		}
	}

	// If we get here, we don't have a local answer (not blocked, no overrides)
	// Forward to upstream DNS
	if h.Forwarder != nil {
		resp, err := h.Forwarder.Forward(ctx, r)
		if err != nil {
			// Forwarding failed, return SERVFAIL
			responseCode = dns.RcodeServerFailure
			msg.SetRcode(r, dns.RcodeServerFailure)
			w.WriteMsg(msg)
			return
		}

		// Track upstream server used (approximation - before the query)
		upstreams := h.Forwarder.Upstreams()
		if len(upstreams) > 0 {
			// Note: This is an approximation since we don't know which exact upstream was used
			// The forwarder uses round-robin and retries, so this may not be 100% accurate
			upstream = upstreams[0] // Just use the first upstream as a placeholder
		}

		// Cache the upstream response (if cache is enabled)
		// This includes both successful responses and negative responses (NXDOMAIN)
		if h.Cache != nil {
			h.Cache.Set(ctx, r, resp)
		}

		// Return the upstream response
		responseCode = resp.Rcode
		w.WriteMsg(resp)
		return
	}

	// No forwarder configured, return NXDOMAIN
	responseCode = dns.RcodeNameError
	msg.SetRcode(r, dns.RcodeNameError)
	w.WriteMsg(msg)
}
