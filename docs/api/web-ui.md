# Web UI User Guide

Complete guide to using the Glory-Hole DNS Server web interface. All statistics and charts are live data.

## Accessing the Web UI

Open your browser and navigate to:

```
http://localhost:8080
```

Or if running on a different machine:

```
http://<server-ip>:8080
```

Configure the port in `config.yml`:
```yaml
server:
  web_ui_address: ":8080"  # Change port if needed
```

## Pages Overview

| Page | URL | Purpose |
|------|-----|---------|
| Dashboard | `/` | Real-time statistics and charts |
| Query Log | `/queries` | Live DNS query history |
| Policies | `/policies` | Manage filtering rules |
| Settings | `/settings` | Configuration and system info |

## Dashboard

**URL:** `http://localhost:8080/`

### Statistics Cards

Four real-time statistics cards at the top:

1. **Total Queries**
   - Total DNS queries since server start
   - Updates every 5 seconds
   - Shows percentage change

2. **Blocked Queries**
   - Number of blocked queries
   - Block rate percentage
   - Red indicator shows blocking ratio

3. **Cached Queries**
   - Cache hits
   - Cache hit rate percentage
   - Green indicator shows cache efficiency

4. **Average Response Time**
   - Average query latency in milliseconds
   - Lower is better (< 10ms is excellent)

### Query Activity Chart

- Line chart showing query volume over time
- X-axis: Time (last 24 hours)
- Y-axis: Query count
- Blue line: Total queries
- Red line: Blocked queries
- Updates every 30 seconds
- Hover over points for exact values

### Top Domains

Two side-by-side lists:

**Top Allowed Domains:**
- Most frequently queried allowed domains
- Shows domain name and query count
- Useful for identifying frequently accessed sites

**Top Blocked Domains:**
- Most frequently blocked domains
- Shows domain name and block count
- Useful for understanding what's being filtered

### Recent Queries

- Last 10 queries
- Shows timestamp, domain, status (allowed/blocked/cached)
- Auto-refreshes every 5 seconds
- Click "View All" to go to full query log

## Query Log

**URL:** `http://localhost:8080/queries`

### Features

**Real-time Updates:**
- Auto-refreshes every 2 seconds
- New queries appear at the top
- Smooth animations for new entries

**Query Information:**
- **Timestamp**: When query was made
- **Client IP**: Source IP address
- **Domain**: Queried domain name
- **Query Type**: A, AAAA, CNAME, etc.
- **Status Badge**: Color-coded status
- **Response Time**: Latency in milliseconds

**Status Badges:**
- **Green (Allowed)**: Query was allowed and forwarded
- **Red (Blocked)**: Query was blocked by filter
- **Blue (Cached)**: Response served from cache

### Filtering

Use the search box to filter queries:

```
# Filter by domain
facebook.com

# Filter by IP
192.168.1.100

# Filter by query type
AAAA
```

### Pagination

- Shows 50 queries per page
- Use "Previous" and "Next" buttons to navigate
- Page number shown at bottom

### Actions

**Refresh Button:**
- Manually refresh query list
- Useful if auto-refresh is paused

**Clear Filters:**
- Reset all filters
- Show all queries again

## Policy Management

**URL:** `http://localhost:8080/policies`

### Policy List

Displays all policy rules as cards:

**Card Information:**
- Policy name
- Expression logic
- Action (BLOCK/ALLOW/REDIRECT)
- Enable/disable toggle
- Edit and delete buttons

**Card Colors:**
- Gray header: Disabled policy
- Blue header: ALLOW action
- Red header: BLOCK action
- Orange header: REDIRECT action

### Adding a Policy

1. Click "Add Policy" button (top right)
2. Fill in the form:

   **Name:**
   - Descriptive name for the rule
   - Example: "Block social media during work hours"

   **Logic:**
   - Expression to evaluate
   - Example: `Hour >= 9 && Hour < 17 && DomainMatches(Domain, "facebook")`
   - See expression examples below

   **Action:**
   - `BLOCK`: Return NXDOMAIN
   - `ALLOW`: Bypass blocklist
   - `REDIRECT`: Redirect to IP (future)

   **Enabled:**
   - Toggle to enable/disable immediately

3. Click "Save Policy"

### Editing a Policy

1. Click "Edit" button on policy card
2. Modify fields as needed
3. Click "Update Policy"

Note: Editing recalculates rule order (last)

