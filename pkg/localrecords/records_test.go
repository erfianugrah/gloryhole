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
		domain      string
		shouldMatch bool
	}{
		{"server.dev.local", true},
		{"web.dev.local", true},
		{"api.dev.local", true},
		{"dev.local", false},            // Exact match of base domain
		{"other.local", false},          // Different domain
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
// TXT Record Tests

func TestAddRecord_TXTRecord_SingleString(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeTXT)
	record.TxtRecords = []string{"v=spf1 mx ~all"}

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	if mgr.Count() != 1 {
		t.Errorf("expected 1 record, got %d", mgr.Count())
	}

	// Lookup the TXT record
	records := mgr.LookupTXT("example.local")
	if len(records) == 0 {
		t.Fatal("TXT record not found")
	}

	if len(records[0].TxtRecords) != 1 {
		t.Fatalf("expected 1 TXT string, got %d", len(records[0].TxtRecords))
	}

	if records[0].TxtRecords[0] != "v=spf1 mx ~all" {
		t.Errorf("expected 'v=spf1 mx ~all', got %s", records[0].TxtRecords[0])
	}
}

func TestAddRecord_TXTRecord_MultipleStrings(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeTXT)
	record.TxtRecords = []string{
		"v=spf1 include:_spf.google.com ~all",
		"google-site-verification=abc123xyz",
		"another-verification=def456",
	}

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup the TXT record
	records := mgr.LookupTXT("example.local")
	if len(records) == 0 {
		t.Fatal("TXT record not found")
	}

	if len(records[0].TxtRecords) != 3 {
		t.Fatalf("expected 3 TXT strings, got %d", len(records[0].TxtRecords))
	}

	// Verify all strings are present
	expected := map[string]bool{
		"v=spf1 include:_spf.google.com ~all": false,
		"google-site-verification=abc123xyz":   false,
		"another-verification=def456":          false,
	}

	for _, txt := range records[0].TxtRecords {
		if _, exists := expected[txt]; exists {
			expected[txt] = true
		}
	}

	for txt, found := range expected {
		if !found {
			t.Errorf("expected TXT string not found: %s", txt)
		}
	}
}

func TestAddRecord_TXTRecord_MultipleSeparateRecords(t *testing.T) {
	mgr := NewManager()

	// Add first TXT record
	record1 := NewLocalRecord("example.local", RecordTypeTXT)
	record1.TxtRecords = []string{"v=spf1 mx ~all"}

	if err := mgr.AddRecord(record1); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Add second TXT record for same domain
	record2 := NewLocalRecord("example.local", RecordTypeTXT)
	record2.TxtRecords = []string{"google-site-verification=xyz"}

	if err := mgr.AddRecord(record2); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	if mgr.Count() != 2 {
		t.Errorf("expected 2 records, got %d", mgr.Count())
	}

	// Lookup should return both records
	records := mgr.LookupTXT("example.local")
	if len(records) != 2 {
		t.Fatalf("expected 2 TXT records, got %d", len(records))
	}
}

func TestAddRecord_TXTRecord_Wildcard(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("*.dev.local", RecordTypeTXT)
	record.TxtRecords = []string{"wildcard-txt-record"}
	record.Wildcard = true

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test wildcard matching
	records := mgr.LookupTXT("server.dev.local")
	if len(records) == 0 {
		t.Fatal("wildcard TXT record not found")
	}

	if records[0].TxtRecords[0] != "wildcard-txt-record" {
		t.Errorf("expected 'wildcard-txt-record', got %s", records[0].TxtRecords[0])
	}

	// Test non-matching domain
	records = mgr.LookupTXT("dev.local")
	if len(records) != 0 {
		t.Error("wildcard should not match parent domain")
	}

	// Test multi-level domain (should not match)
	records = mgr.LookupTXT("sub.server.dev.local")
	if len(records) != 0 {
		t.Error("wildcard should not match multi-level subdomain")
	}
}

func TestAddRecord_TXTRecord_EmptyArray(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeTXT)
	record.TxtRecords = []string{} // Empty array

	err := mgr.AddRecord(record)
	if err == nil {
		t.Fatal("expected error for TXT record with empty array, got nil")
	}

	if err != ErrNoTxtData {
		t.Errorf("expected ErrNoTxtData, got %v", err)
	}
}

func TestAddRecord_TXTRecord_TooLong(t *testing.T) {
	mgr := NewManager()

	// Create a string longer than 255 characters
	longString := string(make([]byte, 256))
	for i := range longString {
		longString = longString[:i] + "a" + longString[i+1:]
	}

	record := NewLocalRecord("example.local", RecordTypeTXT)
	record.TxtRecords = []string{longString}

	err := mgr.AddRecord(record)
	if err == nil {
		t.Fatal("expected error for TXT string exceeding 255 chars, got nil")
	}

	if err != ErrTxtTooLong {
		t.Errorf("expected ErrTxtTooLong, got %v", err)
	}
}

func TestAddRecord_TXTRecord_ExactlyMax(t *testing.T) {
	mgr := NewManager()

	// Create a string exactly 255 characters (should be valid)
	maxString := ""
	for i := 0; i < 255; i++ {
		maxString += "a"
	}

	record := NewLocalRecord("example.local", RecordTypeTXT)
	record.TxtRecords = []string{maxString}

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() should accept 255 char TXT string, got error: %v", err)
	}

	records := mgr.LookupTXT("example.local")
	if len(records) == 0 {
		t.Fatal("TXT record not found")
	}

	if len(records[0].TxtRecords[0]) != 255 {
		t.Errorf("expected 255 char string, got %d", len(records[0].TxtRecords[0]))
	}
}

func TestLookupTXT_CaseInsensitive(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("Example.LOCAL", RecordTypeTXT)
	record.TxtRecords = []string{"test-txt"}

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test with different case
	records := mgr.LookupTXT("example.local")
	if len(records) == 0 {
		t.Fatal("TXT record not found with lowercase lookup")
	}

	records = mgr.LookupTXT("EXAMPLE.LOCAL")
	if len(records) == 0 {
		t.Fatal("TXT record not found with uppercase lookup")
	}

	records = mgr.LookupTXT("ExAmPlE.lOcAl")
	if len(records) == 0 {
		t.Fatal("TXT record not found with mixed case lookup")
	}
}

func TestLookupTXT_DisabledRecord(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeTXT)
	record.TxtRecords = []string{"disabled-txt"}
	record.Enabled = false // Disable the record

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup should not return disabled records
	records := mgr.LookupTXT("example.local")
	if len(records) != 0 {
		t.Error("disabled TXT record should not be returned by lookup")
	}
}

