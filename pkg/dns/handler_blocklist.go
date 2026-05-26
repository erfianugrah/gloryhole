package dns

import (
	"context"
	"net"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/storage"

	"github.com/miekg/dns"
)

func (h *Handler) handleBlocklistAndOverrides(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, domain string, qtype uint16, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	if h.getBlocklistManager() != nil {
		return h.handleFastBlocklistPath(ctx, w, r, msg, domain, qtype, qtypeLabel, trace, outcome)
	}
	return h.handleLegacyBlocklistPath(ctx, w, r, msg, domain, qtype, qtypeLabel, trace, outcome)
}

func (h *Handler) handleFastBlocklistPath(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, domain string, qtype uint16, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	blockMatch := h.getBlocklistManager().Match(domain)
	if blockMatch.Blocked {
		return h.handleBlockedDomain(ctx, w, r, msg, qtypeLabel, trace, outcome, blockMatch)
	}
	return false
}

func (h *Handler) handleLegacyBlocklistPath(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, domain string, qtype uint16, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	if h.Blocklist == nil {
		return false
	}

	h.lookupMu.RLock()
	_, blocked := h.Blocklist[domain]
	h.lookupMu.RUnlock()

	if blocked {
		// Record trace BEFORE response - this appears in query logs
		trace.Record(traceStageBlocklist, "block", func(entry *storage.BlockTraceEntry) {
			entry.Source = "legacy"
			entry.Detail = "Matched legacy blocklist entry"
		})

		h.recordBlockedQuery(ctx, blockMetadata{
			reason:     "blocklist_legacy",
			qtypeLabel: qtypeLabel,
			stage:      traceStageBlocklist,
			source:     "legacy",
		})

		outcome.blocked = true
		// If block page is configured, return the block page IP instead of NXDOMAIN
		bpIP := h.getBlockPageIP()
		if bpIP != "" {
			blockIP := net.ParseIP(bpIP)
			if blockIP != nil && (qtype == dns.TypeA || qtype == dns.TypeAAAA) {
				outcome.responseCode = dns.RcodeSuccess
				if qtype == dns.TypeA && blockIP.To4() != nil {
					addARecord(msg, domain, blockIP, 60)
				} else if qtype == dns.TypeAAAA && blockIP.To4() == nil {
					addAAAARecord(msg, domain, blockIP, 60)
				}
			} else {
				outcome.responseCode = dns.RcodeNameError
				msg.SetRcode(r, dns.RcodeNameError)
			}
		} else {
			outcome.responseCode = dns.RcodeNameError
			msg.SetRcode(r, dns.RcodeNameError)
		}

		// Cache blocked response WITH trace so subsequent cache hits show WHY it was blocked.
		// Cached decisions are cleared when blocklist is toggled ON to prevent stale decisions.
		if c := h.getCache(); c != nil {
			c.SetBlocked(ctx, r, msg, trace.Entries())
		}

		h.writeMsg(w, msg)
		return true
	}

	return false
}

func (h *Handler) handleBlockedDomain(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome, match blocklist.MatchResult) bool {
	outcome.blocked = true

	// If block page is configured, return the block page IP instead of NXDOMAIN
	// so the browser can show a friendly block page instead of a generic error.
	bpIP := h.getBlockPageIP()
	if bpIP != "" && len(r.Question) > 0 {
		qtype := r.Question[0].Qtype
		domain := r.Question[0].Name
		blockIP := net.ParseIP(bpIP)
		if blockIP != nil && (qtype == dns.TypeA || qtype == dns.TypeAAAA) {
			outcome.responseCode = dns.RcodeSuccess
			if qtype == dns.TypeA && blockIP.To4() != nil {
				addARecord(msg, domain, blockIP, 60)
			} else if qtype == dns.TypeAAAA && blockIP.To4() == nil {
				addAAAARecord(msg, domain, blockIP, 60)
			}
		} else {
			outcome.responseCode = dns.RcodeNameError
			msg.SetRcode(r, dns.RcodeNameError)
		}
	} else {
		outcome.responseCode = dns.RcodeNameError
		msg.SetRcode(r, dns.RcodeNameError)
	}

	sourceLabel := blocklistTraceSource(match)
	if sourceLabel == "" {
		sourceLabel = "blocklist"
	}

	// Record trace BEFORE response - this appears in query logs
	trace.Record(traceStageBlocklist, "block", func(entry *storage.BlockTraceEntry) {
		entry.Source = sourceLabel
		if detail := describeBlockMatch(match); detail != "" {
			entry.Detail = detail
		}
		applyBlockMatchMetadata(entry, match)
	})

	h.recordBlockedQuery(ctx, blockMetadata{
		reason:     "blocklist_manager",
		qtypeLabel: qtypeLabel,
		stage:      traceStageBlocklist,
		source:     sourceLabel,
	})

	// Cache blocked response WITH trace so subsequent cache hits show WHY it was blocked.
	// Cached decisions are cleared when blocklist is toggled ON to prevent stale decisions.
	if c := h.getCache(); c != nil {
		c.SetBlocked(ctx, r, msg, trace.Entries())
	}

	h.writeMsg(w, msg)
	return true
}
