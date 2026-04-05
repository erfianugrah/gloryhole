package blocklist

import (
	"fmt"
	"testing"
)

func TestFlatBlocklist_Empty(t *testing.T) {
	f := BuildFlatBlocklist(nil)
	if f.Len() != 0 {
		t.Fatalf("expected 0, got %d", f.Len())
	}
	if f.Contains("anything.") {
		t.Fatal("empty list should not contain anything")
	}
	if _, _, ok := f.LookupSubdomains("anything."); ok {
		t.Fatal("empty list should not match subdomains")
	}
}

func TestFlatBlocklist_Lookup(t *testing.T) {
	m := map[string]uint64{
		"example.com.":   1,
		"blocked.net.":   2,
		"ad.tracker.io.": 3,
	}
	f := BuildFlatBlocklist(m)

	if f.Len() != 3 {
		t.Fatalf("expected 3, got %d", f.Len())
	}

	tests := []struct {
		domain string
		mask   uint64
		found  bool
	}{
		{"example.com.", 1, true},
		{"blocked.net.", 2, true},
		{"ad.tracker.io.", 3, true},
		{"not-blocked.com.", 0, false},
		{"example.com", 0, false}, // no trailing dot
		{"", 0, false},
	}

	for _, tt := range tests {
		mask, ok := f.Lookup(tt.domain)
		if ok != tt.found {
			t.Errorf("Lookup(%q): found=%v, want %v", tt.domain, ok, tt.found)
		}
		if mask != tt.mask {
			t.Errorf("Lookup(%q): mask=%d, want %d", tt.domain, mask, tt.mask)
		}
	}
}

func TestFlatBlocklist_SubdomainWalk(t *testing.T) {
	m := map[string]uint64{
		"example.com.": 1,
		"tracker.io.":  2,
	}
	f := BuildFlatBlocklist(m)

	tests := []struct {
		domain string
		kind   string
		found  bool
	}{
		{"example.com.", "exact", true},
		{"sub.example.com.", "subdomain", true},
		{"deep.sub.example.com.", "subdomain", true},
		{"ad.tracker.io.", "subdomain", true},
		{"notblocked.com.", "", false},
		{"fakeexample.com.", "", false}, // should NOT match example.com
	}

	for _, tt := range tests {
		_, kind, ok := f.LookupSubdomains(tt.domain)
		if ok != tt.found {
			t.Errorf("LookupSubdomains(%q): found=%v, want %v", tt.domain, ok, tt.found)
		}
		if kind != tt.kind {
			t.Errorf("LookupSubdomains(%q): kind=%q, want %q", tt.domain, kind, tt.kind)
		}
	}
}

func TestFlatBlocklist_LargeScale(t *testing.T) {
	// Simulate a realistic blocklist size
	const size = 100_000
	m := make(map[string]uint64, size)
	for i := 0; i < size; i++ {
		m[fmt.Sprintf("domain-%d.blocked.test.", i)] = 1
	}
	// Add a known target
	m["target.example.com."] = 7

	f := BuildFlatBlocklist(m)

	if f.Len() != size+1 {
		t.Fatalf("expected %d, got %d", size+1, f.Len())
	}

	// Exact match
	if !f.Contains("target.example.com.") {
		t.Fatal("should contain target.example.com.")
	}

	// Not present
	if f.Contains("nothere.example.com.") {
		t.Fatal("should not contain nothere.example.com.")
	}

	// Check mask
	mask, ok := f.Lookup("target.example.com.")
	if !ok || mask != 7 {
		t.Fatalf("expected mask=7, got mask=%d ok=%v", mask, ok)
	}

	t.Logf("FlatBlocklist: %d domains, %d bytes (%.1f bytes/domain)",
		f.Len(), f.MemoryUsage(), float64(f.MemoryUsage())/float64(f.Len()))
}

func TestFlatBlocklist_ForEach(t *testing.T) {
	m := map[string]uint64{
		"a.com.": 1,
		"b.com.": 2,
		"c.com.": 3,
	}
	f := BuildFlatBlocklist(m)

	count := 0
	f.ForEach(func(domain string, mask uint64) {
		if _, ok := m[domain]; !ok {
			t.Errorf("unexpected domain %q", domain)
		}
		if m[domain] != mask {
			t.Errorf("domain %q: mask=%d, want %d", domain, mask, m[domain])
		}
		count++
	})
	if count != 3 {
		t.Fatalf("ForEach visited %d domains, want 3", count)
	}
}

func BenchmarkFlatBlocklist_Lookup(b *testing.B) {
	const size = 1_000_000
	m := make(map[string]uint64, size)
	for i := 0; i < size; i++ {
		m[fmt.Sprintf("domain-%d.blocked.test.", i)] = 1
	}
	f := BuildFlatBlocklist(m)

	target := "domain-500000.blocked.test."
	miss := "notblocked.example.com."

	b.Run("hit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			f.Lookup(target)
		}
	})

	b.Run("miss", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			f.Lookup(miss)
		}
	})

	b.Run("subdomain_walk", func(b *testing.B) {
		sub := "deep.sub.domain-500000.blocked.test."
		for i := 0; i < b.N; i++ {
			f.LookupSubdomains(sub)
		}
	})
}

func BenchmarkFlatBlocklist_Build(b *testing.B) {
	const size = 1_000_000
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		m := make(map[string]uint64, size)
		for j := 0; j < size; j++ {
			m[fmt.Sprintf("domain-%d.blocked.test.", j)] = 1
		}
		b.StartTimer()
		BuildFlatBlocklist(m)
	}
}
