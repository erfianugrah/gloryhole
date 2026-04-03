package dns

import (
	"net"
	"sync"
)

// ClientACL enforces an IP/CIDR allowlist for plain DNS queries (UDP/TCP).
// When the allowlist is empty, all clients are permitted.
// DoT and DoH bypass this check — they have their own auth layers.
type ClientACL struct {
	mu      sync.RWMutex
	nets    []*net.IPNet
	singles []net.IP // single-IP entries (no CIDR mask)
	empty   bool     // true = open to all
}

// NewClientACL builds an ACL from a list of IP addresses and CIDR ranges.
// Entries can be "192.168.1.0/24", "10.0.0.1", "::1", "fd00::/8", etc.
// An empty list means all clients are allowed (open resolver).
func NewClientACL(entries []string) *ClientACL {
	acl := &ClientACL{}
	if len(entries) == 0 {
		acl.empty = true
		return acl
	}

	for _, entry := range entries {
		// Try CIDR first
		_, ipNet, err := net.ParseCIDR(entry)
		if err == nil {
			acl.nets = append(acl.nets, ipNet)
			continue
		}
		// Try bare IP
		ip := net.ParseIP(entry)
		if ip != nil {
			acl.singles = append(acl.singles, ip)
			continue
		}
		// Skip unparseable entries (will be caught by validation)
	}

	acl.empty = len(acl.nets) == 0 && len(acl.singles) == 0
	return acl
}

// IsAllowed checks whether a client IP is in the allowlist.
// Returns true if the ACL is empty (open) or the IP matches an entry.
func (a *ClientACL) IsAllowed(clientIP string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.empty {
		return true
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	for _, ipNet := range a.nets {
		if ipNet.Contains(ip) {
			return true
		}
	}
	for _, allowed := range a.singles {
		if allowed.Equal(ip) {
			return true
		}
	}
	return false
}

// Update replaces the ACL entries atomically.
func (a *ClientACL) Update(entries []string) {
	newACL := NewClientACL(entries)

	a.mu.Lock()
	a.nets = newACL.nets
	a.singles = newACL.singles
	a.empty = newACL.empty
	a.mu.Unlock()
}

// IsOpen returns true when the ACL has no entries (all clients allowed).
func (a *ClientACL) IsOpen() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.empty
}
