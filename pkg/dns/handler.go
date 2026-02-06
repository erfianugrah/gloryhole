// Package dns contains the authoritative/recursive handler used by glory-hole,
// including DoH endpoints, blocklist enforcement, and local overrides.
package dns

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"
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

// legacyLogRequest holds a query log request for the legacy storage path.
type legacyLogRequest struct {
	storage storage.Storage
	log     *storage.QueryLog
	logger  *logging.Logger
}

// legacyLogCh is a buffered channel for legacy query logging.
// Using a channel with workers avoids spawning a goroutine per query.
var legacyLogCh = make(chan legacyLogRequest, 10000)

func init() {
	// Start legacy log workers (4 workers to handle concurrent writes)
	for i := 0; i < 4; i++ {
		go legacyLogWorker()
	}
}

func legacyLogWorker() {
	for req := range legacyLogCh {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		if err := req.storage.LogQuery(ctx, req.log); err != nil && req.logger != nil {
			req.logger.Error("Failed to log query to storage",
				"domain", req.log.Domain,
				"error", err)
		}
		cancel()
	}
}

// Handler is a DNS handler
// KillSwitchChecker defines the interface for checking temporary disable state
type KillSwitchChecker interface {
	IsBlocklistDisabled() (disabled bool, until time.Time)
	IsPoliciesDisabled() (disabled bool, until time.Time)
}

type Handler struct {
	Storage          storage.Storage // Legacy: kept for backwards compatibility
	QueryLogger      *QueryLogger    // New: worker pool for async query logging
	BlocklistManager *blocklist.Manager
	Blocklist        map[string]struct{}
	Overrides        map[string]net.IP
	CNAMEOverrides   map[string]string
	LocalRecords     *localrecords.Manager
	PolicyEngine     *policy.Engine
	RuleEvaluator    *forwarder.RuleEvaluator
	Forwarder        *forwarder.Forwarder
	Cache            cache.Interface
	ConfigWatcher    *config.Watcher   // For kill-switch feature (hot-reload config access)
	KillSwitch       KillSwitchChecker // For duration-based temporary disabling
	DecisionTrace    bool
	Metrics          *telemetry.Metrics
	Logger           *logging.Logger
	lookupMu         sync.RWMutex
}

// NewHandler creates a new DNS handler
func NewHandler() *Handler {
	return &Handler{
		Blocklist:      make(map[string]struct{}),
		Overrides:      make(map[string]net.IP),
		CNAMEOverrides: make(map[string]string),
	}
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

// SetQueryLogger sets the query logger worker pool
func (h *Handler) SetQueryLogger(ql *QueryLogger) {
	h.QueryLogger = ql
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

// serveFromCache attempts to serve a cached DNS response.
// With policy-first evaluation, cache only contains upstream responses.
// Policy and blocklist decisions are NOT cached - they are evaluated fresh every time.
func (h *Handler) serveFromCache(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	if h.Cache == nil {
		return false
	}

	cachedResp, cachedTrace := h.Cache.GetWithTrace(ctx, r)
	if cachedResp == nil {
		return false
	}

	cachedResp.Id = r.Id
	HandleEDNS0(r, cachedResp)

	outcome.cached = true
	outcome.responseCode = cachedResp.Rcode

	// Append any trace from cache (e.g., ALLOW/FORWARD policy that led to upstream lookup)
	trace.Append(cachedTrace)

	// Record cache hit - this is always an upstream response
	trace.Record(traceStageCache, "upstream_hit", func(entry *storage.BlockTraceEntry) {
		entry.Source = "response_cache"
		entry.Detail = "cached upstream response"
	})

	h.writeMsg(w, cachedResp)
	return true
}

// ServeDNS implements the dns.Handler interface
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
	startTime := time.Now()
	outcome := getOutcome()
	trace := newBlockTraceRecorder(h.DecisionTrace)
	clientIP := getClientIP(w)

	defer func() {
		h.asyncLogQuery(startTime, r, clientIP, trace, outcome)
		releaseOutcome(outcome)
		trace.Release()
	}()

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

	// Local records always take precedence
	if h.serveFromLocalRecords(w, msg, domain, qtype, outcome) {
		return
	}

	// Resolve feature toggles (permanent config + temporary kill-switches)
	enablePolicies, enableBlocklist := h.resolveFeatureToggles()
	if h.KillSwitch != nil {
		if disabled, _ := h.KillSwitch.IsBlocklistDisabled(); disabled {
			enableBlocklist = false
		}
		if disabled, _ := h.KillSwitch.IsPoliciesDisabled(); disabled {
			enablePolicies = false
		}
	}

	// POLICY-FIRST: Policies are always evaluated fresh (decisions NOT cached).
	// This ensures correct behavior with policy ordering, multiple matches, and toggles.
	// ALLOW/FORWARD actions forward to upstream and cache the upstream response.
	// BLOCK/REDIRECT return immediately without caching.
	if enablePolicies && h.PolicyEngine != nil && h.PolicyEngine.Count() > 0 {
		if h.handlePolicies(ctx, w, r, msg, domain, clientIP, qtype, qtypeLabel, trace, outcome) {
			return
		}
	}

	// BLOCKLIST-FIRST: Blocklist is always evaluated fresh (blocked NOT cached).
	// This ensures blocklist changes take immediate effect.
	if enableBlocklist {
		if h.handleBlocklistAndOverrides(ctx, w, r, msg, domain, qtype, qtypeLabel, trace, outcome) {
			return
		}
	}

	// Cache check - only contains upstream responses, not policy/blocklist decisions
	if h.serveFromCache(ctx, w, r, msg, trace, outcome) {
		return
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
	// Use QueryLogger if available (new worker pool pattern)
	// Fall back to direct Storage for backwards compatibility
	if h.QueryLogger == nil && h.Storage == nil {
		return
	}

	domain := ""
	queryType := ""
	if len(r.Question) > 0 {
		domain = strings.TrimSuffix(r.Question[0].Name, ".")
		queryType = dnsTypeLabel(r.Question[0].Qtype)
	}

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

	// New path: use worker pool (no goroutine spawn)
	if h.QueryLogger != nil {
		if err := h.QueryLogger.LogAsync(queryLog); err != nil && h.Logger != nil {
			// Buffer full - already logged by QueryLogger
			_ = err
		}
		return
	}

	// Legacy path: use select with buffered channel to avoid blocking
	// This is more efficient than spawning a goroutine per query.
	// Note: For best performance, use QueryLogger instead of legacy Storage.
	select {
	case legacyLogCh <- legacyLogRequest{storage: h.Storage, log: queryLog, logger: h.Logger}:
		// Sent to worker
	default:
		// Buffer full, log warning (rare under normal load)
		if h.Logger != nil {
			h.Logger.Warn("Legacy log buffer full, query log dropped",
				"domain", domain)
		}
	}
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
