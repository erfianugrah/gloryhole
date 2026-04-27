package dns

import (
	"context"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func (h *Handler) handleConditionalForwarding(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, domain, clientIP, qtypeLabel string, outcome *serveDNSOutcome) bool {
	re := h.getRuleEvaluator()
	fwd := h.getForwarder()
	if re == nil || re.IsEmpty() || fwd == nil {
		return false
	}

	upstreams := re.Evaluate(
		strings.TrimSuffix(domain, "."),
		clientIP,
		qtypeLabel,
	)

	if len(upstreams) == 0 {
		return false
	}

	if lg := h.getLogger(); lg != nil {
		lg.Debug("Conditional forwarding rule matched",
			"domain", domain,
			"client_ip", clientIP,
			"upstreams", upstreams,
			"query_type", qtypeLabel)
	}

	forwardStart := time.Now()
	resp, err := fwd.ForwardWithUpstreams(ctx, r, upstreams)
	outcome.upstreamDuration = time.Since(forwardStart)
	if err != nil {
		if lg := h.getLogger(); lg != nil {
			lg.Error("Conditional forwarding failed",
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

	// Capture DNSSEC validation status from response
	outcome.dnssecValidated = resp.AuthenticatedData

	// Extract Extended DNS Error (RFC 8914) from upstream response
	if edeCode, edeText, hasEDE := ExtractEDE(resp); hasEDE {
		codeName := EDECodeToString(edeCode)
		if edeText != "" {
			outcome.upstreamError = codeName + ": " + edeText
		} else {
			outcome.upstreamError = codeName
		}
	}

	// Enrich with Unbound dnstap data (best-effort inline correlation)
	h.enrichFromUnbound(r, outcome)

	if c := h.getCache(); c != nil {
		c.Set(ctx, r, resp)
	}

	outcome.responseCode = resp.Rcode
	h.writeMsg(w, resp)
	return true
}

func (h *Handler) forwardToUpstream(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, qtypeLabel string, outcome *serveDNSOutcome) bool {
	fwd := h.getForwarder()
	if fwd == nil {
		return false
	}

	forwardStart := time.Now()
	resp, err := fwd.Forward(ctx, r)
	outcome.upstreamDuration = time.Since(forwardStart)
	if err != nil {
		outcome.responseCode = dns.RcodeServerFailure
		msg.SetRcode(r, dns.RcodeServerFailure)
		h.writeMsg(w, msg)
		return true
	}

	upstreams := fwd.Upstreams()
	if len(upstreams) > 0 {
		outcome.upstream = upstreams[0]
	}

	h.recordForwardedQuery(ctx, "default_forward", qtypeLabel, outcome.upstream)

	// Capture DNSSEC validation status from response
	outcome.dnssecValidated = resp.AuthenticatedData

	// Extract Extended DNS Error (RFC 8914) from upstream response
	if edeCode, edeText, hasEDE := ExtractEDE(resp); hasEDE {
		codeName := EDECodeToString(edeCode)
		if edeText != "" {
			outcome.upstreamError = codeName + ": " + edeText
		} else {
			outcome.upstreamError = codeName
		}
	}

	// Enrich with Unbound dnstap data (best-effort inline correlation)
	h.enrichFromUnbound(r, outcome)

	if c := h.getCache(); c != nil {
		c.Set(ctx, r, resp)
	}

	outcome.responseCode = resp.Rcode
	h.writeMsg(w, resp)
	return true
}
