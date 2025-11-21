package policy

import (
	"fmt"
	"testing"
)

// BenchmarkEvaluate_SimpleRule benchmarks evaluation of a simple rule
func BenchmarkEvaluate_SimpleRule(b *testing.B) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Simple Rule",
		Logic:   "true",
		Action:  ActionBlock,
		Enabled: true,
	}
	_ = e.AddRule(rule)

	ctx := NewContext("example.com", "192.168.1.100", "A")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(ctx)
	}
}

// BenchmarkEvaluate_DomainMatch benchmarks domain matching rule
func BenchmarkEvaluate_DomainMatch(b *testing.B) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Domain Match",
		Logic:   `Domain == "facebook.com"`,
		Action:  ActionBlock,
		Enabled: true,
	}
	_ = e.AddRule(rule)

	ctx := NewContext("facebook.com", "192.168.1.100", "A")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(ctx)
	}
}

// BenchmarkEvaluate_ComplexRule benchmarks complex rule with multiple conditions
func BenchmarkEvaluate_ComplexRule(b *testing.B) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Complex Rule",
		Logic:   `(Hour >= 22 || Hour < 6) && ClientIP == "192.168.1.50" && Domain == "facebook.com"`,
		Action:  ActionBlock,
		Enabled: true,
	}
	_ = e.AddRule(rule)

	ctx := NewContext("facebook.com", "192.168.1.50", "A")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(ctx)
	}
}

// BenchmarkEvaluate_DomainMatchesHelper benchmarks DomainMatches helper function
func BenchmarkEvaluate_DomainMatchesHelper(b *testing.B) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Domain Matches Helper",
		Logic:   `DomainMatches(Domain, "facebook")`,
		Action:  ActionBlock,
		Enabled: true,
	}
	_ = e.AddRule(rule)

	ctx := NewContext("www.facebook.com", "192.168.1.100", "A")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(ctx)
	}
}

// BenchmarkEvaluate_IPInCIDRHelper benchmarks IPInCIDR helper function
func BenchmarkEvaluate_IPInCIDRHelper(b *testing.B) {
	e := NewEngine()
	rule := &Rule{
		Name:    "IP In CIDR",
		Logic:   `IPInCIDR(ClientIP, "192.168.1.0/24")`,
		Action:  ActionBlock,
		Enabled: true,
	}
	_ = e.AddRule(rule)

	ctx := NewContext("example.com", "192.168.1.50", "A")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(ctx)
	}
}

// BenchmarkEvaluate_MultipleRules benchmarks evaluation with multiple rules
func BenchmarkEvaluate_MultipleRules(b *testing.B) {
	e := NewEngine()

	// Add 10 rules
	for i := 0; i < 10; i++ {
		rule := &Rule{
			Name:    fmt.Sprintf("Rule %d", i),
			Logic:   fmt.Sprintf(`Domain == "domain%d.com"`, i),
			Action:  ActionBlock,
			Enabled: true,
		}
		_ = e.AddRule(rule)
	}

	// Add a matching rule at the end
	matchRule := &Rule{
		Name:    "Matching Rule",
		Logic:   `Domain == "test.com"`,
		Action:  ActionBlock,
		Enabled: true,
	}
	_ = e.AddRule(matchRule)

	ctx := NewContext("test.com", "192.168.1.100", "A")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(ctx)
	}
}

// BenchmarkEvaluate_ManyRules benchmarks evaluation with many rules (worst case)
func BenchmarkEvaluate_ManyRules(b *testing.B) {
	e := NewEngine()

	// Add 100 rules
	for i := 0; i < 100; i++ {
		rule := &Rule{
			Name:    fmt.Sprintf("Rule %d", i),
			Logic:   fmt.Sprintf(`Domain == "domain%d.com"`, i),
			Action:  ActionBlock,
			Enabled: true,
		}
		_ = e.AddRule(rule)
	}

	// Add a matching rule at the end (worst case)
	matchRule := &Rule{
		Name:    "Matching Rule",
		Logic:   `Domain == "test.com"`,
		Action:  ActionBlock,
		Enabled: true,
	}
	_ = e.AddRule(matchRule)

	ctx := NewContext("test.com", "192.168.1.100", "A")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(ctx)
	}
}

// BenchmarkEvaluate_NoMatch benchmarks evaluation when no rules match
func BenchmarkEvaluate_NoMatch(b *testing.B) {
	e := NewEngine()

	// Add rules that won't match
	for i := 0; i < 10; i++ {
		rule := &Rule{
			Name:    fmt.Sprintf("Rule %d", i),
			Logic:   fmt.Sprintf(`Domain == "domain%d.com"`, i),
			Action:  ActionBlock,
			Enabled: true,
		}
		_ = e.AddRule(rule)
	}

	ctx := NewContext("test.com", "192.168.1.100", "A")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(ctx)
	}
}

// BenchmarkAddRule benchmarks rule compilation and addition
func BenchmarkAddRule(b *testing.B) {
	e := NewEngine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rule := &Rule{
			Name:    "Test Rule",
			Logic:   `Domain == "facebook.com" && Hour >= 22`,
			Action:  ActionBlock,
			Enabled: true,
		}
		b.StartTimer()

		_ = e.AddRule(rule)
	}
}

// BenchmarkNewContext benchmarks context creation
func BenchmarkNewContext(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewContext("example.com", "192.168.1.100", "A")
	}
}

// BenchmarkDomainMatches benchmarks the DomainMatches helper
func BenchmarkDomainMatches(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DomainMatches("www.facebook.com", "facebook")
	}
}

// BenchmarkIPInCIDR benchmarks the IPInCIDR helper
func BenchmarkIPInCIDR(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IPInCIDR("192.168.1.50", "192.168.1.0/24")
	}
}

// BenchmarkConcurrentEvaluate benchmarks concurrent evaluation
func BenchmarkConcurrentEvaluate(b *testing.B) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Test Rule",
		Logic:   `Domain == "facebook.com"`,
		Action:  ActionBlock,
		Enabled: true,
	}
	_ = e.AddRule(rule)

	ctx := NewContext("facebook.com", "192.168.1.100", "A")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			e.Evaluate(ctx)
		}
	})
}
