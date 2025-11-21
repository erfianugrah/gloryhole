package localrecords

import (
	"fmt"
	"net"
	"testing"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	if mgr == nil {
		t.Fatal("NewManager() returned nil")
	}

	if mgr.Count() != 0 {
		t.Errorf("expected empty manager, got %d records", mgr.Count())
	}
}

func TestAddRecord_ARecord(t *testing.T) {
	mgr := NewManager()

	ip := net.ParseIP("192.168.1.100")
	record := NewARecord("nas.local", ip)

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	if mgr.Count() != 1 {
		t.Errorf("expected 1 record, got %d", mgr.Count())
	}

	// Lookup the record
	ips, ttl, found := mgr.LookupA("nas.local")
	if !found {
		t.Fatal("record not found")
	}

	if len(ips) != 1 {
		t.Fatalf("expected 1 IP, got %d", len(ips))
	}

	if !ips[0].Equal(ip) {
		t.Errorf("expected IP %v, got %v", ip, ips[0])
	}

	if ttl != 300 {
		t.Errorf("expected TTL 300, got %d", ttl)
	}
}

func TestAddRecord_AAAARecord(t *testing.T) {
	mgr := NewManager()

	ip := net.ParseIP("fe80::1")
	record := NewAAAARecord("server.local", ip)

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup the record
	ips, ttl, found := mgr.LookupAAAA("server.local")
	if !found {
		t.Fatal("record not found")
	}

	if len(ips) != 1 {
		t.Fatalf("expected 1 IP, got %d", len(ips))
	}

	if !ips[0].Equal(ip) {
		t.Errorf("expected IP %v, got %v", ip, ips[0])
	}

	if ttl != 300 {
		t.Errorf("expected TTL 300, got %d", ttl)
	}
}

func TestAddRecord_CNAMERecord(t *testing.T) {
	mgr := NewManager()

	record := NewCNAMERecord("storage.local", "nas.local")

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup the CNAME
	target, ttl, found := mgr.LookupCNAME("storage.local")
	if !found {
		t.Fatal("CNAME not found")
	}

	if target != "nas.local." {
		t.Errorf("expected target nas.local., got %s", target)
	}

	if ttl != 300 {
		t.Errorf("expected TTL 300, got %d", ttl)
	}
}

func TestAddRecord_MultipleIPs(t *testing.T) {
	mgr := NewManager()

	ip1 := net.ParseIP("192.168.1.100")
	ip2 := net.ParseIP("192.168.1.101")

	record := NewARecord("server.local", ip1)
	record.IPs = append(record.IPs, ip2)

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup the record
	ips, _, found := mgr.LookupA("server.local")
	if !found {
		t.Fatal("record not found")
	}

	if len(ips) != 2 {
		t.Fatalf("expected 2 IPs, got %d", len(ips))
	}

	if !ips[0].Equal(ip1) || !ips[1].Equal(ip2) {
		t.Errorf("IPs don't match: got %v and %v", ips[0], ips[1])
	}
}

func TestRemoveRecord(t *testing.T) {
	mgr := NewManager()

	ip := net.ParseIP("192.168.1.100")
	record := NewARecord("nas.local", ip)

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Remove the record
	if err := mgr.RemoveRecord("nas.local", RecordTypeA); err != nil {
		t.Fatalf("RemoveRecord() error = %v", err)
	}

	// Verify it's gone
	_, _, found := mgr.LookupA("nas.local")
	if found {
		t.Error("record should have been removed")
	}

	if mgr.Count() != 0 {
		t.Errorf("expected 0 records, got %d", mgr.Count())
	}
}

