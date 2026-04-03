package policy

// TestUIGeneratedExpressions validates every expression pattern the frontend
// ConditionEditor.tsx conditionToExpr() can produce.  Each case uses the EXACT
// string the FIXED UI generates for a given field + operator + value.

import (
	"testing"
)

type uiExprCase struct {
	name        string
	expr        string // exact expression the UI emits
	wantCompile bool
	wantMatch   bool
	domain      string
	clientIP    string
	queryType   string
}

func (tc *uiExprCase) defaults() {
	if tc.domain == "" {
		tc.domain = "ads.example.com"
	}
	if tc.clientIP == "" {
		tc.clientIP = "192.168.1.50"
	}
	if tc.queryType == "" {
		tc.queryType = "A"
	}
}

func TestUIGeneratedExpressions(t *testing.T) {
	cases := []uiExprCase{
		// ── Domain ── every operator ────────────────────────────
		{name: "Domain/equals/match", expr: `Domain == "ads.example.com"`, wantCompile: true, wantMatch: true},
		{name: "Domain/equals/no-match", expr: `Domain == "other.com"`, wantCompile: true, wantMatch: false},
		{name: "Domain/not-equals", expr: `Domain != "other.com"`, wantCompile: true, wantMatch: true},
		{name: "Domain/contains/match", expr: `DomainMatches(Domain, "example")`, wantCompile: true, wantMatch: true},
		{name: "Domain/contains/no-match", expr: `DomainMatches(Domain, "facebook")`, wantCompile: true, wantMatch: false},
		{name: "Domain/starts_with", expr: `DomainStartsWith(Domain, "ads")`, wantCompile: true, wantMatch: true},
		{name: "Domain/ends_with", expr: `DomainEndsWith(Domain, ".example.com")`, wantCompile: true, wantMatch: true},
		{name: "Domain/matches-regex", expr: `DomainRegex(Domain, ".*\\.example\\.com")`, wantCompile: true, wantMatch: true},
		{name: "Domain/matches-regex/no-match", expr: `DomainRegex(Domain, "^facebook")`, wantCompile: true, wantMatch: false},
		{name: "Domain/in-list/match", expr: `Domain in ["ads.example.com", "other.com"]`, wantCompile: true, wantMatch: true},
		{name: "Domain/in-list/no-match", expr: `Domain in ["foo.com", "bar.com"]`, wantCompile: true, wantMatch: false},

		// ── ClientIP ── restricted to ==, !=, in (CIDR) ────────
		{name: "ClientIP/equals", expr: `IPEquals(ClientIP, "192.168.1.50")`, wantCompile: true, wantMatch: true},
		{name: "ClientIP/equals/no-match", expr: `IPEquals(ClientIP, "10.0.0.1")`, wantCompile: true, wantMatch: false},
		{name: "ClientIP/not-equals", expr: `!IPEquals(ClientIP, "10.0.0.1")`, wantCompile: true, wantMatch: true},
		{name: "ClientIP/not-equals/self", expr: `!IPEquals(ClientIP, "192.168.1.50")`, wantCompile: true, wantMatch: false},
		{name: "ClientIP/in-CIDR/match", expr: `IPInCIDR(ClientIP, "192.168.1.0/24")`, wantCompile: true, wantMatch: true},
		{name: "ClientIP/in-CIDR/no-match", expr: `IPInCIDR(ClientIP, "10.0.0.0/8")`, wantCompile: true, wantMatch: false},

		// ── QueryType ── restricted to ==, !=, in ──────────────
		{name: "QueryType/equals", expr: `QueryType == "A"`, wantCompile: true, wantMatch: true},
		{name: "QueryType/not-equals", expr: `QueryType != "AAAA"`, wantCompile: true, wantMatch: true},
		{name: "QueryType/in-list", expr: `QueryTypeIn(QueryType, "A", "AAAA", "CNAME")`, wantCompile: true, wantMatch: true},
		{name: "QueryType/in-list/no-match", expr: `QueryTypeIn(QueryType, "MX", "TXT")`, wantCompile: true, wantMatch: false},
		{name: "QueryType/in-list/case-insensitive", expr: `QueryTypeIn(QueryType, "a")`, wantCompile: true, wantMatch: true},

		// ── Hour (int) ── numeric operators, no quotes ─────────
		{name: "Hour/equals", expr: `Hour == 14`, wantCompile: true, wantMatch: true},
		{name: "Hour/equals/no-match", expr: `Hour == 22`, wantCompile: true, wantMatch: false},
		{name: "Hour/not-equals", expr: `Hour != 22`, wantCompile: true, wantMatch: true},
		{name: "Hour/greater-than", expr: `Hour > 10`, wantCompile: true, wantMatch: true},
		{name: "Hour/greater-than/no-match", expr: `Hour > 20`, wantCompile: true, wantMatch: false},
		{name: "Hour/less-than", expr: `Hour < 20`, wantCompile: true, wantMatch: true},
		{name: "Hour/at-least", expr: `Hour >= 14`, wantCompile: true, wantMatch: true},
		{name: "Hour/at-most", expr: `Hour <= 14`, wantCompile: true, wantMatch: true},
		{name: "Hour/at-most/no-match", expr: `Hour <= 10`, wantCompile: true, wantMatch: false},

		// ── Weekday (int) ── numeric operators, no quotes ──────
		{name: "Weekday/equals", expr: `Weekday == 3`, wantCompile: true, wantMatch: true},
		{name: "Weekday/equals/no-match", expr: `Weekday == 0`, wantCompile: true, wantMatch: false},
		{name: "Weekday/not-equals", expr: `Weekday != 0`, wantCompile: true, wantMatch: true},
		{name: "Weekday/greater-than", expr: `Weekday > 1`, wantCompile: true, wantMatch: true},
		{name: "Weekday/less-than", expr: `Weekday < 5`, wantCompile: true, wantMatch: true},
		{name: "Weekday/at-least", expr: `Weekday >= 3`, wantCompile: true, wantMatch: true},
		{name: "Weekday/at-most", expr: `Weekday <= 3`, wantCompile: true, wantMatch: true},

		// ── Group logic ────────────────────────────────────────
		{
			name:        "AND group",
			expr:        `(DomainMatches(Domain, "example") && DomainEndsWith(Domain, ".com"))`,
			wantCompile: true, wantMatch: true,
		},
		{
			name:        "OR group",
			expr:        `(Domain == "ads.example.com" || Domain == "other.com")`,
			wantCompile: true, wantMatch: true,
		},
		{
			name:        "NOT group",
			expr:        `!((Domain == "other.com" && ClientIP == "10.0.0.1"))`,
			wantCompile: true, wantMatch: true,
		},
		{
			name:        "nested AND inside OR",
			expr:        `((DomainMatches(Domain, "example") && IPEquals(ClientIP, "192.168.1.50")) || Domain == "other.com")`,
			wantCompile: true, wantMatch: true,
		},
		{
			name:        "cross-field: domain + hour",
			expr:        `(DomainMatches(Domain, "example") && Hour >= 8 && Hour <= 18)`,
			wantCompile: true, wantMatch: true,
		},
		{
			name:        "cross-field: IP + weekday",
			expr:        `(IPInCIDR(ClientIP, "192.168.1.0/24") && Weekday >= 1 && Weekday <= 5)`,
			wantCompile: true, wantMatch: true,
		},

		// ── Negated operators (per-condition negation, no NOT groups) ──
		// These patterns are generated by the new ConditionEditor with
		// negated operator variants like "not_contains", "not_starts_with", etc.
		{name: "Domain/not-contains/match", expr: `!DomainMatches(Domain, "ads")`, wantCompile: true, wantMatch: false,
			domain: "ads.example.com"},
		{name: "Domain/not-contains/no-match", expr: `!DomainMatches(Domain, "facebook")`, wantCompile: true, wantMatch: true},
		{name: "Domain/not-starts-with/match", expr: `!DomainStartsWith(Domain, "ads")`, wantCompile: true, wantMatch: false,
			domain: "ads.example.com"},
		{name: "Domain/not-starts-with/no-match", expr: `!DomainStartsWith(Domain, "other")`, wantCompile: true, wantMatch: true},
		{name: "Domain/not-ends-with/match", expr: `!DomainEndsWith(Domain, ".example.com")`, wantCompile: true, wantMatch: false,
			domain: "ads.example.com"},
		{name: "Domain/not-ends-with/no-match", expr: `!DomainEndsWith(Domain, ".other.com")`, wantCompile: true, wantMatch: true},
		{name: "Domain/not-matches-regex/match", expr: `!DomainRegex(Domain, ".*\\.example\\.com")`, wantCompile: true, wantMatch: false,
			domain: "ads.example.com"},
		{name: "Domain/not-matches-regex/no-match", expr: `!DomainRegex(Domain, "^facebook")`, wantCompile: true, wantMatch: true},
		{name: "Domain/not-in-list/match", expr: `!(Domain in ["ads.example.com", "other.com"])`, wantCompile: true, wantMatch: false,
			domain: "ads.example.com"},
		{name: "Domain/not-in-list/no-match", expr: `!(Domain in ["foo.com", "bar.com"])`, wantCompile: true, wantMatch: true},
		{name: "ClientIP/not-in-cidr/match", expr: `!IPInCIDR(ClientIP, "192.168.1.0/24")`, wantCompile: true, wantMatch: false,
			clientIP: "192.168.1.50"},
		{name: "ClientIP/not-in-cidr/no-match", expr: `!IPInCIDR(ClientIP, "10.0.0.0/8")`, wantCompile: true, wantMatch: true,
			clientIP: "192.168.1.50"},
		{name: "QueryType/not-in/match", expr: `!QueryTypeIn(QueryType, "A", "AAAA")`, wantCompile: true, wantMatch: false,
			queryType: "A"},
		{name: "QueryType/not-in/no-match", expr: `!QueryTypeIn(QueryType, "AAAA", "MX")`, wantCompile: true, wantMatch: true,
			queryType: "A"},

		// ── Regression: the Adobe policy from the screenshot ──
		{
			name:        "Adobe policy — should use OR not AND",
			expr:        `(DomainMatches(Domain, "adobe.io") || DomainMatches(Domain, "adobe.com"))`,
			wantCompile: true, wantMatch: false,
			domain: "creativecloud.adobe.com",
			// "adobe.io" is not in "creativecloud.adobe.com" but "adobe.com" IS,
			// so with OR this now correctly matches.
		},
	}

	// Override: the Adobe OR test should actually match
	cases[len(cases)-1].wantMatch = true

	runCases(t, cases)
}

