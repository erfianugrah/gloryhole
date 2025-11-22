package forwarder

import (
	"testing"

	"glory-hole/pkg/config"
)

func TestNewRuleEvaluator_Empty(t *testing.T) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: false,
	}

	evaluator, err := NewRuleEvaluator(cfg)
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	if !evaluator.IsEmpty() {
		t.Error("Evaluator should be empty when disabled")
	}
}

func TestNewRuleEvaluator_SingleRule(t *testing.T) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{
				Name:      "local DNS",
				Priority:  50,
				Domains:   []string{"*.local"},
				Upstreams: []string{"10.0.0.1:53"},
				Enabled:   true,
			},
		},
	}

	evaluator, err := NewRuleEvaluator(cfg)
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	if evaluator.Count() != 1 {
		t.Errorf("Expected 1 rule, got %d", evaluator.Count())
	}
}

func TestNewRuleEvaluator_PrioritySorting(t *testing.T) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{
				Name:      "low priority",
				Priority:  10,
				Domains:   []string{"*.local"},
				Upstreams: []string{"10.0.0.1:53"},
				Enabled:   true,
			},
			{
				Name:      "high priority",
				Priority:  90,
				Domains:   []string{"*.local"},
				Upstreams: []string{"10.0.0.2:53"},
				Enabled:   true,
			},
			{
				Name:      "medium priority",
				Priority:  50,
				Domains:   []string{"*.local"},
				Upstreams: []string{"10.0.0.3:53"},
				Enabled:   true,
			},
		},
	}

	evaluator, err := NewRuleEvaluator(cfg)
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	rules := evaluator.GetRules()
	if len(rules) != 3 {
		t.Fatalf("Expected 3 rules, got %d", len(rules))
	}

	// Check sorting: high → medium → low
	if rules[0].Priority != 90 {
		t.Errorf("First rule should have priority 90, got %d", rules[0].Priority)
	}
	if rules[1].Priority != 50 {
		t.Errorf("Second rule should have priority 50, got %d", rules[1].Priority)
	}
	if rules[2].Priority != 10 {
		t.Errorf("Third rule should have priority 10, got %d", rules[2].Priority)
	}
}

func TestEvaluator_DomainMatching(t *testing.T) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{
				Name:      "local domains",
				Priority:  50,
				Domains:   []string{"*.local"},
				Upstreams: []string{"10.0.0.1:53"},
				Enabled:   true,
			},
		},
	}

	evaluator, err := NewRuleEvaluator(cfg)
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	tests := []struct {
		domain string
		want   []string
	}{
		{"nas.local", []string{"10.0.0.1:53"}},
		{"router.local", []string{"10.0.0.1:53"}},
		{"nas.home", nil},
		{"example.com", nil},
	}

	for _, tt := range tests {
		got := evaluator.Evaluate(tt.domain, "10.0.0.100", "A")
		if !stringSliceEqual(got, tt.want) {
			t.Errorf("Evaluate(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestEvaluator_ClientIPMatching(t *testing.T) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{
				Name:        "VPN clients",
				Priority:    50,
				ClientCIDRs: []string{"10.8.0.0/24"},
				Upstreams:   []string{"10.0.0.1:53"},
				Enabled:     true,
			},
		},
	}

	evaluator, err := NewRuleEvaluator(cfg)
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	tests := []struct {
		clientIP string
		want     []string
	}{
		{"10.8.0.1", []string{"10.0.0.1:53"}},
		{"10.8.0.255", []string{"10.0.0.1:53"}},
		{"10.9.0.1", nil},
		{"192.168.1.1", nil},
	}

	for _, tt := range tests {
		got := evaluator.Evaluate("example.com", tt.clientIP, "A")
		if !stringSliceEqual(got, tt.want) {
			t.Errorf("Evaluate(clientIP=%q) = %v, want %v", tt.clientIP, got, tt.want)
		}
	}
}

func TestEvaluator_QueryTypeMatching(t *testing.T) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{
				Name:       "PTR queries",
				Priority:   50,
				QueryTypes: []string{"PTR"},
				Upstreams:  []string{"10.0.0.1:53"},
				Enabled:    true,
			},
		},
	}

	evaluator, err := NewRuleEvaluator(cfg)
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	tests := []struct {
		queryType string
		want      []string
	}{
		{"PTR", []string{"10.0.0.1:53"}},
		{"ptr", []string{"10.0.0.1:53"}}, // Case insensitive
		{"A", nil},
		{"AAAA", nil},
	}

	for _, tt := range tests {
		got := evaluator.Evaluate("example.com", "10.0.0.100", tt.queryType)
		if !stringSliceEqual(got, tt.want) {
			t.Errorf("Evaluate(queryType=%q) = %v, want %v", tt.queryType, got, tt.want)
		}
	}
}