func TestRemoveRecord_NotFound(t *testing.T) {
	mgr := NewManager()

	err := mgr.RemoveRecord("nonexistent.local", RecordTypeA)
	if err != ErrRecordNotFound {
		t.Errorf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestLookupCaseInsensitive(t *testing.T) {
	mgr := NewManager()

	ip := net.ParseIP("192.168.1.100")
	record := NewARecord("NAS.LOCAL", ip)

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup with different case
	tests := []string{
		"nas.local",
		"NAS.LOCAL",
		"Nas.Local",
		"nas.local.",
		"NAS.LOCAL.",
	}

	for _, domain := range tests {
		ips, _, found := mgr.LookupA(domain)
		if !found {
			t.Errorf("lookup failed for %s", domain)
		}
		if len(ips) != 1 || !ips[0].Equal(ip) {
			t.Errorf("wrong IP for %s", domain)
		}
	}
}

func TestWildcardMatch(t *testing.T) {
	mgr := NewManager()

	ip := net.ParseIP("192.168.1.100")
	record := NewARecord("*.dev.local", ip)
	record.Wildcard = true

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test wildcard matches
	tests := []struct {
		domain string
		shouldMatch bool
	}{
		{"server.dev.local", true},
		{"web.dev.local", true},
		{"api.dev.local", true},
		{"dev.local", false},  // Exact match of base domain
		{"other.local", false}, // Different domain
		{"sub.server.dev.local", false}, // Too many labels
	}

	for _, tt := range tests {
		ips, _, found := mgr.LookupA(tt.domain)
		if found != tt.shouldMatch {
			t.Errorf("%s: expected match=%v, got %v", tt.domain, tt.shouldMatch, found)
		}
		if tt.shouldMatch && (len(ips) != 1 || !ips[0].Equal(ip)) {
			t.Errorf("%s: wrong IP", tt.domain)
		}
	}
}

func TestResolveCNAME_Simple(t *testing.T) {
	mgr := NewManager()

	// Add A record
	ip := net.ParseIP("192.168.1.100")
	aRecord := NewARecord("nas.local", ip)
	if err := mgr.AddRecord(aRecord); err != nil {
		t.Fatalf("AddRecord(A) error = %v", err)
	}

	// Add CNAME pointing to A record
	cnameRecord := NewCNAMERecord("storage.local", "nas.local")
	if err := mgr.AddRecord(cnameRecord); err != nil {
		t.Fatalf("AddRecord(CNAME) error = %v", err)
	}

	// Resolve CNAME
	ips, ttl, found := mgr.ResolveCNAME("storage.local", 10)
	if !found {
		t.Fatal("CNAME resolution failed")
	}

	if len(ips) != 1 || !ips[0].Equal(ip) {
		t.Errorf("expected IP %v, got %v", ip, ips)
	}

	if ttl != 300 {
		t.Errorf("expected TTL 300, got %d", ttl)
	}
}

func TestResolveCNAME_Chain(t *testing.T) {
	mgr := NewManager()

	// Add A record
	ip := net.ParseIP("192.168.1.100")
	aRecord := NewARecord("server.local", ip)
	if err := mgr.AddRecord(aRecord); err != nil {
		t.Fatalf("AddRecord(A) error = %v", err)
	}

	// Add CNAME chain: alias1 -> alias2 -> server
	cname1 := NewCNAMERecord("alias1.local", "alias2.local")
	if err := mgr.AddRecord(cname1); err != nil {
		t.Fatalf("AddRecord(CNAME1) error = %v", err)
	}

	cname2 := NewCNAMERecord("alias2.local", "server.local")
	if err := mgr.AddRecord(cname2); err != nil {
		t.Fatalf("AddRecord(CNAME2) error = %v", err)
	}

	// Resolve CNAME chain
	ips, _, found := mgr.ResolveCNAME("alias1.local", 10)
	if !found {
		t.Fatal("CNAME chain resolution failed")
	}

	if len(ips) != 1 || !ips[0].Equal(ip) {
		t.Errorf("expected IP %v, got %v", ip, ips)
	}
}

func TestResolveCNAME_Loop(t *testing.T) {
	mgr := NewManager()

	// Create CNAME loop: a -> b -> c -> a
	cname1 := NewCNAMERecord("a.local", "b.local")
	cname2 := NewCNAMERecord("b.local", "c.local")
	cname3 := NewCNAMERecord("c.local", "a.local")

	if err := mgr.AddRecord(cname1); err != nil {
		t.Fatalf("Failed to add cname1: %v", err)
	}
	if err := mgr.AddRecord(cname2); err != nil {
		t.Fatalf("Failed to add cname2: %v", err)
	}
	if err := mgr.AddRecord(cname3); err != nil {
		t.Fatalf("Failed to add cname3: %v", err)
	}

	// Should detect loop and return not found
	_, _, found := mgr.ResolveCNAME("a.local", 10)
	if found {
		t.Error("CNAME loop should have been detected")
	}
}

func TestResolveCNAME_MinTTL(t *testing.T) {
	mgr := NewManager()

	// Add A record with TTL 600
	ip := net.ParseIP("192.168.1.100")
	aRecord := NewARecord("server.local", ip)
	aRecord.TTL = 600
	if err := mgr.AddRecord(aRecord); err != nil {
		t.Fatalf("AddRecord(A) error = %v", err)
	}

	// Add CNAME with TTL 60 (lower)
	cnameRecord := NewCNAMERecord("storage.local", "server.local")
	cnameRecord.TTL = 60
	if err := mgr.AddRecord(cnameRecord); err != nil {
		t.Fatalf("AddRecord(CNAME) error = %v", err)
	}

	// Resolve CNAME - should use minimum TTL
	_, ttl, found := mgr.ResolveCNAME("storage.local", 10)
	if !found {
		t.Fatal("CNAME resolution failed")
	}

	// Should use the minimum TTL from the chain
	if ttl != 60 {
		t.Errorf("expected minimum TTL 60, got %d", ttl)
	}
}

func TestHasRecord(t *testing.T) {
	mgr := NewManager()

	if mgr.HasRecord("nas.local") {
		t.Error("HasRecord should return false for empty manager")
	}

	ip := net.ParseIP("192.168.1.100")
	record := NewARecord("nas.local", ip)
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("Failed to add record: %v", err)
	}

	if !mgr.HasRecord("nas.local") {
		t.Error("HasRecord should return true")
	}

	if !mgr.HasRecord("NAS.LOCAL") {
		t.Error("HasRecord should be case-insensitive")
	}

	if mgr.HasRecord("other.local") {
		t.Error("HasRecord should return false for non-existent domain")
	}
}

func TestGetAllRecords(t *testing.T) {
	mgr := NewManager()

	// Add multiple records
	if err := mgr.AddRecord(NewARecord("server1.local", net.ParseIP("192.168.1.100"))); err != nil {
		t.Fatalf("Failed to add server1 record: %v", err)
	}
	if err := mgr.AddRecord(NewARecord("server2.local", net.ParseIP("192.168.1.101"))); err != nil {
		t.Fatalf("Failed to add server2 record: %v", err)
	}
	if err := mgr.AddRecord(NewCNAMERecord("alias.local", "server1.local")); err != nil {
		t.Fatalf("Failed to add alias record: %v", err)
	}

	all := mgr.GetAllRecords()
	if len(all) != 3 {
		t.Errorf("expected 3 records, got %d", len(all))
	}
}

func TestClear(t *testing.T) {
	mgr := NewManager()

	// Add some records
	if err := mgr.AddRecord(NewARecord("server1.local", net.ParseIP("192.168.1.100"))); err != nil {
		t.Fatalf("Failed to add server1 record: %v", err)
	}
	if err := mgr.AddRecord(NewARecord("server2.local", net.ParseIP("192.168.1.101"))); err != nil {
		t.Fatalf("Failed to add server2 record: %v", err)
	}

	if mgr.Count() != 2 {
		t.Errorf("expected 2 records before clear, got %d", mgr.Count())
	}

	// Clear all records
	mgr.Clear()

	if mgr.Count() != 0 {
		t.Errorf("expected 0 records after clear, got %d", mgr.Count())
	}

	// Verify records are gone
	_, _, found := mgr.LookupA("server1.local")
	if found {
		t.Error("records should have been cleared")
	}
}

func TestClone(t *testing.T) {
	ip := net.ParseIP("192.168.1.100")
	original := NewARecord("nas.local", ip)
	original.TTL = 600

	clone := original.Clone()

	// Verify clone has same values
	if clone.Domain != original.Domain {
		t.Error("domain mismatch")
	}
	if clone.TTL != original.TTL {
		t.Error("TTL mismatch")
	}
	if len(clone.IPs) != len(original.IPs) {
		t.Error("IPs length mismatch")
	}

	// Verify deep copy (modifying clone doesn't affect original)
	clone.TTL = 120
	if original.TTL == 120 {
		t.Error("clone modification affected original")
	}
}

func TestConcurrentAccess(t *testing.T) {
	mgr := NewManager()

	// Add initial record
	ip := net.ParseIP("192.168.1.100")
	if err := mgr.AddRecord(NewARecord("server.local", ip)); err != nil {
		t.Fatalf("Failed to add record: %v", err)
	}

	// Concurrent reads and writes
	done := make(chan bool)

	// Reader goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				mgr.LookupA("server.local")
				mgr.HasRecord("server.local")
			}
			done <- true
		}()
	}

	// Writer goroutines
	for i := 0; i < 5; i++ {
		go func(n int) {
			for j := 0; j < 50; j++ {
				domain := fmt.Sprintf("server%d.local", n)
				_ = mgr.AddRecord(NewARecord(domain, net.ParseIP("192.168.1.100")))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name   string
		record *LocalRecord
		want   string
	}{
		{
			name:   "A record",
			record: NewARecord("server.local", net.ParseIP("192.168.1.100")),
			want:   "server.local. 300 IN A [192.168.1.100]",
		},
		{
			name:   "CNAME record",
			record: NewCNAMERecord("alias.local", "server.local"),
			want:   "alias.local. 300 IN CNAME server.local.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.record.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
