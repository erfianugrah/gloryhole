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

	// Also add to whitelist to test override
	whitelist := map[string]struct{}{"whitelisted.example.com.": {}}
	handler.Whitelist.Store(&whitelist)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("whitelisted.example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should not be blocked since it's whitelisted
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN (no upstream), got %d", w.msg.Rcode)
	}
}

// TestServeDNS_BlocklistManagerWithOverride tests override path
func TestServeDNS_BlocklistManagerWithOverride(t *testing.T) {
	handler := NewHandler()

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.Config{
		Blocklists: []string{},
	}

	mgr := blocklist.NewManager(cfg, logger, nil, nil)
	handler.SetBlocklistManager(mgr)

	// Add override for A record
	handler.Overrides["override.local."] = net.ParseIP("192.168.1.1")

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("override.local.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess, got %d", w.msg.Rcode)
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}

	aRecord, ok := w.msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("Expected A record, got %T", w.msg.Answer[0])
	}
	if !aRecord.A.Equal(net.ParseIP("192.168.1.1")) {
		t.Errorf("Expected 192.168.1.1, got %s", aRecord.A)
	}
}

// TestServeDNS_BlocklistManagerWithCNAMEOverride tests CNAME override path
func TestServeDNS_BlocklistManagerWithCNAMEOverride(t *testing.T) {
	handler := NewHandler()

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.Config{
		Blocklists: []string{},
	}

	mgr := blocklist.NewManager(cfg, logger, nil, nil)
	handler.SetBlocklistManager(mgr)

	// Add CNAME override
	handler.CNAMEOverrides["alias.local."] = "target.local."

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("alias.local.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess, got %d", w.msg.Rcode)
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}

	cnameRecord, ok := w.msg.Answer[0].(*dns.CNAME)
	if !ok {
		t.Fatalf("Expected CNAME record, got %T", w.msg.Answer[0])
	}
	if cnameRecord.Target != "target.local." {
		t.Errorf("Expected target.local., got %s", cnameRecord.Target)
	}
}

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
func TestServeDNS_LegacyPathWithAAAAOverride(t *testing.T) {
	handler := NewHandler()

	// Add IPv6 override (legacy path, no BlocklistManager)
	handler.Overrides["ipv6.local."] = net.ParseIP("fe80::1")

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("ipv6.local.", dns.TypeAAAA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess, got %d", w.msg.Rcode)
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}

	aaaaRecord, ok := w.msg.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatalf("Expected AAAA record, got %T", w.msg.Answer[0])
	}
	if !aaaaRecord.AAAA.Equal(net.ParseIP("fe80::1")) {
		t.Errorf("Expected fe80::1, got %s", aaaaRecord.AAAA)
	}
}

// TestServeDNS_LegacyPathWithCNAME tests legacy path with CNAME override
func TestServeDNS_LegacyPathWithCNAME(t *testing.T) {
	handler := NewHandler()

	// Add CNAME override (legacy path)
	handler.CNAMEOverrides["www.local."] = "server.local."

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("www.local.", dns.TypeCNAME)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess, got %d", w.msg.Rcode)
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}

	cnameRecord, ok := w.msg.Answer[0].(*dns.CNAME)
	if !ok {
		t.Fatalf("Expected CNAME record, got %T", w.msg.Answer[0])
	}
	if cnameRecord.Target != "server.local." {
		t.Errorf("Expected server.local., got %s", cnameRecord.Target)
	}
}

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

	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	// Create RuleEvaluator
	evaluator, err := forwarder.NewRuleEvaluator(&cfg.ConditionalForwarding)
	if err != nil {
		t.Fatalf("Failed to create RuleEvaluator: %v", err)
	}
	handler.RuleEvaluator = evaluator

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

	engine := policy.NewEngine()
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

	engine := policy.NewEngine()
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
