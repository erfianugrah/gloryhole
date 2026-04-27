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
	"glory-hole/pkg/policy"
	"glory-hole/pkg/storage"
	"glory-hole/pkg/telemetry"
	"glory-hole/pkg/unbound"

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
// Lazy-initialized only when the legacy Storage path is actually used
// (i.e., QueryLogger is nil). Avoids ~256KB + 4 goroutines when unused.
var legacyLogCh chan legacyLogRequest
var legacyLogOnce sync.Once

func initLegacyLog() {
	legacyLogOnce.Do(func() {
		legacyLogCh = make(chan legacyLogRequest, 10000)
		for i := 0; i < 4; i++ {
			go legacyLogWorker()
		}
	})
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

// handlerDeps bundles all hot-reloadable dependencies for lock-free reads
// on the DNS hot path. Writers (config reload) clone-and-swap via Set* methods.
// Readers call get* accessors which perform a single atomic pointer load.
type handlerDeps struct {
	storage          storage.Storage
	queryLogger      *QueryLogger
	blocklistManager *blocklist.Manager
	localRecords     *localrecords.Manager
	policyEngine     *policy.Engine
	ruleEvaluator    *forwarder.RuleEvaluator
	fwd              *forwarder.Forwarder
	cache            cache.Interface
	configWatcher    *config.Watcher
	killSwitch       KillSwitchChecker
	decisionTrace    bool
	blockPageIP      string
	unboundBuffer    *unbound.ReplyBuffer
	metrics          *telemetry.Metrics
	logger           *logging.Logger
}

type Handler struct {
	deps atomic.Pointer[handlerDeps]

	// Legacy blocklist maps — guarded by lookupMu.
	Blocklist      map[string]struct{}
	Overrides      map[string]net.IP
	CNAMEOverrides map[string]string
	lookupMu       sync.RWMutex
}

// NewHandler creates a new DNS handler
func NewHandler() *Handler {
	h := &Handler{
		Blocklist:      make(map[string]struct{}),
		Overrides:      make(map[string]net.IP),
		CNAMEOverrides: make(map[string]string),
	}
	h.deps.Store(&handlerDeps{})
	return h
}

// clone returns a shallow copy of the current deps for clone-and-swap.
func (h *Handler) clone() handlerDeps {
	if d := h.deps.Load(); d != nil {
		return *d
	}
	return handlerDeps{}
}

// --- Getters: single atomic load per call ---

func (h *Handler) getStorage() storage.Storage          { return h.deps.Load().storage }
func (h *Handler) getQueryLogger() *QueryLogger          { return h.deps.Load().queryLogger }
func (h *Handler) getBlocklistManager() *blocklist.Manager { return h.deps.Load().blocklistManager }
func (h *Handler) getLocalRecords() *localrecords.Manager { return h.deps.Load().localRecords }
func (h *Handler) getPolicyEngine() *policy.Engine        { return h.deps.Load().policyEngine }
func (h *Handler) getRuleEvaluator() *forwarder.RuleEvaluator { return h.deps.Load().ruleEvaluator }
func (h *Handler) getForwarder() *forwarder.Forwarder     { return h.deps.Load().fwd }
func (h *Handler) getCache() cache.Interface              { return h.deps.Load().cache }
func (h *Handler) getConfigWatcher() *config.Watcher      { return h.deps.Load().configWatcher }
func (h *Handler) getKillSwitch() KillSwitchChecker       { return h.deps.Load().killSwitch }
func (h *Handler) getDecisionTrace() bool                 { return h.deps.Load().decisionTrace }
func (h *Handler) getBlockPageIP() string                 { return h.deps.Load().blockPageIP }
func (h *Handler) getUnboundBuffer() *unbound.ReplyBuffer { return h.deps.Load().unboundBuffer }
func (h *Handler) getMetrics() *telemetry.Metrics         { return h.deps.Load().metrics }
func (h *Handler) GetMetrics() *telemetry.Metrics         { return h.deps.Load().metrics }
func (h *Handler) GetCache() cache.Interface              { return h.deps.Load().cache }
func (h *Handler) getLogger() *logging.Logger             { return h.deps.Load().logger }

// --- Setters: clone-and-swap (single writer assumed) ---

func (h *Handler) SetForwarder(f *forwarder.Forwarder) {
	d := h.clone()
	d.fwd = f
	h.deps.Store(&d)
}

func (h *Handler) SetCache(c cache.Interface) {
	d := h.clone()
	d.cache = c
	h.deps.Store(&d)
}

func (h *Handler) SetBlocklistManager(m *blocklist.Manager) {
	d := h.clone()
	d.blocklistManager = m
	h.deps.Store(&d)
}

func (h *Handler) SetStorage(s storage.Storage) {
	d := h.clone()
	d.storage = s
	h.deps.Store(&d)
}

func (h *Handler) SetQueryLogger(ql *QueryLogger) {
	d := h.clone()
	d.queryLogger = ql
	h.deps.Store(&d)
}

func (h *Handler) SetLocalRecords(l *localrecords.Manager) {
	d := h.clone()
	d.localRecords = l
	h.deps.Store(&d)
}

func (h *Handler) SetPolicyEngine(e *policy.Engine) {
	d := h.clone()
	d.policyEngine = e
	h.deps.Store(&d)
}

func (h *Handler) SetMetrics(m *telemetry.Metrics) {
	d := h.clone()
	d.metrics = m
	h.deps.Store(&d)
}

func (h *Handler) SetLogger(l *logging.Logger) {
	d := h.clone()
	d.logger = l
	h.deps.Store(&d)
}

func (h *Handler) SetKillSwitch(ks KillSwitchChecker) {
	d := h.clone()
	d.killSwitch = ks
	h.deps.Store(&d)
}

func (h *Handler) SetDecisionTrace(enabled bool) {
	d := h.clone()
	d.decisionTrace = enabled
	h.deps.Store(&d)
}

func (h *Handler) SetRuleEvaluator(re *forwarder.RuleEvaluator) {
	d := h.clone()
	d.ruleEvaluator = re
	h.deps.Store(&d)
}

func (h *Handler) SetConfigWatcher(cw *config.Watcher) {
	d := h.clone()
	d.configWatcher = cw
	h.deps.Store(&d)
}

func (h *Handler) SetBlockPageIP(ip string) {
	d := h.clone()
	d.blockPageIP = ip
	h.deps.Store(&d)
}

func (h *Handler) SetUnboundReplyBuffer(rb *unbound.ReplyBuffer) {
	d := h.clone()
	d.unboundBuffer = rb
	h.deps.Store(&d)
}

// enrichFromUnbound attempts to match dnstap reply data from the Unbound
// reply buffer and populate the outcome with Unbound-specific fields.
func (h *Handler) enrichFromUnbound(r *dns.Msg, outcome *serveDNSOutcome) {
	buf := h.getUnboundBuffer()
	if buf == nil || len(r.Question) == 0 {
		return
	}
	domain := r.Question[0].Name
	qtype := dns.TypeToString[r.Question[0].Qtype]
	if match := buf.FindReply(domain, qtype, 500*time.Millisecond); match != nil {
		outcome.unboundCached = &match.CachedInUnbound
		outcome.unboundDuration = &match.DurationMs
		outcome.unboundRespSize = &match.ResponseSize
	}
}

// writeMsg writes a DNS message to the response writer with error handling.
// For UDP, if the serialized response exceeds the client's buffer size, the TC
// (truncated) bit is set and the answer section is stripped to force TCP retry.
// This prevents DNS amplification via oversized UDP responses.
func (h *Handler) writeMsg(w dns.ResponseWriter, msg *dns.Msg) {
	// Only enforce size limits on UDP (TCP has no practical size limit)
	if isUDP(w) {
		maxSize := 512 // Default without EDNS0
		if opt := msg.IsEdns0(); opt != nil {
			maxSize = int(opt.UDPSize())
		}
		if msg.Len() > maxSize {
			msg.Truncated = true
			msg.Answer = nil // Strip answers to fit in buffer
		}
	}

	if err := w.WriteMsg(msg); err != nil {
		// Client likely disconnected - nothing we can do
		_ = err
	}
}

// isUDP returns true if the response writer is for a UDP connection.
func isUDP(w dns.ResponseWriter) bool {
	if addr := w.LocalAddr(); addr != nil {
		return addr.Network() == "udp"
	}
	return false
}

// serveFromCache attempts to serve a cached DNS response.
// With policy-first evaluation, cache only contains upstream responses.
// Policy and blocklist decisions are NOT cached - they are evaluated fresh every time.
func (h *Handler) serveFromCache(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	c := h.getCache()
	if c == nil {
		return false
	}

	cachedResp, cachedTrace := c.GetWithTrace(ctx, r)
	if cachedResp == nil {
		return false
	}

	cachedResp.Id = r.Id
	HandleEDNS0(r, cachedResp)

	outcome.cached = true
	outcome.responseCode = cachedResp.Rcode
	outcome.dnssecValidated = cachedResp.AuthenticatedData

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
	trace := newBlockTraceRecorder(h.getDecisionTrace())
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
	if ks := h.getKillSwitch(); ks != nil {
		if disabled, _ := ks.IsBlocklistDisabled(); disabled {
			enableBlocklist = false
		}
		if disabled, _ := ks.IsPoliciesDisabled(); disabled {
			enablePolicies = false
		}
	}

	// POLICY-FIRST: Policies are always evaluated fresh (decisions NOT cached).
	// This ensures correct behavior with policy ordering, multiple matches, and toggles.
	// ALLOW/FORWARD actions forward to upstream and cache the upstream response.
	// BLOCK/REDIRECT return immediately without caching.
	if pe := h.getPolicyEngine(); enablePolicies && pe != nil && pe.Count() > 0 {
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

	// Cache check - contains upstream responses and blocklist decisions (with traces).
	// Policy BLOCK/REDIRECT decisions are NOT cached.
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
	ql := h.getQueryLogger()
	st := h.getStorage()
	lg := h.getLogger()

	// Use QueryLogger if available (new worker pool pattern)
	// Fall back to direct Storage for backwards compatibility
	if ql == nil && st == nil {
		return
	}

	domain := ""
	queryType := ""
	if len(r.Question) > 0 {
		domain = strings.TrimSuffix(r.Question[0].Name, ".")
		queryType = dnsTypeLabel(r.Question[0].Qtype)
	}

	queryLog := &storage.QueryLog{
		Timestamp:         startTime,
		ClientIP:          clientIP,
		Domain:            domain,
		QueryType:         queryType,
		ResponseCode:      outcome.responseCode,
		Blocked:           outcome.blocked,
		Cached:            outcome.cached,
		DNSSECValidated:   outcome.dnssecValidated,
		ResponseTimeMs:    time.Since(startTime).Seconds() * 1000,
		UpstreamTimeMs:    outcome.upstreamDuration.Seconds() * 1000,
		Upstream:          outcome.upstream,
		UpstreamError:     outcome.upstreamError,
		BlockTrace:        trace.Entries(),
		UnboundCached:     outcome.unboundCached,
		UnboundDurationMs: outcome.unboundDuration,
		UnboundRespSize:   outcome.unboundRespSize,
	}

	// New path: use worker pool (no goroutine spawn)
	if ql != nil {
		if err := ql.LogAsync(queryLog); err != nil && lg != nil {
			// Buffer full - already logged by QueryLogger
			_ = err
		}
		return
	}

	// Legacy path: use select with buffered channel to avoid blocking.
	// Lazy-initialize on first use to avoid waste when QueryLogger is active.
	initLegacyLog()
	select {
	case legacyLogCh <- legacyLogRequest{storage: st, log: queryLog, logger: lg}:
		// Sent to worker
	default:
		// Buffer full, log warning (rare under normal load)
		if lg != nil {
			lg.Warn("Legacy log buffer full, query log dropped",
				"domain", domain)
		}
	}
}

func (h *Handler) resolveFeatureToggles() (enablePolicies, enableBlocklist bool) {
	enablePolicies = true
	enableBlocklist = true
	cw := h.getConfigWatcher()
	if cw == nil {
		return
	}
	cfg := cw.Config()
	return cfg.Server.EnablePolicies, cfg.Server.EnableBlocklist
}

// dnsTypeLabel returns a human-readable string for the query type, falling back to TYPE#### per RFC 3597 when unknown.
func dnsTypeLabel(qtype uint16) string {
	if label := dns.TypeToString[qtype]; label != "" {
		return label
	}
	return "TYPE" + strconv.FormatUint(uint64(qtype), 10)
}