### Deleting a Policy

1. Click "Delete" button on policy card
2. Confirm deletion in popup
3. Policy is removed immediately

### Enable/Disable Toggle

- Use toggle switch on card
- Changes apply immediately
- Disabled rules are skipped during evaluation

### Expression Builder

**Context Fields:**
| Field | Type | Example |
|-------|------|---------|
| `Domain` | string | `"example.com"` |
| `ClientIP` | string | `"192.168.1.50"` |
| `QueryType` | string | `"A"`, `"AAAA"` |
| `Hour` | int | `14` (2 PM) |
| `Minute` | int | `30` |
| `Day` | int | `15` |
| `Month` | int | `6` (June) |
| `Weekday` | int | `0` (Sunday) - `6` (Saturday) |

**Helper Functions:**

Domain Matching:
```javascript
DomainMatches(Domain, "pattern")      // Contains check
DomainEndsWith(Domain, ".com")        // Suffix check
DomainStartsWith(Domain, "www")       // Prefix check
DomainRegex(Domain, "regex")          // Regex match
```

IP Matching:
```javascript
IPInCIDR(ClientIP, "192.168.1.0/24") // CIDR range
IPEquals(ClientIP, "192.168.1.100")   // Exact match
```

Time Functions:
```javascript
IsWeekend(Weekday)                    // Sat/Sun
InTimeRange(Hour, Minute, 9, 0, 17, 0) // 9 AM - 5 PM
```

**Example Expressions:**

Block social media after hours:
```javascript
(Hour >= 22 || Hour < 6) && (
  DomainMatches(Domain, "facebook") ||
  DomainMatches(Domain, "twitter") ||
  DomainMatches(Domain, "instagram")
)
```

Allow admin subnet:
```javascript
IPInCIDR(ClientIP, "192.168.100.0/24")
```

Block gaming on weekdays:
```javascript
(Weekday >= 1 && Weekday <= 5) &&
DomainMatches(Domain, "steam")
```

Time-based restrictions:
```javascript
InTimeRange(Hour, Minute, 9, 0, 17, 30) &&
DomainEndsWith(Domain, ".game.com")
```

### Rule Order

- Rules are evaluated top to bottom
- First matching rule wins
- Reorder by deleting and re-adding
- Disabled rules are skipped

## Settings Page

**URL:** `http://localhost:8080/settings`

### System Information

**Server Info:**
- Version number
- Uptime
- Build time

**Current Configuration:**
- DNS server address
- Web UI address
- Log level
- Database backend

### Blocklist Management

**Blocklist Information:**
- Number of domains blocked
- Last update time
- Auto-update status

**Actions:**
- **Reload Blocklists**: Manual trigger
- Shows loading spinner during reload
- Updates domain count on success

### Cache Statistics

- Current cache size
- Maximum cache size
- Cache hit rate
- Eviction stats

### Database Information

- Backend type (SQLite; D1 deferred for future release)
- Database path
- Retention period
- Total queries logged

### Configuration Review

Read-only view of current configuration:

**Server:**
- Listen address
- TCP/UDP status
- Web UI address

**Cache:**
- Enabled status
- Max entries
- TTL settings

**Database:**
- Backend
- Buffer size
- Flush interval
- Retention days

**Telemetry:**
- Prometheus enabled
- Prometheus port
- Tracing enabled

### Whitelist Management

**Access:** Click "Whitelist" in Settings sidebar

Manage domains that should never be blocked, even if they appear in blocklists. Whitelist entries can be exact domains, wildcard patterns, or regular expressions.

**Current Whitelist Display:**
- Shows all whitelisted domains organized by type:
  - **Exact Matches**: Precise domain names (e.g., `analytics.google.com`)
  - **Wildcard Patterns**: Domains with wildcards (e.g., `*.cdn.example.com`)
  - **Regex Patterns**: Regular expression patterns (e.g., `^api\d+\.example\.com$`)
- Each entry shows a delete button (×) for removal
- Total count displayed at top

**Adding Whitelist Entries:**

1. Click "Add Domain" button
2. Enter domain or pattern in the input field:
   - **Exact domain**: `google-analytics.com`
   - **Wildcard pattern**: `*.taskassist-pa.clients6.google.com` (matches any subdomain)
   - **Regex pattern**: `^cdn\d+\.example\.com$` (matches cdn1, cdn2, etc.)