func TestEvaluator_CombinedMatching(t *testing.T) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{
				Name:        "VPN clients accessing local domains",
				Priority:    50,
				Domains:     []string{"*.local"},
				ClientCIDRs: []string{"10.8.0.0/24"},
				QueryTypes:  []string{"A", "AAAA"},
				Upstreams:   []string{"10.0.0.1:53"},
				Enabled:     true,
			},
		},
	}

	evaluator, err := NewRuleEvaluator(cfg)
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	tests := []struct {
		domain    string
		clientIP  string
		queryType string
		want      []string
	}{
		// All conditions match
		{"nas.local", "10.8.0.1", "A", []string{"10.0.0.1:53"}},
		{"nas.local", "10.8.0.1", "AAAA", []string{"10.0.0.1:53"}},

		// Domain doesn't match
		{"nas.home", "10.8.0.1", "A", nil},

		// Client IP doesn't match
		{"nas.local", "192.168.1.1", "A", nil},

		// Query type doesn't match
		{"nas.local", "10.8.0.1", "PTR", nil},

		// Multiple conditions fail
		{"nas.home", "192.168.1.1", "PTR", nil},
	}

	for _, tt := range tests {
		got := evaluator.Evaluate(tt.domain, tt.clientIP, tt.queryType)
		if !stringSliceEqual(got, tt.want) {
			t.Errorf("Evaluate(%q, %q, %q) = %v, want %v",
				tt.domain, tt.clientIP, tt.queryType, got, tt.want)
		}
	}
}

func TestEvaluator_FirstMatchWins(t *testing.T) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{
				Name:      "high priority - specific",
				Priority:  90,
				Domains:   []string{"nas.local"},
				Upstreams: []string{"10.0.0.1:53"},
				Enabled:   true,
			},
			{
				Name:      "low priority - wildcard",
				Priority:  10,
				Domains:   []string{"*.local"},
				Upstreams: []string{"10.0.0.2:53"},
				Enabled:   true,
			},
		},
	}

	evaluator, err := NewRuleEvaluator(cfg)
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	// nas.local matches both rules, but higher priority should win
	got := evaluator.Evaluate("nas.local", "10.0.0.100", "A")
	want := []string{"10.0.0.1:53"}
	if !stringSliceEqual(got, want) {
		t.Errorf("Evaluate('nas.local') = %v, want %v (higher priority should win)", got, want)
	}

	// router.local only matches wildcard rule
	got = evaluator.Evaluate("router.local", "10.0.0.100", "A")
	want = []string{"10.0.0.2:53"}
	if !stringSliceEqual(got, want) {
		t.Errorf("Evaluate('router.local') = %v, want %v", got, want)
	}
}

func TestEvaluator_DisabledRules(t *testing.T) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{
				Name:      "disabled rule",
				Priority:  50,
				Domains:   []string{"*.local"},
				Upstreams: []string{"10.0.0.1:53"},
				Enabled:   false, // Disabled
			},
		},
	}

	evaluator, err := NewRuleEvaluator(cfg)
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	// Should have no rules (disabled rule not compiled)
	if !evaluator.IsEmpty() {
		t.Error("Evaluator should be empty when all rules are disabled")
	}

	// Should not match anything
	got := evaluator.Evaluate("nas.local", "10.0.0.100", "A")
	if got != nil {
		t.Errorf("Evaluate should return nil for disabled rules, got %v", got)
	}
}

// Helper function to compare string slices
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Benchmark rule evaluation
func BenchmarkEvaluator_SingleRule(b *testing.B) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{
				Name:      "local domains",
				Priority:  50,
				Domains:   []string{"*.local"},
				Upstreams: []string{"10.0.0.1:53"},
				Enabled:   true,
			},
		},
	}

	evaluator, _ := NewRuleEvaluator(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evaluator.Evaluate("nas.local", "10.0.0.100", "A")
	}
}

func BenchmarkEvaluator_MultipleRules(b *testing.B) {
	cfg := &config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{
				Name:      "rule 1",
				Priority:  90,
				Domains:   []string{"*.com"},
				Upstreams: []string{"10.0.0.1:53"},
				Enabled:   true,
			},
			{
				Name:      "rule 2",
				Priority:  80,
				Domains:   []string{"*.net"},
				Upstreams: []string{"10.0.0.2:53"},
				Enabled:   true,
			},
			{
				Name:      "rule 3",
				Priority:  70,
				Domains:   []string{"*.local"},
				Upstreams: []string{"10.0.0.3:53"},
				Enabled:   true,
			},
		},
	}

	evaluator, _ := NewRuleEvaluator(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evaluator.Evaluate("nas.local", "10.0.0.100", "A")
	}
}
