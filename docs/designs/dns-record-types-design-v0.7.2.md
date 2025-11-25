# DNS Record Type Support - Design Document v0.7.2

**Status:** Draft
**Version:** 0.7.2
**Date:** 2025-01-23
**Author:** Design Review

---

## Executive Summary

This document outlines the design and implementation plan for comprehensive DNS record type support in Glory-Hole v0.7.2. The goal is to extend the current implementation (A, AAAA, CNAME only) to support all commonly-used DNS record types, with a focus on:

1. **Correctness** - Proper RFC compliance
2. **Safety** - No breaking changes to existing functionality
3. **Incremental Implementation** - Small, testable changes
4. **Future-proofing** - Extensible design for EDNS0 and DNSSEC

---

## Table of Contents

1. [Current State Analysis](#current-state-analysis)
2. [Scope and Objectives](#scope-and-objectives)
3. [Data Structure Design](#data-structure-design)
4. [Component Changes](#component-changes)
5. [Implementation Phases](#implementation-phases)
6. [Testing Strategy](#testing-strategy)
7. [Migration and Compatibility](#migration-and-compatibility)
8. [Performance Considerations](#performance-considerations)
9. [Security Considerations](#security-considerations)
10. [Future Enhancements](#future-enhancements)

---

## 1. Current State Analysis

### 1.1 What Works Today

**Supported Record Types:**
- ✅ A (IPv4 addresses) - Lines 184-206 in `pkg/dns/server.go`
- ✅ AAAA (IPv6 addresses) - Lines 231-253 in `pkg/dns/server.go`
- ✅ CNAME (Canonical names) - Lines 278-294 in `pkg/dns/server.go`

**Existing Infrastructure:**
- ✅ `LocalRecord` struct with fields: Domain, Type, Target, IPs, TTL, Priority, Weight, Port
- ✅ Validation framework in `pkg/localrecords/util.go`
- ✅ Manager with exact match and wildcard support
- ✅ CNAME chain resolution (max depth 10)
- ✅ Test coverage for A, AAAA, CNAME

### 1.2 What's Defined But Not Implemented

**Record Types Defined (constants exist but no handler):**
- ⚠️ MX (Mail exchange) - Validation exists (util.go:91-94), no handler
- ⚠️ TXT (Text records) - Validation exists (util.go:104-107), **uses wrong field**
- ⚠️ SRV (Service records) - Validation exists (util.go:96-102), no handler
- ⚠️ PTR (Pointer/reverse DNS) - Validation exists (util.go:109-112), no handler

### 1.3 Critical Issues Found

#### Issue #1: TXT Record Field Mismatch
**File:** `pkg/localrecords/util.go:104-107`

```go
case RecordTypeTXT:
    if record.Target == "" {
        return ErrEmptyTarget
    }
```

**Problem:** TXT records use `Target` field (single string) but RFC 1035 specifies TXT records as **array of strings** (multiple TXT values per domain).

**Impact:** Cannot store multiple TXT records (e.g., SPF + DKIM + verification tokens).

**Example of correct usage:**
```
example.com.  IN TXT "v=spf1 include:_spf.google.com ~all"
example.com.  IN TXT "google-site-verification=abc123"
```

#### Issue #2: Missing Lookup Methods
**File:** `pkg/localrecords/records.go`

Only 3 lookup methods exist:
- `LookupA()` (line 122)
- `LookupAAAA()` (line 138)
- `LookupCNAME()` (line 155)

Missing:
- `LookupMX()`
- `LookupTXT()`
- `LookupSRV()`
- `LookupPTR()`
- `LookupNS()`
- `LookupSOA()`

#### Issue #3: Config Schema Incomplete
**File:** `pkg/config/config.go:56-63`

```go
type LocalRecordEntry struct {
    Domain   string   `yaml:"domain"`
    Type     string   `yaml:"type"`
    Target   string   `yaml:"target"`
    IPs      []string `yaml:"ips"`
    TTL      uint32   `yaml:"ttl"`
    Wildcard bool     `yaml:"wildcard"`
}
```

**Missing fields:**
- `Priority` (for MX, SRV)
- `Weight` (for SRV)
- `Port` (for SRV)
- `TxtRecords` (for TXT)
- All SOA fields (Ns, Mbox, Serial, Refresh, Retry, Expire, Minttl)
- All CAA fields (Flag, Tag, Value)

#### Issue #4: No DNS Handler Cases
**File:** `pkg/dns/server.go:183-295`

Only 3 `case` statements in the switch for local records:
- `case dns.TypeA:`
- `case dns.TypeAAAA:`
- `case dns.TypeCNAME:`

All other query types fall through and are forwarded upstream, even if local records exist.

---

## 2. Scope and Objectives

### 2.1 In Scope for v0.7.2

#### Priority 1: Essential Common Records (Must Have)
1. **TXT** - Text records (SPF, DKIM, verification tokens)
2. **MX** - Mail exchange records
3. **PTR** - Reverse DNS lookups
4. **SRV** - Service discovery (LDAP, SIP, etc.)

#### Priority 2: Zone Management (Should Have)
5. **NS** - Name server delegation
6. **SOA** - Start of Authority (zone metadata)

#### Priority 3: Modern DNS (Nice to Have)
7. **EDNS0** - Extended DNS (UDP buffer size, DNS cookies)
8. **CAA** - Certificate Authority Authorization

### 2.2 Out of Scope (Future Releases)

- DNSSEC records (DNSKEY, RRSIG, NSEC, DS, etc.) - v0.9.0
- DoH/DoT protocols - v0.9.0
- Advanced records (NAPTR, SSHFP, TLSA, SVCB/HTTPS) - v0.10.0+
- DNS64 - v0.9.0
- Response Policy Zones (RPZ) - v0.9.0

### 2.3 Success Criteria

1. ✅ All Priority 1 records fully functional with tests
2. ✅ No breaking changes to existing A/AAAA/CNAME functionality
3. ✅ Config file backward compatibility maintained
4. ✅ Test coverage ≥70% for new code
5. ✅ Documentation updated with examples
6. ✅ Zero performance regression for existing queries

---

## 3. Data Structure Design

### 3.1 LocalRecord Structure Changes

**File:** `pkg/localrecords/records.go`

#### Current Structure (v0.7.1)
```go
type LocalRecord struct {
    Domain   string      // ✅ All records
    Type     RecordType  // ✅ All records
    Target   string      // ✅ CNAME, MX, PTR, SRV, NS
    IPs      []net.IP    // ✅ A, AAAA
    TTL      uint32      // ✅ All records
    Priority uint16      // ✅ MX, SRV
    Weight   uint16      // ✅ SRV
    Port     uint16      // ✅ SRV
    Wildcard bool        // ✅ All records
    Enabled  bool        // ✅ All records
}
```

#### Proposed Changes (v0.7.2)
```go
type LocalRecord struct {
    // Core fields (unchanged)
    Domain   string      // All records
    Type     RecordType  // All records
    TTL      uint32      // All records (default: 300)
    Wildcard bool        // All records
    Enabled  bool        // All records (default: true)

    // IP-based records (A, AAAA)
    IPs      []net.IP    // A, AAAA records

    // Target-based records (CNAME, MX, PTR, SRV, NS)
    Target   string      // CNAME, MX, PTR, SRV, NS target

    // Priority records (MX, SRV)
    Priority uint16      // MX preference, SRV priority

    // Service discovery (SRV)
    Weight   uint16      // SRV weight
    Port     uint16      // SRV port

    // *** NEW FIELD ***
    // Text records (TXT)
    TxtRecords []string  // TXT record values (multiple strings)

    // *** NEW FIELDS (Phase 2) ***
    // Start of Authority (SOA)
    Ns      string       // SOA: Primary nameserver
    Mbox    string       // SOA: Responsible person email
    Serial  uint32       // SOA: Zone serial number
    Refresh uint32       // SOA: Refresh interval (seconds)
    Retry   uint32       // SOA: Retry interval (seconds)
    Expire  uint32       // SOA: Expiration time (seconds)
    Minttl  uint32       // SOA: Minimum TTL (seconds)

    // *** NEW FIELDS (Phase 3) ***
    // Certificate Authority Authorization (CAA)
    CaaFlag  uint8       // CAA: Flags (usually 0 or 128)
    CaaTag   string      // CAA: Property tag (issue/issuewild/iodef)
    CaaValue string      // CAA: Property value (CA domain or URL)
}
```

**Rationale:**
- ✅ **Backward Compatible** - Existing fields unchanged
- ✅ **Memory Efficient** - New fields only allocated when used (Go zero values)
- ✅ **Type Safe** - Proper types for each field
- ✅ **Extensible** - Easy to add more record types later

**Memory Impact:**
- Current size: ~104 bytes per record
- With new fields: ~168 bytes per record
- For 1000 records: +64KB (negligible)

### 3.2 RecordType Constants

**File:** `pkg/localrecords/records.go:10-20`

```go
const (
    RecordTypeA     RecordType = "A"      // ✅ Implemented
    RecordTypeAAAA  RecordType = "AAAA"   // ✅ Implemented
    RecordTypeCNAME RecordType = "CNAME"  // ✅ Implemented
    RecordTypeMX    RecordType = "MX"     // ⚠️ Defined, not implemented
    RecordTypeTXT   RecordType = "TXT"    // ⚠️ Defined, not implemented
    RecordTypeSRV   RecordType = "SRV"    // ⚠️ Defined, not implemented
    RecordTypePTR   RecordType = "PTR"    // ⚠️ Defined, not implemented

    // *** NEW CONSTANTS ***
    RecordTypeNS    RecordType = "NS"     // ➕ New in v0.7.2
    RecordTypeSOA   RecordType = "SOA"    // ➕ New in v0.7.2
    RecordTypeCAA   RecordType = "CAA"    // ➕ New in v0.7.2 (Phase 3)
)
```

### 3.3 Config Structure Changes

**File:** `pkg/config/config.go:56-63`

#### Current Structure
```go
type LocalRecordEntry struct {
    Domain   string   `yaml:"domain"`
    Type     string   `yaml:"type"`
    Target   string   `yaml:"target"`
    IPs      []string `yaml:"ips"`
    TTL      uint32   `yaml:"ttl"`
    Wildcard bool     `yaml:"wildcard"`
}
```

#### Proposed Structure
```go
type LocalRecordEntry struct {
    // Core fields
    Domain   string   `yaml:"domain"`
    Type     string   `yaml:"type"`
    TTL      uint32   `yaml:"ttl"`
    Wildcard bool     `yaml:"wildcard"`
    Enabled  bool     `yaml:"enabled"` // ➕ New: Allow disabling records

    // Type-specific fields
    IPs      []string `yaml:"ips,omitempty"`       // A, AAAA
    Target   string   `yaml:"target,omitempty"`    // CNAME, MX, PTR, SRV, NS

    // *** NEW FIELDS ***
    TxtRecords []string `yaml:"txt,omitempty"`      // TXT

    Priority   *uint16  `yaml:"priority,omitempty"` // MX, SRV (pointer for omitempty)
    Weight     *uint16  `yaml:"weight,omitempty"`   // SRV
    Port       *uint16  `yaml:"port,omitempty"`     // SRV

    // SOA fields (all optional)
    Ns         string   `yaml:"ns,omitempty"`       // SOA: Primary nameserver
    Mbox       string   `yaml:"mbox,omitempty"`     // SOA: Responsible person
    Serial     *uint32  `yaml:"serial,omitempty"`   // SOA: Zone serial
    Refresh    *uint32  `yaml:"refresh,omitempty"`  // SOA: Refresh interval
    Retry      *uint32  `yaml:"retry,omitempty"`    // SOA: Retry interval
    Expire     *uint32  `yaml:"expire,omitempty"`   // SOA: Expire time
    Minttl     *uint32  `yaml:"minttl,omitempty"`   // SOA: Minimum TTL

    // CAA fields
    CaaFlag    *uint8   `yaml:"caa_flag,omitempty"`  // CAA: Flags
    CaaTag     string   `yaml:"caa_tag,omitempty"`   // CAA: Tag
    CaaValue   string   `yaml:"caa_value,omitempty"` // CAA: Value
}
```

**Notes:**
- Pointers (`*uint16`, `*uint32`) used for optional numeric fields to distinguish between "not set" (nil) and "zero value" (0)
- `omitempty` ensures clean YAML output (no empty fields)

---

## 4. Component Changes

### 4.1 Validation Updates

**File:** `pkg/localrecords/util.go:52-119`

#### Current Validation Issues
1. TXT validation uses wrong field (`Target` instead of `TxtRecords`)
2. No validation for NS, SOA, CAA

#### Proposed Changes

```go
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
        for _, ip := range record.IPs {
            if ip.To4() == nil {
                return ErrInvalidIP
            }
        }

    case RecordTypeAAAA:
        if len(record.IPs) == 0 {
            return ErrNoIPs
        }
        for _, ip := range record.IPs {
            if ip.To4() != nil {
                return ErrInvalidIP
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
        // Priority is optional (default: 10)

    case RecordTypeTXT:
        // *** FIXED: Use TxtRecords instead of Target ***
        if len(record.TxtRecords) == 0 {
            return ErrNoTxtData // New error
        }
        // Validate each TXT string length (max 255 chars per string)
        for _, txt := range record.TxtRecords {
            if len(txt) > 255 {
                return ErrTxtTooLong // New error
            }
        }

    case RecordTypeSRV:
        if record.Target == "" {
            return ErrEmptyTarget
        }
        if record.Port == 0 {
            return ErrInvalidRecord
        }
        // Priority and Weight are optional (default: 0)

    case RecordTypePTR:
        if record.Target == "" {
            return ErrEmptyTarget
        }

    case RecordTypeNS:
        if record.Target == "" {
            return ErrEmptyTarget
        }

    case RecordTypeSOA:
        if record.Ns == "" {
            return ErrInvalidSOA // New error: "SOA record must have primary nameserver"
        }
        if record.Mbox == "" {
            return ErrInvalidSOA // New error: "SOA record must have responsible person"
        }
        // Serial defaults to 1 if not set
        // Other fields have sensible defaults

    case RecordTypeCAA:
        if record.CaaTag == "" {
            return ErrInvalidCAA // New error: "CAA record must have tag"
        }
        // Validate tag is one of: issue, issuewild, iodef
        validTags := map[string]bool{
            "issue":     true,
            "issuewild": true,
            "iodef":     true,
        }
        if !validTags[record.CaaTag] {
            return ErrInvalidCAA
        }
        if record.CaaValue == "" {
            return ErrInvalidCAA // New error: "CAA record must have value"
        }

    default:
        return ErrInvalidRecord
    }

    return nil
}
```

**New Error Constants Needed** (`pkg/localrecords/errors.go`):
```go
var (
    // Existing errors...

    // *** NEW ERRORS ***
    ErrNoTxtData   = errors.New("TXT record must have at least one text string")
    ErrTxtTooLong  = errors.New("TXT string exceeds 255 characters")
    ErrInvalidSOA  = errors.New("invalid SOA record")
    ErrInvalidCAA  = errors.New("invalid CAA record")
)
```

### 4.2 Lookup Methods

**File:** `pkg/localrecords/records.go`

Add new lookup methods following the existing pattern:

```go
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
// Returns list of records sorted by priority, or empty if not found
func (m *Manager) LookupMX(domain string) []*LocalRecord {
    domain = normalizeDomain(domain)

    m.mu.RLock()
    defer m.mu.RUnlock()

    result := make([]*LocalRecord, 0)

    // Check exact matches
    if records, exists := m.records[domain]; exists {
        for _, r := range records {
            if r.Type == RecordTypeMX && r.Enabled {
                result = append(result, r)
            }
        }
    }

    // Check wildcards
    for _, wc := range m.wildcards {
        if wc.Type == RecordTypeMX && wc.Enabled && matchesWildcard(domain, wc.Domain) {
            result = append(result, wc)
        }
    }

    // Sort by priority (lower priority = higher preference)
    sort.Slice(result, func(i, j int) bool {
        return result[i].Priority < result[j].Priority
    })

    return result
}

// LookupSRV looks up SRV records for a domain
// Returns list of records sorted by priority then weight
func (m *Manager) LookupSRV(domain string) []*LocalRecord {
    domain = normalizeDomain(domain)

    m.mu.RLock()
    defer m.mu.RUnlock()

    result := make([]*LocalRecord, 0)

    // Check exact matches
    if records, exists := m.records[domain]; exists {
        for _, r := range records {
            if r.Type == RecordTypeSRV && r.Enabled {
                result = append(result, r)
            }
        }
    }

    // Check wildcards
    for _, wc := range m.wildcards {
        if wc.Type == RecordTypeSRV && wc.Enabled && matchesWildcard(domain, wc.Domain) {
            result = append(result, wc)
        }
    }

    // Sort by priority, then weight (RFC 2796)
    sort.Slice(result, func(i, j int) bool {
        if result[i].Priority != result[j].Priority {
            return result[i].Priority < result[j].Priority
        }
        return result[i].Weight > result[j].Weight
    })

    return result
}

// LookupPTR looks up PTR records for a domain (reverse DNS)
func (m *Manager) LookupPTR(domain string) []*LocalRecord {
    domain = normalizeDomain(domain)

    m.mu.RLock()
    defer m.mu.RUnlock()

    result := make([]*LocalRecord, 0)

    // Check exact matches
    if records, exists := m.records[domain]; exists {
        for _, r := range records {
            if r.Type == RecordTypePTR && r.Enabled {
                result = append(result, r)
            }
        }
    }

    // Check wildcards
    for _, wc := range m.wildcards {
        if wc.Type == RecordTypePTR && wc.Enabled && matchesWildcard(domain, wc.Domain) {
            result = append(result, wc)
        }
    }

    return result
}

// LookupNS looks up NS records for a domain
func (m *Manager) LookupNS(domain string) []*LocalRecord {
    domain = normalizeDomain(domain)

    m.mu.RLock()
    defer m.mu.RUnlock()

    result := make([]*LocalRecord, 0)

    if records, exists := m.records[domain]; exists {
        for _, r := range records {
            if r.Type == RecordTypeNS && r.Enabled {
                result = append(result, r)
            }
        }
    }

    for _, wc := range m.wildcards {
        if wc.Type == RecordTypeNS && wc.Enabled && matchesWildcard(domain, wc.Domain) {
            result = append(result, wc)
        }
    }

    return result
}

// LookupSOA looks up SOA record for a domain
// Returns first match only (SOA is unique per zone)
func (m *Manager) LookupSOA(domain string) *LocalRecord {
    domain = normalizeDomain(domain)

    m.mu.RLock()
    defer m.mu.RUnlock()

    if records, exists := m.records[domain]; exists {
        for _, r := range records {
            if r.Type == RecordTypeSOA && r.Enabled {
                return r // Return first SOA found
            }
        }
    }

    // SOA doesn't typically use wildcards, but check anyway
    for _, wc := range m.wildcards {
        if wc.Type == RecordTypeSOA && wc.Enabled && matchesWildcard(domain, wc.Domain) {
            return wc
        }
    }

    return nil
}

// LookupCAA looks up CAA records for a domain
func (m *Manager) LookupCAA(domain string) []*LocalRecord {
    domain = normalizeDomain(domain)

    m.mu.RLock()
    defer m.mu.RUnlock()

    result := make([]*LocalRecord, 0)

    if records, exists := m.records[domain]; exists {
        for _, r := range records {
            if r.Type == RecordTypeCAA && r.Enabled {
                result = append(result, r)
            }
        }
    }

    for _, wc := range m.wildcards {
        if wc.Type == RecordTypeCAA && wc.Enabled && matchesWildcard(domain, wc.Domain) {
            result = append(result, wc)
        }
    }

    return result
}
```

**Note:** Need to `import "sort"` at top of file.

### 4.3 DNS Handler Updates

**File:** `pkg/dns/server.go:183-295`

Add new cases after the existing CNAME case (line 295):

```go
case dns.TypeTXT:
    if h.LocalRecords != nil {
        records := h.LocalRecords.LookupTXT(domain)
        if len(records) > 0 {
            for _, rec := range records {
                rr := &dns.TXT{
                    Hdr: dns.RR_Header{
                        Name:   domain,
                        Rrtype: dns.TypeTXT,
                        Class:  dns.ClassINET,
                        Ttl:    rec.TTL,
                    },
                    Txt: rec.TxtRecords,
                }
                msg.Answer = append(msg.Answer, rr)
            }
            responseCode = dns.RcodeSuccess
            h.writeMsg(w, msg)
            return
        }
    }

case dns.TypeMX:
    if h.LocalRecords != nil {
        records := h.LocalRecords.LookupMX(domain)
        if len(records) > 0 {
            for _, rec := range records {
                rr := &dns.MX{
                    Hdr: dns.RR_Header{
                        Name:   domain,
                        Rrtype: dns.TypeMX,
                        Class:  dns.ClassINET,
                        Ttl:    rec.TTL,
                    },
                    Preference: rec.Priority,
                    Mx:         rec.Target,
                }
                msg.Answer = append(msg.Answer, rr)
            }
            responseCode = dns.RcodeSuccess
            h.writeMsg(w, msg)
            return
        }
    }

case dns.TypeSRV:
    if h.LocalRecords != nil {
        records := h.LocalRecords.LookupSRV(domain)
        if len(records) > 0 {
            for _, rec := range records {
                rr := &dns.SRV{
                    Hdr: dns.RR_Header{
                        Name:   domain,
                        Rrtype: dns.TypeSRV,
                        Class:  dns.ClassINET,
                        Ttl:    rec.TTL,
                    },
                    Priority: rec.Priority,
                    Weight:   rec.Weight,
                    Port:     rec.Port,
                    Target:   rec.Target,
                }
                msg.Answer = append(msg.Answer, rr)
            }
            responseCode = dns.RcodeSuccess
            h.writeMsg(w, msg)
            return
        }
    }

case dns.TypePTR:
    if h.LocalRecords != nil {
        records := h.LocalRecords.LookupPTR(domain)
        if len(records) > 0 {
            for _, rec := range records {
                rr := &dns.PTR{
                    Hdr: dns.RR_Header{
                        Name:   domain,
                        Rrtype: dns.TypePTR,
                        Class:  dns.ClassINET,
                        Ttl:    rec.TTL,
                    },
                    Ptr: rec.Target,
                }
                msg.Answer = append(msg.Answer, rr)
            }
            responseCode = dns.RcodeSuccess
            h.writeMsg(w, msg)
            return
        }
    }

case dns.TypeNS:
    if h.LocalRecords != nil {
        records := h.LocalRecords.LookupNS(domain)
        if len(records) > 0 {
            for _, rec := range records {
                rr := &dns.NS{
                    Hdr: dns.RR_Header{
                        Name:   domain,
                        Rrtype: dns.TypeNS,
                        Class:  dns.ClassINET,
                        Ttl:    rec.TTL,
                    },
                    Ns: rec.Target,
                }
                msg.Answer = append(msg.Answer, rr)
            }
            responseCode = dns.RcodeSuccess
            h.writeMsg(w, msg)
            return
        }
    }

case dns.TypeSOA:
    if h.LocalRecords != nil {
        record := h.LocalRecords.LookupSOA(domain)
        if record != nil {
            rr := &dns.SOA{
                Hdr: dns.RR_Header{
                    Name:   domain,
                    Rrtype: dns.TypeSOA,
                    Class:  dns.ClassINET,
                    Ttl:    record.TTL,
                },
                Ns:      record.Ns,
                Mbox:    record.Mbox,
                Serial:  record.Serial,
                Refresh: record.Refresh,
                Retry:   record.Retry,
                Expire:  record.Expire,
                Minttl:  record.Minttl,
            }
            msg.Answer = append(msg.Answer, rr)
            responseCode = dns.RcodeSuccess
            h.writeMsg(w, msg)
            return
        }
    }

case dns.TypeCAA:
    if h.LocalRecords != nil {
        records := h.LocalRecords.LookupCAA(domain)
        if len(records) > 0 {
            for _, rec := range records {
                rr := &dns.CAA{
                    Hdr: dns.RR_Header{
                        Name:   domain,
                        Rrtype: dns.TypeCAA,
                        Class:  dns.ClassINET,
                        Ttl:    rec.TTL,
                    },
                    Flag:  rec.CaaFlag,
                    Tag:   rec.CaaTag,
                    Value: rec.CaaValue,
                }
                msg.Answer = append(msg.Answer, rr)
            }
            responseCode = dns.RcodeSuccess
            h.writeMsg(w, msg)
            return
        }
    }
```

**Insertion Point:** After line 294 (end of CNAME case), before line 298 (get client IP).

### 4.4 Config Parsing Updates

**File:** Need to update config loading logic (likely in `cmd/glory-hole/main.go` or config package)

When loading `LocalRecordEntry` from YAML, convert to `LocalRecord`:

```go
func convertConfigToLocalRecord(entry config.LocalRecordEntry) (*localrecords.LocalRecord, error) {
    record := &localrecords.LocalRecord{
        Domain:   entry.Domain,
        Type:     localrecords.RecordType(entry.Type),
        TTL:      entry.TTL,
        Wildcard: entry.Wildcard,
        Enabled:  entry.Enabled, // Default to true if not set
    }

    // Set defaults
    if record.TTL == 0 {
        record.TTL = 300
    }
    if !entry.Enabled {
        record.Enabled = true // Default enabled unless explicitly disabled
    }

    // Parse type-specific fields
    switch record.Type {
    case localrecords.RecordTypeA, localrecords.RecordTypeAAAA:
        ips, err := parseIPs(entry.IPs)
        if err != nil {
            return nil, err
        }
        record.IPs = ips

    case localrecords.RecordTypeCNAME, localrecords.RecordTypeMX,
         localrecords.RecordTypePTR, localrecords.RecordTypeNS:
        record.Target = entry.Target

    case localrecords.RecordTypeTXT:
        record.TxtRecords = entry.TxtRecords

    case localrecords.RecordTypeSRV:
        record.Target = entry.Target
        if entry.Priority != nil {
            record.Priority = *entry.Priority
        }
        if entry.Weight != nil {
            record.Weight = *entry.Weight
        }
        if entry.Port != nil {
            record.Port = *entry.Port
        }

    case localrecords.RecordTypeSOA:
        record.Ns = entry.Ns
        record.Mbox = entry.Mbox
        if entry.Serial != nil {
            record.Serial = *entry.Serial
        } else {
            record.Serial = 1 // Default serial
        }
        // Set defaults for timing fields if not provided
        if entry.Refresh != nil {
            record.Refresh = *entry.Refresh
        } else {
            record.Refresh = 86400 // 24 hours
        }
        if entry.Retry != nil {
            record.Retry = *entry.Retry
        } else {
            record.Retry = 7200 // 2 hours
        }
        if entry.Expire != nil {
            record.Expire = *entry.Expire
        } else {
            record.Expire = 3600000 // ~42 days
        }
        if entry.Minttl != nil {
            record.Minttl = *entry.Minttl
        } else {
            record.Minttl = 300 // 5 minutes
        }

    case localrecords.RecordTypeCAA:
        if entry.CaaFlag != nil {
            record.CaaFlag = *entry.CaaFlag
        }
        record.CaaTag = entry.CaaTag
        record.CaaValue = entry.CaaValue
    }

    return record, nil
}
```

---

## 5. Implementation Phases

### Phase 1: TXT Record Support (Day 1)
**Goal:** Fix TXT records and validate the approach

**Changes:**
1. Add `TxtRecords []string` field to `LocalRecord`
2. Update TXT validation to use `TxtRecords`
3. Add `LookupTXT()` method
4. Add `case dns.TypeTXT:` handler
5. Update config parsing for TXT
6. Write comprehensive tests

**Files Modified:**
- `pkg/localrecords/records.go` (add field)
- `pkg/localrecords/util.go` (fix validation)
- `pkg/localrecords/errors.go` (new errors)
- `pkg/dns/server.go` (add handler case)
- `pkg/config/config.go` (add TxtRecords field)
- `pkg/localrecords/records_test.go` (new tests)

**Test Coverage:**
- Single TXT record
- Multiple TXT records for same domain
- TXT with wildcards
- TXT with long strings (validate 255 char limit)
- Empty TXT array (should fail validation)

**Success Criteria:**
- All tests pass
- TXT queries return correct responses
- No regression in A/AAAA/CNAME tests

### Phase 2: MX Record Support (Day 2)
**Goal:** Add mail exchange record support

**Changes:**
1. Add `LookupMX()` method with priority sorting
2. Add `case dns.TypeMX:` handler
3. Update config parsing for MX priority
4. Write tests

**Files Modified:**
- `pkg/localrecords/records.go` (add method)
- `pkg/dns/server.go` (add handler)
- `pkg/config/config.go` (add Priority field)
- `pkg/localrecords/records_test.go` (new tests)

**Test Coverage:**
- Single MX record
- Multiple MX records with different priorities
- Priority sorting (lower number = higher priority)
- MX with wildcards
- Default priority (when not specified)

### Phase 3: PTR Record Support (Day 2)
**Goal:** Add reverse DNS support

**Changes:**
1. Add `LookupPTR()` method
2. Add `case dns.TypePTR:` handler
3. Write tests

**Files Modified:**
- `pkg/localrecords/records.go`
- `pkg/dns/server.go`
- `pkg/localrecords/records_test.go`

**Test Coverage:**
- Standard PTR (192.168.1.1 → 1.1.168.192.in-addr.arpa)
- IPv6 PTR (reverse nibble format)
- PTR with wildcards
- Multiple PTR records

### Phase 4: SRV Record Support (Day 3)
**Goal:** Add service discovery support

**Changes:**
1. Add `LookupSRV()` method with priority/weight sorting
2. Add `case dns.TypeSRV:` handler
3. Update config parsing for Weight and Port
4. Write tests

**Files Modified:**
- `pkg/localrecords/records.go`
- `pkg/dns/server.go`
- `pkg/config/config.go`
- `pkg/localrecords/records_test.go`

**Test Coverage:**
- Single SRV record
- Multiple SRV records with different priorities/weights
- Priority then weight sorting (RFC 2782)
- SRV with port 0 (validation should fail)
- _service._proto.domain format

### Phase 5: NS and SOA Records (Day 4)
**Goal:** Add zone management records

**Changes:**
1. Add all SOA fields to `LocalRecord`
2. Add `LookupNS()` and `LookupSOA()` methods
3. Add handler cases
4. Update config structure
5. Write tests

**Files Modified:**
- `pkg/localrecords/records.go` (add fields + methods)
- `pkg/dns/server.go` (add handlers)
- `pkg/config/config.go` (add SOA fields)
- `pkg/localrecords/util.go` (add validation)
- `pkg/localrecords/errors.go` (add ErrInvalidSOA)
- `pkg/localrecords/records_test.go`

**Test Coverage:**
- SOA with all fields
- SOA with defaults
- SOA validation (must have Ns and Mbox)
- NS records (single and multiple)
- NS delegation scenarios

### Phase 6: EDNS0 Support (Day 5)
**Goal:** Add modern DNS extension support

**Changes:**
1. Create new file `pkg/dns/edns.go`
2. Check for EDNS0 in incoming requests
3. Set EDNS0 in responses with appropriate buffer size
4. Add DNS cookie placeholder (for future)
5. Write tests

**Files Modified:**
- `pkg/dns/edns.go` (new file)
- `pkg/dns/server.go` (integrate EDNS handling)
- `pkg/dns/server_test.go` (new tests)

**Test Coverage:**
- Request with EDNS0 → Response with EDNS0
- Request without EDNS0 → Response without EDNS0
- Buffer size negotiation
- EDNS0 flags preservation

### Phase 7: CAA Records (Day 6)
**Goal:** Add certificate authority authorization

**Changes:**
1. Add CAA fields to `LocalRecord`
2. Add validation
3. Add `LookupCAA()` method
4. Add handler case
5. Update config
6. Write tests

**Files Modified:**
- `pkg/localrecords/records.go`
- `pkg/localrecords/util.go`
- `pkg/localrecords/errors.go`
- `pkg/dns/server.go`
- `pkg/config/config.go`
- `pkg/localrecords/records_test.go`

**Test Coverage:**
- CAA issue tag
- CAA issuewild tag
- CAA iodef tag
- Invalid tag (validation should fail)
- CAA flag values (0, 128)

### Phase 8: Integration Testing (Day 7)
**Goal:** End-to-end validation

**Tests:**
1. Full config file with all record types
2. Query each record type via DNS protocol
3. Performance benchmarks (no regression)
4. Wildcard handling for all types
5. Cache behavior with new record types
6. Multiple records of same type for one domain

### Phase 9: Documentation (Day 7-8)
**Goal:** Update all documentation

**Files to Update:**
1. `docs/configuration/local-records.md` - Add examples for all record types
2. `docs/api/records.md` - Update API docs
3. `docs/architecture/overview.md` - Update LocalRecord struct diagram
4. `config.example.yml` - Add examples for all record types
5. `CHANGELOG.md` - Document v0.7.2 changes
6. `README.md` - Update feature list

### Phase 10: Release (Day 8)
**Goal:** Package and release v0.7.2

**Checklist:**
- [ ] All tests passing
- [ ] Test coverage ≥70%
- [ ] Documentation updated
- [ ] CHANGELOG.md complete
- [ ] Version bumped in all files
- [ ] Git tag created
- [ ] Release notes written

---

## 6. Testing Strategy

### 6.1 Unit Tests

**Coverage Target:** ≥70% for new code

**Test Files:**
- `pkg/localrecords/records_test.go` - Manager and lookup tests
- `pkg/localrecords/util_test.go` - Validation tests
- `pkg/dns/server_test.go` - DNS handler tests
- `pkg/dns/edns_test.go` - EDNS0 tests

**Test Categories:**

1. **Validation Tests**
   - Valid records for each type
   - Invalid records (missing required fields)
   - Edge cases (empty strings, nil values, zero ports)

2. **Lookup Tests**
   - Exact domain matches
   - Wildcard matches
   - Case insensitivity
   - Multiple records per domain
   - Disabled records (should be filtered)

3. **DNS Handler Tests**
   - Each record type query/response
   - Missing records (should forward upstream)
   - Malformed queries
   - Cache integration

4. **EDNS0 Tests**
   - Buffer size negotiation
   - EDNS0 flags
   - Backward compatibility (non-EDNS0 clients)

### 6.2 Integration Tests

**Test Scenarios:**

1. **Full Config Load**
   ```yaml
   local_records:
     enabled: true
     records:
       - domain: example.local
         type: A
         ips: [192.168.1.100]
       - domain: example.local
         type: TXT
         txt: ["v=spf1 mx ~all"]
       - domain: example.local
         type: MX
         target: mail.example.local
         priority: 10
   ```

2. **DNS Query Flow**
   - Send DNS query via UDP/TCP
   - Verify response matches expected records
   - Check TTL values
   - Verify EDNS0 behavior

3. **Performance Benchmarks**
   - Compare v0.7.1 vs v0.7.2 query latency
   - Memory usage comparison
   - Cache hit/miss rates

### 6.3 Regression Tests

**Ensure No Breaking Changes:**

1. Run all existing tests (should pass without modification)
2. Verify A/AAAA/CNAME behavior unchanged
3. Check existing config files still work
4. Validate API backward compatibility

---

## 7. Migration and Compatibility

### 7.1 Backward Compatibility

**Guaranteed:**
- ✅ Existing A/AAAA/CNAME configs work without changes
- ✅ Old config files with only A/AAAA/CNAME load successfully
- ✅ No breaking changes to API endpoints
- ✅ Database schema unchanged (query logs)

**Breaking Changes:**
- ❌ **TXT Record Format Changed** - If anyone was using TXT records with `target:` field, they must migrate to `txt:` array format

**Migration Path for TXT Records:**
```yaml
# OLD (v0.7.1) - NO LONGER WORKS
local_records:
  records:
    - domain: example.com
      type: TXT
      target: "v=spf1 mx ~all"  # ❌ WRONG FIELD

# NEW (v0.7.2) - CORRECT FORMAT
local_records:
  records:
    - domain: example.com
      type: TXT
      txt:  # ✅ CORRECT
        - "v=spf1 mx ~all"
        - "google-site-verification=abc123"
```

### 7.2 Config File Validation

Add validation check on startup:
```go
// In config loading, detect legacy TXT format
if entry.Type == "TXT" && entry.Target != "" && len(entry.TxtRecords) == 0 {
    return fmt.Errorf("TXT record for '%s' uses deprecated 'target' field. Please migrate to 'txt' array format. See: https://docs.glory-hole.io/migration-v0.7.2", entry.Domain)
}
```

### 7.3 Database Schema

**No changes required** - Query logging already stores:
- Domain (string)
- QueryType (string) - Can store "TXT", "MX", etc.
- ResponseCode (int)

All new record types work with existing schema.

---

## 8. Performance Considerations

### 8.1 Memory Impact

**Current Memory per Record:** ~104 bytes
**New Memory per Record:** ~168 bytes (+64 bytes)

**For 10,000 records:**
- Current: ~1.04 MB
- New: ~1.68 MB
- Increase: +640 KB (0.6% of typical DNS server memory)

**Mitigation:** Go's zero-value optimization means unused fields consume minimal memory.

### 8.2 Lookup Performance

**Current Lookups:** O(1) exact match + O(n) wildcard scan
**New Lookups:** Same complexity - no change

**Sorting Overhead:**
- MX records: O(k log k) where k = number of MX records per domain (typically <5)
- SRV records: O(k log k) where k = number of SRV records per domain (typically <10)
- **Impact:** Negligible (<1µs for typical cases)

### 8.3 DNS Handler Performance

**Current Handler:** 3 switch cases
**New Handler:** 10 switch cases

**Impact:** Switch statement performance is O(1) in Go (jump table optimization)
**Expected Overhead:** <10ns per query

### 8.4 Benchmarks to Run

```go
func BenchmarkLookupTXT(b *testing.B)
func BenchmarkLookupMX(b *testing.B)
func BenchmarkLookupSRV(b *testing.B)
func BenchmarkDNSHandlerTXT(b *testing.B)
func BenchmarkDNSHandlerMX(b *testing.B)
```

**Acceptance Criteria:** No more than 5% performance degradation for existing A/AAAA/CNAME queries.

---

## 9. Security Considerations

### 9.1 Input Validation

**Threats:**
1. **Malformed TXT Strings** - Excessively long strings
   - **Mitigation:** Validate max 255 chars per TXT string
2. **DNS Amplification** - Large responses used for DDoS
   - **Mitigation:** EDNS0 buffer size limits
3. **CNAME Loops** - Already handled (max depth 10)
4. **Invalid CAA Tags** - Incorrect CA authorization
   - **Mitigation:** Whitelist valid tags (issue/issuewild/iodef)

### 9.2 EDNS0 Security

**DNS Cookies (RFC 7873):**
- Not implemented in Phase 6 (placeholder only)
- Future enhancement for DDoS mitigation
- Prevents source IP spoofing

**Buffer Size:**
- Max 4096 bytes (prevent amplification)
- Min 512 bytes (RFC 1035 compliance)

### 9.3 Rate Limiting

**No changes required** - Existing rate limiting applies to all query types.

---

## 10. Future Enhancements

### 10.1 Not in v0.7.2 (Future Versions)

1. **DNSSEC Support (v0.9.0)**
   - DNSKEY, RRSIG, NSEC, DS records
   - Signature validation
   - Chain of trust verification

2. **Advanced Record Types (v0.10.0)**
   - NAPTR (VoIP/telephony routing)
   - SSHFP (SSH fingerprint verification)
   - TLSA (DANE certificate association)
   - SVCB/HTTPS (Service binding, RFC 9460)

3. **Dynamic DNS (v0.11.0)**
   - REST API to add/update/delete records at runtime
   - No config file restart required
   - Persistence to database

4. **DNS Cookies (v0.8.0)**
   - Complete EDNS0 cookie implementation
   - DDoS mitigation

### 10.2 Architectural Improvements

1. **Record Storage Optimization**
   - Consider using `interface{}` for type-specific fields
   - Reduce memory footprint
   - Faster serialization

2. **Query Pipeline**
   - Pluggable record type handlers
   - Easier to add new types

3. **Wildcard Performance**
   - Trie data structure for O(log n) wildcard matching
   - Currently O(n) scan

---

## 11. Example Configurations

### 11.1 Complete config.yml Example

```yaml
local_records:
  enabled: true
  records:
    # A Record - IPv4 address
    - domain: nas.local
      type: A
      ips:
        - 192.168.1.100
        - 192.168.1.101  # Multiple IPs
      ttl: 300

    # AAAA Record - IPv6 address
    - domain: server.local
      type: AAAA
      ips:
        - fe80::1
      ttl: 300

    # CNAME Record - Alias
    - domain: storage.local
      type: CNAME
      target: nas.local
      ttl: 300

    # TXT Record - SPF, DKIM, verification
    - domain: example.local
      type: TXT
      txt:
        - "v=spf1 mx include:_spf.google.com ~all"
        - "google-site-verification=abc123xyz"
      ttl: 3600

    # MX Record - Mail server
    - domain: example.local
      type: MX
      target: mail.example.local
      priority: 10
      ttl: 3600

    - domain: example.local
      type: MX
      target: mail2.example.local
      priority: 20  # Backup mail server
      ttl: 3600

    # SRV Record - Service discovery
    - domain: _ldap._tcp.example.local
      type: SRV
      target: ldap-server.example.local
      priority: 0
      weight: 5
      port: 389
      ttl: 3600

    # PTR Record - Reverse DNS
    - domain: 100.1.168.192.in-addr.arpa
      type: PTR
      target: nas.local
      ttl: 300

    # NS Record - Nameserver delegation
    - domain: subdomain.example.local
      type: NS
      target: ns1.subdomain.example.local
      ttl: 86400

    # SOA Record - Zone authority
    - domain: example.local
      type: SOA
      ns: ns1.example.local
      mbox: admin.example.local
      serial: 2025012301
      refresh: 86400   # 24 hours
      retry: 7200      # 2 hours
      expire: 3600000  # ~42 days
      minttl: 300      # 5 minutes
      ttl: 86400

    # CAA Record - Certificate authority
    - domain: example.local
      type: CAA
      caa_flag: 0
      caa_tag: issue
      caa_value: letsencrypt.org
      ttl: 86400

    # Wildcard Record
    - domain: "*.dev.local"
      type: A
      ips:
        - 192.168.1.200
      wildcard: true
      ttl: 300
```

### 11.2 Minimal config.yml Example

```yaml
local_records:
  enabled: true
  records:
    - domain: router.local
      type: A
      ips: [192.168.1.1]

    - domain: example.com
      type: TXT
      txt: ["v=spf1 mx ~all"]
```

---

## 12. Documentation Updates Required

### 12.1 Files to Create
- `docs/configuration/record-types.md` - Complete reference for all record types

### 12.2 Files to Update
- `docs/configuration/local-records.md` - Add new record types
- `docs/api/records.md` - Update API docs
- `docs/architecture/overview.md` - Update LocalRecord struct diagram
- `config.example.yml` - Add examples
- `CHANGELOG.md` - v0.7.2 changes
- `README.md` - Update feature list

### 12.3 New Sections Needed

**In `docs/configuration/record-types.md`:**
1. A Record Reference
2. AAAA Record Reference
3. CNAME Record Reference
4. TXT Record Reference (with SPF/DKIM examples)
5. MX Record Reference (with priority explanation)
6. SRV Record Reference (with service discovery examples)
7. PTR Record Reference (with reverse DNS format)
8. NS Record Reference
9. SOA Record Reference (with zone management explanation)
10. CAA Record Reference

Each section should include:
- Description and use case
- Required fields
- Optional fields with defaults
- YAML examples
- Common mistakes and troubleshooting

---

## 13. Risk Assessment

### 13.1 High Risk Items

1. **TXT Format Breaking Change**
   - **Risk:** Users with existing TXT records will break
   - **Mitigation:** Clear error messages, migration guide, version check
   - **Likelihood:** Low (TXT usage estimated <5% of deployments)

2. **Performance Regression**
   - **Risk:** New code slows down existing queries
   - **Mitigation:** Comprehensive benchmarks, performance tests in CI
   - **Likelihood:** Low (switch statement is O(1))

### 13.2 Medium Risk Items

1. **Config Parsing Errors**
   - **Risk:** New optional fields cause parsing issues
   - **Mitigation:** Extensive validation, unit tests
   - **Likelihood:** Medium

2. **Memory Usage Increase**
   - **Risk:** Larger struct size causes memory issues
   - **Mitigation:** Benchmark with large datasets, document limits
   - **Likelihood:** Low (+64 bytes per record is negligible)

### 13.3 Low Risk Items

1. **Test Coverage Gaps**
   - **Risk:** Edge cases not tested
   - **Mitigation:** Code review, test coverage tools
   - **Likelihood:** Medium (mitigated by thorough testing)

---

## 14. Success Metrics

### 14.1 Functional Metrics
- ✅ All 8 new record types (TXT, MX, PTR, SRV, NS, SOA, CAA, EDNS0) implemented
- ✅ All record types have ≥5 unit tests each
- ✅ Integration tests cover all record types
- ✅ Config examples provided for each type

### 14.2 Quality Metrics
- ✅ Test coverage ≥70% overall
- ✅ Zero linter warnings
- ✅ Zero race conditions (go test -race)
- ✅ All existing tests pass (no regressions)

### 14.3 Performance Metrics
- ✅ A/AAAA/CNAME query latency unchanged (±5%)
- ✅ New record type queries <5ms (cached)
- ✅ Memory usage increase <1MB for 10k records
- ✅ Cache hit rate unchanged

### 14.4 Documentation Metrics
- ✅ All record types documented with examples
- ✅ Migration guide published
- ✅ CHANGELOG.md complete
- ✅ API documentation updated

---

## 15. Approval and Sign-off

**Design Review Checklist:**
- [ ] Data structures reviewed and approved
- [ ] Implementation phases agreed upon
- [ ] Test strategy approved
- [ ] Migration plan accepted
- [ ] Risk assessment reviewed
- [ ] Resource allocation confirmed

**Estimated Timeline:** 8 days
**Estimated Effort:** 40-50 hours

**Next Steps:**
1. Review this document
2. Get approval for design
3. Begin Phase 1 implementation (TXT records)
4. Proceed incrementally through phases

---

## Appendix A: File Change Summary

### Files to Modify
1. `pkg/localrecords/records.go` - Add fields, add lookup methods
2. `pkg/localrecords/util.go` - Update validation
3. `pkg/localrecords/errors.go` - Add new error types
4. `pkg/dns/server.go` - Add DNS handler cases
5. `pkg/config/config.go` - Update LocalRecordEntry struct
6. `pkg/localrecords/records_test.go` - Add tests
7. `pkg/dns/server_test.go` - Add integration tests

### Files to Create
1. `pkg/dns/edns.go` - EDNS0 support
2. `pkg/dns/edns_test.go` - EDNS0 tests
3. `docs/configuration/record-types.md` - Complete reference
4. `docs/designs/dns-record-types-design-v0.7.2.md` - This document

### Files to Update
1. `docs/configuration/local-records.md`
2. `docs/api/records.md`
3. `docs/architecture/overview.md`
4. `config.example.yml`
5. `CHANGELOG.md`
6. `README.md`

---

## Appendix B: DNS Record Type Quick Reference

| Type  | RFC    | Priority | Fields Used                      | Use Case                           |
|-------|--------|----------|----------------------------------|------------------------------------|
| A     | 1035   | P0       | IPs                              | IPv4 address resolution            |
| AAAA  | 3596   | P0       | IPs                              | IPv6 address resolution            |
| CNAME | 1035   | P0       | Target                           | Domain aliasing                    |
| TXT   | 1035   | P1       | TxtRecords                       | SPF, DKIM, verification            |
| MX    | 1035   | P1       | Target, Priority                 | Mail routing                       |
| PTR   | 1035   | P1       | Target                           | Reverse DNS                        |
| SRV   | 2782   | P1       | Target, Priority, Weight, Port   | Service discovery                  |
| NS    | 1035   | P2       | Target                           | Nameserver delegation              |
| SOA   | 1035   | P2       | Ns, Mbox, Serial, Refresh, etc.  | Zone authority                     |
| CAA   | 6844   | P3       | CaaFlag, CaaTag, CaaValue        | Certificate authorization          |

**Priority Legend:**
- P0 = Already implemented
- P1 = Essential (Phase 1-4)
- P2 = Zone management (Phase 5)
- P3 = Security enhancement (Phase 7)

---

**End of Design Document**
