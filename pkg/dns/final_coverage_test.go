package dns

import (
	"context"
	"net"
	"testing"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/config"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"

	"github.com/miekg/dns"
)

// TestServeDNS_OverrideIPv4TypMismatch tests IPv4 override with AAAA query (no match)
func TestServeDNS_OverrideIPv4TypeMismatch(t *testing.T) {
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

	// Add IPv4 override
	handler.Overrides["override.local."] = net.ParseIP("192.168.1.1")

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	// Query for AAAA but override is IPv4 - should not match
	req := new(dns.Msg)
	req.SetQuestion("override.local.", dns.TypeAAAA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should return empty response (not match IPv4 override for AAAA query)
	if len(w.msg.Answer) != 0 {
		t.Errorf("Expected 0 answers for IPv4 override with AAAA query, got %d", len(w.msg.Answer))
	}
}

// TestServeDNS_OverrideIPv6TypeMismatch tests IPv6 override with A query (no match)
func TestServeDNS_OverrideIPv6TypeMismatch(t *testing.T) {
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

	// Add IPv6 override
	handler.Overrides["override.local."] = net.ParseIP("fe80::1")

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	// Query for A but override is IPv6 - should not match
	req := new(dns.Msg)
	req.SetQuestion("override.local.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should return empty response (not match IPv6 override for A query)
	if len(w.msg.Answer) != 0 {
		t.Errorf("Expected 0 answers for IPv6 override with A query, got %d", len(w.msg.Answer))
	}
}

// TestServeDNS_CNAMEQueryWithoutCNAME tests querying for CNAME record specifically
func TestServeDNS_CNAMEQueryWithoutCNAME(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add A record only (no CNAME)
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:  "test.local.",
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

	// Query specifically for CNAME record (doesn't exist)
	req := new(dns.Msg)
	req.SetQuestion("test.local.", dns.TypeCNAME)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should fallthrough (no CNAME record exists)
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN, got %d", w.msg.Rcode)
	}
}

// TestServeDNS_LegacyPathIPv4OverrideTypeMismatch tests IPv4 override type mismatch in legacy path
func TestServeDNS_LegacyPathIPv4OverrideTypeMismatch(t *testing.T) {
	handler := NewHandler()

	// Use legacy path (no BlocklistManager)
	handler.Overrides["override.local."] = net.ParseIP("192.168.1.1")

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	// Query for AAAA but override is IPv4
	req := new(dns.Msg)
	req.SetQuestion("override.local.", dns.TypeAAAA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should not match IPv4 override for AAAA query
	if len(w.msg.Answer) != 0 {
		t.Errorf("Expected 0 answers for type mismatch, got %d", len(w.msg.Answer))
	}
}

// TestServeDNS_LegacyPathCNAMEForAAAAQuery tests CNAME override for AAAA query in legacy path
func TestServeDNS_LegacyPathCNAMEForAAAAQuery(t *testing.T) {
	handler := NewHandler()

	// Use legacy path (no BlocklistManager)
	handler.CNAMEOverrides["alias.local."] = "target.local."

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	// Query for AAAA - CNAME should match
	req := new(dns.Msg)
	req.SetQuestion("alias.local.", dns.TypeAAAA)

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

// TestServeDNS_BlocklistManagerCNAMEForAAAAQuery tests CNAME override for AAAA query with BlocklistManager
func TestServeDNS_BlocklistManagerCNAMEForAAAAQuery(t *testing.T) {
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

	// Query for AAAA - CNAME should match
	req := new(dns.Msg)
	req.SetQuestion("alias.local.", dns.TypeAAAA)

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