func TestLookupTXT_EmptyManager(t *testing.T) {
	mgr := NewManager()

	records := mgr.LookupTXT("nonexistent.local")
	if len(records) != 0 {
		t.Errorf("expected empty result for nonexistent domain, got %d records", len(records))
	}
}

func TestTXTRecord_CustomTTL(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeTXT)
	record.TxtRecords = []string{"test"}
	record.TTL = 3600 // 1 hour

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupTXT("example.local")
	if len(records) == 0 {
		t.Fatal("TXT record not found")
	}

	if records[0].TTL != 3600 {
		t.Errorf("expected TTL 3600, got %d", records[0].TTL)
	}
}

// MX Record Tests

func TestAddRecord_MXRecord_Single(t *testing.T) {
	mgr := NewManager()

	record := NewMXRecord("example.local", "mail.example.local", 10)

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	if mgr.Count() != 1 {
		t.Errorf("expected 1 record, got %d", mgr.Count())
	}

	// Lookup the MX record
	records := mgr.LookupMX("example.local")
	if len(records) == 0 {
		t.Fatal("MX record not found")
	}

	if records[0].Target != "mail.example.local." {
		t.Errorf("expected target 'mail.example.local.', got %s", records[0].Target)
	}

	if records[0].Priority != 10 {
		t.Errorf("expected priority 10, got %d", records[0].Priority)
	}
}

func TestAddRecord_MXRecord_MultiplePriorities(t *testing.T) {
	mgr := NewManager()

	// Add MX records with different priorities
	mx1 := NewMXRecord("example.local", "mail1.example.local", 10)
	mx2 := NewMXRecord("example.local", "mail2.example.local", 20)
	mx3 := NewMXRecord("example.local", "mail3.example.local", 5)

	if err := mgr.AddRecord(mx1); err != nil {
		t.Fatalf("AddRecord(mx1) error = %v", err)
	}
	if err := mgr.AddRecord(mx2); err != nil {
		t.Fatalf("AddRecord(mx2) error = %v", err)
	}
	if err := mgr.AddRecord(mx3); err != nil {
		t.Fatalf("AddRecord(mx3) error = %v", err)
	}

	if mgr.Count() != 3 {
		t.Errorf("expected 3 records, got %d", mgr.Count())
	}

	// Lookup should return records sorted by priority (lowest first)
	records := mgr.LookupMX("example.local")
	if len(records) != 3 {
		t.Fatalf("expected 3 MX records, got %d", len(records))
	}

	// Verify priority order: 5, 10, 20
	if records[0].Priority != 5 {
		t.Errorf("expected first record priority 5, got %d", records[0].Priority)
	}
	if records[0].Target != "mail3.example.local." {
		t.Errorf("expected first record target 'mail3.example.local.', got %s", records[0].Target)
	}

	if records[1].Priority != 10 {
		t.Errorf("expected second record priority 10, got %d", records[1].Priority)
	}
	if records[1].Target != "mail1.example.local." {
		t.Errorf("expected second record target 'mail1.example.local.', got %s", records[1].Target)
	}

	if records[2].Priority != 20 {
		t.Errorf("expected third record priority 20, got %d", records[2].Priority)
	}
	if records[2].Target != "mail2.example.local." {
		t.Errorf("expected third record target 'mail2.example.local.', got %s", records[2].Target)
	}
}

func TestAddRecord_MXRecord_SamePriority(t *testing.T) {
	mgr := NewManager()

	// Add MX records with same priority
	mx1 := NewMXRecord("example.local", "mail1.example.local", 10)
	mx2 := NewMXRecord("example.local", "mail2.example.local", 10)

	if err := mgr.AddRecord(mx1); err != nil {
		t.Fatalf("AddRecord(mx1) error = %v", err)
	}
	if err := mgr.AddRecord(mx2); err != nil {
		t.Fatalf("AddRecord(mx2) error = %v", err)
	}

	// Lookup should return both records
	records := mgr.LookupMX("example.local")
	if len(records) != 2 {
		t.Fatalf("expected 2 MX records, got %d", len(records))
	}

	// Both should have priority 10
	if records[0].Priority != 10 || records[1].Priority != 10 {
		t.Error("expected both records to have priority 10")
	}
}

func TestAddRecord_MXRecord_Wildcard(t *testing.T) {
	mgr := NewManager()

	record := NewMXRecord("*.example.local", "mail.example.local", 10)
	record.Wildcard = true

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test wildcard matching
	records := mgr.LookupMX("subdomain.example.local")
	if len(records) == 0 {
		t.Fatal("wildcard MX record not found")
	}

	if records[0].Priority != 10 {
		t.Errorf("expected priority 10, got %d", records[0].Priority)
	}

	// Test non-matching domain
	records = mgr.LookupMX("example.local")
	if len(records) != 0 {
		t.Error("wildcard should not match parent domain")
	}
}

func TestAddRecord_MXRecord_NoTarget(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeMX)
	record.Target = ""
	record.Priority = 10

	err := mgr.AddRecord(record)
	if err == nil {
		t.Fatal("expected error for MX record with no target, got nil")
	}

	if err != ErrEmptyTarget {
		t.Errorf("expected ErrEmptyTarget, got %v", err)
	}
}

func TestLookupMX_CaseInsensitive(t *testing.T) {
	mgr := NewManager()

	record := NewMXRecord("Example.LOCAL", "mail.example.local", 10)

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test with different case
	records := mgr.LookupMX("example.local")
	if len(records) == 0 {
		t.Fatal("MX record not found with lowercase lookup")
	}

	records = mgr.LookupMX("EXAMPLE.LOCAL")
	if len(records) == 0 {
		t.Fatal("MX record not found with uppercase lookup")
	}
}

func TestLookupMX_DisabledRecord(t *testing.T) {
	mgr := NewManager()

	record := NewMXRecord("example.local", "mail.example.local", 10)
	record.Enabled = false // Disable the record

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup should not return disabled records
	records := mgr.LookupMX("example.local")
	if len(records) != 0 {
		t.Error("disabled MX record should not be returned by lookup")
	}
}

func TestLookupMX_EmptyManager(t *testing.T) {
	mgr := NewManager()

	records := mgr.LookupMX("nonexistent.local")
	if len(records) != 0 {
		t.Errorf("expected empty result for nonexistent domain, got %d records", len(records))
	}
}

func TestMXRecord_CustomTTL(t *testing.T) {
	mgr := NewManager()

	record := NewMXRecord("example.local", "mail.example.local", 10)
	record.TTL = 3600 // 1 hour

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupMX("example.local")
	if len(records) == 0 {
		t.Fatal("MX record not found")
	}

	if records[0].TTL != 3600 {
		t.Errorf("expected TTL 3600, got %d", records[0].TTL)
	}
}

