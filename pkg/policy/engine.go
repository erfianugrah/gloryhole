package policy

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Engine is the policy engine that evaluates filtering rules
type Engine struct {
	rules []*Rule
	mu    sync.RWMutex
}

// Rule represents a single policy rule
type Rule struct {
	Name       string // Human-readable name
	Logic      string // Expression to evaluate (e.g., "Hour >= 22 && Domain matches 'facebook.com'")
	Action     string // Action to take: BLOCK, ALLOW, or REDIRECT
	ActionData string // Optional data for the action (e.g., redirect target)
	Enabled    bool   // Whether the rule is active
	program    *vm.Program
}

// Action constants
const (
	ActionBlock    = "BLOCK"
	ActionAllow    = "ALLOW"
	ActionRedirect = "REDIRECT"
)

// Context represents the evaluation context for a DNS query
type Context struct {
	Domain   string    // Fully qualified domain name
	ClientIP string    // Client IP address
	QueryType string   // Query type (A, AAAA, CNAME, etc.)
	Hour     int       // Current hour (0-23)
	Minute   int       // Current minute (0-59)
	Day      int       // Day of month (1-31)
	Month    int       // Month (1-12)
	Weekday  int       // Day of week (0-6, Sunday=0)
	Time     time.Time // Full timestamp
}

// NewEngine creates a new policy engine
func NewEngine() *Engine {
	return &Engine{
		rules: make([]*Rule, 0),
	}
}

// AddRule adds and compiles a rule
func (e *Engine) AddRule(rule *Rule) error {
	if rule == nil {
		return fmt.Errorf("rule cannot be nil")
	}

	// Compile the expression with environment and helper functions
	program, err := expr.Compile(rule.Logic,
		expr.Env(Context{}),
		expr.Function("DomainMatches",
			func(params ...any) (any, error) {
				return DomainMatches(params[0].(string), params[1].(string)), nil
			},
			new(func(string, string) bool),
		),
		expr.Function("DomainEndsWith",
			func(params ...any) (any, error) {
				return DomainEndsWith(params[0].(string), params[1].(string)), nil
			},
			new(func(string, string) bool),
		),
		expr.Function("DomainStartsWith",
			func(params ...any) (any, error) {
				return DomainStartsWith(params[0].(string), params[1].(string)), nil
			},
			new(func(string, string) bool),
		),
		expr.Function("IPInCIDR",
			func(params ...any) (any, error) {
				return IPInCIDR(params[0].(string), params[1].(string)), nil
			},
			new(func(string, string) bool),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to compile rule '%s': %w", rule.Name, err)
	}

	rule.program = program

	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = append(e.rules, rule)
	return nil
}

// Evaluate evaluates all rules against the given context
// Returns (matched, rule) where matched is true if a rule matched
func (e *Engine) Evaluate(ctx Context) (bool, *Rule) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}

		// Run the compiled program
		result, err := vm.Run(rule.program, ctx)
		if err != nil {
			// Log error but continue evaluating other rules
			continue
		}

		// Check if result is true
		if matched, ok := result.(bool); ok && matched {
			return true, rule
		}
	}

	return false, nil
}

// RemoveRule removes a rule by name
func (e *Engine) RemoveRule(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, rule := range e.rules {
		if rule.Name == name {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			return true
		}
	}

	return false
}

// GetRules returns a copy of all rules
func (e *Engine) GetRules() []*Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rules := make([]*Rule, len(e.rules))
	copy(rules, e.rules)
	return rules
}

// Count returns the number of rules
func (e *Engine) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return len(e.rules)
}

// Clear removes all rules
func (e *Engine) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = make([]*Rule, 0)
}

// Helper functions that can be used in expressions

// DomainMatches checks if domain matches a pattern (case-insensitive)
func DomainMatches(domain, pattern string) bool {
	domain = strings.ToLower(domain)
	pattern = strings.ToLower(pattern)

	// Simple contains check
	if strings.Contains(domain, pattern) {
		return true
	}

	// Suffix check (e.g., ".facebook.com" matches "www.facebook.com" or "facebook.com")
	if strings.HasPrefix(pattern, ".") {
		suffix := pattern[1:] // Remove leading dot
		// Match if domain ends with pattern OR equals pattern without dot
		return strings.HasSuffix(domain, pattern) || domain == suffix
	}

	return false
}

// DomainEndsWith checks if domain ends with a suffix
func DomainEndsWith(domain, suffix string) bool {
	return strings.HasSuffix(strings.ToLower(domain), strings.ToLower(suffix))
}

// DomainStartsWith checks if domain starts with a prefix
func DomainStartsWith(domain, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(domain), strings.ToLower(prefix))
}

// IPInCIDR checks if an IP is in a CIDR range
func IPInCIDR(ipStr, cidrStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return false
	}

	return ipNet.Contains(ip)
}

// NewContext creates a new evaluation context from a DNS query
func NewContext(domain, clientIP, queryType string) Context {
	now := time.Now()

	return Context{
		Domain:    domain,
		ClientIP:  clientIP,
		QueryType: queryType,
		Hour:      now.Hour(),
		Minute:    now.Minute(),
		Day:       now.Day(),
		Month:     int(now.Month()),
		Weekday:   int(now.Weekday()),
		Time:      now,
	}
}
