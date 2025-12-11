# Missing UI Features Implementation Plan

## Overview

Three major configuration features exist in `config.yml` but are completely missing from the Web UI:

1. **Whitelist** - Allow domains to bypass blocking
2. **Local Records** - Custom DNS records (A, AAAA, CNAME, MX, TXT, SOA, CAA, etc.)
3. **Conditional Forwarding** - Route specific domains to specific upstreams

## Current State Analysis

### What Exists in Config
**Location:** `pkg/config/config.go`

```go
type Config struct {
    // ...
    Whitelist             []string                    `yaml:"whitelist"`                  // Line 29
    LocalRecords          LocalRecordsConfig          `yaml:"local_records"`              // Line 25
    ConditionalForwarding ConditionalForwardingConfig `yaml:"conditional_forwarding"`     // Line 26
    // ...
}
```

### What Exists in UI
**Location:** `pkg/api/ui/templates/base.html:52-57`

```
- Dashboard      ✅ (shows stats, charts, top domains)
- Queries        ✅ (query log with filtering)
- Policies       ✅ (full CRUD for policy rules)
- Clients        ✅ (client management with groups)
- Blocklists     ✅ (basic blocklist info)
- Settings       ✅ (partial config editing)
```

**Missing:**
- ❌ Whitelist management
- ❌ Local records management
- ❌ Conditional forwarding rules

### Settings Page Limitations
The Settings page (`settings.html:625-627`) acknowledges this:
> "Upstreams, cache, and logging settings can be edited live above. **Other sections remain read-only** and require editing config.yml followed by a restart."

## Feature 1: Whitelist Management

### Current Implementation
**Config:**
```yaml
whitelist:
  - "example.com"
  - "*.google.com"
  - "cdn.jsdelivr.net"
```

**Backend:** `pkg/dns/handler.go` - checks whitelist before blocklist matching

### Proposed UI

#### Navigation
Add new page: **"Whitelist"** between "Blocklists" and "Settings"

#### Page Layout
```
┌─────────────────────────────────────────────────────────────┐
│  Whitelist                                              [+]  │
│  Domains that bypass all blocking rules                     │
├─────────────────────────────────────────────────────────────┤
│  Search: [__________________]                     [Import]  │
├─────────────────────────────────────────────────────────────┤
│  ┌───────────────────────────────────────────────────────┐ │
│  │  example.com                                      [×]  │ │
│  │  Added: 2025-01-15  |  Matches: 1,234 queries        │ │
│  └───────────────────────────────────────────────────────┘ │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  *.google.com                                     [×]  │ │
│  │  Added: 2025-01-10  |  Matches: 5,678 queries        │ │
│  └───────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

#### Features
- **Add Domain**: Modal with domain input + pattern validation
- **Remove Domain**: Click × to remove (with confirmation)
- **Bulk Import**: Upload/paste list of domains
- **Search/Filter**: Filter whitelist entries
- **Statistics**: Show match count per domain (requires query log analysis)
- **Export**: Download whitelist as text file

#### API Endpoints
```
GET  /api/whitelist          - List all whitelisted domains
POST /api/whitelist          - Add domain to whitelist
DELETE /api/whitelist/:domain - Remove from whitelist
POST /api/whitelist/bulk     - Bulk import domains
```

#### Implementation Checklist
- [ ] Create `pkg/api/handlers_whitelist.go`
  - [ ] `handleGetWhitelist()`
  - [ ] `handleAddWhitelist()`
  - [ ] `handleRemoveWhitelist()`
  - [ ] `handleBulkImportWhitelist()`
- [ ] Create `pkg/api/ui/templates/whitelist.html`
  - [ ] Main page template
  - [ ] Whitelist partial for HTMX updates
  - [ ] Add modal
  - [ ] Import modal
- [ ] Add navigation link in `base.html`
- [ ] Update config write logic to persist whitelist changes
- [ ] Add whitelist metrics (queries matched per domain)

## Feature 2: Local Records Management

### Current Implementation
**Config:**
```yaml
local_records:
  enabled: true
  records:
    - domain: "router.local"
      type: "A"
      value: "192.168.1.1"
      ttl: 300
    - domain: "mail.local"
      type: "MX"
      value: "smtp.local"
      priority: 10
      ttl: 3600