func TestMXRecord_DefaultPriority(t *testing.T) {
	mgr := NewManager()

	// Create MX record with priority 0 (should be valid)
	record := NewMXRecord("example.local", "mail.example.local", 0)

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupMX("example.local")
	if len(records) == 0 {
		t.Fatal("MX record not found")
	}

	if records[0].Priority != 0 {
		t.Errorf("expected priority 0, got %d", records[0].Priority)
	}
}

func TestMXRecord_PrioritySortingStability(t *testing.T) {
	mgr := NewManager()

	// Add multiple MX records in specific order
	for i := 0; i < 5; i++ {
		target := fmt.Sprintf("mail%d.example.local", i)
		record := NewMXRecord("example.local", target, uint16(i*10))
		if err := mgr.AddRecord(record); err != nil {
			t.Fatalf("AddRecord() error = %v", err)
		}
	}

	records := mgr.LookupMX("example.local")
	if len(records) != 5 {
		t.Fatalf("expected 5 MX records, got %d", len(records))
	}

	// Verify they are sorted by priority
	for i := 0; i < len(records)-1; i++ {
		if records[i].Priority > records[i+1].Priority {
			t.Errorf("records not sorted correctly: priority %d followed by %d",
				records[i].Priority, records[i+1].Priority)
		}
	}
}

// PTR Record Tests

func TestAddRecord_PTRRecord_Single(t *testing.T) {
	mgr := NewManager()

	// Standard reverse DNS format for 192.168.1.100
	record := NewPTRRecord("100.1.168.192.in-addr.arpa", "server.example.local")

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	if mgr.Count() != 1 {
		t.Errorf("expected 1 record, got %d", mgr.Count())
	}

	// Lookup the PTR record
	records := mgr.LookupPTR("100.1.168.192.in-addr.arpa")
	if len(records) == 0 {
		t.Fatal("PTR record not found")
	}

	if records[0].Target != "server.example.local." {
		t.Errorf("expected target 'server.example.local.', got %s", records[0].Target)
	}
}

func TestAddRecord_PTRRecord_IPv6(t *testing.T) {
	mgr := NewManager()

	// IPv6 reverse DNS format (simplified for testing)
	record := NewPTRRecord("1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa", "ipv6.example.local")

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup the PTR record
	records := mgr.LookupPTR("1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa")
	if len(records) == 0 {
		t.Fatal("IPv6 PTR record not found")
	}

	if records[0].Target != "ipv6.example.local." {
		t.Errorf("expected target 'ipv6.example.local.', got %s", records[0].Target)
	}
}

func TestAddRecord_PTRRecord_Multiple(t *testing.T) {
	mgr := NewManager()

	// Add multiple PTR records for different IPs
	ptr1 := NewPTRRecord("100.1.168.192.in-addr.arpa", "server1.example.local")
	ptr2 := NewPTRRecord("101.1.168.192.in-addr.arpa", "server2.example.local")
	ptr3 := NewPTRRecord("102.1.168.192.in-addr.arpa", "server3.example.local")

	if err := mgr.AddRecord(ptr1); err != nil {
		t.Fatalf("AddRecord(ptr1) error = %v", err)
	}
	if err := mgr.AddRecord(ptr2); err != nil {
		t.Fatalf("AddRecord(ptr2) error = %v", err)
	}
	if err := mgr.AddRecord(ptr3); err != nil {
		t.Fatalf("AddRecord(ptr3) error = %v", err)
	}

	if mgr.Count() != 3 {
		t.Errorf("expected 3 records, got %d", mgr.Count())
	}

	// Lookup each PTR record
	records1 := mgr.LookupPTR("100.1.168.192.in-addr.arpa")
	if len(records1) == 0 || records1[0].Target != "server1.example.local." {
		t.Error("PTR record 1 not found or incorrect")
	}

	records2 := mgr.LookupPTR("101.1.168.192.in-addr.arpa")
	if len(records2) == 0 || records2[0].Target != "server2.example.local." {
		t.Error("PTR record 2 not found or incorrect")
	}

	records3 := mgr.LookupPTR("102.1.168.192.in-addr.arpa")
	if len(records3) == 0 || records3[0].Target != "server3.example.local." {
		t.Error("PTR record 3 not found or incorrect")
	}
}

func TestAddRecord_PTRRecord_MultipleForSameIP(t *testing.T) {
	mgr := NewManager()

	// Add multiple PTR records for same IP (rare but valid)
	ptr1 := NewPTRRecord("100.1.168.192.in-addr.arpa", "server1.example.local")
	ptr2 := NewPTRRecord("100.1.168.192.in-addr.arpa", "server2.example.local")

	if err := mgr.AddRecord(ptr1); err != nil {
		t.Fatalf("AddRecord(ptr1) error = %v", err)
	}
	if err := mgr.AddRecord(ptr2); err != nil {
		t.Fatalf("AddRecord(ptr2) error = %v", err)
	}

	// Lookup should return both records
	records := mgr.LookupPTR("100.1.168.192.in-addr.arpa")
	if len(records) != 2 {
		t.Fatalf("expected 2 PTR records, got %d", len(records))
	}
}

func TestAddRecord_PTRRecord_Wildcard(t *testing.T) {
	mgr := NewManager()

	// Wildcard PTR (e.g., for subnet)
	record := NewPTRRecord("*.1.168.192.in-addr.arpa", "subnet.example.local")
	record.Wildcard = true

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test wildcard matching
	records := mgr.LookupPTR("50.1.168.192.in-addr.arpa")
	if len(records) == 0 {
		t.Fatal("wildcard PTR record not found")
	}

	if records[0].Target != "subnet.example.local." {
		t.Errorf("expected target 'subnet.example.local.', got %s", records[0].Target)
	}

	// Test non-matching domain
	records = mgr.LookupPTR("1.168.192.in-addr.arpa")
	if len(records) != 0 {
		t.Error("wildcard should not match parent domain")
	}
}

func TestAddRecord_PTRRecord_NoTarget(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("100.1.168.192.in-addr.arpa", RecordTypePTR)
	record.Target = ""

	err := mgr.AddRecord(record)
	if err == nil {
		t.Fatal("expected error for PTR record with no target, got nil")
	}

	if err != ErrEmptyTarget {
		t.Errorf("expected ErrEmptyTarget, got %v", err)
	}
}

func TestLookupPTR_CaseInsensitive(t *testing.T) {
	mgr := NewManager()

	record := NewPTRRecord("100.1.168.192.IN-ADDR.ARPA", "server.example.local")

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test with different case
	records := mgr.LookupPTR("100.1.168.192.in-addr.arpa")
	if len(records) == 0 {
		t.Fatal("PTR record not found with lowercase lookup")
	}

	records = mgr.LookupPTR("100.1.168.192.IN-ADDR.ARPA")
	if len(records) == 0 {
		t.Fatal("PTR record not found with uppercase lookup")
	}
}

