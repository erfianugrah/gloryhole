package policy

import (
	"testing"
	"time"
)

func TestNewEngine(t *testing.T) {
	e := NewEngine()
	if e == nil {
		t.Fatal("NewEngine() returned nil")
	}

	if e.Count() != 0 {
		t.Errorf("expected 0 rules, got %d", e.Count())
	}
}

func TestAddRule(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Test Rule",
		Logic:   "true",
		Action:  "BLOCK",
		Enabled: true,
	}
	err := e.AddRule(rule)
	if err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}
	if e.Count() != 1 {
		t.Errorf("expected 1 rule, got %d", e.Count())
	}
}

func TestAddRule_InvalidLogic(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Invalid Rule",
		Logic:   "invalid expression!!",
		Action:  "BLOCK",
		Enabled: true,
	}
	err := e.AddRule(rule)
	if err == nil {
		t.Error("expected error for invalid logic, got nil")
	}
}

func TestEvaluate_SimpleTrue(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Always Match",
		Logic:   "true",
		Action:  ActionBlock,
		Enabled: true,
	}

	if err := e.AddRule(rule); err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}

	ctx := NewContext("example.com", "192.168.1.100", "A")
	matched, matchedRule := e.Evaluate(ctx)

	if !matched {
		t.Error("expected rule to match")
	}

	if matchedRule == nil {
		t.Fatal("expected matched rule, got nil")
	}

	if matchedRule.Name != "Always Match" {
		t.Errorf("expected rule 'Always Match', got '%s'", matchedRule.Name)
	}
}

func TestEvaluate_SimpleFalse(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Never Match",
		Logic:   "false",
		Action:  ActionBlock,
		Enabled: true,
	}

	if err := e.AddRule(rule); err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}

	ctx := NewContext("example.com", "192.168.1.100", "A")
	matched, matchedRule := e.Evaluate(ctx)

	if matched {
		t.Error("expected rule not to match")
	}

	if matchedRule != nil {
		t.Error("expected no matched rule")
	}
}

func TestEvaluate_DomainMatch(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Block Facebook",
		Logic:   `Domain == "facebook.com"`,
		Action:  ActionBlock,
		Enabled: true,
	}

	if err := e.AddRule(rule); err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}

	// Should match
	ctx := NewContext("facebook.com", "192.168.1.100", "A")
	matched, _ := e.Evaluate(ctx)
	if !matched {
		t.Error("expected rule to match facebook.com")
	}

	// Should not match
	ctx = NewContext("google.com", "192.168.1.100", "A")
	matched, _ = e.Evaluate(ctx)
	if matched {
		t.Error("expected rule not to match google.com")
	}
}

func TestEvaluate_ClientIPMatch(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Block Kids Device",
		Logic:   `ClientIP == "192.168.1.50"`,
		Action:  ActionBlock,
		Enabled: true,
	}

	if err := e.AddRule(rule); err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}

	// Should match
	ctx := NewContext("example.com", "192.168.1.50", "A")
	matched, _ := e.Evaluate(ctx)
	if !matched {
		t.Error("expected rule to match IP 192.168.1.50")
	}

	// Should not match
	ctx = NewContext("example.com", "192.168.1.100", "A")
	matched, _ = e.Evaluate(ctx)
	if matched {
		t.Error("expected rule not to match IP 192.168.1.100")
	}
}

func TestEvaluate_TimeBasedRule(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Block After 10 PM",
		Logic:   "Hour >= 22 || Hour < 6",
		Action:  ActionBlock,
		Enabled: true,
	}

	if err := e.AddRule(rule); err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}

	// Test with current time (we'll just check it compiles and runs)
	ctx := NewContext("example.com", "192.168.1.100", "A")
	_, _ = e.Evaluate(ctx)

	// We can't easily test specific hours without mocking time,
	// but we've verified the expression compiles and evaluates
}