```

**Supported Record Types:**
- A, AAAA, CNAME, MX, TXT, NS, PTR, SOA, CAA, SRV

**Backend:** `pkg/dns/local_records.go` - serves local records before upstream

### Proposed UI

#### Navigation
Add new page: **"Local Records"** after "Whitelist"

#### Page Layout
```
┌─────────────────────────────────────────────────────────────┐
│  Local DNS Records                  [Enable/Disable] [Add]  │
│  Custom DNS responses served locally without upstream       │
├─────────────────────────────────────────────────────────────┤
│  Filter by Type: [All ▾] [A] [AAAA] [CNAME] [MX] [TXT] ... │
│  Search: [__________________]                               │
├─────────────────────────────────────────────────────────────┤
│  ┌───────────────────────────────────────────────────────┐ │
│  │  [A] router.local → 192.168.1.1                       │ │
│  │      TTL: 300s  |  Queries: 234  |  [Edit] [Delete]  │ │
│  └───────────────────────────────────────────────────────┘ │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  [MX] mail.local → smtp.local (Priority: 10)          │ │
│  │      TTL: 3600s  |  Queries: 12  |  [Edit] [Delete]   │ │
│  └───────────────────────────────────────────────────────┘ │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  [CNAME] www.local → router.local                     │ │
│  │      TTL: 300s  |  Queries: 89  |  [Edit] [Delete]    │ │
│  └───────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

#### Add/Edit Modal
```
┌─────────────────────────────────────────────┐
│  Add Local Record                       [×] │
├─────────────────────────────────────────────┤
│  Domain:  [______________.local]            │
│  Type:    [A ▾]                             │
│  ┌─────────────────────────────────────────┐
│  │  Value:    [192.168.1.__]               │  # Dynamic fields
│  │  TTL:      [300______] seconds          │  # based on type
│  └─────────────────────────────────────────┘
│               [Cancel]  [Save Record]       │
└─────────────────────────────────────────────┘
```

**Dynamic Fields by Type:**
- **A**: IPv4 address
- **AAAA**: IPv6 address
- **CNAME**: Target domain
- **MX**: Priority + Target
- **TXT**: Text content
- **SRV**: Priority, Weight, Port, Target
- **SOA**: Nameserver, Email, Serial, Refresh, Retry, Expire, MinTTL
- **CAA**: Flag, Tag (issue/issuewild/iodef), Value

#### Features
- **Record Type Selector**: Dropdown with all supported types
- **Dynamic Form**: Form fields change based on record type
- **Validation**: Type-specific validation (IP format, domain format, etc.)
- **Bulk Import**: Import from zone file or CSV
- **Export**: Export to zone file format
- **Enable/Disable**: Toggle local records feature globally
- **Test Record**: Query test button to verify record works

#### API Endpoints
```
GET  /api/local-records           - List all local records
POST /api/local-records           - Create new record
PUT  /api/local-records/:id       - Update record
DELETE /api/local-records/:id     - Delete record
POST /api/local-records/bulk      - Bulk import
GET  /api/local-records/export    - Export to zone file
PUT  /api/local-records/toggle    - Enable/disable feature
POST /api/local-records/test      - Test query a record
```

#### Implementation Checklist
- [ ] Create `pkg/api/handlers_local_records.go`
  - [ ] Full CRUD operations
  - [ ] Validation for each record type
  - [ ] Bulk import/export
  - [ ] Feature toggle
- [ ] Create `pkg/api/ui/templates/local_records.html`
  - [ ] Main page with records table
  - [ ] Dynamic add/edit modal with type-specific fields
  - [ ] Import/export modals
- [ ] Create `pkg/api/ui/static/js/local-records-init.js`
  - [ ] Dynamic form field generation based on type
  - [ ] Client-side validation
  - [ ] Modal management
- [ ] Add navigation link
- [ ] Update config persistence
- [ ] Add metrics for local record queries

## Feature 3: Conditional Forwarding Management

