// Package policy implements the expression-based DNS policy engine used to
// block, allow, or redirect queries.
package policy

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"glory-hole/pkg/logging"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// asString safely converts a parameter to string, returning an error instead of panicking.
func asString(p any, name string) (string, error) {
	s, ok := p.(string)
	if !ok {
		return "", fmt.Errorf("%s: expected string, got %T", name, p)
	}
	return s, nil
}

// asInt safely converts a parameter to int.
func asInt(p any, name string) (int, error) {
	i, ok := p.(int)
	if !ok {
		return 0, fmt.Errorf("%s: expected int, got %T", name, p)
	}
	return i, nil
}

// Engine is the policy engine that evaluates filtering rules
type Engine struct {
	rules  []*Rule
	logger *logging.Logger
	mu     sync.RWMutex
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
	ActionBlock     = "BLOCK"
	ActionAllow     = "ALLOW"
	ActionRedirect  = "REDIRECT"
	ActionForward   = "FORWARD"
	ActionRateLimit = "RATE_LIMIT"
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
func NewEngine(logger *logging.Logger) *Engine {
	return &Engine{
		rules:  make([]*Rule, 0),
		logger: logger,
	}
}

// HasAction reports whether any enabled rule uses the given action.
func (e *Engine) HasAction(action string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, r := range e.rules {
		if strings.EqualFold(r.Action, action) {
			return true
		}
	}
	return false
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

	// Compile the expression with environment and safe helper function wrappers.
	// All type assertions use asString/asInt to return errors instead of panicking.
	program, err := expr.Compile(rule.Logic,
		expr.Env(Context{}),
		// Domain matching functions
		expr.Function("DomainMatches",
			func(params ...any) (any, error) {
				a, e := asString(params[0], "DomainMatches.domain")
				if e != nil {
					return false, e
				}
				b, e := asString(params[1], "DomainMatches.pattern")
				if e != nil {
					return false, e
				}
				return DomainMatches(a, b), nil
			},
			new(func(string, string) bool),
		),
		expr.Function("DomainEndsWith",
			func(params ...any) (any, error) {
				a, e := asString(params[0], "DomainEndsWith.domain")
				if e != nil {
					return false, e
				}
				b, e := asString(params[1], "DomainEndsWith.suffix")
				if e != nil {
					return false, e
				}
				return DomainEndsWith(a, b), nil
			},
			new(func(string, string) bool),
		),
		expr.Function("DomainStartsWith",
			func(params ...any) (any, error) {
				a, e := asString(params[0], "DomainStartsWith.domain")
				if e != nil {
					return false, e
				}
				b, e := asString(params[1], "DomainStartsWith.prefix")
				if e != nil {
					return false, e
				}
				return DomainStartsWith(a, b), nil
			},
			new(func(string, string) bool),
		),
		expr.Function("DomainRegex",
			func(params ...any) (any, error) {
				a, e := asString(params[0], "DomainRegex.domain")
				if e != nil {
					return false, e
				}
				b, e := asString(params[1], "DomainRegex.pattern")
				if e != nil {
					return false, e
				}
				return DomainRegex(a, b)
			},
			new(func(string, string) bool),
		),
		expr.Function("DomainLevelCount",
			func(params ...any) (any, error) {
				a, e := asString(params[0], "DomainLevelCount.domain")
				if e != nil {
					return 0, e
				}
				return DomainLevelCount(a), nil
			},
			new(func(string) int),
		),
		// IP matching functions
		expr.Function("IPInCIDR",
			func(params ...any) (any, error) {
				a, e := asString(params[0], "IPInCIDR.ip")
				if e != nil {
					return false, e
				}
				b, e := asString(params[1], "IPInCIDR.cidr")
				if e != nil {
					return false, e
				}
				return IPInCIDR(a, b), nil
			},
			new(func(string, string) bool),
		),
		expr.Function("IPEquals",
			func(params ...any) (any, error) {
				a, e := asString(params[0], "IPEquals.ip")
				if e != nil {
					return false, e
				}
				b, e := asString(params[1], "IPEquals.target")
				if e != nil {
					return false, e
				}
				return IPEquals(a, b), nil
			},
			new(func(string, string) bool),
		),
		// Query type functions
		expr.Function("QueryTypeIn",
			func(params ...any) (any, error) {
				qt, e := asString(params[0], "QueryTypeIn.queryType")
				if e != nil {
					return false, e
				}
				types := make([]string, len(params)-1)
				for i := 1; i < len(params); i++ {
					t, tErr := asString(params[i], fmt.Sprintf("QueryTypeIn.type[%d]", i-1))
					if tErr != nil {
						return false, tErr
					}
					types[i-1] = t
				}
				return QueryTypeIn(qt, types...), nil
			},
		),
		// Time functions
		expr.Function("IsWeekend",
			func(params ...any) (any, error) {
				a, e := asInt(params[0], "IsWeekend.dayOfWeek")
				if e != nil {
					return false, e
				}
				return IsWeekend(a), nil
			},
			new(func(int) bool),
		),
		expr.Function("InTimeRange",
			func(params ...any) (any, error) {
				h, e := asInt(params[0], "InTimeRange.hour")
				if e != nil {
					return false, e
				}
				m, e := asInt(params[1], "InTimeRange.minute")
				if e != nil {
					return false, e
				}
				sh, e := asInt(params[2], "InTimeRange.startHour")
				if e != nil {
					return false, e
				}
				sm, e := asInt(params[3], "InTimeRange.startMin")
				if e != nil {
					return false, e
				}
				eh, e := asInt(params[4], "InTimeRange.endHour")
				if e != nil {
					return false, e
				}
				em, e := asInt(params[5], "InTimeRange.endMin")
				if e != nil {
					return false, e
				}
				return InTimeRange(h, m, sh, sm, eh, em), nil
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

// UpdateRule updates a rule at the given index, preserving evaluation order
// Returns an error if the index is out of bounds or the rule fails validation
func (e *Engine) UpdateRule(index int, rule *Rule) error {
	if rule == nil {
		return fmt.Errorf("rule cannot be nil")
	}

	// Validate action and action_data
	if err := validateAction(rule); err != nil {
		return fmt.Errorf("invalid rule '%s': %w", rule.Name, err)
	}

	// Compile the expression (same compilation logic as AddRule)
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

	if index < 0 || index >= len(e.rules) {
		return fmt.Errorf("index %d out of bounds (have %d rules)", index, len(e.rules))
	}

	e.rules[index] = rule
	return nil
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

// Stop terminates all background goroutines
func (e *Engine) Stop() {
	// No-op: no background goroutines in policy engine anymore
}

// Helper functions that can be used in expressions

// DomainMatches checks if a domain matches a pattern using domain-boundary
// aware matching (case-insensitive). Unlike a raw substring check, the pattern
// must align on dot boundaries to prevent false positives like "ad" matching
// "readme.io".
//
// Matching rules:
//   - Exact match: "facebook.com" matches "facebook.com"
//   - Subdomain: "api.facebook.com" matches "facebook.com" (dot-boundary suffix)
//   - Label prefix: "facebook.com" matches "facebook" (pattern is a label prefix at a dot boundary)
//   - Leading dot: ".facebook.com" matches "www.facebook.com" and "facebook.com"
func DomainMatches(domain, pattern string) bool {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	pattern = strings.ToLower(strings.TrimSuffix(pattern, "."))

	if domain == pattern {
		return true
	}

	// Leading-dot pattern: ".facebook.com" matches subdomains and the apex
	if strings.HasPrefix(pattern, ".") {
		suffix := pattern[1:]
		return domain == suffix || strings.HasSuffix(domain, pattern)
	}

	// Pattern is a full domain: "facebook.com" matches "api.facebook.com"
	if strings.HasSuffix(domain, "."+pattern) {
		return true
	}

	// Pattern is a label (no dots): "facebook" matches "facebook.com" or "api.facebook.com"
	// but NOT "myfacebook.com" — must be on a dot boundary
	if !strings.Contains(pattern, ".") {
		// Match if domain starts with pattern. or has .pattern. inside
		if domain == pattern {
			return true
		}
		if strings.HasPrefix(domain, pattern+".") {
			return true
		}
		if strings.Contains(domain, "."+pattern+".") {
			return true
		}
		// Match if domain has .pattern at the end (label is the SLD)
		if strings.HasSuffix(domain, "."+pattern) {
			return true
		}
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

// regexCache stores compiled regular expressions to avoid recompilation on every call.
// Using sync.Map for concurrent read/write safety with minimal contention.
var regexCache sync.Map

// DomainRegex checks if domain matches a regular expression pattern.
// Compiled regexes are cached for performance - regex compilation is expensive
// and patterns are typically reused across many queries.
func DomainRegex(domain, pattern string) (bool, error) {
	// Try to get cached compiled regex
	cached, ok := regexCache.Load(pattern)
	if !ok {
		// Compile and cache the regex
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("invalid regex pattern: %w", err)
		}
		// Store returns the existing value if another goroutine stored first
		actual, _ := regexCache.LoadOrStore(pattern, re)
		cached = actual
	}

	re := cached.(*regexp.Regexp)
	return re.MatchString(strings.ToLower(domain)), nil
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

	case ActionRateLimit:
		// RATE_LIMIT was never implemented and has no effect.
		// Reject it at validation time so users don't rely on non-functional policy.
		return fmt.Errorf("RATE_LIMIT action is not supported — use HTTP rate limiting instead")

	default:
		return fmt.Errorf("unknown action: %s (valid: BLOCK, ALLOW, REDIRECT, FORWARD)", action)
	}
}

// validateRateLimitData validates the format of rate limit action_data
func validateRateLimitData(actionData string) error {
	parts := strings.Split(actionData, ",")
	if len(parts) != 2 {
		return fmt.Errorf("format must be 'req_per_sec,burst', got: %s", actionData)
	}
	rps, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil || rps <= 0 {
		return fmt.Errorf("invalid requests_per_second: %s", parts[0])
	}
	burst, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || burst <= 0 {
		return fmt.Errorf("invalid burst: %s", parts[1])
	}
	return nil
}

// parseRateLimitData parses rate limit action_data into requests per second and burst
func parseRateLimitData(actionData string) (float64, int) {
	parts := strings.Split(actionData, ",")
	rps, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	burst, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return rps, burst
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