func TestEvaluate_ComplexRule(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Block Social Media After Hours for Kids",
		Logic:   `(Hour >= 22 || Hour < 6) && ClientIP == "192.168.1.50" && (Domain == "facebook.com" || Domain == "instagram.com")`,
		Action:  ActionBlock,
		Enabled: true,
	}

	if err := e.AddRule(rule); err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}

	// Complex rules are compiled successfully
	if e.Count() != 1 {
		t.Error("expected 1 rule")
	}
}

func TestEvaluate_DisabledRule(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Disabled Rule",
		Logic:   "true",
		Action:  ActionBlock,
		Enabled: false, // Disabled
	}

	if err := e.AddRule(rule); err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}

	ctx := NewContext("example.com", "192.168.1.100", "A")
	matched, _ := e.Evaluate(ctx)

	if matched {
		t.Error("expected disabled rule not to match")
	}
}

func TestEvaluate_MultipleRules_FirstMatch(t *testing.T) {
	e := NewEngine()

	rule1 := &Rule{
		Name:    "Rule 1",
		Logic:   "Domain == \"google.com\"",
		Action:  ActionAllow,
		Enabled: true,
	}

	rule2 := &Rule{
		Name:    "Rule 2",
		Logic:   "true", // Matches everything
		Action:  ActionBlock,
		Enabled: true,
	}

	if err := e.AddRule(rule1); err != nil {
		t.Fatalf("AddRule(rule1) failed: %v", err)
	}
	if err := e.AddRule(rule2); err != nil {
		t.Fatalf("AddRule(rule2) failed: %v", err)
	}

	// Should match rule2 (first matching rule wins)
	ctx := NewContext("facebook.com", "192.168.1.100", "A")
	matched, matchedRule := e.Evaluate(ctx)

	if !matched {
		t.Error("expected a rule to match")
	}

	if matchedRule.Name != "Rule 2" {
		t.Errorf("expected 'Rule 2' to match, got '%s'", matchedRule.Name)
	}
}

func TestRemoveRule(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:    "Test Rule",
		Logic:   "true",
		Action:  ActionBlock,
		Enabled: true,
	}

	if err := e.AddRule(rule); err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}

	if e.Count() != 1 {
		t.Errorf("expected 1 rule, got %d", e.Count())
	}

	removed := e.RemoveRule("Test Rule")
	if !removed {
		t.Error("expected rule to be removed")
	}

	if e.Count() != 0 {
		t.Errorf("expected 0 rules after removal, got %d", e.Count())
	}
}

func TestRemoveRule_NotFound(t *testing.T) {
	e := NewEngine()

	removed := e.RemoveRule("Nonexistent Rule")
	if removed {
		t.Error("expected false for nonexistent rule")
	}
}

func TestGetRules(t *testing.T) {
	e := NewEngine()

	rule1 := &Rule{Name: "Rule 1", Logic: "true", Action: ActionBlock, Enabled: true}
	rule2 := &Rule{Name: "Rule 2", Logic: "false", Action: ActionAllow, Enabled: true}

	_ = e.AddRule(rule1)
	_ = e.AddRule(rule2)

	rules := e.GetRules()
	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}

	// Modifying returned slice shouldn't affect engine
	rules[0] = &Rule{Name: "Modified", Logic: "true", Action: ActionBlock, Enabled: true}

	if e.rules[0].Name == "Modified" {
		t.Error("modifying returned slice affected engine internals")
	}
}

func TestClear(t *testing.T) {
	e := NewEngine()

	for i := 0; i < 5; i++ {
		rule := &Rule{
			Name:    "Rule",
			Logic:   "true",
			Action:  ActionBlock,
			Enabled: true,
		}
		_ = e.AddRule(rule)
	}

	if e.Count() != 5 {
		t.Errorf("expected 5 rules, got %d", e.Count())
	}

	e.Clear()

	if e.Count() != 0 {
		t.Errorf("expected 0 rules after clear, got %d", e.Count())
	}
}