// TestRealWorldPolicies reproduces the exact user workflow for creating
// policies via the UI visual builder for real domains: sentry.io, adobe.io,
// adobe.com.  Each test builds the expression string exactly as the TS
// conditionToExpr() + groupToExpr() would for a given builder configuration,
// then verifies it compiles and matches (or doesn't) against realistic
// domain lists.
func TestRealWorldPolicies(t *testing.T) {
	// ── Scenario 1: Adobe policy ────────────────────────────────────
	// User creates an ALLOW policy named "Adobe" with two conditions:
	//   Domain "contains" adobe.io   → DomainMatches(Domain, "adobe.io")
	//   Domain "contains" adobe.com  → DomainMatches(Domain, "adobe.com")
	// Visual builder has OR selected → joined with ||

	adobeOR := `(DomainMatches(Domain, "adobe.io") || DomainMatches(Domain, "adobe.com"))`
	adobeAND := `(DomainMatches(Domain, "adobe.io") && DomainMatches(Domain, "adobe.com"))`

	adobeDomains := []struct {
		domain  string
		wantOR  bool
		wantAND bool
	}{
		{"creativecloud.adobe.com", true, false}, // contains "adobe.com" but not "adobe.io"
		{"api.adobe.io", true, false},            // contains "adobe.io" but not "adobe.com"
		{"license.adobe.io", true, false},
		{"exchange.adobe.com", true, false},
		{"cdn.adobe.io.adobe.com", true, true},   // contains both
		{"google.com", false, false},             // contains neither
		{"notadobe.io.example.com", true, false}, // substring "adobe.io" IS in "notadobe.io..."
		{"adobe.io", true, false},                // exact
		{"adobe.com", true, false},               // exact
	}

	t.Run("Adobe/OR", func(t *testing.T) {
		engine := NewEngine(nil)
		if err := engine.AddRule(&Rule{
			Name: "Adobe", Logic: adobeOR, Action: ActionAllow, Enabled: true,
		}); err != nil {
			t.Fatalf("compile failed: %v\n  expr: %s", err, adobeOR)
		}
		for _, d := range adobeDomains {
			ctx := Context{Domain: d.domain, ClientIP: "127.0.0.1", QueryType: "A"}
			matched, _ := engine.Evaluate(ctx)
			if matched != d.wantOR {
				t.Errorf("domain %q: expected match=%v, got %v", d.domain, d.wantOR, matched)
			}
		}
	})

	t.Run("Adobe/AND", func(t *testing.T) {
		engine := NewEngine(nil)
		if err := engine.AddRule(&Rule{
			Name: "Adobe", Logic: adobeAND, Action: ActionAllow, Enabled: true,
		}); err != nil {
			t.Fatalf("compile failed: %v\n  expr: %s", err, adobeAND)
		}
		for _, d := range adobeDomains {
			ctx := Context{Domain: d.domain, ClientIP: "127.0.0.1", QueryType: "A"}
			matched, _ := engine.Evaluate(ctx)
			if matched != d.wantAND {
				t.Errorf("domain %q: expected match=%v, got %v", d.domain, d.wantAND, matched)
			}
		}
	})

	// ── Scenario 2: Sentry policy ──────────────────────────────────
	// User creates an ALLOW policy for Sentry with:
	//   Domain "contains" sentry.io    → DomainMatches(Domain, "sentry.io")
	//
	// Single condition — no group wrapper needed.

	sentryExpr := `DomainMatches(Domain, "sentry.io")`

	sentryDomains := []struct {
		domain string
		want   bool
	}{
		{"sentry.io", true},
		{"o123456.ingest.sentry.io", true},
		{"browser.sentry-cdn.com", false}, // "sentry.io" not in "sentry-cdn.com"
		{"api.sentry.io", true},
		{"sentry.io.example.com", true}, // substring match
		{"google.com", false},
	}

	t.Run("Sentry/contains", func(t *testing.T) {
		engine := NewEngine(nil)
		if err := engine.AddRule(&Rule{
			Name: "Sentry", Logic: sentryExpr, Action: ActionAllow, Enabled: true,
		}); err != nil {
			t.Fatalf("compile failed: %v\n  expr: %s", err, sentryExpr)
		}
		for _, d := range sentryDomains {
			ctx := Context{Domain: d.domain, ClientIP: "127.0.0.1", QueryType: "A"}
			matched, _ := engine.Evaluate(ctx)
			if matched != d.want {
				t.Errorf("domain %q: expected match=%v, got %v", d.domain, d.want, matched)
			}
		}
	})

	// ── Scenario 3: Sentry with "ends with" ────────────────────────
	// User selects "ends with" + "sentry.io"
	// UI generates: DomainEndsWith(Domain, ".sentry.io")  (dot prepended)

	sentryEndsExpr := `DomainEndsWith(Domain, ".sentry.io")`

	sentryEndsDomains := []struct {
		domain string
		want   bool
	}{
		{"sentry.io", false},               // does NOT end with ".sentry.io"
		{"o123456.ingest.sentry.io", true}, // ends with ".sentry.io"
		{"api.sentry.io", true},
		{"notsentry.io", false}, // ends with "sentry.io" but not ".sentry.io"
		{"google.com", false},
	}

	t.Run("Sentry/ends_with", func(t *testing.T) {
		engine := NewEngine(nil)
		if err := engine.AddRule(&Rule{
			Name: "Sentry", Logic: sentryEndsExpr, Action: ActionAllow, Enabled: true,
		}); err != nil {
			t.Fatalf("compile failed: %v\n  expr: %s", err, sentryEndsExpr)
		}
		for _, d := range sentryEndsDomains {
			ctx := Context{Domain: d.domain, ClientIP: "127.0.0.1", QueryType: "A"}
			matched, _ := engine.Evaluate(ctx)
			if matched != d.want {
				t.Errorf("domain %q: expected match=%v, got %v", d.domain, d.want, matched)
			}
		}
	})

	// ── Scenario 4: Adobe + Sentry combined ────────────────────────
	// User creates a policy with OR group containing three conditions:
	//   Domain contains "adobe.io"
	//   Domain contains "adobe.com"
	//   Domain contains "sentry.io"

	combinedExpr := `(DomainMatches(Domain, "adobe.io") || DomainMatches(Domain, "adobe.com") || DomainMatches(Domain, "sentry.io"))`

	combinedDomains := []struct {
		domain string
		want   bool
	}{
		{"creativecloud.adobe.com", true},
		{"api.adobe.io", true},
		{"o123456.ingest.sentry.io", true},
		{"google.com", false},
		{"facebook.com", false},
	}

	t.Run("Combined/Adobe+Sentry", func(t *testing.T) {
		engine := NewEngine(nil)
		if err := engine.AddRule(&Rule{
			Name: "Allow Adobe+Sentry", Logic: combinedExpr, Action: ActionAllow, Enabled: true,
		}); err != nil {
			t.Fatalf("compile failed: %v\n  expr: %s", err, combinedExpr)
		}
		for _, d := range combinedDomains {
			ctx := Context{Domain: d.domain, ClientIP: "127.0.0.1", QueryType: "A"}
			matched, _ := engine.Evaluate(ctx)
			if matched != d.want {
				t.Errorf("domain %q: expected match=%v, got %v", d.domain, d.want, matched)
			}
		}
	})

	// ── Scenario 5: Adobe with "equals" operator ───────────────────
	// User selects "equals" instead of "contains"
	// UI generates: Domain == "adobe.com"
	// This is an EXACT match — subdomains won't match.

	t.Run("Adobe/equals-exact", func(t *testing.T) {
		expr := `Domain == "adobe.com"`
		engine := NewEngine(nil)
		if err := engine.AddRule(&Rule{
			Name: "Adobe exact", Logic: expr, Action: ActionAllow, Enabled: true,
		}); err != nil {
			t.Fatalf("compile failed: %v", err)
		}

		tests := []struct {
			domain string
			want   bool
		}{
			{"adobe.com", true},
			{"creativecloud.adobe.com", false}, // NOT equal
			{"adobe.com.evil.com", false},
		}
		for _, d := range tests {
			ctx := Context{Domain: d.domain, ClientIP: "127.0.0.1", QueryType: "A"}
			matched, _ := engine.Evaluate(ctx)
			if matched != d.want {
				t.Errorf("domain %q: expected match=%v, got %v", d.domain, d.want, matched)
			}
		}
	})

	// ── Scenario 6: Regex match ────────────────────────────────────
	// User selects "matches (regex)" with pattern ".*\.sentry\.io$"
	// UI generates: DomainRegex(Domain, ".*\\.sentry\\.io$")

	t.Run("Sentry/regex", func(t *testing.T) {
		expr := `DomainRegex(Domain, ".*\\.sentry\\.io$")`
		engine := NewEngine(nil)
		if err := engine.AddRule(&Rule{
			Name: "Sentry regex", Logic: expr, Action: ActionAllow, Enabled: true,
		}); err != nil {
			t.Fatalf("compile failed: %v", err)
		}

		tests := []struct {
			domain string
			want   bool
		}{
			{"o123456.ingest.sentry.io", true},
			{"api.sentry.io", true},
			{"sentry.io", false},    // no dot before "sentry" — pattern requires subdomain
			{"notsentry.io", false}, // no dot before sentry
			{"sentry.io.evil.com", false},
		}
		for _, d := range tests {
			ctx := Context{Domain: d.domain, ClientIP: "127.0.0.1", QueryType: "A"}
			matched, _ := engine.Evaluate(ctx)
			if matched != d.want {
				t.Errorf("domain %q: expected match=%v, got %v", d.domain, d.want, matched)
			}
		}
	})
}

