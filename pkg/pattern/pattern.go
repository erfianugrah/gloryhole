// Package pattern provides domain pattern matching for Glory-Hole.
// It supports three types of patterns:
//   - Exact: example.com
//   - Wildcard: *.example.com
//   - Regex: (\.|^)example\.com$
package pattern

import (
	"fmt"
	"regexp"
	"strings"
)

// PatternType represents the type of domain pattern.
type PatternType int

const (
	// PatternTypeExact matches exact domain names (e.g., example.com)
	PatternTypeExact PatternType = iota
	// PatternTypeWildcard matches wildcard patterns (e.g., *.example.com)
	PatternTypeWildcard
	// PatternTypeRegex matches regex patterns (e.g., (\.|^)example\.com$)
	PatternTypeRegex
)

// String returns a human-readable name for the pattern type.
func (pt PatternType) String() string {
	switch pt {
	case PatternTypeExact:
		return "exact"
	case PatternTypeWildcard:
		return "wildcard"
	case PatternTypeRegex:
		return "regex"
	default:
		return "unknown"
	}
}

// Pattern represents a domain matching pattern.
type Pattern struct {
	Raw      string         // Original pattern string
	Type     PatternType    // Pattern type
	Compiled *regexp.Regexp // Compiled regex (only for regex patterns)
}

// isRegexPattern detects if a pattern contains regex metacharacters.
func isRegexPattern(pattern string) bool {
	// Check for common regex metacharacters
	regexChars := []string{
		"(", ")", "[", "]", "{", "}",
		"^", "$", "|", "\\",
		"+", "?",
	}

	for _, char := range regexChars {
		if strings.Contains(pattern, char) {
			return true
		}
	}

	// Check for .* or .+ patterns (common regex)
	if strings.Contains(pattern, ".*") || strings.Contains(pattern, ".+") {
		return true
	}

	return false
}

// ParsePattern parses a pattern string and determines its type.
// It automatically detects whether the pattern is exact, wildcard, or regex.
func ParsePattern(pattern string) (*Pattern, error) {
	if pattern == "" {
		return nil, fmt.Errorf("empty pattern")
	}

	// Detect wildcards (*.example.com)
	if strings.HasPrefix(pattern, "*.") {
		return &Pattern{
			Raw:  pattern,
			Type: PatternTypeWildcard,
		}, nil
	}

	// Detect regex (contains regex metacharacters)
	if isRegexPattern(pattern) {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern %q: %w", pattern, err)
		}
		return &Pattern{
			Raw:      pattern,
			Type:     PatternTypeRegex,
			Compiled: compiled,
		}, nil
	}

	// Default to exact match
	return &Pattern{
		Raw:  pattern,
		Type: PatternTypeExact,
	}, nil
}

// Match checks if a domain matches this pattern.
func (p *Pattern) Match(domain string) bool {
	switch p.Type {
	case PatternTypeExact:
		return domain == p.Raw
	case PatternTypeWildcard:
		// *.example.com matches foo.example.com but not example.com
		suffix := strings.TrimPrefix(p.Raw, "*.")
		return strings.HasSuffix(domain, suffix) && domain != suffix
	case PatternTypeRegex:
		if p.Compiled == nil {
			return false
		}
		return p.Compiled.MatchString(domain)
	}
	return false
}

// String returns a string representation of the pattern.
func (p *Pattern) String() string {
	return fmt.Sprintf("%s(%s)", p.Type, p.Raw)
}

// Matcher provides efficient multi-tier pattern matching.
// It separates patterns by type for optimal performance:
//   - Exact matches use O(1) map lookup
//   - Wildcards use O(n) string operations
//   - Regex uses O(n) compiled regex matching
type Matcher struct {
	exact    map[string]struct{} // O(1) lookup
	wildcard []*Pattern          // O(n) but fast string ops
	regex    []*Pattern          // O(n) regex matching
}

// NewMatcher creates a new Matcher from a list of pattern strings.
func NewMatcher(patterns []string) (*Matcher, error) {
	m := &Matcher{
		exact:    make(map[string]struct{}),
		wildcard: make([]*Pattern, 0),
		regex:    make([]*Pattern, 0),
	}

	for _, patternStr := range patterns {
		pattern, err := ParsePattern(patternStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pattern %q: %w", patternStr, err)
		}

		switch pattern.Type {
		case PatternTypeExact:
			m.exact[pattern.Raw] = struct{}{}
		case PatternTypeWildcard:
			m.wildcard = append(m.wildcard, pattern)
		case PatternTypeRegex:
			m.regex = append(m.regex, pattern)
		}
	}

	return m, nil
}

// Match checks if a domain matches any pattern in this matcher.
// It uses a multi-tier strategy for optimal performance:
//  1. Try exact match first (fastest - O(1))
//  2. Try wildcard matches (fast - O(n) string ops)
//  3. Try regex matches last (slower - O(n) regex ops)
func (m *Matcher) Match(domain string) bool {
	// Try exact first (fastest)
	if _, ok := m.exact[domain]; ok {
		return true
	}

	// Try wildcards (fast)
	for _, pattern := range m.wildcard {
		if pattern.Match(domain) {
			return true
		}
	}

	// Try regex last (slowest)
	for _, pattern := range m.regex {
		if pattern.Match(domain) {
			return true
		}
	}

	return false
}

// Stats returns statistics about the patterns in this matcher.
func (m *Matcher) Stats() map[string]int {
	return map[string]int{
		"exact":    len(m.exact),
		"wildcard": len(m.wildcard),
		"regex":    len(m.regex),
		"total":    len(m.exact) + len(m.wildcard) + len(m.regex),
	}
}
