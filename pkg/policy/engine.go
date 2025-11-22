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
	program    *vm.Program
	Name       string
	Logic      string
	Action     string
	ActionData string
	Enabled    bool
}

// Action constants
const (
	ActionBlock    = "BLOCK"
	ActionAllow    = "ALLOW"
	ActionRedirect = "REDIRECT"
	ActionForward  = "FORWARD"
)

// Context represents the evaluation context for a DNS query
type Context struct {
	Time      time.Time
	Domain    string
	ClientIP  string
	QueryType string
	Hour      int
	Minute    int
	Day       int
	Month     int
	Weekday   int
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

	// Validate action and action_data
	if err := validateAction(rule); err != nil {
		return fmt.Errorf("invalid rule '%s': %w", rule.Name, err)
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

// validateAction validates the action and action_data fields of a rule
func validateAction(rule *Rule) error {
	action := strings.ToUpper(rule.Action)
	rule.Action = action // Normalize to uppercase

	switch action {
	case ActionBlock, ActionAllow:
		// No action_data needed
		return nil

	case ActionRedirect:
		// Validate IP address in action_data
		if rule.ActionData == "" {
			return fmt.Errorf("REDIRECT action requires action_data (IP address)")
		}
		if net.ParseIP(rule.ActionData) == nil {
			return fmt.Errorf("invalid IP address in action_data: %s", rule.ActionData)
		}
		return nil

	case ActionForward:
		// Validate upstream list in action_data
		if rule.ActionData == "" {
			return fmt.Errorf("FORWARD action requires action_data (upstream DNS servers)")
		}
		upstreams, err := ParseUpstreams(rule.ActionData)
		if err != nil {
			return fmt.Errorf("invalid upstreams in action_data: %w", err)
		}
		if len(upstreams) == 0 {
			return fmt.Errorf("FORWARD action requires at least one upstream server")
		}
		return nil

	default:
		return fmt.Errorf("unknown action: %s (valid: BLOCK, ALLOW, REDIRECT, FORWARD)", action)
	}
}

// ParseUpstreams parses a comma-separated list of upstream DNS servers
// Format: "host:port,host:port" or just "host:port"
// Adds default port :53 if not specified
func ParseUpstreams(actionData string) ([]string, error) {
	if actionData == "" {
		return nil, fmt.Errorf("empty upstream list")
	}

	parts := strings.Split(actionData, ",")
	upstreams := make([]string, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Validate format: must be host:port or just host
		if strings.Contains(part, ":") {
			// Already has port, validate it
			host, port, err := net.SplitHostPort(part)
			if err != nil {
				return nil, fmt.Errorf("invalid upstream format '%s': %w", part, err)
			}
			if host == "" {
				return nil, fmt.Errorf("invalid upstream '%s': empty host", part)
			}
			if port == "" {
				return nil, fmt.Errorf("invalid upstream '%s': empty port", part)
			}
			upstreams = append(upstreams, part)
		} else {
			// No port specified, add default :53
			upstreams = append(upstreams, part+":53")
		}
	}

	return upstreams, nil
}

// GetUpstreams returns the parsed upstream list from a FORWARD rule's action_data
// Returns nil if the rule is not a FORWARD action or parsing fails
func (r *Rule) GetUpstreams() []string {
	if r.Action != ActionForward {
		return nil
	}

	upstreams, err := ParseUpstreams(r.ActionData)
	if err != nil {
		return nil
	}

	return upstreams
}
