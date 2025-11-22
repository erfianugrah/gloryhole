package forwarder

import "testing"

func TestDomainMatcher_Exact(t *testing.T) {
	patterns := []string{"nas.local", "router.local", "server.home"}
	matcher, err := NewDomainMatcher(patterns)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		domain string
		want   bool
	}{
		{"nas.local", true},
		{"router.local", true},
		{"server.home", true},
		{"other.local", false},
		{"nas.home", false},
		{"", false},
	}

	for _, tt := range tests {
		got := matcher.Matches(tt.domain)
		if got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestDomainMatcher_WildcardSuffix(t *testing.T) {
	patterns := []string{"*.local", "*.lan"}
	matcher, err := NewDomainMatcher(patterns)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		domain string
		want   bool
	}{
		{"nas.local", true},
		{"router.local", true},
		{"server.lan", true},
		{"sub.nas.local", true}, // Matches nested subdomains
		{"local", true},          // Matches root
		{"lan", true},
		{"nas.home", false},
		{"other.com", false},
	}

	for _, tt := range tests {
		got := matcher.Matches(tt.domain)
		if got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestDomainMatcher_WildcardPrefix(t *testing.T) {
	patterns := []string{"internal.*"}
	matcher, err := NewDomainMatcher(patterns)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		domain string
		want   bool
	}{
		{"internal.corp", true},
		{"internal.net", true},
		{"internal.local", true},
		{"nas.internal", false},
		{"other.corp", false},
	}

	for _, tt := range tests {
		got := matcher.Matches(tt.domain)
		if got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestDomainMatcher_Regex(t *testing.T) {
	patterns := []string{"/^[a-z]+\\.local$/", "/test\\d+\\.com/"}
	matcher, err := NewDomainMatcher(patterns)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		domain string
		want   bool
	}{
		{"nas.local", true},
		{"router.local", true},
		{"test123.com", true},
		{"test.com", false},      // No digit
		{"nas123.local", false},  // Has digit
		{"sub.nas.local", false}, // Multiple levels
	}

	for _, tt := range tests {
		got := matcher.Matches(tt.domain)
		if got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestDomainMatcher_Mixed(t *testing.T) {
	patterns := []string{
		"nas.local",   // Exact
		"*.home",      // Wildcard suffix
		"internal.*",  // Wildcard prefix
		"/test\\d+/",  // Regex
	}
	matcher, err := NewDomainMatcher(patterns)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		domain string
		want   bool
	}{
		{"nas.local", true},      // Exact match
		{"router.home", true},    // Wildcard suffix
		{"internal.corp", true},  // Wildcard prefix
		{"test123.com", true},    // Regex
		{"other.local", false},
		{"test.com", false},
	}

	for _, tt := range tests {
		got := matcher.Matches(tt.domain)
		if got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestDomainMatcher_CaseInsensitive(t *testing.T) {
	patterns := []string{"NAS.LOCAL", "*.HOME"}
	matcher, err := NewDomainMatcher(patterns)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		domain string
		want   bool
	}{
		{"nas.local", true},
		{"NAS.LOCAL", true},
		{"Nas.Local", true},
		{"router.home", true},
		{"ROUTER.HOME", true},
	}

	for _, tt := range tests {
		got := matcher.Matches(tt.domain)
		if got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestDomainMatcher_TrailingDot(t *testing.T) {
	patterns := []string{"nas.local.", "*.home."}
	matcher, err := NewDomainMatcher(patterns)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		domain string
		want   bool
	}{
		{"nas.local", true},
		{"nas.local.", true},
		{"router.home", true},
		{"router.home.", true},
	}

	for _, tt := range tests {
		got := matcher.Matches(tt.domain)
		if got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestDomainMatcher_IsEmpty(t *testing.T) {
	matcher, _ := NewDomainMatcher([]string{})
	if !matcher.IsEmpty() {
		t.Error("Empty matcher should return true for IsEmpty()")
	}

	matcher, _ = NewDomainMatcher([]string{"nas.local"})
	if matcher.IsEmpty() {
		t.Error("Non-empty matcher should return false for IsEmpty()")
	}
}

func TestDomainMatcher_Count(t *testing.T) {
	patterns := []string{"nas.local", "*.home", "internal.*", "/test/"}
	matcher, err := NewDomainMatcher(patterns)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	if got := matcher.Count(); got != 4 {
		t.Errorf("Count() = %d, want 4", got)
	}
}

func TestCIDRMatcher(t *testing.T) {
	cidrs := []string{"10.0.0.0/8", "192.168.1.0/24"}
	matcher, err := NewCIDRMatcher(cidrs)
	if err != nil {
		t.Fatalf("Failed to create CIDR matcher: %v", err)
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"10.5.10.50", true},
		{"192.168.1.100", true},
		{"192.168.2.1", false},
		{"8.8.8.8", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		got := matcher.Matches(tt.ip)
		if got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestCIDRMatcher_IsEmpty(t *testing.T) {
	matcher, _ := NewCIDRMatcher([]string{})
	if !matcher.IsEmpty() {
		t.Error("Empty matcher should return true for IsEmpty()")
	}

	matcher, _ = NewCIDRMatcher([]string{"10.0.0.0/8"})
	if matcher.IsEmpty() {
		t.Error("Non-empty matcher should return false for IsEmpty()")
	}
}

func TestCIDRMatcher_Count(t *testing.T) {
	cidrs := []string{"10.0.0.0/8", "192.168.1.0/24", "172.16.0.0/12"}
	matcher, err := NewCIDRMatcher(cidrs)
	if err != nil {
		t.Fatalf("Failed to create CIDR matcher: %v", err)
	}

	if got := matcher.Count(); got != 3 {
		t.Errorf("Count() = %d, want 3", got)
	}
}

func TestQueryTypeMatcher(t *testing.T) {
	types := []string{"A", "AAAA", "PTR"}
	matcher := NewQueryTypeMatcher(types)

	tests := []struct {
		qtype string
		want  bool
	}{
		{"A", true},
		{"AAAA", true},
		{"PTR", true},
		{"a", true},    // Case insensitive
		{"aaaa", true},
		{"MX", false},
		{"CNAME", false},
		{"", false},
	}

	for _, tt := range tests {
		got := matcher.Matches(tt.qtype)
		if got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.qtype, got, tt.want)
		}
	}
}

func TestQueryTypeMatcher_IsEmpty(t *testing.T) {
	matcher := NewQueryTypeMatcher([]string{})
	if !matcher.IsEmpty() {
		t.Error("Empty matcher should return true for IsEmpty()")
	}

	matcher = NewQueryTypeMatcher([]string{"A"})
	if matcher.IsEmpty() {
		t.Error("Non-empty matcher should return false for IsEmpty()")
	}
}

func TestQueryTypeMatcher_Count(t *testing.T) {
	types := []string{"A", "AAAA", "PTR", "MX"}
	matcher := NewQueryTypeMatcher(types)

	if got := matcher.Count(); got != 4 {
		t.Errorf("Count() = %d, want 4", got)
	}
}

// Benchmark domain matching performance
func BenchmarkDomainMatcher_Exact(b *testing.B) {
	patterns := []string{"nas.local", "router.local", "server.home"}
	matcher, _ := NewDomainMatcher(patterns)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Matches("nas.local")
	}
}

func BenchmarkDomainMatcher_Wildcard(b *testing.B) {
	patterns := []string{"*.local", "*.home"}
	matcher, _ := NewDomainMatcher(patterns)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Matches("nas.local")
	}
}

func BenchmarkDomainMatcher_Regex(b *testing.B) {
	patterns := []string{"/^[a-z]+\\.local$/"}
	matcher, _ := NewDomainMatcher(patterns)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Matches("nas.local")
	}
}

func BenchmarkCIDRMatcher(b *testing.B) {
	cidrs := []string{"10.0.0.0/8", "192.168.0.0/16"}
	matcher, _ := NewCIDRMatcher(cidrs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Matches("10.0.0.1")
	}
}
