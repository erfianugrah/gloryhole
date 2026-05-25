package dns

import (
	"context"
	"net"
	"testing"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"

	"github.com/miekg/dns"
)

// TestServeDNS_BlocklistManagerPath tests the fast path with BlocklistManager
func TestServeDNS_BlocklistManagerPath(t *testing.T) {
	handler := NewHandler()

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.Config{
		Blocklists: []string{}, // Empty blocklists
	}

	// Set up blocklist manager (fast path)
	mgr := blocklist.NewManager(cfg, logger, nil, nil)
	handler.SetBlocklistManager(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should return NXDOMAIN (no upstream)
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN (no upstream), got %d", w.msg.Rcode)
	}
}

// TestServeDNS_BlocklistManagerWithOverride tests override path

// TestServeDNS_BlocklistManagerWithCNAMEOverride tests CNAME override path

// TestServeDNS_TCPClientIP tests client IP extraction from TCP connection
func TestServeDNS_TCPClientIP(t *testing.T) {
	handler := NewHandler()

	w := &mockResponseWriter{
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("192.168.1.50"), Port: 5353},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Just verify the handler processes TCP connections
}

// TestServeDNS_LocalRecordAAAAFallthrough tests AAAA query with no match
func TestServeDNS_LocalRecordAAAAFallthrough(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add only A record, not AAAA
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:  "ipv4only.local.",
		Type:    localrecords.RecordTypeA,
		IPs:     []net.IP{net.ParseIP("192.168.1.1")},
		TTL:     300,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Failed to add A record: %v", err)
	}
	handler.SetLocalRecords(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	// Query for AAAA record that doesn't exist
	req := new(dns.Msg)
	req.SetQuestion("ipv4only.local.", dns.TypeAAAA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should return NXDOMAIN since no AAAA record and no forwarder
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN, got %d", w.msg.Rcode)
	}
}

// TestServeDNS_LocalRecordCNAMEResolutionEmpty tests CNAME resolution that returns empty
func TestServeDNS_LocalRecordCNAMEResolutionEmpty(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add CNAME that points to non-existent target
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:  "broken.local.",
		Type:    localrecords.RecordTypeCNAME,
		Target:  "nonexistent.local.",
		TTL:     300,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Failed to add CNAME record: %v", err)
	}
	handler.SetLocalRecords(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	// Query for A record - will try to resolve CNAME but find nothing
	req := new(dns.Msg)
	req.SetQuestion("broken.local.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should fallthrough to next handler (no forwarder = NXDOMAIN)
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN, got %d", w.msg.Rcode)
	}
}

// TestServeDNS_LegacyBlocklistPath tests the slow path without BlocklistManager
func TestServeDNS_LegacyBlocklistPath(t *testing.T) {
	handler := NewHandler()
	// Don't set BlocklistManager - force legacy path

	handler.Blocklist["blocked.example.com."] = struct{}{}

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("blocked.example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN for blocked domain, got %d", w.msg.Rcode)
	}
}

// TestServeDNS_LegacyPathWithAAA AOverride tests legacy path with IPv6 override

// TestServeDNS_LegacyPathWithCNAME tests legacy path with CNAME override

// TestServeDNS_ConditionalForwardingEvaluation tests RuleEvaluator path
func TestServeDNS_ConditionalForwardingEvaluation(t *testing.T) {
	handler := NewHandler()

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		ConditionalForwarding: config.ConditionalForwardingConfig{
			Enabled: true,
			Rules: []config.ForwardingRule{
				{
					Name:      "corp-rule",
					Domains:   []string{"corp.local"},
					Upstreams: []string{"192.168.1.1:53"},
					Enabled:   true,
					Priority:  50,
				},
			},
		},
	}

	fwd := forwarder.NewForwarder(cfg, logger, nil)
	handler.SetForwarder(fwd)

	// Create RuleEvaluator
	evaluator, err := forwarder.NewRuleEvaluator(&cfg.ConditionalForwarding)
	if err != nil {
		t.Fatalf("Failed to create RuleEvaluator: %v", err)
	}
	handler.SetRuleEvaluator(evaluator)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("server.corp.local.", dns.TypeA)

	// Will try to forward to conditional upstream (may fail in test)
	handler.ServeDNS(context.Background(), w, req)

	// Just verify we got a response
	if w.msg == nil {
		t.Fatal("Expected response message")
	}
}

// TestServeDNS_PolicyEngineAllowNoForwarder tests policy allow without forwarder
func TestServeDNS_PolicyEngineAllowNoForwarder(t *testing.T) {
	handler := NewHandler()

	engine := policy.NewEngine(nil)
	rule := &policy.Rule{
		Name:    "allow-test",
		Logic:   `Domain == "allowed.example.com"`,
		Enabled: true,
		Action:  policy.ActionAllow,
	}
	if err := engine.AddRule(rule); err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}
	handler.SetPolicyEngine(engine)

	// No forwarder set

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("allowed.example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should return NXDOMAIN when no forwarder
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN when no forwarder, got %d", w.msg.Rcode)
	}
}

// TestServeDNS_PolicyEngineRedirectTypeMismatchAAAA tests redirect type mismatch for AAAA
func TestServeDNS_PolicyEngineRedirectTypeMismatchAAAA(t *testing.T) {
	handler := NewHandler()

	engine := policy.NewEngine(nil)
	// Redirect to IPv4 address but query for AAAA
	rule := &policy.Rule{
		Name:       "redirect-mismatch",
		Logic:      `Domain == "mismatch.example.com"`,
		Enabled:    true,
		Action:     policy.ActionRedirect,
		ActionData: "192.168.1.1", // IPv4 address
	}
	if err := engine.AddRule(rule); err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}
	handler.SetPolicyEngine(engine)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("mismatch.example.com.", dns.TypeAAAA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should return empty answer (NODATA) for type mismatch
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess (NODATA), got %d", w.msg.Rcode)
	}
	if len(w.msg.Answer) != 0 {
		t.Errorf("Expected 0 answers for type mismatch, got %d", len(w.msg.Answer))
	}
}
