package dns

import (
	"context"
	"net"
	"sync"

	"codeberg.org/miekg/dns"
)

// Handler is a DNS handler
type Handler struct {
	Blocklist        map[string]struct{}
	BlocklistMu      sync.RWMutex
	Whitelist        map[string]struct{}
	WhitelistMu      sync.RWMutex
	Overrides        map[string]net.IP
	OverridesMu      sync.RWMutex
	CNAMEOverrides   map[string]string
	CNAMEOverridesMu sync.RWMutex
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

// ServeDNS implements the dns.Handler interface
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
	// The logic from gemini.md will go here.
	// For now, we just return a simple response.
	msg := new(dns.Msg)
	msg.ID = r.ID
	msg.Response = true
	msg.Authoritative = true
	msg.Opcode = r.Opcode
	msg.RecursionDesired = r.RecursionDesired
	msg.Question = r.Question
	msg.WriteTo(w)
}
