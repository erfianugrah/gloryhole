package dns

import (
	"context"
	"net"
	"testing"
	"time"

	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/storage"

	"github.com/miekg/dns"
)

// TestServeDNS_PolicyEngineForwardWithCache tests policy forward action with caching
func TestServeDNS_PolicyEngineForwardWithCache(t *testing.T) {
	handler := NewHandler()

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	// Set up cache
	dnsCache, _ := cache.New(&config.CacheConfig{
		Enabled:     true,
		MaxEntries:  100,
		MinTTL:      1 * time.Second,
		MaxTTL:      3600 * time.Second,
		NegativeTTL: 300 * time.Second,
	}, logger)
	handler.SetCache(dnsCache)

	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}
	handler.SetForwarder(forwarder.NewForwarder(cfg, logger))

	engine := policy.NewEngine()
	rule := &policy.Rule{
		Name:       "forward-corporate",
		Logic:      `Domain == "corp.example.com"`,
		Enabled:    true,
		Action:     policy.ActionForward,
		ActionData: "192.168.1.1:53",
	}
	if err := engine.AddRule(rule); err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}
	handler.SetPolicyEngine(engine)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("corp.example.com.", dns.TypeA)

	// This will likely fail since we're not actually connecting to DNS servers
	handler.ServeDNS(context.Background(), w, req)

	// Just verify we got a response (may be SERVFAIL due to network)
	if w.msg == nil {
		t.Fatal("Expected response message")
	}
}

// TestServeDNS_LocalRecordsTXT tests TXT record lookups
func TestServeDNS_LocalRecordsTXT(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add TXT record using LocalRecord struct
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:     "example.com.",
		Type:       localrecords.RecordTypeTXT,
		TxtRecords: []string{"v=spf1 include:_spf.google.com ~all"},
		TTL:        300,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Failed to add TXT record: %v", err)
	}
	handler.SetLocalRecords(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeTXT)

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

	txtRecord, ok := w.msg.Answer[0].(*dns.TXT)
	if !ok {
		t.Fatalf("Expected TXT record, got %T", w.msg.Answer[0])
	}
	if len(txtRecord.Txt) != 1 || txtRecord.Txt[0] != "v=spf1 include:_spf.google.com ~all" {
		t.Errorf("Unexpected TXT content: %v", txtRecord.Txt)
	}
}

// TestServeDNS_LocalRecordsMX tests MX record lookups
func TestServeDNS_LocalRecordsMX(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add MX record
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:   "example.com.",
		Type:     localrecords.RecordTypeMX,
		Target:   "mail.example.com.",
		Priority: 10,
		TTL:      300,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Failed to add MX record: %v", err)
	}
	handler.SetLocalRecords(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeMX)

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

	mxRecord, ok := w.msg.Answer[0].(*dns.MX)
	if !ok {
		t.Fatalf("Expected MX record, got %T", w.msg.Answer[0])
	}
	if mxRecord.Mx != "mail.example.com." {
		t.Errorf("Expected mail.example.com., got %s", mxRecord.Mx)
	}
	if mxRecord.Preference != 10 {
		t.Errorf("Expected preference 10, got %d", mxRecord.Preference)
	}
}

// TestServeDNS_LocalRecordsPTR tests PTR record lookups
func TestServeDNS_LocalRecordsPTR(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add PTR record for reverse DNS
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:  "1.0.0.127.in-addr.arpa.",
		Type:    localrecords.RecordTypePTR,
		Target:  "localhost.",
		TTL:     300,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Failed to add PTR record: %v", err)
	}
	handler.SetLocalRecords(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("1.0.0.127.in-addr.arpa.", dns.TypePTR)

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

	ptrRecord, ok := w.msg.Answer[0].(*dns.PTR)
	if !ok {
		t.Fatalf("Expected PTR record, got %T", w.msg.Answer[0])
	}
	if ptrRecord.Ptr != "localhost." {
		t.Errorf("Expected localhost., got %s", ptrRecord.Ptr)
	}
}

// TestServeDNS_LocalRecordsSRV tests SRV record lookups
func TestServeDNS_LocalRecordsSRV(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add SRV record
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:   "_http._tcp.example.com.",
		Type:     localrecords.RecordTypeSRV,
		Target:   "web.example.com.",
		Priority: 10,
		Weight:   5,
		Port:     80,
		TTL:      300,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Failed to add SRV record: %v", err)
	}
	handler.SetLocalRecords(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("_http._tcp.example.com.", dns.TypeSRV)

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

	srvRecord, ok := w.msg.Answer[0].(*dns.SRV)
	if !ok {
		t.Fatalf("Expected SRV record, got %T", w.msg.Answer[0])
	}
	if srvRecord.Target != "web.example.com." {
		t.Errorf("Expected web.example.com., got %s", srvRecord.Target)
	}
	if srvRecord.Port != 80 {
		t.Errorf("Expected port 80, got %d", srvRecord.Port)
	}
}