func TestNewContext(t *testing.T) {
	ctx := NewContext("example.com", "192.168.1.100", "A")

	if ctx.Domain != "example.com" {
		t.Errorf("expected domain 'example.com', got '%s'", ctx.Domain)
	}

	if ctx.ClientIP != "192.168.1.100" {
		t.Errorf("expected IP '192.168.1.100', got '%s'", ctx.ClientIP)
	}

	if ctx.QueryType != "A" {
		t.Errorf("expected query type 'A', got '%s'", ctx.QueryType)
	}

	// Time fields should be set to current time
	now := time.Now()
	if ctx.Hour < 0 || ctx.Hour > 23 {
		t.Errorf("invalid hour: %d", ctx.Hour)
	}

	if ctx.Minute < 0 || ctx.Minute > 59 {
		t.Errorf("invalid minute: %d", ctx.Minute)
	}

	if ctx.Day < 1 || ctx.Day > 31 {
		t.Errorf("invalid day: %d", ctx.Day)
	}

	if ctx.Month < 1 || ctx.Month > 12 {
		t.Errorf("invalid month: %d", ctx.Month)
	}

	if ctx.Weekday < 0 || ctx.Weekday > 6 {
		t.Errorf("invalid weekday: %d", ctx.Weekday)
	}

	// Time should be close to now (within 1 second)
	if ctx.Time.Sub(now) > time.Second {
		t.Errorf("context time too far from current time")
	}
}

func TestDomainMatches(t *testing.T) {
	tests := []struct {
		domain  string
		pattern string
		want    bool
	}{
		{"facebook.com", "facebook", true},
		{"www.facebook.com", ".facebook.com", true},
		{"facebook.com", ".facebook.com", true},
		{"example.com", "facebook", false},
		{"Facebook.com", "facebook", true}, // Case insensitive
		{"api.facebook.com", "facebook", true},
	}

	for _, tt := range tests {
		got := DomainMatches(tt.domain, tt.pattern)
		if got != tt.want {
			t.Errorf("DomainMatches(%q, %q) = %v, want %v",
				tt.domain, tt.pattern, got, tt.want)
		}
	}
}

func TestDomainEndsWith(t *testing.T) {
	tests := []struct {
		domain string
		suffix string
		want   bool
	}{
		{"example.com", ".com", true},
		{"test.example.com", "example.com", true},
		{"Example.com", ".COM", true}, // Case insensitive
		{"example.org", ".com", false},
	}

	for _, tt := range tests {
		got := DomainEndsWith(tt.domain, tt.suffix)
		if got != tt.want {
			t.Errorf("DomainEndsWith(%q, %q) = %v, want %v",
				tt.domain, tt.suffix, got, tt.want)
		}
	}
}

func TestDomainStartsWith(t *testing.T) {
	tests := []struct {
		domain string
		prefix string
		want   bool
	}{
		{"www.example.com", "www", true},
		{"api.example.com", "api", true},
		{"WWW.Example.com", "www", true}, // Case insensitive
		{"example.com", "www", false},
	}

	for _, tt := range tests {
		got := DomainStartsWith(tt.domain, tt.prefix)
		if got != tt.want {
			t.Errorf("DomainStartsWith(%q, %q) = %v, want %v",
				tt.domain, tt.prefix, got, tt.want)
		}
	}
}

func TestIPInCIDR(t *testing.T) {
	tests := []struct {
		ip   string
		cidr string
		want bool
	}{
		{"192.168.1.50", "192.168.1.0/24", true},
		{"192.168.1.1", "192.168.1.0/24", true},
		{"192.168.1.255", "192.168.1.0/24", true},
		{"192.168.2.1", "192.168.1.0/24", false},
		{"10.0.0.1", "192.168.1.0/24", false},
		{"invalid", "192.168.1.0/24", false},
		{"192.168.1.1", "invalid", false},
	}

	for _, tt := range tests {
		got := IPInCIDR(tt.ip, tt.cidr)
		if got != tt.want {
			t.Errorf("IPInCIDR(%q, %q) = %v, want %v",
				tt.ip, tt.cidr, got, tt.want)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	e := NewEngine()

	// Add initial rules
	for i := 0; i < 5; i++ {
		rule := &Rule{
			Name:    "Rule",
			Logic:   "true",
			Action:  ActionBlock,
			Enabled: true,
		}
		_ = e.AddRule(rule)
	}

	done := make(chan bool)

	// Concurrent readers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ctx := NewContext("example.com", "192.168.1.100", "A")
				e.Evaluate(ctx)
				e.Count()
				e.GetRules()
			}
			done <- true
		}()
	}

	// Concurrent writers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				rule := &Rule{
					Name:    "Concurrent Rule",
					Logic:   "true",
					Action:  ActionBlock,
					Enabled: true,
				}
				_ = e.AddRule(rule)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}
}

