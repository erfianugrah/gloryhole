package localrecords

import (
	"fmt"
	"net"
	"sync"
)

// RecordType represents the type of DNS record
type RecordType string

const (
	RecordTypeA     RecordType = "A"
	RecordTypeAAAA  RecordType = "AAAA"
	RecordTypeCNAME RecordType = "CNAME"
	RecordTypeMX    RecordType = "MX"
	RecordTypeTXT   RecordType = "TXT"
	RecordTypeSRV   RecordType = "SRV"
	RecordTypePTR   RecordType = "PTR"
	RecordTypeNS    RecordType = "NS"
	RecordTypeSOA   RecordType = "SOA"
)

// LocalRecord represents a single local DNS record
type LocalRecord struct {
	Domain   string
	Type     RecordType
	Target   string
	IPs      []net.IP
	TTL      uint32
	Priority uint16
	Weight   uint16
	Port     uint16
	Wildcard bool
	Enabled  bool

	// TXT record data (multiple strings per record)
	TxtRecords []string

	// SOA record data (Start of Authority)
	Ns      string // SOA: Primary nameserver
	Mbox    string // SOA: Responsible person email
	Serial  uint32 // SOA: Zone serial number
	Refresh uint32 // SOA: Refresh interval (seconds)
	Retry   uint32 // SOA: Retry interval (seconds)
	Expire  uint32 // SOA: Expiration time (seconds)
	Minttl  uint32 // SOA: Minimum TTL (seconds)
}

// Manager manages local DNS records with efficient lookups
type Manager struct {
	// Exact domain matches (normalized to lowercase, FQDN with trailing dot)
	records map[string][]*LocalRecord // domain → list of records

	// Wildcard records (e.g., *.local, *.dev.home)
	wildcards []*LocalRecord

	mu sync.RWMutex
}

// NewManager creates a new local records manager
func NewManager() *Manager {
	return &Manager{
		records:   make(map[string][]*LocalRecord),
		wildcards: make([]*LocalRecord, 0),
	}
}

// AddRecord adds a local DNS record
// Domain names are normalized to lowercase FQDN with trailing dot
func (m *Manager) AddRecord(record *LocalRecord) error {
	if record == nil {
		return ErrInvalidRecord
	}

	// Validate record
	if err := validateRecord(record); err != nil {
		return err
	}

	// Normalize domain name (lowercase, ensure trailing dot)
	record.Domain = normalizeDomain(record.Domain)

	m.mu.Lock()
	defer m.mu.Unlock()

	if record.Wildcard {
		// Add to wildcard list
		m.wildcards = append(m.wildcards, record)
	} else {
		// Add to exact match map
		m.records[record.Domain] = append(m.records[record.Domain], record)
	}

	return nil
}

// RemoveRecord removes a local DNS record
func (m *Manager) RemoveRecord(domain string, recordType RecordType) error {
	domain = normalizeDomain(domain)

	m.mu.Lock()
	defer m.mu.Unlock()

	records, exists := m.records[domain]
	if !exists {
		return ErrRecordNotFound
	}

	// Filter out the record type
	filtered := make([]*LocalRecord, 0, len(records))
	found := false
	for _, r := range records {
		if r.Type != recordType {
			filtered = append(filtered, r)
		} else {
			found = true
		}
	}

	if !found {
		return ErrRecordNotFound
	}

	if len(filtered) == 0 {
		delete(m.records, domain)
	} else {
		m.records[domain] = filtered
	}

	return nil
}