func TestLookupPTR_DisabledRecord(t *testing.T) {
	mgr := NewManager()

	record := NewPTRRecord("100.1.168.192.in-addr.arpa", "server.example.local")
	record.Enabled = false // Disable the record

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup should not return disabled records
	records := mgr.LookupPTR("100.1.168.192.in-addr.arpa")
	if len(records) != 0 {
		t.Error("disabled PTR record should not be returned by lookup")
	}
}

func TestLookupPTR_EmptyManager(t *testing.T) {
	mgr := NewManager()

	records := mgr.LookupPTR("100.1.168.192.in-addr.arpa")
	if len(records) != 0 {
		t.Errorf("expected empty result for nonexistent PTR, got %d records", len(records))
	}
}

func TestPTRRecord_CustomTTL(t *testing.T) {
	mgr := NewManager()

	record := NewPTRRecord("100.1.168.192.in-addr.arpa", "server.example.local")
	record.TTL = 7200 // 2 hours

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupPTR("100.1.168.192.in-addr.arpa")
	if len(records) == 0 {
		t.Fatal("PTR record not found")
	}

	if records[0].TTL != 7200 {
		t.Errorf("expected TTL 7200, got %d", records[0].TTL)
	}
}

func TestPTRRecord_LocalNetwork(t *testing.T) {
	mgr := NewManager()

	// Common local network PTR records
	ptr1 := NewPTRRecord("1.0.168.192.in-addr.arpa", "router.local")
	ptr2 := NewPTRRecord("10.0.168.192.in-addr.arpa", "nas.local")
	ptr3 := NewPTRRecord("100.0.168.192.in-addr.arpa", "server.local")

	mgr.AddRecord(ptr1)
	mgr.AddRecord(ptr2)
	mgr.AddRecord(ptr3)

	// Verify all local PTR records are accessible
	if len(mgr.LookupPTR("1.0.168.192.in-addr.arpa")) == 0 {
		t.Error("router PTR not found")
	}
	if len(mgr.LookupPTR("10.0.168.192.in-addr.arpa")) == 0 {
		t.Error("nas PTR not found")
	}
	if len(mgr.LookupPTR("100.0.168.192.in-addr.arpa")) == 0 {
		t.Error("server PTR not found")
	}
}

// SRV Record Tests

func TestAddRecord_SRVRecord_Single(t *testing.T) {
	mgr := NewManager()

	// LDAP service record: _ldap._tcp.example.local
	record := NewSRVRecord("_ldap._tcp.example.local", "ldap.example.local", 0, 5, 389)

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	if mgr.Count() != 1 {
		t.Errorf("expected 1 record, got %d", mgr.Count())
	}

	// Lookup the SRV record
	records := mgr.LookupSRV("_ldap._tcp.example.local")
	if len(records) == 0 {
		t.Fatal("SRV record not found")
	}

	if records[0].Target != "ldap.example.local." {
		t.Errorf("expected target 'ldap.example.local.', got %s", records[0].Target)
	}

	if records[0].Priority != 0 {
		t.Errorf("expected priority 0, got %d", records[0].Priority)
	}

	if records[0].Weight != 5 {
		t.Errorf("expected weight 5, got %d", records[0].Weight)
	}

	if records[0].Port != 389 {
		t.Errorf("expected port 389, got %d", records[0].Port)
	}
}

func TestAddRecord_SRVRecord_MultiplePriorities(t *testing.T) {
	mgr := NewManager()

	// Add SRV records with different priorities and weights
	srv1 := NewSRVRecord("_http._tcp.example.local", "server1.example.local", 10, 60, 80)
	srv2 := NewSRVRecord("_http._tcp.example.local", "server2.example.local", 10, 40, 80)
	srv3 := NewSRVRecord("_http._tcp.example.local", "server3.example.local", 20, 100, 80)

	if err := mgr.AddRecord(srv1); err != nil {
		t.Fatalf("AddRecord(srv1) error = %v", err)
	}
	if err := mgr.AddRecord(srv2); err != nil {
		t.Fatalf("AddRecord(srv2) error = %v", err)
	}
	if err := mgr.AddRecord(srv3); err != nil {
		t.Fatalf("AddRecord(srv3) error = %v", err)
	}

	if mgr.Count() != 3 {
		t.Errorf("expected 3 records, got %d", mgr.Count())
	}

	// Lookup should return records sorted by priority, then weight
	records := mgr.LookupSRV("_http._tcp.example.local")
	if len(records) != 3 {
		t.Fatalf("expected 3 SRV records, got %d", len(records))
	}

	// Verify priority 10 records come first, sorted by weight (higher weight first)
	// Order should be: priority=10 weight=60, priority=10 weight=40, priority=20 weight=100
	if records[0].Priority != 10 || records[0].Weight != 60 {
		t.Errorf("expected first record priority=10 weight=60, got priority=%d weight=%d", 
			records[0].Priority, records[0].Weight)
	}

	if records[1].Priority != 10 || records[1].Weight != 40 {
		t.Errorf("expected second record priority=10 weight=40, got priority=%d weight=%d", 
			records[1].Priority, records[1].Weight)
	}

	if records[2].Priority != 20 || records[2].Weight != 100 {
		t.Errorf("expected third record priority=20 weight=100, got priority=%d weight=%d", 
			records[2].Priority, records[2].Weight)
	}
}

func TestAddRecord_SRVRecord_DifferentServices(t *testing.T) {
	mgr := NewManager()

	// Add different service records
	ldap := NewSRVRecord("_ldap._tcp.example.local", "ldap.example.local", 0, 5, 389)
	http := NewSRVRecord("_http._tcp.example.local", "www.example.local", 0, 10, 80)
	https := NewSRVRecord("_https._tcp.example.local", "www.example.local", 0, 10, 443)

	mgr.AddRecord(ldap)
	mgr.AddRecord(http)
	mgr.AddRecord(https)

	// Verify each service has its own records
	if len(mgr.LookupSRV("_ldap._tcp.example.local")) == 0 {
		t.Error("LDAP SRV record not found")
	}
	if len(mgr.LookupSRV("_http._tcp.example.local")) == 0 {
		t.Error("HTTP SRV record not found")
	}
	if len(mgr.LookupSRV("_https._tcp.example.local")) == 0 {
		t.Error("HTTPS SRV record not found")
	}

	// Different services should not interfere
	if mgr.Count() != 3 {
		t.Errorf("expected 3 total records, got %d", mgr.Count())
	}
}


