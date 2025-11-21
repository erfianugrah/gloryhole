package policy

import (
	"fmt"
	"net"
	"regexp"
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
	Domain    string    // Fully qualified domain name
	ClientIP  string    // Client IP address
	QueryType string    // Query type (A, AAAA, CNAME, etc.)
	Hour      int       // Current hour (0-23)
	Minute    int       // Current minute (0-59)
	Day       int       // Day of month (1-31)
	Month     int       // Month (1-12)
	Weekday   int       // Day of week (0-6, Sunday=0)
	Time      time.Time // Full timestamp
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
		// Domain matching functions
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
		expr.Function("DomainRegex",
			func(params ...any) (any, error) {
				result, err := DomainRegex(params[0].(string), params[1].(string))
				if err != nil {
					return false, err
				}
				return result, nil
			},
			new(func(string, string) bool),
		),
		expr.Function("DomainLevelCount",
			func(params ...any) (any, error) {
				return DomainLevelCount(params[0].(string)), nil
			},
			new(func(string) int),
		),
		// IP matching functions
		expr.Function("IPInCIDR",
			func(params ...any) (any, error) {
				return IPInCIDR(params[0].(string), params[1].(string)), nil
			},
			new(func(string, string) bool),
		),
		expr.Function("IPEquals",
			func(params ...any) (any, error) {
				return IPEquals(params[0].(string), params[1].(string)), nil
			},
			new(func(string, string) bool),
		),
		// Query type functions
		expr.Function("QueryTypeIn",
			func(params ...any) (any, error) {
				queryType := params[0].(string)
				types := make([]string, len(params)-1)
				for i := 1; i < len(params); i++ {
					types[i-1] = params[i].(string)
				}
				return QueryTypeIn(queryType, types...), nil
			},
		),
		// Time functions
		expr.Function("IsWeekend",
			func(params ...any) (any, error) {
				return IsWeekend(params[0].(int)), nil
			},
			new(func(int) bool),
		),
		expr.Function("InTimeRange",
			func(params ...any) (any, error) {
				return InTimeRange(
					params[0].(int), params[1].(int),
					params[2].(int), params[3].(int),
					params[4].(int), params[5].(int),
				), nil
			},
			new(func(int, int, int, int, int, int) bool),
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

// DomainRegex checks if domain matches a regular expression pattern
func DomainRegex(domain, pattern string) (bool, error) {
	matched, err := regexp.MatchString(pattern, strings.ToLower(domain))
	if err != nil {
		return false, fmt.Errorf("invalid regex pattern: %w", err)
	}
	return matched, nil
}

// DomainLevelCount returns the number of levels in a domain (e.g., "www.example.com" = 3)
func DomainLevelCount(domain string) int {
	if domain == "" {
		return 0
	}
	// Remove trailing dot if present
	domain = strings.TrimSuffix(domain, ".")
	return strings.Count(domain, ".") + 1
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

// IPEquals checks if two IP addresses are equal (handles IPv4/IPv6 normalization)
func IPEquals(ip1Str, ip2Str string) bool {
	ip1 := net.ParseIP(ip1Str)
	ip2 := net.ParseIP(ip2Str)

	if ip1 == nil || ip2 == nil {
		return false
	}

	return ip1.Equal(ip2)
}

// QueryTypeIn checks if query type is in a list of types
func QueryTypeIn(queryType string, types ...string) bool {
	queryType = strings.ToUpper(queryType)
	for _, t := range types {
		if strings.ToUpper(t) == queryType {
			return true
		}
	}
	return false
}

// IsWeekend checks if the given weekday is Saturday (6) or Sunday (0)
func IsWeekend(weekday int) bool {
	return weekday == 0 || weekday == 6
}

// InTimeRange checks if the current time (hour:minute) is within the specified range
// Handles ranges that cross midnight (e.g., 23:00 - 02:00)
func InTimeRange(hour, minute, startHour, startMinute, endHour, endMinute int) bool {
	currentMinutes := hour*60 + minute
	startMinutes := startHour*60 + startMinute
	endMinutes := endHour*60 + endMinute

	// Normal range (doesn't cross midnight)
	if startMinutes <= endMinutes {
		return currentMinutes >= startMinutes && currentMinutes <= endMinutes
	}

	// Range crosses midnight (e.g., 23:00 - 02:00)
	return currentMinutes >= startMinutes || currentMinutes <= endMinutes
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