// LookupA looks up A records for a domain
// Returns IPs and TTL, or nil if not found
func (m *Manager) LookupA(domain string) ([]net.IP, uint32, bool) {
	domain = normalizeDomain(domain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check exact matches first
	if records, exists := m.records[domain]; exists {
		return extractIPs(records, RecordTypeA)
	}

	// Check wildcard matches
	return m.matchWildcard(domain, RecordTypeA)
}

// LookupAAAA looks up AAAA records for a domain
func (m *Manager) LookupAAAA(domain string) ([]net.IP, uint32, bool) {
	domain = normalizeDomain(domain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check exact matches first
	if records, exists := m.records[domain]; exists {
		return extractIPs(records, RecordTypeAAAA)
	}

	// Check wildcard matches
	return m.matchWildcard(domain, RecordTypeAAAA)
}

// LookupCNAME looks up CNAME record for a domain
// Returns target domain and TTL, or empty string if not found
func (m *Manager) LookupCNAME(domain string) (string, uint32, bool) {
	domain = normalizeDomain(domain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check exact matches first
	if records, exists := m.records[domain]; exists {
		for _, r := range records {
			if r.Type == RecordTypeCNAME && r.Enabled {
				return r.Target, r.TTL, true
			}
		}
	}

	// Check wildcard matches
	for _, wc := range m.wildcards {
		if wc.Type == RecordTypeCNAME && wc.Enabled && matchesWildcard(domain, wc.Domain) {
			return wc.Target, wc.TTL, true
		}
	}

	return "", 0, false
}

// LookupTXT looks up TXT records for a domain
// Returns list of records with TXT data, or empty if not found
func (m *Manager) LookupTXT(domain string) []*LocalRecord {
	domain = normalizeDomain(domain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*LocalRecord, 0)

	// Check exact matches first
	if records, exists := m.records[domain]; exists {
		for _, r := range records {
			if r.Type == RecordTypeTXT && r.Enabled {
				result = append(result, r)
			}
		}
	}

	// Check wildcard matches
	for _, wc := range m.wildcards {
		if wc.Type == RecordTypeTXT && wc.Enabled && matchesWildcard(domain, wc.Domain) {
			result = append(result, wc)
		}
	}

	return result
}

// LookupMX looks up MX records for a domain
// Returns list of records sorted by priority (lower priority = higher preference)
func (m *Manager) LookupMX(domain string) []*LocalRecord {
	domain = normalizeDomain(domain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*LocalRecord, 0)

	// Check exact matches first
	if records, exists := m.records[domain]; exists {
		for _, r := range records {
			if r.Type == RecordTypeMX && r.Enabled {
				result = append(result, r)
			}
		}
	}

	// Check wildcard matches
	for _, wc := range m.wildcards {
		if wc.Type == RecordTypeMX && wc.Enabled && matchesWildcard(domain, wc.Domain) {
			result = append(result, wc)
		}
	}

	// Sort by priority (lower priority number = higher preference per RFC 5321)
	// If priorities are equal, maintain insertion order
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Priority > result[j].Priority {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// LookupPTR looks up PTR records for a domain (reverse DNS)
// Returns list of records with pointer targets
func (m *Manager) LookupPTR(domain string) []*LocalRecord {
	domain = normalizeDomain(domain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*LocalRecord, 0)

	// Check exact matches first
	if records, exists := m.records[domain]; exists {
		for _, r := range records {
			if r.Type == RecordTypePTR && r.Enabled {
				result = append(result, r)
			}
		}
	}

	// Check wildcard matches
	for _, wc := range m.wildcards {
		if wc.Type == RecordTypePTR && wc.Enabled && matchesWildcard(domain, wc.Domain) {
			result = append(result, wc)
		}
	}

	return result
}

// LookupSRV looks up SRV records for a domain (service discovery)
// Returns list of records sorted by priority, then weight (RFC 2782)
func (m *Manager) LookupSRV(domain string) []*LocalRecord {
	domain = normalizeDomain(domain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*LocalRecord, 0)

	// Check exact matches first
	if records, exists := m.records[domain]; exists {
		for _, r := range records {
			if r.Type == RecordTypeSRV && r.Enabled {
				result = append(result, r)
			}
		}
	}

	// Check wildcard matches
	for _, wc := range m.wildcards {
		if wc.Type == RecordTypeSRV && wc.Enabled && matchesWildcard(domain, wc.Domain) {
			result = append(result, wc)
		}
	}

	// Sort by priority (lower first), then by weight (higher first) per RFC 2782
	// Priority ordering: servers with lower priority values are contacted first
	// Weight ordering: within same priority, higher weight = more likely to be selected
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			// Primary sort: priority (ascending)
			if result[i].Priority > result[j].Priority {
				result[i], result[j] = result[j], result[i]
			} else if result[i].Priority == result[j].Priority {
				// Secondary sort: weight (descending)
				if result[i].Weight < result[j].Weight {
					result[i], result[j] = result[j], result[i]
				}
			}
		}
	}

	return result
}

// LookupNS looks up NS records for a domain (nameserver records)
// Returns list of records with nameserver targets
func (m *Manager) LookupNS(domain string) []*LocalRecord {
	domain = normalizeDomain(domain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*LocalRecord, 0)

	// Check exact matches first
	if records, exists := m.records[domain]; exists {
		for _, r := range records {
			if r.Type == RecordTypeNS && r.Enabled {
				result = append(result, r)
			}
		}
	}

	// Check wildcard matches
	for _, wc := range m.wildcards {
		if wc.Type == RecordTypeNS && wc.Enabled && matchesWildcard(domain, wc.Domain) {
			result = append(result, wc)
		}
	}

	return result
}

// LookupSOA looks up SOA record for a domain (Start of Authority)
// Returns list of records (typically only one SOA per zone)
func (m *Manager) LookupSOA(domain string) []*LocalRecord {
	domain = normalizeDomain(domain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*LocalRecord, 0)

	// Check exact matches first
	if records, exists := m.records[domain]; exists {
		for _, r := range records {
			if r.Type == RecordTypeSOA && r.Enabled {
				result = append(result, r)
			}
		}
	}

	// Check wildcard matches
	for _, wc := range m.wildcards {
		if wc.Type == RecordTypeSOA && wc.Enabled && matchesWildcard(domain, wc.Domain) {
			result = append(result, wc)
		}
	}

	return result
}

// ResolveCNAME follows CNAME chains and returns the final A/AAAA records
// Prevents infinite loops with max depth of 10
func (m *Manager) ResolveCNAME(domain string, maxDepth int) ([]net.IP, uint32, bool) {
	if maxDepth <= 0 {
		maxDepth = 10 // Default max CNAME chain depth
	}

	domain = normalizeDomain(domain)
	visited := make(map[string]bool)
	minTTL := uint32(300) // Track minimum TTL in chain

	for depth := 0; depth < maxDepth; depth++ {
		// Prevent infinite loops
		if visited[domain] {
			return nil, 0, false
		}
		visited[domain] = true

		// Check for CNAME
		if target, ttl, found := m.LookupCNAME(domain); found {
			if ttl < minTTL {
				minTTL = ttl
			}
			domain = target
			continue
		}

		// No more CNAMEs, check for A/AAAA records
		if ips, ttl, found := m.LookupA(domain); found {
			if ttl < minTTL {
				minTTL = ttl
			}
			return ips, minTTL, true
		}

		if ips, ttl, found := m.LookupAAAA(domain); found {
			if ttl < minTTL {
				minTTL = ttl
			}
			return ips, minTTL, true
		}

		// Not found
		return nil, 0, false
	}

	// Max depth reached (loop detected)
	return nil, 0, false
}

// HasRecord checks if any record exists for a domain
func (m *Manager) HasRecord(domain string) bool {
	domain = normalizeDomain(domain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, exists := m.records[domain]; exists {
		return true
	}

	// Check wildcards
	for _, wc := range m.wildcards {
		if wc.Enabled && matchesWildcard(domain, wc.Domain) {
			return true
		}
	}

	return false
}

// GetAllRecords returns all records (for debugging/admin)
func (m *Manager) GetAllRecords() []*LocalRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	all := make([]*LocalRecord, 0)

	for _, records := range m.records {
		all = append(all, records...)
	}

	all = append(all, m.wildcards...)

	return all
}

// Count returns the total number of records
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, records := range m.records {
		count += len(records)
	}
	count += len(m.wildcards)

	return count
}

// Clear removes all records
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.records = make(map[string][]*LocalRecord)
	m.wildcards = make([]*LocalRecord, 0)
}

// matchWildcard checks wildcard records for a match
// Must be called with read lock held
func (m *Manager) matchWildcard(domain string, recordType RecordType) ([]net.IP, uint32, bool) {
	for _, wc := range m.wildcards {
		if wc.Type == recordType && wc.Enabled && matchesWildcard(domain, wc.Domain) {
			if recordType == RecordTypeA || recordType == RecordTypeAAAA {
				return wc.IPs, wc.TTL, len(wc.IPs) > 0
			}
		}
	}
	return nil, 0, false
}

// extractIPs extracts IPs from records matching the given type
func extractIPs(records []*LocalRecord, recordType RecordType) ([]net.IP, uint32, bool) {
	ips := make([]net.IP, 0)
	var ttl uint32 = 300 // Default TTL

	for _, r := range records {
		if r.Type == recordType && r.Enabled {
			ips = append(ips, r.IPs...)
			if r.TTL > 0 {
				ttl = r.TTL
			}
		}
	}

	return ips, ttl, len(ips) > 0
}

// NewLocalRecord creates a new local record with sensible defaults
func NewLocalRecord(domain string, recordType RecordType) *LocalRecord {
	return &LocalRecord{
		Domain:   normalizeDomain(domain),
		Type:     recordType,
		TTL:      300, // 5 minutes default
		Enabled:  true,
		Wildcard: false,
	}
}

// NewARecord creates a new A record
func NewARecord(domain string, ip net.IP) *LocalRecord {
	r := NewLocalRecord(domain, RecordTypeA)
	r.IPs = []net.IP{ip}
	return r
}

// NewAAAARecord creates a new AAAA record
func NewAAAARecord(domain string, ip net.IP) *LocalRecord {
	r := NewLocalRecord(domain, RecordTypeAAAA)
	r.IPs = []net.IP{ip}
	return r
}

// NewCNAMERecord creates a new CNAME record
func NewCNAMERecord(domain, target string) *LocalRecord {
	r := NewLocalRecord(domain, RecordTypeCNAME)
	r.Target = normalizeDomain(target)
	return r
}

// NewTXTRecord creates a new TXT record
func NewTXTRecord(domain string, txtRecords []string) *LocalRecord {
	r := NewLocalRecord(domain, RecordTypeTXT)
	r.TxtRecords = txtRecords
	return r
}

// NewMXRecord creates a new MX record
func NewMXRecord(domain, target string, priority uint16) *LocalRecord {
	r := NewLocalRecord(domain, RecordTypeMX)
	r.Target = normalizeDomain(target)
	r.Priority = priority
	return r
}

// NewPTRRecord creates a new PTR record
func NewPTRRecord(domain, target string) *LocalRecord {
	r := NewLocalRecord(domain, RecordTypePTR)
	r.Target = normalizeDomain(target)
	return r
}

// NewSRVRecord creates a new SRV record
func NewSRVRecord(domain, target string, priority, weight, port uint16) *LocalRecord {
	r := NewLocalRecord(domain, RecordTypeSRV)
	r.Target = normalizeDomain(target)
	r.Priority = priority
	r.Weight = weight
	r.Port = port
	return r
}

// NewNSRecord creates a new NS record
func NewNSRecord(domain, target string) *LocalRecord {
	r := NewLocalRecord(domain, RecordTypeNS)
	r.Target = normalizeDomain(target)
	return r
}

// NewSOARecord creates a new SOA record
func NewSOARecord(domain, ns, mbox string, serial, refresh, retry, expire, minttl uint32) *LocalRecord {
	r := NewLocalRecord(domain, RecordTypeSOA)
	r.Ns = normalizeDomain(ns)
	r.Mbox = mbox // Email format, not normalized as domain
	r.Serial = serial
	r.Refresh = refresh
	r.Retry = retry
	r.Expire = expire
	r.Minttl = minttl
	return r
}

// Clone creates a deep copy of the record
func (r *LocalRecord) Clone() *LocalRecord {
	clone := &LocalRecord{
		Domain:   r.Domain,
		Type:     r.Type,
		TTL:      r.TTL,
		Priority: r.Priority,
		Weight:   r.Weight,
		Port:     r.Port,
		Target:   r.Target,
		Wildcard: r.Wildcard,
		Enabled:  r.Enabled,
		Ns:       r.Ns,
		Mbox:     r.Mbox,
		Serial:   r.Serial,
		Refresh:  r.Refresh,
		Retry:    r.Retry,
		Expire:   r.Expire,
		Minttl:   r.Minttl,
	}

	if len(r.IPs) > 0 {
		clone.IPs = make([]net.IP, len(r.IPs))
		copy(clone.IPs, r.IPs)
	}

	if len(r.TxtRecords) > 0 {
		clone.TxtRecords = make([]string, len(r.TxtRecords))
		copy(clone.TxtRecords, r.TxtRecords)
	}

	return clone
}

// String returns a human-readable representation of the record
func (r *LocalRecord) String() string {
	switch r.Type {
	case RecordTypeA, RecordTypeAAAA:
		return fmt.Sprintf("%s %d IN %s %v", r.Domain, r.TTL, r.Type, r.IPs)
	case RecordTypeCNAME:
		return fmt.Sprintf("%s %d IN CNAME %s", r.Domain, r.TTL, r.Target)
	default:
		return fmt.Sprintf("%s %d IN %s", r.Domain, r.TTL, r.Type)
	}
}
