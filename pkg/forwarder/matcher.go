package forwarder

import (
	"net"
	"regexp"
	"strings"
)

// DomainMatcher efficiently matches domain names against patterns
type DomainMatcher struct {
	exact    map[string]struct{} // Exact domain matches (O(1) lookup)
	suffixes []string            // Wildcard suffixes like "*.local"
	prefixes []string            // Wildcard prefixes like "internal.*"
	regexes  []*regexp.Regexp    // Regex patterns (slowest, optional)
}

// NewDomainMatcher creates a new domain matcher from a list of patterns
func NewDomainMatcher(patterns []string) (*DomainMatcher, error) {
	matcher := &DomainMatcher{
		exact:    make(map[string]struct{}),
		suffixes: make([]string, 0),
		prefixes: make([]string, 0),
		regexes:  make([]*regexp.Regexp, 0),
	}

	for _, pattern := range patterns {
		if err := matcher.AddPattern(pattern); err != nil {
			return nil, err
		}
	}

	return matcher, nil
}

// AddPattern adds a domain pattern to the matcher
// Supports:
//   - Exact: "nas.local" → matches only "nas.local"
//   - Wildcard suffix: "*.local" → matches "nas.local", "router.local", etc.
//   - Wildcard prefix: "internal.*" → matches "internal.corp", "internal.net", etc.
//   - Regex: "/^[a-z]+\.local$/" → advanced pattern matching
func (dm *DomainMatcher) AddPattern(pattern string) error {
	if pattern == "" {
		return nil // Skip empty patterns
	}

	// Normalize: lowercase and trim trailing dot
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	pattern = strings.TrimSuffix(pattern, ".")

	// Check if it's a regex pattern (starts with / and ends with /)
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
		// Extract regex (remove / delimiters)
		regexStr := pattern[1 : len(pattern)-1]
		re, err := regexp.Compile(regexStr)
		if err != nil {
			return err
		}
		dm.regexes = append(dm.regexes, re)
		return nil
	}

	// Check for wildcard suffix (*.local)
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[2:] // Remove "*."
		if suffix != "" {
			dm.suffixes = append(dm.suffixes, "."+suffix) // Store as ".local" for HasSuffix
		}
		return nil
	}

	// Check for wildcard prefix (internal.*)
	if strings.HasSuffix(pattern, ".*") {
		prefix := pattern[:len(pattern)-2] // Remove ".*"
		if prefix != "" {
			dm.prefixes = append(dm.prefixes, prefix+".") // Store as "internal." for HasPrefix
		}
		return nil
	}

	// Exact match
	dm.exact[pattern] = struct{}{}
	return nil
}

// Matches checks if a domain matches any of the patterns
// Returns true if the domain matches any pattern
func (dm *DomainMatcher) Matches(domain string) bool {
	// Normalize domain: lowercase and remove trailing dot
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimSuffix(domain, ".")

	// 1. Check exact match (O(1), fastest)
	if _, ok := dm.exact[domain]; ok {
		return true
	}

	// 2. Check suffix wildcards (O(n), fast for small n)
	for _, suffix := range dm.suffixes {
		if strings.HasSuffix(domain, suffix) || domain == suffix[1:] {
			// Match *.local against both "nas.local" and "local"
			return true
		}
	}

	// 3. Check prefix wildcards (O(n))
	for _, prefix := range dm.prefixes {
		if strings.HasPrefix(domain, prefix) {
			return true
		}
	}

	// 4. Check regex patterns (O(m), slowest)
	for _, re := range dm.regexes {
		if re.MatchString(domain) {
			return true
		}
	}

	return false
}

// IsEmpty returns true if the matcher has no patterns
func (dm *DomainMatcher) IsEmpty() bool {
	return len(dm.exact) == 0 && len(dm.suffixes) == 0 && len(dm.prefixes) == 0 && len(dm.regexes) == 0
}

// Count returns the total number of patterns
func (dm *DomainMatcher) Count() int {
	return len(dm.exact) + len(dm.suffixes) + len(dm.prefixes) + len(dm.regexes)
}

// CIDRMatcher efficiently matches IP addresses against CIDR ranges
type CIDRMatcher struct {
	networks []*net.IPNet
}

// NewCIDRMatcher creates a new CIDR matcher from a list of CIDR strings
func NewCIDRMatcher(cidrs []string) (*CIDRMatcher, error) {
	matcher := &CIDRMatcher{
		networks: make([]*net.IPNet, 0, len(cidrs)),
	}

	for _, cidr := range cidrs {
		if err := matcher.AddCIDR(cidr); err != nil {
			return nil, err
		}
	}

	return matcher, nil
}

// AddCIDR adds a CIDR range to the matcher
func (cm *CIDRMatcher) AddCIDR(cidr string) error {
	if cidr == "" {
		return nil // Skip empty CIDRs
	}

	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return err
	}

	cm.networks = append(cm.networks, ipNet)
	return nil
}

// Matches checks if an IP address is in any of the CIDR ranges
func (cm *CIDRMatcher) Matches(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, network := range cm.networks {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// IsEmpty returns true if the matcher has no networks
func (cm *CIDRMatcher) IsEmpty() bool {
	return len(cm.networks) == 0
}

// Count returns the number of CIDR ranges
func (cm *CIDRMatcher) Count() int {
	return len(cm.networks)
}

// QueryTypeMatcher matches DNS query types
type QueryTypeMatcher struct {
	types map[string]struct{}
}

// NewQueryTypeMatcher creates a new query type matcher
func NewQueryTypeMatcher(types []string) *QueryTypeMatcher {
	matcher := &QueryTypeMatcher{
		types: make(map[string]struct{}),
	}

	for _, qtype := range types {
		qtype = strings.ToUpper(strings.TrimSpace(qtype))
		if qtype != "" {
			matcher.types[qtype] = struct{}{}
		}
	}

	return matcher
}

// Matches checks if a query type matches
func (qm *QueryTypeMatcher) Matches(queryType string) bool {
	queryType = strings.ToUpper(strings.TrimSpace(queryType))
	_, ok := qm.types[queryType]
	return ok
}

// IsEmpty returns true if the matcher has no types
func (qm *QueryTypeMatcher) IsEmpty() bool {
	return len(qm.types) == 0
}

// Count returns the number of query types
func (qm *QueryTypeMatcher) Count() int {
	return len(qm.types)
}