func TestAddRecord_SRVRecord_NoTarget(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("_ldap._tcp.example.local", RecordTypeSRV)
	record.Target = ""
	record.Port = 389

	err := mgr.AddRecord(record)
	if err == nil {
		t.Fatal("expected error for SRV record with no target, got nil")
	}

	if err != ErrEmptyTarget {
		t.Errorf("expected ErrEmptyTarget, got %v", err)
	}
}

func TestAddRecord_SRVRecord_NoPort(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("_ldap._tcp.example.local", RecordTypeSRV)
	record.Target = "ldap.example.local"
	record.Port = 0

	err := mgr.AddRecord(record)
	if err == nil {
		t.Fatal("expected error for SRV record with port 0, got nil")
	}

	if err != ErrInvalidRecord {
		t.Errorf("expected ErrInvalidRecord, got %v", err)
	}
}

func TestLookupSRV_CaseInsensitive(t *testing.T) {
	mgr := NewManager()

	record := NewSRVRecord("_LDAP._TCP.EXAMPLE.LOCAL", "ldap.example.local", 0, 5, 389)

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test with different case
	records := mgr.LookupSRV("_ldap._tcp.example.local")
	if len(records) == 0 {
		t.Fatal("SRV record not found with lowercase lookup")
	}

	records = mgr.LookupSRV("_LDAP._TCP.EXAMPLE.LOCAL")
	if len(records) == 0 {
		t.Fatal("SRV record not found with uppercase lookup")
	}
}

func TestLookupSRV_DisabledRecord(t *testing.T) {
	mgr := NewManager()

	record := NewSRVRecord("_ldap._tcp.example.local", "ldap.example.local", 0, 5, 389)
	record.Enabled = false // Disable the record

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup should not return disabled records
	records := mgr.LookupSRV("_ldap._tcp.example.local")
	if len(records) != 0 {
		t.Error("disabled SRV record should not be returned by lookup")
	}
}

func TestLookupSRV_EmptyManager(t *testing.T) {
	mgr := NewManager()

	records := mgr.LookupSRV("_ldap._tcp.example.local")
	if len(records) != 0 {
		t.Errorf("expected empty result for nonexistent service, got %d records", len(records))
	}
}

func TestSRVRecord_CustomTTL(t *testing.T) {
	mgr := NewManager()

	record := NewSRVRecord("_ldap._tcp.example.local", "ldap.example.local", 0, 5, 389)
	record.TTL = 1800 // 30 minutes

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupSRV("_ldap._tcp.example.local")
	if len(records) == 0 {
		t.Fatal("SRV record not found")
	}

	if records[0].TTL != 1800 {
		t.Errorf("expected TTL 1800, got %d", records[0].TTL)
	}
}

func TestSRVRecord_PriorityWeightSorting(t *testing.T) {
	mgr := NewManager()

	// Add records in random order
	srv1 := NewSRVRecord("_sip._tcp.example.local", "sip1.example.local", 20, 30, 5060)
	srv2 := NewSRVRecord("_sip._tcp.example.local", "sip2.example.local", 10, 50, 5060)
	srv3 := NewSRVRecord("_sip._tcp.example.local", "sip3.example.local", 10, 100, 5060)
	srv4 := NewSRVRecord("_sip._tcp.example.local", "sip4.example.local", 10, 20, 5060)

	mgr.AddRecord(srv1)
	mgr.AddRecord(srv2)
	mgr.AddRecord(srv3)
	mgr.AddRecord(srv4)

	records := mgr.LookupSRV("_sip._tcp.example.local")
	if len(records) != 4 {
		t.Fatalf("expected 4 SRV records, got %d", len(records))
	}

	// Priority 10 records should come first, sorted by weight (descending)
	// Order: priority=10 weight=100, priority=10 weight=50, priority=10 weight=20, priority=20 weight=30
	expectedOrder := []struct {
		target   string
		priority uint16
		weight   uint16
	}{
		{"sip3.example.local.", 10, 100},
		{"sip2.example.local.", 10, 50},
		{"sip4.example.local.", 10, 20},
		{"sip1.example.local.", 20, 30},
	}

	for i, expected := range expectedOrder {
		if records[i].Priority != expected.priority {
			t.Errorf("record %d: expected priority %d, got %d", i, expected.priority, records[i].Priority)
		}
		if records[i].Weight != expected.weight {
			t.Errorf("record %d: expected weight %d, got %d", i, expected.weight, records[i].Weight)
		}
		if records[i].Target != expected.target {
			t.Errorf("record %d: expected target %s, got %s", i, expected.target, records[i].Target)
		}
	}
}

func TestSRVRecord_CommonServices(t *testing.T) {
	mgr := NewManager()

	// Common service records
	services := []struct {
		service string
		target  string
		port    uint16
	}{
		{"_ldap._tcp.example.local", "ldap.example.local", 389},
		{"_ldaps._tcp.example.local", "ldap.example.local", 636},
		{"_xmpp-client._tcp.example.local", "xmpp.example.local", 5222},
		{"_xmpp-server._tcp.example.local", "xmpp.example.local", 5269},
		{"_sip._tcp.example.local", "sip.example.local", 5060},
		{"_sip._udp.example.local", "sip.example.local", 5060},
	}

	for _, svc := range services {
		record := NewSRVRecord(svc.service, svc.target, 0, 10, svc.port)
		if err := mgr.AddRecord(record); err != nil {
			t.Fatalf("AddRecord(%s) error = %v", svc.service, err)
		}
	}

	// Verify all services are accessible
	for _, svc := range services {
		records := mgr.LookupSRV(svc.service)
		if len(records) == 0 {
			t.Errorf("service %s not found", svc.service)
			continue
		}
		if records[0].Port != svc.port {
			t.Errorf("service %s: expected port %d, got %d", svc.service, svc.port, records[0].Port)
		}
	}
}
func TestAddRecord_SRVRecord_Wildcard(t *testing.T) {
	mgr := NewManager()

	// Use wildcard for service names: *.example.local matches service.example.local
	record := NewSRVRecord("*.example.local", "www.example.local", 0, 10, 80)
	record.Wildcard = true

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test wildcard matching - wildcard matches one label
	records := mgr.LookupSRV("service.example.local")
	if len(records) == 0 {
		t.Fatal("wildcard SRV record not found")
	}

	if records[0].Port != 80 {
		t.Errorf("expected port 80, got %d", records[0].Port)
	}

	// Test non-matching domain (no label before suffix)
	records = mgr.LookupSRV("example.local")
	if len(records) != 0 {
		t.Error("wildcard should not match parent domain")
	}
}
// ========================
// NS Record Tests
// ========================

func TestAddRecord_NSRecord(t *testing.T) {
	mgr := NewManager()

	record := NewNSRecord("example.local", "ns1.example.local")
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupNS("example.local")
	if len(records) == 0 {
		t.Fatal("NS record not found")
	}

	if records[0].Target != "ns1.example.local." {
		t.Errorf("expected ns1.example.local., got %s", records[0].Target)
	}
}

