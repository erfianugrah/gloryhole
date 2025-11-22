package forwarder

import (
	"fmt"
	"sort"

	"glory-hole/pkg/config"
)

// ConditionalRule represents a compiled conditional forwarding rule
type ConditionalRule struct {
	DomainMatcher    *DomainMatcher
	CIDRMatcher      *CIDRMatcher
	QueryTypeMatcher *QueryTypeMatcher
	Name             string
	Upstreams        []string
	Priority         int
	Enabled          bool
}

// RuleEvaluator evaluates conditional forwarding rules
type RuleEvaluator struct {
	rules []*ConditionalRule
}

// NewRuleEvaluator creates a new rule evaluator from config
func NewRuleEvaluator(cfg *config.ConditionalForwardingConfig) (*RuleEvaluator, error) {
	if cfg == nil || !cfg.Enabled {
		return &RuleEvaluator{rules: []*ConditionalRule{}}, nil
	}

	evaluator := &RuleEvaluator{
		rules: make([]*ConditionalRule, 0, len(cfg.Rules)),
	}

	// Compile each rule
	for i := range cfg.Rules {
		rule, err := compileRule(&cfg.Rules[i])
		if err != nil {
			return nil, fmt.Errorf("failed to compile rule '%s': %w", cfg.Rules[i].Name, err)
		}
		if rule.Enabled {
			evaluator.rules = append(evaluator.rules, rule)
		}
	}

	// Sort rules by priority (highest first)
	sort.Slice(evaluator.rules, func(i, j int) bool {
		return evaluator.rules[i].Priority > evaluator.rules[j].Priority
	})

	return evaluator, nil
}

// compileRule compiles a configuration rule into an optimized conditional rule.
// This function transforms the YAML-based config into an efficient runtime structure
// with pre-compiled matchers for fast evaluation during DNS query processing.
//
// Compilation steps:
// 1. Domain matcher: Compiles domain patterns (exact match, wildcard, regex)
// 2. CIDR matcher: Parses and validates client IP ranges
// 3. Query type matcher: Creates lookup table for DNS query types
//
// Errors are returned if any pattern is invalid, allowing validation at
// startup rather than during query processing. This fail-fast approach
// prevents runtime surprises.
//
// Performance: Compilation happens once at startup (or config reload),
// while matching happens millions of times, so optimization focuses on
// fast matching at the cost of slower compilation.
func compileRule(cfgRule *config.ForwardingRule) (*ConditionalRule, error) {
	// Build domain matcher
	domainMatcher, err := NewDomainMatcher(cfgRule.Domains)
	if err != nil {
		return nil, fmt.Errorf("invalid domain patterns: %w", err)
	}

	// Build CIDR matcher
	cidrMatcher, err := NewCIDRMatcher(cfgRule.ClientCIDRs)
	if err != nil {
		return nil, fmt.Errorf("invalid client CIDRs: %w", err)
	}

	// Build query type matcher
	queryTypeMatcher := NewQueryTypeMatcher(cfgRule.QueryTypes)

	return &ConditionalRule{
		Name:             cfgRule.Name,
		Priority:         cfgRule.Priority,
		DomainMatcher:    domainMatcher,
		CIDRMatcher:      cidrMatcher,
		QueryTypeMatcher: queryTypeMatcher,
		Upstreams:        cfgRule.Upstreams,
		Enabled:          cfgRule.Enabled,
	}, nil
}

// Evaluate evaluates all rules in priority order and returns upstreams for the first match.
// Rules are pre-sorted by priority (highest first) during initialization, so evaluation
// stops at the first matching rule, implementing a "first-match-wins" strategy.
//
// Parameters:
// - domain: The DNS query domain (e.g., "example.com")
// - clientIP: The client's IP address (used for CIDR matching)
// - queryType: The DNS query type (e.g., "A", "AAAA", "MX")
//
// Returns:
// - []string: List of upstream servers if a rule matches
// - nil: If no rule matches (caller should use default upstreams)
//
// Performance: O(n) where n is the number of rules, but typically exits early
// due to priority ordering and first-match behavior. Empty evaluators (no rules)
// return immediately.
func (e *RuleEvaluator) Evaluate(domain, clientIP, queryType string) []string {
	for _, rule := range e.rules {
		if rule.Matches(domain, clientIP, queryType) {
			return rule.Upstreams
		}
	}
	return nil
}

// Matches checks if a rule matches the given query parameters using AND logic.
// A rule matches if ALL configured matchers match their respective criteria.
// Empty/unconfigured matchers are ignored (treated as wildcards).
//
// Matching logic:
// - Domain matcher: Must match if domains are configured
// - CIDR matcher: Must match if client CIDRs are configured
// - Query type matcher: Must match if query types are configured
//
// Example: A rule with domains=["*.local"] and queryTypes=["A", "AAAA"]
// will match queries for foo.local/A but NOT foo.local/MX
//
// Short-circuit evaluation: Returns false immediately on first non-match,
// avoiding unnecessary matcher checks for performance.
//
// Parameters:
// - domain: DNS query domain (without trailing dot)
// - clientIP: Client IP address string
// - queryType: DNS query type string (e.g., "A", "AAAA")
func (r *ConditionalRule) Matches(domain, clientIP, queryType string) bool {
	// Check domain matcher (if configured)
	if !r.DomainMatcher.IsEmpty() {
		if !r.DomainMatcher.Matches(domain) {
			return false
		}
	}

	// Check CIDR matcher (if configured)
	if !r.CIDRMatcher.IsEmpty() {
		if !r.CIDRMatcher.Matches(clientIP) {
			return false
		}
	}

	// Check query type matcher (if configured)
	if !r.QueryTypeMatcher.IsEmpty() {
		if !r.QueryTypeMatcher.Matches(queryType) {
			return false
		}
	}

	// All configured matchers passed
	return true
}

// GetRules returns all rules
func (e *RuleEvaluator) GetRules() []*ConditionalRule {
	return e.rules
}

// Count returns the number of rules
func (e *RuleEvaluator) Count() int {
	return len(e.rules)
}

// IsEmpty returns true if there are no rules
func (e *RuleEvaluator) IsEmpty() bool {
	return len(e.rules) == 0
}
