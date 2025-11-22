package forwarder

import (
	"fmt"
	"sort"

	"glory-hole/pkg/config"
)

// ConditionalRule represents a compiled conditional forwarding rule
type ConditionalRule struct {
	Name           string
	Priority       int
	DomainMatcher  *DomainMatcher
	CIDRMatcher    *CIDRMatcher
	QueryTypeMatcher *QueryTypeMatcher
	Upstreams      []string
	Enabled        bool
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

// compileRule compiles a config rule into a conditional rule with matchers
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

// Evaluate evaluates all rules and returns upstreams for the first matching rule
// Returns nil if no rule matches
func (e *RuleEvaluator) Evaluate(domain, clientIP, queryType string) []string {
	for _, rule := range e.rules {
		if rule.Matches(domain, clientIP, queryType) {
			return rule.Upstreams
		}
	}
	return nil
}

// Matches checks if a rule matches the given query parameters
// A rule matches if ALL non-empty matchers match (AND logic)
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
