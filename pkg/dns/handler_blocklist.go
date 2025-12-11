package dns

import (
	"context"
	"net"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/storage"

	"github.com/miekg/dns"
)

func (h *Handler) handleBlocklistAndOverrides(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, domain string, qtype uint16, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	if h.BlocklistManager != nil {
		return h.handleFastBlocklistPath(ctx, w, r, msg, domain, qtype, qtypeLabel, trace, outcome)
	}
	return h.handleLegacyBlocklistPath(ctx, w, r, msg, domain, qtype, qtypeLabel, trace, outcome)
}

func (h *Handler) handleFastBlocklistPath(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, domain string, qtype uint16, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	blockMatch := h.BlocklistManager.Match(domain)
	blocked := blockMatch.Blocked

	if h.isDomainWhitelisted(domain) {
		blocked = false
		blockMatch = blocklist.MatchResult{}
		// Record whitelisted query metric
		if h.Metrics != nil && h.Metrics.DNSWhitelistedQueries != nil {
			h.Metrics.DNSWhitelistedQueries.Add(ctx, 1)
		}
	}

	overrideIP, hasOverride, cnameTarget, hasCNAME := h.lookupOverrides(domain, qtype, blocked)

	if blocked {
		return h.handleBlockedDomain(ctx, w, r, msg, qtypeLabel, trace, outcome, blockMatch)
	}

	if hasOverride && respondWithOverride(msg, qtype, domain, overrideIP) {
		outcome.responseCode = dns.RcodeSuccess
		h.writeMsg(w, msg)
		return true
	}

	if hasCNAME {
		respondWithCNAME(msg, domain, cnameTarget)
		outcome.responseCode = dns.RcodeSuccess
		h.writeMsg(w, msg)
		return true
	}

	return false
}

func (h *Handler) handleLegacyBlocklistPath(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, domain string, qtype uint16, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	if h.Blocklist == nil {
		return false
	}

	whitelisted := h.isDomainWhitelisted(domain)
	blocked := false

	if whitelisted {
		// Record whitelisted query metric
		if h.Metrics != nil && h.Metrics.DNSWhitelistedQueries != nil {
			h.Metrics.DNSWhitelistedQueries.Add(ctx, 1)
		}
	} else {
		h.lookupMu.RLock()
		_, blocked = h.Blocklist[domain]
		h.lookupMu.RUnlock()
	}

	overrideIP, hasOverride, cnameTarget, hasCNAME := h.lookupOverrides(domain, qtype, blocked)

	if blocked {
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
		outcome.responseCode = dns.RcodeNameError
		msg.SetRcode(r, dns.RcodeNameError)

		if h.Cache != nil {
			h.Cache.SetBlocked(ctx, r, msg, trace.Entries())
		}

		h.writeMsg(w, msg)
		return true
	}

	if hasOverride && respondWithOverride(msg, qtype, domain, overrideIP) {
		outcome.responseCode = dns.RcodeSuccess
		h.writeMsg(w, msg)
		return true
	}

	if hasCNAME {
		respondWithCNAME(msg, domain, cnameTarget)
		outcome.responseCode = dns.RcodeSuccess
		h.writeMsg(w, msg)
		return true
	}

	return false
}

func (h *Handler) handleBlockedDomain(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome, match blocklist.MatchResult) bool {
	outcome.blocked = true
	outcome.responseCode = dns.RcodeNameError
	msg.SetRcode(r, dns.RcodeNameError)

	sourceLabel := blocklistTraceSource(match)
	if sourceLabel == "" {
		sourceLabel = "blocklist"
	}

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

	if h.Cache != nil {
		h.Cache.SetBlocked(ctx, r, msg, trace.Entries())
	}

	h.writeMsg(w, msg)
	return true
}

func (h *Handler) lookupOverrides(domain string, qtype uint16, blocked bool) (net.IP, bool, string, bool) {
	h.lookupMu.RLock()
	defer h.lookupMu.RUnlock()

	var overrideIP net.IP
	var hasOverride bool
	if !blocked && (qtype == dns.TypeA || qtype == dns.TypeAAAA) {
		overrideIP, hasOverride = h.Overrides[domain]
	}

	var cnameTarget string
	var hasCNAME bool
	if !blocked && !hasOverride && (qtype == dns.TypeCNAME || qtype == dns.TypeA || qtype == dns.TypeAAAA) {
		cnameTarget, hasCNAME = h.CNAMEOverrides[domain]
	}

	return overrideIP, hasOverride, cnameTarget, hasCNAME
}

func (h *Handler) isDomainWhitelisted(domain string) bool {
	whitelist := h.Whitelist.Load()
	if whitelist != nil {
		if _, ok := (*whitelist)[domain]; ok {
			return true
		}
	}

	patterns := h.WhitelistPatterns.Load()
	if patterns != nil {
		return patterns.Match(domain)
	}
	return false
}
