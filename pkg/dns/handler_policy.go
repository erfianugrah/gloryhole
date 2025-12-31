package dns

import (
	"context"
	"net"
	"strings"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/storage"

	"github.com/miekg/dns"
)

func (h *Handler) handlePolicies(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, domain, clientIP string, qtype uint16, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	policyCtx := policy.NewContext(
		strings.TrimSuffix(domain, "."),
		clientIP,
		qtypeLabel,
	)

	matched, rule := h.PolicyEngine.Evaluate(policyCtx)
	if !matched || rule == nil {
		return false
	}

	if h.Logger != nil {
		h.Logger.Info("Policy rule matched",
			"rule", rule.Name,
			"action", rule.Action,
			"domain", domain,
			"client_ip", clientIP,
			"query_type", qtypeLabel)
	}

	switch rule.Action {
	case policy.ActionBlock:
		return h.handlePolicyBlock(ctx, w, r, msg, rule, domain, clientIP, qtypeLabel, trace, outcome)
	case policy.ActionAllow:
		return h.handlePolicyAllow(ctx, w, r, msg, rule, domain, clientIP, qtypeLabel, outcome)
	case policy.ActionRedirect:
		return h.handlePolicyRedirect(ctx, w, r, msg, rule, domain, clientIP, qtype, qtypeLabel, trace, outcome)
	case policy.ActionForward:
		return h.handlePolicyForward(ctx, w, r, msg, rule, domain, clientIP, qtypeLabel, outcome)
	case policy.ActionRateLimit:
		return h.handlePolicyRateLimit(ctx, w, r, msg, rule, domain, clientIP, qtypeLabel, trace, outcome)
	default:
		return false
	}
}

func (h *Handler) handlePolicyBlock(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, rule *policy.Rule, domain, clientIP, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	outcome.blocked = true
	outcome.responseCode = dns.RcodeNameError
	msg.SetRcode(r, dns.RcodeNameError)

	trace.Record(traceStagePolicy, string(rule.Action), func(entry *storage.BlockTraceEntry) {
		entry.Rule = rule.Name
		entry.Source = "policy_engine"
		entry.Detail = "rule matched"
	})

	h.recordBlockedQuery(ctx, blockMetadata{
		reason:     "policy_block",
		qtypeLabel: qtypeLabel,
		stage:      traceStagePolicy,
		rule:       rule.Name,
		source:     "policy_engine",
	})

	if h.Logger != nil {
		h.Logger.Debug("Policy blocked query",
			"rule", rule.Name,
			"domain", domain,
			"client_ip", clientIP)
	}

	if h.Cache != nil {
		h.Cache.SetBlocked(ctx, r, msg, trace.Entries())
	}

	h.writeMsg(w, msg)
	return true
}

func (h *Handler) handlePolicyAllow(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, rule *policy.Rule, domain, clientIP, qtypeLabel string, outcome *serveDNSOutcome) bool {
	if h.Forwarder == nil {
		if h.Logger != nil {
			h.Logger.Warn("Policy allow action but no forwarder configured",
				"rule", rule.Name,
				"domain", domain,
				"client_ip", clientIP)
		}
		outcome.responseCode = dns.RcodeNameError
		msg.SetRcode(r, dns.RcodeNameError)
		h.writeMsg(w, msg)
		return true
	}

	if h.Logger != nil {
		h.Logger.Warn("Policy ALLOW action bypasses blocklist checks - forwarding directly to upstream",
			"rule", rule.Name,
			"domain", domain,
			"client_ip", clientIP,
			"bypasses_blocklist", true)
	}

	forwardStart := time.Now()
	resp, err := h.Forwarder.Forward(ctx, r)
	outcome.upstreamDuration = time.Since(forwardStart)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("Failed to forward allowed query",
				"rule", rule.Name,
				"domain", domain,
				"client_ip", clientIP,
				"error", err)
		}
		outcome.responseCode = dns.RcodeServerFailure
		msg.SetRcode(r, dns.RcodeServerFailure)
		h.writeMsg(w, msg)
		return true
	}

	upstreams := h.Forwarder.Upstreams()
	if len(upstreams) > 0 {
		outcome.upstream = upstreams[0]
	}

	h.recordForwardedQuery(ctx, "policy_allow", qtypeLabel, outcome.upstream)

	if h.Cache != nil {
		h.Cache.Set(ctx, r, resp)
	}

	outcome.responseCode = resp.Rcode
	h.writeMsg(w, resp)
	return true
}