3. Click "Add" button
4. Success message displays with updated total count
5. Entry appears in appropriate section

**Removing Whitelist Entries:**
1. Click the × button next to any domain
2. Confirmation message displays
3. Domain is removed from whitelist
4. Total count updates

**Use Cases:**
- Allow analytics services that blocklists incorrectly flag
- Whitelist CDN domains that break websites when blocked
- Allow specific subdomains while keeping parent domain blocked
- Use regex for dynamic subdomain patterns

**Notes:**
- Whitelist has highest priority - overrides all blocklists and policies
- Changes persist to config file (if running with `--config`)
- Wildcard patterns support `*` for subdomain matching
- Regex patterns use Go regular expression syntax

### Local DNS Records

**Access:** Click "Local Records" in Settings sidebar

Create custom DNS records for your local network. Define A (IPv4), AAAA (IPv6), and CNAME records that resolve without querying upstream DNS servers.

**Records List Display:**
- Shows all configured local DNS records in a table
- Columns: Domain, Type, Value, TTL, Actions
- Organized by domain name
- Each record has a delete button (×)
- Total count displayed at top

**Record Types:**

**A Record (IPv4):**
- Maps domain name to IPv4 address(es)
- Example: `nas.local` → `192.168.1.100`
- Supports multiple IPs for round-robin load balancing

**AAAA Record (IPv6):**
- Maps domain name to IPv6 address(es)
- Example: `server.local` → `2001:db8::1`
- Supports multiple IPs for load balancing

**CNAME Record (Alias):**
- Creates an alias pointing to another domain
- Example: `www.local` → `nas.local`
- Target domain can be local or external

**Adding Local Records:**

1. Click "Add Record" button
2. Fill in the form:
   - **Domain**: Enter the domain name (e.g., `nas.local`)
     - Trailing dot (`.`) is optional, will be added automatically
     - Can include subdomains (e.g., `www.nas.local`)
   - **Type**: Select record type from dropdown:
     - `A` - IPv4 address
     - `AAAA` - IPv6 address
     - `CNAME` - Domain alias
   - **IP Addresses** (for A/AAAA records):
     - Enter one or more IP addresses
     - Multiple IPs enable round-robin DNS (load balancing)
     - Click "+ Add IP" to add additional addresses
     - Validates IP format (IPv4 for A, IPv6 for AAAA)
   - **Target Domain** (for CNAME records):
     - Enter the domain this alias should point to
     - Must be a valid domain name
   - **TTL** (Time-to-Live):
     - Seconds to cache the DNS response
     - Default: 300 seconds (5 minutes)
     - Range: 60-86400 seconds (1 minute to 24 hours)
3. Click "Save" button
4. Record is validated and added to configuration
5. Success message displays
6. Record appears in the list

**Removing Local Records:**
1. Click the × button next to any record
2. Confirmation message displays
3. Record is removed from configuration
4. List updates to show remaining records

**Common Use Cases:**

**Home Network:**
```
nas.local        → 192.168.1.100  (A)
router.local     → 192.168.1.1    (A)
printer.local    → 192.168.1.10   (A)
www.nas.local    → nas.local      (CNAME)
```

**Development Environment:**
```
dev.local        → 127.0.0.1      (A)
api.dev.local    → 127.0.0.1      (A)
db.dev.local     → 192.168.1.50   (A)
```

**Load Balancing (Round-Robin):**
```
web.local → 192.168.1.10, 192.168.1.11, 192.168.1.12 (A)
```
Clients will receive different IPs in rotating order.

**IPv6 Network:**
```
server.local → 2001:db8::1       (AAAA)
nas.local    → 2001:db8::100     (AAAA)
```

**Notes:**
- Local records have highest priority (resolved before upstream)
- Changes persist to config file (if running with `--config`)
- Supports wildcard domains (configure in YAML, not via UI)
- CNAME records cannot coexist with A/AAAA for the same domain
- DNS cache applies to local records based on configured TTL

### Conditional Forwarding

**Access:** Click "Conditional Forwarding" in Settings sidebar

Route specific DNS queries to designated upstream DNS servers based on domain patterns, client IP ranges, or query types. Essential for split-horizon DNS in corporate networks, VPNs, and multi-site configurations.

**Rules List Display:**
- Shows all conditional forwarding rules sorted by priority
- Displays: Name, Matchers, Upstreams, Priority, Status, Actions
- Color-coded priority badges (red=high, yellow=medium, blue=low)
- Enabled/disabled toggle for each rule
- Each rule has an edit and delete button
- Total count displayed at top

