package localrecords

import (
	"net"
	"strings"
)

// normalizeDomain normalizes a domain name to lowercase FQDN with trailing dot
func normalizeDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))

	// Ensure trailing dot for FQDN
	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}

	return domain
}

// matchesWildcard checks if a domain matches a wildcard pattern
// Example: "server.local." matches "*.local."
// The wildcard matches exactly one label (not multiple levels)
func matchesWildcard(domain, pattern string) bool {
	// Both should already be normalized (lowercase, trailing dot)

	// Simple wildcard: *.domain.local. matches anything.domain.local.
	// but NOT sub.anything.domain.local. (only one label)
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[2:] // Remove "*."

		// Domain must end with the suffix and not be the suffix itself
		if !strings.HasSuffix(domain, suffix) || domain == suffix {
			return false
		}

		// Extract the prefix before the suffix
		prefix := domain[:len(domain)-len(suffix)]

		// The prefix should be exactly one label (no dots)
		// Remove trailing dot from prefix if it exists
		prefix = strings.TrimSuffix(prefix, ".")

		// If the prefix contains a dot, it has multiple labels (no match)
		return !strings.Contains(prefix, ".")
	}

	// Exact match (shouldn't be called for wildcards, but handle anyway)
	return domain == pattern
}

// validateRecord validates a local DNS record
func validateRecord(record *LocalRecord) error {
	if record == nil {
		return ErrInvalidRecord
	}

	// Domain must not be empty
	if record.Domain == "" {
		return ErrInvalidDomain
	}

	// Validate based on record type
	switch record.Type {
	case RecordTypeA:
		if len(record.IPs) == 0 {
			return ErrNoIPs
		}
		// Validate all IPs are IPv4
		for _, ip := range record.IPs {
			if ip.To4() == nil {
				return ErrInvalidIP
			}
		}

	case RecordTypeAAAA:
		if len(record.IPs) == 0 {
			return ErrNoIPs
		}
		// Validate all IPs are IPv6
		for _, ip := range record.IPs {
			if ip.To4() != nil {
				return ErrInvalidIP // IPv4 in AAAA record
			}
		}

	case RecordTypeCNAME:
		if record.Target == "" {
			return ErrEmptyTarget
		}

	case RecordTypeMX:
		if record.Target == "" {
			return ErrEmptyTarget
		}

	case RecordTypeSRV:
		if record.Target == "" {
			return ErrEmptyTarget
		}
		if record.Port == 0 {
			return ErrInvalidRecord // SRV requires port
		}

	case RecordTypeTXT:
		if len(record.TxtRecords) == 0 {
			return ErrNoTxtData
		}
		// Validate each TXT string length (max 255 chars per string per RFC 1035)
		for _, txt := range record.TxtRecords {
			if len(txt) > 255 {
				return ErrTxtTooLong
			}
		}

	case RecordTypePTR:
		if record.Target == "" {
			return ErrEmptyTarget
		}

	case RecordTypeNS:
		if record.Target == "" {
			return ErrEmptyTarget
		}

	case RecordTypeSOA:
		// SOA records must have primary nameserver and responsible person email
		if record.Ns == "" || record.Mbox == "" {
			return ErrInvalidSOA
		}

	case RecordTypeCAA:
		// CAA records must have tag and value
		if record.CaaTag == "" {
			return ErrInvalidCAA
		}
		if record.CaaValue == "" {
			return ErrInvalidCAA
		}
		// Validate tag (must be issue, issuewild, or iodef)
		validTags := map[string]bool{
			"issue":     true,
			"issuewild": true,
			"iodef":     true,
		}
		if !validTags[record.CaaTag] {
			return ErrInvalidCAA
		}

	default:
		return ErrInvalidRecord
	}

	return nil
}

// isValidDomain performs basic domain name validation
func isValidDomain(domain string) bool {
	if domain == "" || len(domain) > 253 {
		return false
	}

	// Domain should not start or end with dot (except trailing dot which is handled by normalization)
	domain = strings.TrimSuffix(domain, ".")
	if strings.HasPrefix(domain, ".") {
		return false
	}

	// Split into labels and validate each
	labels := strings.Split(domain, ".")
	if len(labels) == 0 {
		return false
	}

	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}

		// Label should start with alphanumeric
		if !isAlphanumeric(label[0]) && label[0] != '*' {
			return false
		}

		// Label can contain alphanumeric and hyphens
		for i, c := range label {
			if !isAlphanumeric(byte(c)) && c != '-' && c != '*' {
				return false
			}
			// Hyphen cannot be first or last
			if c == '-' && (i == 0 || i == len(label)-1) {
				return false
			}
		}
	}

	return true
}

// isAlphanumeric checks if a byte is alphanumeric
func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// parseIP parses an IP address string into net.IP
func parseIP(s string) (net.IP, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, ErrInvalidIP
	}
	return ip, nil
}

// parseIPs parses multiple IP addresses
func parseIPs(ips []string) ([]net.IP, error) {
	result := make([]net.IP, 0, len(ips))
	for _, ipStr := range ips {
		ip, err := parseIP(ipStr)
		if err != nil {
			return nil, err
		}
		result = append(result, ip)
	}
	return result, nil
}