func TestAddRecord_NSRecord_Multiple(t *testing.T) {
	mgr := NewManager()

	// Add multiple NS records for the same domain
	ns1 := NewNSRecord("example.local", "ns1.example.local")
	ns2 := NewNSRecord("example.local", "ns2.example.local")

	if err := mgr.AddRecord(ns1); err != nil {
		t.Fatalf("AddRecord(ns1) error = %v", err)
	}
	if err := mgr.AddRecord(ns2); err != nil {
		t.Fatalf("AddRecord(ns2) error = %v", err)
	}

	records := mgr.LookupNS("example.local")
	if len(records) != 2 {
		t.Fatalf("expected 2 NS records, got %d", len(records))
	}

	// Verify both nameservers are present
	targets := make(map[string]bool)
	for _, rec := range records {
		targets[rec.Target] = true
	}

	if !targets["ns1.example.local."] || !targets["ns2.example.local."] {
		t.Error("missing expected nameserver targets")
	}
}

func TestLookupNS_NotFound(t *testing.T) {
	mgr := NewManager()

	records := mgr.LookupNS("nonexistent.local")
	if len(records) != 0 {
		t.Errorf("expected no records, got %d", len(records))
	}
}

func TestAddRecord_NSRecord_Wildcard(t *testing.T) {
	mgr := NewManager()

	record := NewNSRecord("*.example.local", "ns.example.local")
	record.Wildcard = true

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test wildcard matching - wildcard matches one label
	records := mgr.LookupNS("subdomain.example.local")
	if len(records) == 0 {
		t.Fatal("wildcard NS record not found")
	}

	if records[0].Target != "ns.example.local." {
		t.Errorf("expected ns.example.local., got %s", records[0].Target)
	}

	// Test non-matching domain (no label before suffix)
	records = mgr.LookupNS("example.local")
	if len(records) != 0 {
		t.Error("wildcard should not match parent domain")
	}
}

func TestAddRecord_NSRecord_Disabled(t *testing.T) {
	mgr := NewManager()

	record := NewNSRecord("example.local", "ns.example.local")
	record.Enabled = false

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupNS("example.local")
	if len(records) != 0 {
		t.Error("disabled NS record should not be returned")
	}
}

func TestAddRecord_NSRecord_EmptyTarget(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeNS)
	record.Target = ""

	err := mgr.AddRecord(record)
	if err == nil {
		t.Error("expected error for NS record with empty target")
	}
	if err != ErrEmptyTarget {
		t.Errorf("expected ErrEmptyTarget, got %v", err)
	}
}

func TestAddRecord_NSRecord_DomainNormalization(t *testing.T) {
	mgr := NewManager()

	record := NewNSRecord("EXAMPLE.LOCAL", "NS.EXAMPLE.LOCAL")
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup with different case
	records := mgr.LookupNS("example.local")
	if len(records) == 0 {
		t.Fatal("NS record not found (case normalization failed)")
	}

	if records[0].Domain != "example.local." {
		t.Errorf("expected normalized domain example.local., got %s", records[0].Domain)
	}
}

func TestAddRecord_NSRecord_TTL(t *testing.T) {
	mgr := NewManager()

	record := NewNSRecord("example.local", "ns.example.local")
	record.TTL = 7200

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupNS("example.local")
	if len(records) == 0 {
		t.Fatal("NS record not found")
	}

	if records[0].TTL != 7200 {
		t.Errorf("expected TTL 7200, got %d", records[0].TTL)
	}
}

func TestAddRecord_NSRecord_MultipleDomains(t *testing.T) {
	mgr := NewManager()

	record1 := NewNSRecord("example.local", "ns1.example.local")
	record2 := NewNSRecord("test.local", "ns1.test.local")

	if err := mgr.AddRecord(record1); err != nil {
		t.Fatalf("AddRecord(record1) error = %v", err)
	}
	if err := mgr.AddRecord(record2); err != nil {
		t.Fatalf("AddRecord(record2) error = %v", err)
	}

	// Verify both domains have their NS records
	records1 := mgr.LookupNS("example.local")
	if len(records1) != 1 || records1[0].Target != "ns1.example.local." {
		t.Error("example.local NS record not found or incorrect")
	}

	records2 := mgr.LookupNS("test.local")
	if len(records2) != 1 || records2[0].Target != "ns1.test.local." {
		t.Error("test.local NS record not found or incorrect")
	}
}

func TestAddRecord_NSRecord_CaseInsensitive(t *testing.T) {
	mgr := NewManager()

	record := NewNSRecord("Example.Local", "NS.Example.Local")
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test lookup with different case combinations
	testCases := []string{
		"example.local",
		"EXAMPLE.LOCAL",
		"Example.Local",
		"eXaMpLe.LoCaL",
	}

	for _, tc := range testCases {
		records := mgr.LookupNS(tc)
		if len(records) == 0 {
			t.Errorf("NS record not found for %s", tc)
		}
	}
}

func TestAddRecord_NSRecord_ExactMatchPrecedence(t *testing.T) {
	mgr := NewManager()

	// Add wildcard NS record
	wildcard := NewNSRecord("*.example.local", "ns-wildcard.example.local")
	wildcard.Wildcard = true

	// Add exact match NS record
	exact := NewNSRecord("subdomain.example.local", "ns-exact.example.local")

	if err := mgr.AddRecord(wildcard); err != nil {
		t.Fatalf("AddRecord(wildcard) error = %v", err)
	}
	if err := mgr.AddRecord(exact); err != nil {
		t.Fatalf("AddRecord(exact) error = %v", err)
	}

	// Exact match should take precedence
	records := mgr.LookupNS("subdomain.example.local")
	if len(records) == 0 {
		t.Fatal("no NS records found")
	}

	// Should find exact match first (it's checked before wildcards)
	if records[0].Target != "ns-exact.example.local." {
		t.Errorf("expected exact match first, got %s", records[0].Target)
	}
}

// ========================
// SOA Record Tests
// ========================

func TestAddRecord_SOARecord(t *testing.T) {
	mgr := NewManager()

	record := NewSOARecord("example.local", "ns1.example.local", "admin.example.local", 1, 3600, 600, 86400, 300)
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupSOA("example.local")
	if len(records) == 0 {
		t.Fatal("SOA record not found")
	}

	soa := records[0]
	if soa.Ns != "ns1.example.local." {
		t.Errorf("expected ns1.example.local., got %s", soa.Ns)
	}
	if soa.Mbox != "admin.example.local" {
		t.Errorf("expected admin.example.local, got %s", soa.Mbox)
	}
	if soa.Serial != 1 {
		t.Errorf("expected serial 1, got %d", soa.Serial)
	}
}

