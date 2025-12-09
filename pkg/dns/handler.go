// Package dns contains the authoritative/recursive handler used by glory-hole,
// including DoH endpoints, blocklist enforcement, and local overrides.
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
	Whitelist         atomic.Pointer[map[string]struct{}] // Exact-match whitelist (hot-reloadable)
	WhitelistPatterns atomic.Pointer[pattern.Matcher]     // Pattern-based whitelist (wildcard/regex)
	Overrides         map[string]net.IP
	CNAMEOverrides    map[string]string
	LocalRecords      *localrecords.Manager
	PolicyEngine      *policy.Engine
	RuleEvaluator     *forwarder.RuleEvaluator
	Forwarder         *forwarder.Forwarder
	Cache             cache.Interface
	ConfigWatcher     *config.Watcher   // For kill-switch feature (hot-reload config access)
	KillSwitch        KillSwitchChecker // For duration-based temporary disabling
	DecisionTrace     bool
	RateLimiter       *ratelimit.Manager
	Metrics           *telemetry.Metrics
	Logger            *logging.Logger
	lookupMu          sync.RWMutex
}

// NewHandler creates a new DNS handler
func NewHandler() *Handler {
	h := &Handler{
		Blocklist:      make(map[string]struct{}),
		Overrides:      make(map[string]net.IP),
		CNAMEOverrides: make(map[string]string),
	}
	// Initialize Whitelist with empty map
	emptyWhitelist := make(map[string]struct{})
	h.Whitelist.Store(&emptyWhitelist)
	return h
}

// SetForwarder sets the upstream DNS forwarder
func (h *Handler) SetForwarder(f *forwarder.Forwarder) {
	h.Forwarder = f
}

// SetCache sets the DNS response cache
func (h *Handler) SetCache(c cache.Interface) {
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

// SetDecisionTrace enables or disables decision trace capture.
func (h *Handler) SetDecisionTrace(enabled bool) {
	h.DecisionTrace = enabled
}

// SetRateLimiter wires a rate limiter implementation.
func (h *Handler) SetRateLimiter(rl *ratelimit.Manager) {
	h.RateLimiter = rl
}

// If a policy explicitly uses RATE_LIMIT action, we defer rate limiting to that policy rule
// so operators can scope limits by domain/client/type. Otherwise apply the global limiter
// up front.
func (h *Handler) shouldDeferRateLimitToPolicies() bool {
	return h.PolicyEngine != nil && h.PolicyEngine.HasAction(policy.ActionRateLimit)
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
	startTime := time.Now()
	outcome := &serveDNSOutcome{}
	trace := newBlockTraceRecorder(h.DecisionTrace)
	clientIP := getClientIP(w)

	defer h.asyncLogQuery(startTime, r, clientIP, trace, outcome)

	msg := msgPool.Get().(*dns.Msg)
	defer msgPool.Put(msg)

	*msg = dns.Msg{}
	msg.SetReply(r)
	msg.Authoritative = true
	msg.RecursionAvailable = true
	HandleEDNS0(r, msg)

	if len(r.Question) == 0 {
		msg.SetRcode(r, dns.RcodeFormatError)
		outcome.responseCode = dns.RcodeFormatError
		h.writeMsg(w, msg)
		return
	}

	question := r.Question[0]
	domain := question.Name
	qtype := question.Qtype
	qtypeLabel := dnsTypeLabel(qtype)

	if !h.shouldDeferRateLimitToPolicies() && h.enforceRateLimit(ctx, w, r, msg, clientIP, domain, qtypeLabel, trace, outcome) {
		return
	}
	if h.serveFromCache(ctx, w, r, msg, trace, outcome) {
		return
	}
	if h.serveFromLocalRecords(w, msg, domain, qtype, outcome) {
		return
	}

	enablePolicies, enableBlocklist := h.resolveFeatureToggles()
	if h.KillSwitch != nil {
		if disabled, _ := h.KillSwitch.IsBlocklistDisabled(); disabled {
			enableBlocklist = false
		}
		if disabled, _ := h.KillSwitch.IsPoliciesDisabled(); disabled {
			enablePolicies = false
		}
	}

	if enablePolicies && h.PolicyEngine != nil && h.PolicyEngine.Count() > 0 {
		if h.handlePolicies(ctx, w, r, msg, domain, clientIP, qtype, qtypeLabel, trace, outcome) {
			return
		}
	}

	if enableBlocklist {
		if h.handleBlocklistAndOverrides(ctx, w, r, msg, domain, qtype, qtypeLabel, trace, outcome) {
			return
		}
	}

	if h.handleConditionalForwarding(ctx, w, r, msg, domain, clientIP, qtypeLabel, outcome) {
		return
	}

	if h.forwardToUpstream(ctx, w, r, msg, qtypeLabel, outcome) {
		return
	}

	outcome.responseCode = dns.RcodeNameError
	msg.SetRcode(r, dns.RcodeNameError)
	h.writeMsg(w, msg)
}

func (h *Handler) asyncLogQuery(startTime time.Time, r *dns.Msg, clientIP string, trace *blockTraceRecorder, outcome *serveDNSOutcome) {
	if h.Storage == nil {
		return
	}

	domain := ""
	queryType := ""
	if len(r.Question) > 0 {
		domain = strings.TrimSuffix(r.Question[0].Name, ".")
		queryType = dnsTypeLabel(r.Question[0].Qtype)
	}

	go func() {
		logCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		queryLog := &storage.QueryLog{
			Timestamp:      startTime,
			ClientIP:       clientIP,
			Domain:         domain,
			QueryType:      queryType,
			ResponseCode:   outcome.responseCode,
			Blocked:        outcome.blocked,
			Cached:         outcome.cached,
			ResponseTimeMs: time.Since(startTime).Seconds() * 1000,
			UpstreamTimeMs: outcome.upstreamDuration.Seconds() * 1000,
			Upstream:       outcome.upstream,
			BlockTrace:     trace.Entries(),
		}

		if err := h.Storage.LogQuery(logCtx, queryLog); err != nil && h.Logger != nil {
			h.Logger.Error("Failed to log query to storage",
				"domain", domain,
				"client_ip", clientIP,
				"error", err)
		}
	}()
}

func (h *Handler) resolveFeatureToggles() (enablePolicies, enableBlocklist bool) {
	enablePolicies = true
	enableBlocklist = true
	if h.ConfigWatcher == nil {
		return
	}
	cfg := h.ConfigWatcher.Config()
	return cfg.Server.EnablePolicies, cfg.Server.EnableBlocklist
}

// dnsTypeLabel returns a human-readable string for the query type, falling back to TYPE#### per RFC 3597 when unknown.
func dnsTypeLabel(qtype uint16) string {
	if label := dns.TypeToString[qtype]; label != "" {
		return label
	}
	return "TYPE" + strconv.FormatUint(uint64(qtype), 10)
}