**Rule Components:**

**Matchers (at least one required):**
- **Domains**: Domain name patterns (e.g., `*.local`, `*.corp.example.com`)
- **Client CIDRs**: IP ranges in CIDR notation (e.g., `192.168.1.0/24`, `10.0.0.0/8`)
- **Query Types**: DNS query types (e.g., `A`, `AAAA`, `PTR`, `MX`)

**Upstreams:**
- List of DNS servers to forward matching queries to
- Format: `IP:port` (e.g., `10.0.0.1:53`)
- Supports multiple servers for redundancy

**Priority:**
- Integer from 1 to 100 (higher = evaluated first)
- Default: 50 (medium priority)
- Higher priority rules are checked before lower priority
- Use 80-100 for critical/specific rules
- Use 50-79 for general rules
- Use 1-49 for fallback rules

**Advanced Options:**
- **Timeout**: Query timeout duration (e.g., `2s`, `500ms`)
- **Max Retries**: Number of retry attempts on failure (0-5)
- **Failover**: Try next upstream server if one fails

**Adding Conditional Forwarding Rules:**

1. Click "Add Rule" button
2. Fill in the form:

   **Basic Information:**
   - **Rule Name**: Descriptive name (e.g., "Corporate VPN DNS")
     - Must be unique
     - Used for identification in logs and UI

   **Matching Conditions (select at least one):**

   - **Domain Patterns**:
     - Click "+ Add Domain" to add patterns
     - Wildcards supported: `*.local`, `*.corp.example.com`
     - Exact domains: `intranet.company.local`
     - Multiple patterns evaluated with OR logic

   - **Client IP Ranges (CIDR)**:
     - Click "+ Add CIDR" to add ranges
     - Examples: `192.168.1.0/24`, `10.0.0.0/8`, `172.16.0.0/12`
     - Matches queries from clients in these ranges
     - Multiple CIDRs evaluated with OR logic

   - **Query Types**:
     - Click "+ Add Type" to add types
     - Common types: `A`, `AAAA`, `PTR`, `MX`, `TXT`, `SRV`
     - Multiple types evaluated with OR logic

   **Upstream DNS Servers:**
   - Click "+ Add Upstream" to add servers
   - Format: `IP:port` (e.g., `10.0.0.1:53`)
   - At least one upstream required
   - Multiple upstreams provide redundancy
   - Queried in order unless failover is enabled

   **Priority:**
   - Enter value from 1-100
   - Default: 50
   - Higher values evaluated first

   **Advanced Settings (optional):**
   - **Timeout**: Query timeout (e.g., `2s`, `3s`, `500ms`)
   - **Max Retries**: Retry attempts (0-5)
   - **Failover**: Enable to try next upstream on failure

3. Click "Save Rule" button
4. Rule is validated and added to configuration
5. Success message displays
6. Rule appears in the list sorted by priority

**Editing Rules:**
1. Click the edit button (pencil icon) next to a rule
2. Modify any settings in the form
3. Click "Save Changes"
4. Rule is updated in configuration

**Removing Rules:**
1. Click the delete button (×) next to a rule
2. Confirmation prompt appears
3. Click "Confirm" to remove
4. Rule is deleted from configuration

**Enabling/Disabling Rules:**
1. Click the toggle switch next to a rule
2. Rule status updates immediately
3. Disabled rules are skipped during query processing
4. Useful for temporary rule suspension without deletion

**Common Configurations:**

**Local Network DNS:**
```
Name: Local Network Domains
Domains: *.local, *.lan
Upstreams: 192.168.1.1:53
Priority: 80
```

**Corporate VPN:**
```
Name: Corporate Network
Domains: *.corp.example.com, *.internal
Upstreams: 10.0.0.1:53, 10.0.0.2:53
Priority: 90
Timeout: 3s
Failover: Yes
```

**Reverse DNS (PTR queries):**
```
Name: Reverse DNS Lookups
Query Types: PTR
Upstreams: 10.0.0.1:53
Priority: 70
```

**VPN Client Specific:**
```
Name: VPN User DNS
Client CIDRs: 172.16.0.0/12
Domains: *.vpn.corp
Upstreams: 172.16.0.1:53
Priority: 85
```