func TestHelperFunctions_InExpressions(t *testing.T) {
	e := NewEngine()

	tests := []struct {
		name        string
		logic       string
		domain      string
		clientIP    string
		shouldMatch bool
	}{
		{
			name:        "DomainMatches with substring",
			logic:       `DomainMatches(Domain, "facebook")`,
			domain:      "www.facebook.com",
			clientIP:    "192.168.1.100",
			shouldMatch: true,
		},
		{
			name:        "DomainEndsWith",
			logic:       `DomainEndsWith(Domain, ".com")`,
			domain:      "example.com",
			clientIP:    "192.168.1.100",
			shouldMatch: true,
		},
		{
			name:        "DomainStartsWith",
			logic:       `DomainStartsWith(Domain, "www")`,
			domain:      "www.example.com",
			clientIP:    "192.168.1.100",
			shouldMatch: true,
		},
		{
			name:        "IPInCIDR",
			logic:       `IPInCIDR(ClientIP, "192.168.1.0/24")`,
			domain:      "example.com",
			clientIP:    "192.168.1.50",
			shouldMatch: true,
		},
		{
			name:        "Combined helper functions",
			logic:       `DomainMatches(Domain, "facebook") && IPInCIDR(ClientIP, "192.168.1.0/24")`,
			domain:      "facebook.com",
			clientIP:    "192.168.1.50",
			shouldMatch: true,
		},
		{
			name:        "Helper function returns false",
			logic:       `DomainMatches(Domain, "twitter")`,
			domain:      "facebook.com",
			clientIP:    "192.168.1.100",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear previous rules
			e.Clear()

			rule := &Rule{
				Name:    tt.name,
				Logic:   tt.logic,
				Action:  ActionBlock,
				Enabled: true,
			}

			if err := e.AddRule(rule); err != nil {
				t.Fatalf("AddRule() failed: %v", err)
			}

			ctx := NewContext(tt.domain, tt.clientIP, "A")
			matched, _ := e.Evaluate(ctx)

			if matched != tt.shouldMatch {
				t.Errorf("expected match=%v, got %v", tt.shouldMatch, matched)
			}
		})
	}
}

func TestParseUpstreams(t *testing.T) {
	tests := []struct {
		name       string
		actionData string
		want       []string
		wantErr    bool
	}{
		{
			name:       "single upstream with port",
			actionData: "1.1.1.1:53",
			want:       []string{"1.1.1.1:53"},
			wantErr:    false,
		},
		{
			name:       "single upstream without port",
			actionData: "8.8.8.8",
			want:       []string{"8.8.8.8:53"},
			wantErr:    false,
		},
		{
			name:       "multiple upstreams",
			actionData: "1.1.1.1:53, 8.8.8.8:53, 9.9.9.9",
			want:       []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"},
			wantErr:    false,
		},
		{
			name:       "empty string",
			actionData: "",
			want:       nil,
			wantErr:    true,
		},
		{
			name:       "whitespace only entries filtered",
			actionData: "1.1.1.1, , 8.8.8.8",
			want:       []string{"1.1.1.1:53", "8.8.8.8:53"},
			wantErr:    false,
		},
		{
			name:       "invalid format - empty host",
			actionData: ":53",
			want:       nil,
			wantErr:    true,
		},
		{
			name:       "invalid format - empty port",
			actionData: "1.1.1.1:",
			want:       nil,
			wantErr:    true,
		},
		{
			name:       "hostname with port",
			actionData: "dns.google:853",
			want:       []string{"dns.google:853"},
			wantErr:    false,
		},
		{
			name:       "hostname without port",
			actionData: "dns.google",
			want:       []string{"dns.google:53"},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUpstreams(tt.actionData)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseUpstreams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("ParseUpstreams() got %d upstreams, want %d", len(got), len(tt.want))
					return
				}
				for i, upstream := range got {
					if upstream != tt.want[i] {
						t.Errorf("ParseUpstreams()[%d] = %v, want %v", i, upstream, tt.want[i])
					}
				}
			}
		})
	}
}