func TestAddRecord_SOARecord_AllFields(t *testing.T) {
	mgr := NewManager()

	record := NewSOARecord("example.local", "ns1.example.local", "admin.example.local", 
		2023010101, // Serial
		7200,       // Refresh (2 hours)
		1800,       // Retry (30 minutes)
		604800,     // Expire (7 days)
		86400,      // Minimum TTL (1 day)
	)
	
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupSOA("example.local")
	if len(records) == 0 {
		t.Fatal("SOA record not found")
	}

	soa := records[0]
	if soa.Serial != 2023010101 {
		t.Errorf("expected serial 2023010101, got %d", soa.Serial)
	}
	if soa.Refresh != 7200 {
		t.Errorf("expected refresh 7200, got %d", soa.Refresh)
	}
	if soa.Retry != 1800 {
		t.Errorf("expected retry 1800, got %d", soa.Retry)
	}
	if soa.Expire != 604800 {
		t.Errorf("expected expire 604800, got %d", soa.Expire)
	}
	if soa.Minttl != 86400 {
		t.Errorf("expected minttl 86400, got %d", soa.Minttl)
	}
}

func TestLookupSOA_NotFound(t *testing.T) {
	mgr := NewManager()

	records := mgr.LookupSOA("nonexistent.local")
	if len(records) != 0 {
		t.Errorf("expected no records, got %d", len(records))
	}
}

func TestAddRecord_SOARecord_Wildcard(t *testing.T) {
	mgr := NewManager()

	record := NewSOARecord("*.example.local", "ns.example.local", "admin.example.local", 1, 3600, 600, 86400, 300)
	record.Wildcard = true

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test wildcard matching - wildcard matches one label
	records := mgr.LookupSOA("subdomain.example.local")
	if len(records) == 0 {
		t.Fatal("wildcard SOA record not found")
	}

	if records[0].Ns != "ns.example.local." {
		t.Errorf("expected ns.example.local., got %s", records[0].Ns)
	}

	// Test non-matching domain (no label before suffix)
	records = mgr.LookupSOA("example.local")
	if len(records) != 0 {
		t.Error("wildcard should not match parent domain")
	}
}

func TestAddRecord_SOARecord_Disabled(t *testing.T) {
	mgr := NewManager()

	record := NewSOARecord("example.local", "ns.example.local", "admin.example.local", 1, 3600, 600, 86400, 300)
	record.Enabled = false

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupSOA("example.local")
	if len(records) != 0 {
		t.Error("disabled SOA record should not be returned")
	}
}

func TestAddRecord_SOARecord_MissingNs(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeSOA)
	record.Ns = ""
	record.Mbox = "admin.example.local"

	err := mgr.AddRecord(record)
	if err == nil {
		t.Error("expected error for SOA record with empty Ns")
	}
	if err != ErrInvalidSOA {
		t.Errorf("expected ErrInvalidSOA, got %v", err)
	}
}

func TestAddRecord_SOARecord_MissingMbox(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeSOA)
	record.Ns = "ns1.example.local"
	record.Mbox = ""

	err := mgr.AddRecord(record)
	if err == nil {
		t.Error("expected error for SOA record with empty Mbox")
	}
	if err != ErrInvalidSOA {
		t.Errorf("expected ErrInvalidSOA, got %v", err)
	}
}

func TestAddRecord_SOARecord_DomainNormalization(t *testing.T) {
	mgr := NewManager()

	record := NewSOARecord("EXAMPLE.LOCAL", "NS.EXAMPLE.LOCAL", "admin@example.local", 1, 3600, 600, 86400, 300)
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup with different case
	records := mgr.LookupSOA("example.local")
	if len(records) == 0 {
		t.Fatal("SOA record not found (case normalization failed)")
	}

	if records[0].Domain != "example.local." {
		t.Errorf("expected normalized domain example.local., got %s", records[0].Domain)
	}
	if records[0].Ns != "ns.example.local." {
		t.Errorf("expected normalized ns ns.example.local., got %s", records[0].Ns)
	}
}

func TestAddRecord_SOARecord_TTL(t *testing.T) {
	mgr := NewManager()

	record := NewSOARecord("example.local", "ns.example.local", "admin.example.local", 1, 3600, 600, 86400, 300)
	record.TTL = 86400

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupSOA("example.local")
	if len(records) == 0 {
		t.Fatal("SOA record not found")
	}

	if records[0].TTL != 86400 {
		t.Errorf("expected TTL 86400, got %d", records[0].TTL)
	}
}

func TestAddRecord_SOARecord_Multiple(t *testing.T) {
	mgr := NewManager()

	// Even though typically there's only one SOA per zone, the system should handle multiple
	soa1 := NewSOARecord("example.local", "ns1.example.local", "admin.example.local", 1, 3600, 600, 86400, 300)
	soa2 := NewSOARecord("example.local", "ns2.example.local", "admin.example.local", 2, 3600, 600, 86400, 300)

	if err := mgr.AddRecord(soa1); err != nil {
		t.Fatalf("AddRecord(soa1) error = %v", err)
	}
	if err := mgr.AddRecord(soa2); err != nil {
		t.Fatalf("AddRecord(soa2) error = %v", err)
	}

	records := mgr.LookupSOA("example.local")
	if len(records) != 2 {
		t.Fatalf("expected 2 SOA records, got %d", len(records))
	}
}

func TestAddRecord_SOARecord_CaseInsensitive(t *testing.T) {
	mgr := NewManager()

	record := NewSOARecord("Example.Local", "NS.Example.Local", "Admin@Example.Local", 1, 3600, 600, 86400, 300)
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test lookup with different case combinations
	testCases := []string{
		"example.local",
		"EXAMPLE.LOCAL",
		"Example.Local",
		"eXaMpLe.LoCaL",
	}

	for _, tc := range testCases {
		records := mgr.LookupSOA(tc)
		if len(records) == 0 {
			t.Errorf("SOA record not found for %s", tc)
		}
	}
}

// ========================
// CAA Record Tests
// ========================

func TestAddRecord_CAARecord_Issue(t *testing.T) {
	mgr := NewManager()

	record := NewCAARecord("example.local", "issue", "letsencrypt.org", 0)
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupCAA("example.local")
	if len(records) == 0 {
		t.Fatal("CAA record not found")
	}

	if records[0].CaaTag != "issue" {
		t.Errorf("expected tag 'issue', got '%s'", records[0].CaaTag)
	}
	if records[0].CaaValue != "letsencrypt.org" {
		t.Errorf("expected value 'letsencrypt.org', got '%s'", records[0].CaaValue)
	}
	if records[0].CaaFlag != 0 {
		t.Errorf("expected flag 0, got %d", records[0].CaaFlag)
	}
}