**Multi-Condition Rule:**
```
Name: Internal A/AAAA Records
Domains: *.internal.local
Client CIDRs: 10.0.0.0/8
Query Types: A, AAAA
Upstreams: 10.0.0.1:53, 10.0.0.2:53
Priority: 95
Timeout: 2s
Max Retries: 2
Failover: Yes
```

**Rule Evaluation:**
1. Rules are evaluated in priority order (highest first)
2. First matching rule handles the query
3. If match conditions are met, query is forwarded to rule's upstreams
4. If no rules match, query goes to default upstream servers
5. Disabled rules are skipped

**Notes:**
- Changes persist to config file (if running with `--config`)
- Rule order matters - highest priority evaluated first
- Multiple matchers within a rule use OR logic
- Use specific rules (high priority) before general rules
- Failover requires multiple upstreams
- Empty timeout uses default forwarder timeout (2s)
- Maximum retries default is 0 (no retries)

## Mobile Support

The Web UI is fully responsive:

**Phone (< 768px):**
- Single column layout
- Stacked cards
- Hamburger menu
- Optimized touch targets

**Tablet (768px - 1024px):**
- Two-column layout
- Larger cards
- Touch-friendly buttons

**Desktop (> 1024px):**
- Full multi-column layout
- Sidebar navigation
- Keyboard shortcuts

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `/` | Focus search box |
| `r` | Refresh current page |
| `Esc` | Close modals |
| `n` | Next page (pagination) |
| `p` | Previous page |

## Browser Compatibility

**Supported Browsers:**
- Chrome 90+
- Firefox 88+
- Safari 14+
- Edge 90+

**Features Used:**
- CSS Grid and Flexbox
- Fetch API
- ES6+ JavaScript
- HTMX for dynamic updates

**Not Supported:**
- Internet Explorer (any version)
- Chrome < 90
- Firefox < 88

## Customization

### Theme Colors

Colors are defined in `/static/css/style.css`:

```css
:root {
  --primary-color: #3498db;
  --success-color: #2ecc71;
  --danger-color: #e74c3c;
  --warning-color: #f39c12;
  --dark-bg: #2c3e50;
  --light-bg: #ecf0f1;
}
```

### Auto-Refresh Intervals

Configured in page JavaScript:

**Dashboard:**
- Stats cards: 5 seconds
- Chart: 30 seconds
- Recent queries: 5 seconds

**Query Log:**
- Query list: 2 seconds

To modify, edit template files in `pkg/api/ui/templates/`

### Page Size

Change queries per page in `handleQueriesPage`:

```go
// Default: 50
limit := 50
```

## Troubleshooting

### Web UI not loading

1. Check server is running:
   ```bash
   curl http://localhost:8080/api/health
   ```

2. Check firewall:
   ```bash
   sudo ufw allow 8080/tcp
   ```

3. Verify port in config:
   ```yaml
   server:
     web_ui_address: ":8080"
   ```

### Data not showing

1. Verify database is enabled:
   ```yaml
   database:
     enabled: true
   ```

2. Check API endpoints:
   ```bash
   curl http://localhost:8080/api/stats
   curl http://localhost:8080/api/queries
   ```

3. Make some DNS queries:
   ```bash
   dig @localhost google.com
   ```

### Auto-refresh not working

1. Check JavaScript console for errors (F12)
2. Verify HTMX is loading (check Network tab)
3. Disable browser ad-blockers
4. Try hard refresh (Ctrl+Shift+R)

### Performance issues

If Web UI is slow:

1. Reduce query limit:
   - Default shows 50 queries
   - Lower if needed

2. Disable auto-refresh temporarily:
   - Remove `hx-trigger` attributes

3. Check database performance:
   - Enable WAL mode
   - Increase cache size

## API Integration

The Web UI uses the REST API extensively. You can integrate with:

**JavaScript:**
```javascript
// Get statistics
fetch('http://localhost:8080/api/stats')
  .then(res => res.json())
  .then(data => console.log(data));

// Add policy
fetch('http://localhost:8080/api/policies', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({
    name: 'My Rule',
    logic: 'Hour >= 22',
    action: 'BLOCK',
    enabled: true
  })
});
```

**Python:**
```python
import requests

# Get queries
r = requests.get('http://localhost:8080/api/queries')
queries = r.json()

# Reload blocklists
r = requests.post('http://localhost:8080/api/blocklist/reload')
print(r.json())
```

See [REST API Reference](rest-api.md) for complete documentation.