func (h *Handler) handlePolicyRedirect(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, rule *policy.Rule, domain, clientIP string, qtype uint16, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	targetIP := net.ParseIP(rule.ActionData)
	if targetIP == nil {
		if h.Logger != nil {
			h.Logger.Error("Policy redirect has invalid IP address",
				"rule", rule.Name,
				"domain", domain,
				"client_ip", clientIP,
				"action_data", rule.ActionData)
		}
		outcome.responseCode = dns.RcodeNameError
		msg.SetRcode(r, dns.RcodeNameError)
		h.writeMsg(w, msg)
		return true
	}

	if h.Logger != nil {
		h.Logger.Debug("Policy redirecting query",
			"rule", rule.Name,
			"domain", domain,
			"client_ip", clientIP,
			"redirect_ip", targetIP.String(),
			"query_type", qtypeLabel)
	}

	switch {
	case qtype == dns.TypeA && targetIP.To4() != nil:
		addARecord(msg, domain, targetIP, 300)
		outcome.responseCode = dns.RcodeSuccess
	case qtype == dns.TypeAAAA && targetIP.To4() == nil:
		addAAAARecord(msg, domain, targetIP, 300)
		outcome.responseCode = dns.RcodeSuccess
	default:
		outcome.responseCode = dns.RcodeSuccess
	}

	if h.Cache != nil {
		h.Cache.Set(ctx, r, msg)
	}

	h.writeMsg(w, msg)

	trace.Record(traceStagePolicy, string(rule.Action), func(entry *storage.BlockTraceEntry) {
		entry.Rule = rule.Name
		entry.Source = "policy_engine"
		entry.Detail = "redirect"
		if rule.ActionData != "" {
			entry.Metadata = map[string]string{"target": rule.ActionData}
		}
	})
	return true
}

func (h *Handler) handlePolicyForward(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, rule *policy.Rule, domain, clientIP, qtypeLabel string, outcome *serveDNSOutcome) bool {
	upstreams := rule.GetUpstreams()
	if len(upstreams) == 0 || h.Forwarder == nil {
		if h.Logger != nil {
			h.Logger.Error("Policy forward action has no upstreams configured",
				"rule", rule.Name,
				"domain", domain,
				"client_ip", clientIP)
		}
		outcome.responseCode = dns.RcodeServerFailure
		msg.SetRcode(r, dns.RcodeServerFailure)
		h.writeMsg(w, msg)
		return true
	}

	if h.Logger != nil {
		h.Logger.Debug("Policy forwarding query to specific upstreams",
			"rule", rule.Name,
			"domain", domain,
			"client_ip", clientIP,
			"upstreams", upstreams)
	}

	forwardStart := time.Now()
	resp, err := h.Forwarder.ForwardWithUpstreams(ctx, r, upstreams)
	outcome.upstreamDuration = time.Since(forwardStart)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("Failed to forward query to policy upstreams",
				"rule", rule.Name,
				"domain", domain,
				"client_ip", clientIP,
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

	h.recordForwardedQuery(ctx, "conditional_rule", qtypeLabel, outcome.upstream)

	if h.Cache != nil {
		h.Cache.Set(ctx, r, resp)
	}

	outcome.responseCode = resp.Rcode
	h.writeMsg(w, resp)
	return true
}

func (h *Handler) handlePolicyRateLimit(ctx context.Context, w dns.ResponseWriter, r, msg *dns.Msg, rule *policy.Rule, domain, clientIP, qtypeLabel string, trace *blockTraceRecorder, outcome *serveDNSOutcome) bool {
	if rule.RateLimiter == nil {
		if h.Logger != nil {
			h.Logger.Warn("Policy rule has RATE_LIMIT action but no limiter configured",
				"rule", rule.Name,
				"domain", domain,
				"client_ip", clientIP)
		}
		return false
	}

	// Apply THIS rule's rate limiter (not global)
	allowed, limited, action, _ := rule.RateLimiter.Allow(clientIP)
	if !allowed && limited {
		// Rate limit exceeded for THIS specific rule
		trace.Record(traceStagePolicy, string(action), func(entry *storage.BlockTraceEntry) {
			entry.Rule = rule.Name
			entry.Source = "policy_engine"
			entry.Metadata = map[string]string{
				"client_ip": clientIP,
				"limit":     rule.ActionData,
			}
		})

		h.recordRateLimit(ctx, clientIP, qtypeLabel, string(action), false)

		if h.Logger != nil && rule.RateLimiter.LogViolations() {
			h.Logger.Warn("Policy rate limit exceeded",
				"rule", rule.Name,
				"client_ip", clientIP,
				"domain", domain,
				"action", action,
				"limit", rule.ActionData,
			)
		}

		// Return NXDOMAIN or drop
		if action == config.RateLimitActionDrop {
			outcome.responseCode = dns.RcodeRefused
			return true
		}

		outcome.responseCode = dns.RcodeNameError
		msg.SetRcode(r, dns.RcodeNameError)
		h.writeMsg(w, msg)
		return true
	}

	// Not limited; continue to next rule
	return false
}