func TestAddRecord_CAARecord_IssueWild(t *testing.T) {
	mgr := NewManager()

	record := NewCAARecord("example.local", "issuewild", "letsencrypt.org", 0)
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupCAA("example.local")
	if len(records) == 0 {
		t.Fatal("CAA record not found")
	}

	if records[0].CaaTag != "issuewild" {
		t.Errorf("expected tag 'issuewild', got '%s'", records[0].CaaTag)
	}
}

func TestAddRecord_CAARecord_Iodef(t *testing.T) {
	mgr := NewManager()

	record := NewCAARecord("example.local", "iodef", "mailto:security@example.local", 0)
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupCAA("example.local")
	if len(records) == 0 {
		t.Fatal("CAA record not found")
	}

	if records[0].CaaTag != "iodef" {
		t.Errorf("expected tag 'iodef', got '%s'", records[0].CaaTag)
	}
	if records[0].CaaValue != "mailto:security@example.local" {
		t.Errorf("expected value 'mailto:security@example.local', got '%s'", records[0].CaaValue)
	}
}

func TestAddRecord_CAARecord_CriticalFlag(t *testing.T) {
	mgr := NewManager()

	record := NewCAARecord("example.local", "issue", "letsencrypt.org", 128)
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupCAA("example.local")
	if len(records) == 0 {
		t.Fatal("CAA record not found")
	}

	if records[0].CaaFlag != 128 {
		t.Errorf("expected flag 128, got %d", records[0].CaaFlag)
	}
}

func TestAddRecord_CAARecord_Multiple(t *testing.T) {
	mgr := NewManager()

	// Add multiple CAA records for same domain
	caa1 := NewCAARecord("example.local", "issue", "letsencrypt.org", 0)
	caa2 := NewCAARecord("example.local", "issue", "digicert.com", 0)
	caa3 := NewCAARecord("example.local", "iodef", "mailto:security@example.local", 0)

	if err := mgr.AddRecord(caa1); err != nil {
		t.Fatalf("AddRecord(caa1) error = %v", err)
	}
	if err := mgr.AddRecord(caa2); err != nil {
		t.Fatalf("AddRecord(caa2) error = %v", err)
	}
	if err := mgr.AddRecord(caa3); err != nil {
		t.Fatalf("AddRecord(caa3) error = %v", err)
	}

	records := mgr.LookupCAA("example.local")
	if len(records) != 3 {
		t.Fatalf("expected 3 CAA records, got %d", len(records))
	}
}

func TestLookupCAA_NotFound(t *testing.T) {
	mgr := NewManager()

	records := mgr.LookupCAA("nonexistent.local")
	if len(records) != 0 {
		t.Errorf("expected no records, got %d", len(records))
	}
}

func TestAddRecord_CAARecord_Wildcard(t *testing.T) {
	mgr := NewManager()

	record := NewCAARecord("*.example.local", "issue", "letsencrypt.org", 0)
	record.Wildcard = true

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test wildcard matching
	records := mgr.LookupCAA("subdomain.example.local")
	if len(records) == 0 {
		t.Fatal("wildcard CAA record not found")
	}

	if records[0].CaaTag != "issue" {
		t.Errorf("expected tag 'issue', got '%s'", records[0].CaaTag)
	}

	// Test non-matching domain
	records = mgr.LookupCAA("example.local")
	if len(records) != 0 {
		t.Error("wildcard should not match parent domain")
	}
}

func TestAddRecord_CAARecord_Disabled(t *testing.T) {
	mgr := NewManager()

	record := NewCAARecord("example.local", "issue", "letsencrypt.org", 0)
	record.Enabled = false

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupCAA("example.local")
	if len(records) != 0 {
		t.Error("disabled CAA record should not be returned")
	}
}

func TestAddRecord_CAARecord_EmptyTag(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeCAA)
	record.CaaTag = ""
	record.CaaValue = "letsencrypt.org"

	err := mgr.AddRecord(record)
	if err == nil {
		t.Error("expected error for CAA record with empty tag")
	}
	if err != ErrInvalidCAA {
		t.Errorf("expected ErrInvalidCAA, got %v", err)
	}
}

func TestAddRecord_CAARecord_EmptyValue(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeCAA)
	record.CaaTag = "issue"
	record.CaaValue = ""

	err := mgr.AddRecord(record)
	if err == nil {
		t.Error("expected error for CAA record with empty value")
	}
	if err != ErrInvalidCAA {
		t.Errorf("expected ErrInvalidCAA, got %v", err)
	}
}

func TestAddRecord_CAARecord_InvalidTag(t *testing.T) {
	mgr := NewManager()

	record := NewLocalRecord("example.local", RecordTypeCAA)
	record.CaaTag = "invalid"
	record.CaaValue = "letsencrypt.org"

	err := mgr.AddRecord(record)
	if err == nil {
		t.Error("expected error for CAA record with invalid tag")
	}
	if err != ErrInvalidCAA {
		t.Errorf("expected ErrInvalidCAA, got %v", err)
	}
}

func TestAddRecord_CAARecord_DomainNormalization(t *testing.T) {
	mgr := NewManager()

	record := NewCAARecord("EXAMPLE.LOCAL", "issue", "letsencrypt.org", 0)
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Lookup with different case
	records := mgr.LookupCAA("example.local")
	if len(records) == 0 {
		t.Fatal("CAA record not found (case normalization failed)")
	}

	if records[0].Domain != "example.local." {
		t.Errorf("expected normalized domain example.local., got %s", records[0].Domain)
	}
}

func TestAddRecord_CAARecord_TTL(t *testing.T) {
	mgr := NewManager()

	record := NewCAARecord("example.local", "issue", "letsencrypt.org", 0)
	record.TTL = 7200

	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	records := mgr.LookupCAA("example.local")
	if len(records) == 0 {
		t.Fatal("CAA record not found")
	}

	if records[0].TTL != 7200 {
		t.Errorf("expected TTL 7200, got %d", records[0].TTL)
	}
}

func TestAddRecord_CAARecord_CaseInsensitive(t *testing.T) {
	mgr := NewManager()

	record := NewCAARecord("Example.Local", "issue", "letsencrypt.org", 0)
	if err := mgr.AddRecord(record); err != nil {
		t.Fatalf("AddRecord() error = %v", err)
	}

	// Test lookup with different case combinations
	testCases := []string{
		"example.local",
		"EXAMPLE.LOCAL",
		"Example.Local",
		"eXaMpLe.LoCaL",
	}

	for _, tc := range testCases {
		records := mgr.LookupCAA(tc)
		if len(records) == 0 {
			t.Errorf("CAA record not found for %s", tc)
		}
	}
}