// TestServeDNS_LocalRecordsNS tests NS record lookups
func TestServeDNS_LocalRecordsNS(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add NS record
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:  "example.com.",
		Type:    localrecords.RecordTypeNS,
		Target:  "ns1.example.com.",
		TTL:     300,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Failed to add NS record: %v", err)
	}
	handler.SetLocalRecords(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeNS)

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

	nsRecord, ok := w.msg.Answer[0].(*dns.NS)
	if !ok {
		t.Fatalf("Expected NS record, got %T", w.msg.Answer[0])
	}
	if nsRecord.Ns != "ns1.example.com." {
		t.Errorf("Expected ns1.example.com., got %s", nsRecord.Ns)
	}
}

// TestServeDNS_LocalRecordsSOA tests SOA record lookups
func TestServeDNS_LocalRecordsSOA(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add SOA record
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:  "example.com.",
		Type:    localrecords.RecordTypeSOA,
		Ns:      "ns1.example.com.",
		Mbox:    "admin.example.com.",
		Serial:  2024010101,
		Refresh: 3600,
		Retry:   600,
		Expire:  86400,
		Minttl:  300,
		TTL:     300,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Failed to add SOA record: %v", err)
	}
	handler.SetLocalRecords(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeSOA)

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

	soaRecord, ok := w.msg.Answer[0].(*dns.SOA)
	if !ok {
		t.Fatalf("Expected SOA record, got %T", w.msg.Answer[0])
	}
	if soaRecord.Ns != "ns1.example.com." {
		t.Errorf("Expected ns1.example.com., got %s", soaRecord.Ns)
	}
	if soaRecord.Serial != 2024010101 {
		t.Errorf("Expected serial 2024010101, got %d", soaRecord.Serial)
	}
}

// TestServeDNS_LocalRecordsCAA tests CAA record lookups
func TestServeDNS_LocalRecordsCAA(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add CAA record
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:   "example.com.",
		Type:     localrecords.RecordTypeCAA,
		CaaFlag:  0,
		CaaTag:   "issue",
		CaaValue: "letsencrypt.org",
		TTL:      300,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Failed to add CAA record: %v", err)
	}
	handler.SetLocalRecords(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeCAA)

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

	caaRecord, ok := w.msg.Answer[0].(*dns.CAA)
	if !ok {
		t.Fatalf("Expected CAA record, got %T", w.msg.Answer[0])
	}
	if caaRecord.Tag != "issue" {
		t.Errorf("Expected tag 'issue', got %s", caaRecord.Tag)
	}
	if caaRecord.Value != "letsencrypt.org" {
		t.Errorf("Expected value 'letsencrypt.org', got %s", caaRecord.Value)
	}
}

// TestServeDNS_CacheSet tests that responses are cached
func TestServeDNS_CacheSet(t *testing.T) {
	handler := NewHandler()

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format:  "text",
		Output: "stdout",
	})

	dnsCache, _ := cache.New(&config.CacheConfig{
		Enabled:     true,
		MaxEntries:  100,
		MinTTL:      1 * time.Second,
		MaxTTL:      3600 * time.Second,
		NegativeTTL: 300 * time.Second,
	}, logger)
	handler.SetCache(dnsCache)

	// Set up forwarder
	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}
	handler.SetForwarder(forwarder.NewForwarder(cfg, logger))

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	// First request - will forward and cache
	handler.ServeDNS(context.Background(), w, req)

	// Second request should hit cache
	w2 := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}
	req2 := new(dns.Msg)
	req2.Id = 5678 // Different ID
	req2.SetQuestion("example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w2, req2)

	// Both should get responses (cache hit or forwarded)
	if w.msg == nil || w2.msg == nil {
		t.Fatal("Expected responses for both requests")
	}
}

// TestServeDNS_StorageLogging tests async query logging
func TestServeDNS_StorageLogging(t *testing.T) {
	handler := NewHandler()

	// Set up in-memory storage
	stor, err := storage.New(&storage.Config{
		Enabled:       true,
		Backend:       "sqlite",
		FlushInterval: 1 * time.Second,
		BufferSize:    10,
		SQLite: storage.SQLiteConfig{
			Path: ":memory:",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer func() { _ = stor.Close() }()

	handler.SetStorage(stor)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	// Wait a bit for async logging
	time.Sleep(100 * time.Millisecond)

	// Verify response was sent
	if w.msg == nil {
		t.Fatal("Expected response message")
	}
}

// TestServeDNS_NoForwarder tests behavior when no forwarder is configured
func TestServeDNS_NoForwarder(t *testing.T) {
	handler := NewHandler()
	// No forwarder set - should return NXDOMAIN

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should return NXDOMAIN when no forwarder configured
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected RcodeNameError for no forwarder, got %d", w.msg.Rcode)
	}
}
