package dns

import (
	"context"
	"time"

	"github.com/miekg/dns"
)

// forwardToUpstream is the default-upstream forwarding path used when no
// policy rule has selected an upstream and no cache hit was available.
//
// The legacy "conditional forwarding" code path was removed in v0.27 — its
// functionality is now subsumed by Policy rules with Action=FORWARD. See
// docs/plans/2026-05-25-v026-policy-consolidation.md §1 for context, and the
// migrator at cmd/glory-hole/main.go::migrateConditionalForwardingToPolicies
// which still runs at boot to move legacy YAML rules into the policy table.
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