### Current Implementation
**Config:**
```yaml
conditional_forwarding:
  enabled: true
  rules:
    - name: "Corporate VPN"
      priority: 90
      domains:
        - "*.corp.example.com"
        - "*.internal"
      upstreams:
        - "10.0.0.1:53"
        - "10.0.0.2:53"
      enabled: true
    - name: "Home Network"
      priority: 50
      client_cidrs:
        - "192.168.1.0/24"
      query_types:
        - "PTR"
      upstreams:
        - "192.168.1.1:53"
      enabled: true
```

**Matchers:** Domains, Client IPs/CIDRs, Query Types
**Priority:** 0-100 (higher = evaluated first)
**Backend:** `pkg/dns/conditional_forwarding.go`

### Proposed UI

#### Navigation
Add as tab/section within **"Settings"** or new page **"Forwarding"**

#### Page Layout
```
┌─────────────────────────────────────────────────────────────┐
│  Conditional Forwarding Rules          [Enable/Disable] [+] │
│  Route specific queries to dedicated upstream servers       │
├─────────────────────────────────────────────────────────────┤
│  ┌───────────────────────────────────────────────────────┐ │
│  │  🟢 Corporate VPN                          Priority: 90│ │
│  │  Domains: *.corp.example.com, *.internal              │ │
│  │  Upstreams: 10.0.0.1:53, 10.0.0.2:53                  │ │
│  │  Queries matched: 1,234                               │ │
│  │              [Toggle] [Edit] [Duplicate] [Delete]     │ │
│  └───────────────────────────────────────────────────────┘ │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  🔴 Home Network PTR                       Priority: 50│ │
│  │  Client CIDRs: 192.168.1.0/24                         │ │
│  │  Query Types: PTR                                     │ │
│  │  Upstreams: 192.168.1.1:53                            │ │
│  │  Queries matched: 89                                  │ │
│  │              [Toggle] [Edit] [Duplicate] [Delete]     │ │
│  └───────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

#### Add/Edit Modal
```
┌─────────────────────────────────────────────────────────────┐
│  Add Conditional Forwarding Rule                        [×] │
├─────────────────────────────────────────────────────────────┤
│  Name:      [______________________________________]        │
│  Priority:  [50____] (0-100, higher = evaluated first)      │
│                                                             │
│  ┌─ Match Conditions ──────────────────────────────────┐   │
│  │  Domains (one per line):                            │   │
│  │  ┌───────────────────────────────────────────────┐  │   │
│  │  │ *.corp.example.com                            │  │   │
│  │  │ *.internal                                    │  │   │
│  │  └───────────────────────────────────────────────┘  │   │
│  │                                                     │   │
│  │  Client CIDRs (comma or line separated):           │   │
│  │  [192.168.1.0/24, 10.0.0.0/8_________________]     │   │
│  │                                                     │   │
│  │  Query Types:                                       │   │
│  │  [×] A  [×] AAAA  [ ] CNAME  [×] PTR  [ ] MX  ...  │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─ Upstream Servers ──────────────────────────────────┐   │
│  │  ┌───────────────────────────────────────────────┐  │   │
│  │  │ 10.0.0.1:53                                   │  │   │
│  │  │ 10.0.0.2:53                                   │  │   │
│  │  └───────────────────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  [×] Enable this rule                                       │
│                                                             │
│                      [Cancel]  [Save Rule]                  │
└─────────────────────────────────────────────────────────────┘
```

#### Features
- **Priority-Based Ordering**: Visual indication of evaluation order
- **Multiple Matchers**: Combine domains, client IPs, and query types
- **Enable/Disable**: Per-rule and global feature toggle
- **Duplicate**: Clone existing rule for similar configs
- **Reorder**: Drag-and-drop to change priority
- **Test**: Test if a query would match this rule
- **Statistics**: Track queries matched per rule
- **Validation**: Ensure at least one matcher and one upstream

#### API Endpoints
```
GET  /api/conditional-forwarding           - List all rules
POST /api/conditional-forwarding           - Create rule
PUT  /api/conditional-forwarding/:name     - Update rule
DELETE /api/conditional-forwarding/:name   - Delete rule
PUT  /api/conditional-forwarding/toggle    - Enable/disable feature
POST /api/conditional-forwarding/reorder   - Change rule priorities
POST /api/conditional-forwarding/test      - Test if query matches rule
```

#### Implementation Checklist
- [ ] Create `pkg/api/handlers_conditional_forwarding.go`
  - [ ] Full CRUD operations
  - [ ] Priority management
  - [ ] Rule validation (matchers, upstreams)
  - [ ] Feature toggle
  - [ ] Test endpoint
- [ ] Create `pkg/api/ui/templates/conditional_forwarding.html`
  - [ ] Rules list with priority indicators
  - [ ] Add/edit modal with dynamic conditions
  - [ ] Test modal
- [ ] Create `pkg/api/ui/static/js/forwarding-init.js`
  - [ ] Drag-and-drop reordering
  - [ ] Dynamic condition builder
  - [ ] Modal management
- [ ] Add navigation (as page or Settings tab)
- [ ] Update config persistence
- [ ] Add metrics for rule matching

## Architecture Considerations

### Configuration Persistence
**Current:** Most config is read-only, requiring manual editing of `config.yml`

**Proposed:** Runtime updates with automatic config file writing

```go
// pkg/config/writer.go
type ConfigWriter struct {
    path    string
    mu      sync.Mutex
    current *Config
}

