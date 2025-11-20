package dns

import (
	"context"
	"net"
	"sync"

	"glory-hole/pkg/cache"
	"glory-hole/pkg/forwarder"

	"github.com/miekg/dns"
)

// msgPool provides object pooling for dns.Msg to reduce allocations
var msgPool = sync.Pool{
	New: func() interface{} {
		return new(dns.Msg)
	},
}

// Handler is a DNS handler
type Handler struct {
	// Single lock for all lookup maps (performance optimization)
	// Using one lock instead of 4 separate locks reduces overhead from ~2-4μs to ~500ns
	lookupMu sync.RWMutex

	Blocklist      map[string]struct{}
	Whitelist      map[string]struct{}
	Overrides      map[string]net.IP
	CNAMEOverrides map[string]string
	Forwarder      *forwarder.Forwarder
	Cache          *cache.Cache // Optional DNS response cache
}

// NewHandler creates a new DNS handler
func NewHandler() *Handler {
	return &Handler{
		Blocklist:      make(map[string]struct{}),
		Whitelist:      make(map[string]struct{}),
		Overrides:      make(map[string]net.IP),
		CNAMEOverrides: make(map[string]string),
	}
}

// SetForwarder sets the upstream DNS forwarder
func (h *Handler) SetForwarder(f *forwarder.Forwarder) {
	h.Forwarder = f
}

// SetCache sets the DNS response cache
func (h *Handler) SetCache(c *cache.Cache) {
	h.Cache = c
}

// ServeDNS implements the dns.Handler interface
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
	// Create response message
	// Note: We don't pool these because ResponseWriter may hold references
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true
	msg.RecursionAvailable = true

	// Validate request
	if len(r.Question) == 0 {
		msg.SetRcode(r, dns.RcodeFormatError)
		w.WriteMsg(msg)
		return
	}

	question := r.Question[0]
	domain := question.Name
	qtype := question.Qtype

	// Check cache first (before any lookups)
	// Cache lookups are fast (~100ns) and can save upstream roundtrip (~10ms)
	if h.Cache != nil {
		if cached := h.Cache.Get(ctx, r); cached != nil {
			// Important: Update the message ID to match the query
			// Cached responses have the original query's ID, but we need this query's ID
			cached.Id = r.Id
			w.WriteMsg(cached)
			return
		}
	}

	// Single lock for all map lookups (performance optimization)
	// Check whitelist, blocklist, and overrides in one critical section
	h.lookupMu.RLock()

	// Check whitelist first (always allow)
	_, whitelisted := h.Whitelist[domain]

	// Check blocklist (if not whitelisted)
	var blocked bool
	if !whitelisted {
		_, blocked = h.Blocklist[domain]
	}

	// Check local overrides for A/AAAA records
	var overrideIP net.IP
	var hasOverride bool
	if !blocked && (qtype == dns.TypeA || qtype == dns.TypeAAAA) {
		overrideIP, hasOverride = h.Overrides[domain]
	}

	// Check CNAME overrides
	var cnameTarget string
	var hasCNAME bool
	if !blocked && !hasOverride && (qtype == dns.TypeCNAME || qtype == dns.TypeA || qtype == dns.TypeAAAA) {
		cnameTarget, hasCNAME = h.CNAMEOverrides[domain]
	}

	h.lookupMu.RUnlock()
	// All lookups done - lock released

	// Handle blocked domains
	if blocked {
		msg.SetRcode(r, dns.RcodeNameError)
		w.WriteMsg(msg)
		return
	}

	// Handle A/AAAA overrides
	if hasOverride {
		if qtype == dns.TypeA && overrideIP.To4() != nil {
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				A: overrideIP.To4(),
			}
			msg.Answer = append(msg.Answer, rr)
		} else if qtype == dns.TypeAAAA && overrideIP.To16() != nil && overrideIP.To4() == nil {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				AAAA: overrideIP.To16(),
			}
			msg.Answer = append(msg.Answer, rr)
		}
		w.WriteMsg(msg)
		return
	}

	// Handle CNAME overrides
	if hasCNAME {
		rr := &dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   domain,
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
				Ttl:    300,
			},
			Target: cnameTarget,
		}
		msg.Answer = append(msg.Answer, rr)
		w.WriteMsg(msg)
		return
	}

	// If we get here, we don't have a local answer
	// Forward to upstream DNS
	if h.Forwarder != nil {
		resp, err := h.Forwarder.Forward(ctx, r)
		if err != nil {
			// Forwarding failed, return SERVFAIL
			msg.SetRcode(r, dns.RcodeServerFailure)
			w.WriteMsg(msg)
			return
		}

		// Cache the upstream response (if cache is enabled)
		// This includes both successful responses and negative responses (NXDOMAIN)
		if h.Cache != nil {
			h.Cache.Set(ctx, r, resp)
		}

		// Return the upstream response
		w.WriteMsg(resp)
		return
	}

	// No forwarder configured, return NXDOMAIN
	msg.SetRcode(r, dns.RcodeNameError)
	w.WriteMsg(msg)
}
