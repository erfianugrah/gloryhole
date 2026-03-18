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