func (w *ConfigWriter) UpdateWhitelist(domains []string) error
func (w *ConfigWriter) UpdateLocalRecords(records []LocalRecordEntry) error
func (w *ConfigWriter) UpdateConditionalForwarding(rules []ForwardingRule) error
```

**Options:**
1. **Direct File Writing**: Update YAML directly (requires careful formatting)
2. **Hybrid**: Store in DB, export to YAML on change
3. **Runtime Only**: Changes lost on restart (not recommended)

**Recommendation:** Direct file writing with atomic writes and backups

### Hot Reload
When config changes via UI:
1. Write to `config.yml`
2. Reload affected subsystems without full restart
3. Validate before applying
4. Rollback on error

```go
func (h *Handler) ReloadWhitelist(newList []string) error
func (h *Handler) ReloadLocalRecords(records []LocalRecordEntry) error
func (h *Handler) ReloadConditionalForwarding(rules []ForwardingRule) error
```

### Metrics & Observability
Track usage of each feature:
```
dns_whitelist_matches_total{domain="example.com"}
dns_local_record_queries_total{domain="router.local",type="A"}
dns_conditional_forwarding_matches_total{rule="Corporate VPN"}
```

## Implementation Phases

### Phase 1: Whitelist Management (Simplest)
**Effort:** 1-2 days
- Single list of domains
- Simple CRUD operations
- No complex validation

**Deliverables:**
- Whitelist page
- API endpoints
- Config persistence
- Basic metrics

### Phase 2: Local Records Management (Medium)
**Effort:** 3-4 days
- Multiple record types with different fields
- Type-specific validation
- Dynamic forms

**Deliverables:**
- Local records page
- Type-aware forms
- Import/export functionality
- Testing capability

### Phase 3: Conditional Forwarding (Most Complex)
**Effort:** 4-5 days
- Multiple matchers (domains, IPs, types)
- Priority ordering
- Complex validation
- Rule testing

**Deliverables:**
- Forwarding rules page
- Priority management
- Test framework
- Reordering UI

### Phase 4: Polish & Integration
**Effort:** 2-3 days
- Navigation updates
- Consistent UI/UX across all features
- Comprehensive testing
- Documentation

## UI/UX Consistency

### Design System
Use existing patterns from Policies page:

**Cards:**
```css
.whitelist-item, .local-record-card, .forwarding-rule-card {
    /* Mirror .policy-card styling */
    background: var(--surface-card-gradient);
    border-radius: 0.75rem;
    padding: 1.5rem;
    box-shadow: var(--shadow);
}
```

**Modals:**
```css
.add-whitelist-modal, .edit-record-modal, .edit-rule-modal {
    /* Use existing .modal styles */
}
```

**Buttons:**
- Primary: "Save", "Add"
- Secondary: "Cancel", "Reset"
- Danger: "Delete", "Remove"

### Navigation Updates
**Base Template (`base.html:51-57`):**
```html
<div class="nav-links">
    <a href="/" class="nav-link">Dashboard</a>
    <a href="/queries" class="nav-link">Queries</a>
    <a href="/policies" class="nav-link">Policies</a>
    <a href="/clients" class="nav-link">Clients</a>
    <a href="/blocklists" class="nav-link">Blocklists</a>
    <!-- NEW -->
    <a href="/whitelist" class="nav-link">Whitelist</a>
    <a href="/local-records" class="nav-link">Local Records</a>
    <a href="/forwarding" class="nav-link">Forwarding</a>
    <!-- /NEW -->
    <a href="/settings" class="nav-link">Settings</a>
    <!-- ... -->
