package dns

import (
	"context"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func (h *Handler) handleConditionalForwarding(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, domain, clientIP, qtypeLabel string, outcome *serveDNSOutcome) bool {
	if h.RuleEvaluator == nil || h.RuleEvaluator.IsEmpty() || h.Forwarder == nil {
		return false
	}

	upstreams := h.RuleEvaluator.Evaluate(
		strings.TrimSuffix(domain, "."),
		clientIP,
		qtypeLabel,
	)

	if len(upstreams) == 0 {
		return false
	}

	if h.Logger != nil {
		h.Logger.Debug("Conditional forwarding rule matched",
			"domain", domain,
			"client_ip", clientIP,
			"upstreams", upstreams,
			"query_type", qtypeLabel)
	}

	forwardStart := time.Now()
	resp, err := h.Forwarder.ForwardWithUpstreams(ctx, r, upstreams)
	outcome.upstreamDuration = time.Since(forwardStart)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("Conditional forwarding failed",
				"domain", domain,
				"upstreams", upstreams,
				"error", err)
		}
		outcome.responseCode = dns.RcodeServerFailure
		msg.SetRcode(r, dns.RcodeServerFailure)
		h.writeMsg(w, msg)
		return true
	}

	if len(upstreams) > 0 {
		outcome.upstream = upstreams[0]
	}

	h.recordForwardedQuery(ctx, "policy_forward", qtypeLabel, outcome.upstream)

	if h.Cache != nil {
		h.Cache.Set(ctx, r, resp)
	}

	outcome.responseCode = resp.Rcode
	h.writeMsg(w, resp)
	return true
}

func (h *Handler) forwardToUpstream(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, qtypeLabel string, outcome *serveDNSOutcome) bool {
	if h.Forwarder == nil {
		return false
	}

	forwardStart := time.Now()
	resp, err := h.Forwarder.Forward(ctx, r)
	outcome.upstreamDuration = time.Since(forwardStart)
	if err != nil {
		outcome.responseCode = dns.RcodeServerFailure
		msg.SetRcode(r, dns.RcodeServerFailure)
		h.writeMsg(w, msg)
		return true
	}

	upstreams := h.Forwarder.Upstreams()
	if len(upstreams) > 0 {
		outcome.upstream = upstreams[0]
	}

	h.recordForwardedQuery(ctx, "default_forward", qtypeLabel, outcome.upstream)

	if h.Cache != nil {
		h.Cache.Set(ctx, r, resp)
	}

	outcome.responseCode = resp.Rcode
	h.writeMsg(w, resp)
	return true
}
