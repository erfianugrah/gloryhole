# Web UI User Guide

Complete guide to using the Glory-Hole DNS Server web interface.

> **Note**: The dashboard activity chart currently displays mock data for visualization purposes. Real-time chart integration is planned for a future release. All other statistics and features display live data from the DNS server.

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

- Backend type (SQLite/D1)
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