func runCases(t *testing.T, cases []uiExprCase) {
	for _, tc := range cases {
		tc.defaults()
		t.Run(tc.name, func(t *testing.T) {
			engine := NewEngine(nil)
			rule := &Rule{
				Name:    "test",
				Logic:   tc.expr,
				Action:  ActionBlock,
				Enabled: true,
			}

			err := engine.AddRule(rule)
			compiled := err == nil

			if compiled != tc.wantCompile {
				if tc.wantCompile {
					t.Fatalf("expected expression to compile but got error: %v\n  expr: %s", err, tc.expr)
				} else {
					t.Fatalf("expected compilation to FAIL but it succeeded\n  expr: %s", tc.expr)
				}
			}

			if !compiled {
				return
			}

			ctx := Context{
				Domain:    tc.domain,
				ClientIP:  tc.clientIP,
				QueryType: tc.queryType,
				Hour:      14,
				Minute:    30,
				Day:       15,
				Month:     6,
				Weekday:   3, // Wednesday
			}

			matched, _ := engine.Evaluate(ctx)
			if matched != tc.wantMatch {
				t.Errorf("expected match=%v, got %v\n  expr: %s\n  ctx: Domain=%q ClientIP=%q QueryType=%q Hour=%d Weekday=%d",
					tc.wantMatch, matched, tc.expr, ctx.Domain, ctx.ClientIP, ctx.QueryType, ctx.Hour, ctx.Weekday)
			}
		})
	}
}