</div>
```

## Testing Strategy

### Unit Tests
- [ ] Config parsing/writing
- [ ] Validation logic for each feature
- [ ] API handler logic

### Integration Tests
- [ ] End-to-end CRUD operations
- [ ] Config persistence and reload
- [ ] Feature interaction (whitelist + policies, local records + forwarding)

### UI Tests
- [ ] Form validation
- [ ] Modal workflows
- [ ] HTMX partial updates

## Documentation Requirements

### User Documentation
- [ ] `/docs/guide/whitelist-management.md`
- [ ] `/docs/guide/local-records.md`
- [ ] `/docs/guide/conditional-forwarding.md`

### API Documentation
- [ ] Update `/docs/api/rest-api.md` with new endpoints
- [ ] OpenAPI spec updates

### Migration Guide
- [ ] How to migrate from YAML-only config to UI management
- [ ] Backup recommendations

## Security Considerations

### Input Validation
- **Domains**: Validate DNS name format, prevent injection
- **IP Addresses**: Validate IPv4/IPv6 format
- **CIDRs**: Validate CIDR notation
- **Upstreams**: Validate host:port format

### Authorization
- All config-modifying endpoints require authentication
- Consider admin-only access for sensitive features

### Audit Logging
```
[INFO] Admin user@example.com added domain 'example.com' to whitelist
[INFO] Admin user@example.com created local record: router.local A 192.168.1.1
[WARN] Admin user@example.com deleted forwarding rule 'Corporate VPN'
```

## Performance Impact

### Memory
- **Whitelist**: Negligible (list of strings)
- **Local Records**: Low (in-memory map of records)
- **Conditional Forwarding**: Low (small list of rules with compiled matchers)

### Latency
- **Whitelist Check**: O(log n) with sorted list or map
- **Local Record Lookup**: O(1) with map
- **Conditional Forwarding**: O(n) for rule iteration, but n is small (<50 rules typically)

## Risks & Mitigation

### Risk 1: Config File Corruption
**Mitigation:**
- Atomic writes with temp files
- Automatic backups before writes
- Validation before writing

### Risk 2: Breaking Changes
**Mitigation:**
- Maintain backward compatibility
- Validate existing config on startup
- Provide migration scripts

### Risk 3: Complex UI
**Mitigation:**
- Progressive disclosure (start simple, add advanced options)
- Inline help text
- Example presets

## Success Criteria

✅ **Feature Complete:**
- All three features fully implemented with CRUD operations
- UI matches existing design system
- Config changes persist and reload without restart

✅ **Quality:**
- 90%+ test coverage
- No performance regression
- Zero config corruption incidents

✅ **Usability:**
- Users can manage all features without touching YAML
- Clear documentation
- Intuitive workflows

## Estimated Timeline

**Total: 10-14 days**

```
Week 1:
  Day 1-2:  Whitelist Management
  Day 3-6:  Local Records Management
  Day 7:    Buffer/catch-up

Week 2:
  Day 8-12: Conditional Forwarding
  Day 13-14: Polish, integration, documentation
```

## Conclusion

These three features represent the last major gap between the powerful YAML configuration and the Web UI. Implementing them will:

1. **Improve Usability**: Users can manage everything from the UI
2. **Reduce Errors**: Visual validation prevents config mistakes
3. **Increase Adoption**: Lower barrier to entry for new users
4. **Feature Parity**: UI finally matches backend capabilities

**Recommendation: Implement all three features** in phases to maintain focus and quality.

**Priority Order:**
1. Whitelist (quickest win, high value)
2. Conditional Forwarding (complex but powerful)
3. Local Records (nice-to-have for advanced users)
