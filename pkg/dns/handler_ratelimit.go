package dns

import (
	"context"

	"glory-hole/pkg/config"
	"glory-hole/pkg/storage"

	"github.com/miekg/dns"
)

func (h *Handler) enforceRateLimit(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, clientIP, domain, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	if h.RateLimiter == nil {
		return false
	}

	allowed, limited, action, label := h.RateLimiter.Allow(clientIP)
	if allowed || !limited {
		return false
	}

	trace.Record(traceStageRateLimit, string(action), func(entry *storage.BlockTraceEntry) {
		entry.Source = label
		entry.Metadata = map[string]string{
			"client_ip": clientIP,
		}
	})

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
		outcome.responseCode = dns.RcodeRefused
		return true
	}

	outcome.responseCode = dns.RcodeNameError
	msg.SetRcode(r, dns.RcodeNameError)
	h.writeMsg(w, msg)
	return true
}

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
	trace.Append(cachedTrace)
	if cachedResp.Rcode == dns.RcodeNameError {
		outcome.blocked = true
		trace.Record(traceStageCache, "blocked_hit", func(entry *storage.BlockTraceEntry) {
			entry.Source = "response_cache"
		})
	}

	h.writeMsg(w, cachedResp)
	return true
}