func TestGetUpstreams(t *testing.T) {
	tests := []struct {
		name       string
		rule       *Rule
		want       []string
		wantNil    bool
	}{
		{
			name: "FORWARD action with valid upstreams",
			rule: &Rule{
				Action:     ActionForward,
				ActionData: "1.1.1.1:53, 8.8.8.8",
			},
			want:    []string{"1.1.1.1:53", "8.8.8.8:53"},
			wantNil: false,
		},
		{
			name: "BLOCK action returns nil",
			rule: &Rule{
				Action:     ActionBlock,
				ActionData: "1.1.1.1:53",
			},
			want:    nil,
			wantNil: true,
		},
		{
			name: "ALLOW action returns nil",
			rule: &Rule{
				Action:     ActionAllow,
				ActionData: "1.1.1.1:53",
			},
			want:    nil,
			wantNil: true,
		},
		{
			name: "FORWARD with invalid upstreams returns nil",
			rule: &Rule{
				Action:     ActionForward,
				ActionData: "",
			},
			want:    nil,
			wantNil: true,
		},
		{
			name: "FORWARD with malformed upstreams returns nil",
			rule: &Rule{
				Action:     ActionForward,
				ActionData: ":53",
			},
			want:    nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rule.GetUpstreams()
			if tt.wantNil {
				if got != nil {
					t.Errorf("GetUpstreams() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Error("GetUpstreams() = nil, want non-nil")
					return
				}
				if len(got) != len(tt.want) {
					t.Errorf("GetUpstreams() got %d upstreams, want %d", len(got), len(tt.want))
					return
				}
				for i, upstream := range got {
					if upstream != tt.want[i] {
						t.Errorf("GetUpstreams()[%d] = %v, want %v", i, upstream, tt.want[i])
					}
				}
			}
		})
	}
}

func TestValidateActionViaAddRule(t *testing.T) {
	// validateAction is unexported, so test it indirectly through AddRule
	tests := []struct {
		name       string
		action     string
		actionData string
		wantErr    bool
	}{
		{
			name:       "BLOCK action with empty data",
			action:     ActionBlock,
			actionData: "",
			wantErr:    false,
		},
		{
			name:       "ALLOW action with empty data",
			action:     ActionAllow,
			actionData: "",
			wantErr:    false,
		},
		{
			name:       "REDIRECT action with target",
			action:     ActionRedirect,
			actionData: "127.0.0.1",
			wantErr:    false,
		},
		{
			name:       "REDIRECT action without target",
			action:     ActionRedirect,
			actionData: "",
			wantErr:    true,
		},
		{
			name:       "FORWARD action with upstreams",
			action:     ActionForward,
			actionData: "1.1.1.1:53, 8.8.8.8",
			wantErr:    false,
		},
		{
			name:       "FORWARD action without upstreams",
			action:     ActionForward,
			actionData: "",
			wantErr:    true,
		},
		{
			name:       "FORWARD action with invalid upstreams",
			action:     ActionForward,
			actionData: ":53",
			wantErr:    true,
		},
	}

	engine := NewEngine()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := &Rule{
				Name:       tt.name,
				Logic:      "true", // Simple logic that always matches
				Action:     tt.action,
				ActionData: tt.actionData,
				Enabled:    true,
			}
			err := engine.AddRule(rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddRule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
