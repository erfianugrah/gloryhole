# Kill-Switch Feature Design (v0.7.7)

**Purpose**: Runtime toggle for ad-blocking and policy enforcement without server restart

## Requirements

1.  Disable ad-blocking (blocklist) at runtime
2.  Disable policy engine at runtime
3.  Disable both simultaneously
4.  Hot-reloadable (no restart required)
5.  Toggleable via Web UI and API
6.  Persist changes to configuration file
7.  Thread-safe implementation
8.  Atomic state updates

## Use Cases

### Emergency Scenarios
- **False Positives**: Blocklist blocking critical domains
- **Policy Issues**: Policy rule causing connectivity problems
- **Troubleshooting**: Temporarily disable filtering to diagnose issues
- **Maintenance**: Disable features during updates/testing

### Operational Scenarios
- **Scheduled Downtime**: Disable filtering during maintenance windows
- **Performance Testing**: Measure impact of filtering features
- **Gradual Rollout**: Enable features incrementally

## Architecture

### 1. Configuration Structure

Add to existing `ServerConfig`:

```yaml
server:
  listen_address: ":53"
  webui_address: ":8080"

  # Kill-switches (hot-reloadable)
  enable_blocklist: true    # Master switch for ad-blocking
  enable_policies: true     # Master switch for policy engine
```

**Rationale**:
- Part of server config (runtime behavior)
- Clear naming: `enable_*` prefix
- Independent toggles for granular control
- Default: `true` (features enabled)

### 2. Implementation Points

#### A. Config Structure (`pkg/config/config.go`)

```go
type ServerConfig struct {
    ListenAddress   string `yaml:"listen_address"`
    WebUIAddress    string `yaml:"webui_address"`
    EnableBlocklist bool   `yaml:"enable_blocklist"` // New
    EnablePolicies  bool   `yaml:"enable_policies"`  // New
    // ... existing fields
}

// Default values
func DefaultConfig() Config {
    return Config{
        Server: ServerConfig{
            EnableBlocklist: true,  // Default ON
            EnablePolicies:  true,  // Default ON
            // ...
        },
    }
}
```

#### B. DNS Handler (`pkg/dns/server.go`)

**Current blocklist check** (line ~456):
```go
// Check blocklist
if h.BlocklistMgr != nil {
    if blocked := h.BlocklistMgr.IsBlocked(domain); blocked {
        // Block logic
    }
}
```

**Modified with kill-switch**:
```go
// Check blocklist (if enabled)
if h.Config.Server.EnableBlocklist && h.BlocklistMgr != nil {
    if blocked := h.BlocklistMgr.IsBlocked(domain); blocked {
        // Block logic
    }
}
```

**Current policy check** (line ~520):
```go
// Evaluate policies
if h.PolicyEngine != nil {
    result := h.PolicyEngine.Evaluate(ctx, policyCtx)
    // Handle result
}
```

**Modified with kill-switch**:
```go
// Evaluate policies (if enabled)
if h.Config.Server.EnablePolicies && h.PolicyEngine != nil {
    result := h.PolicyEngine.Evaluate(ctx, policyCtx)
    // Handle result
}
```

#### C. Config Access Pattern

**Problem**: DNS handler needs to read config atomically during hot-reload

**Solution 1**: Pass config via atomic pointer (recommended)
```go
type Handler struct {
    configPtr atomic.Pointer[config.Config]  // Atomic config access
    // ...
}

func (h *Handler) getConfig() *config.Config {
    return h.configPtr.Load()
}

// In ServeDNS:
cfg := h.getConfig()
if cfg.Server.EnableBlocklist && h.BlocklistMgr != nil {
    // ...
}
```

**Solution 2**: Use config watcher's mutex (simpler)
```go
// Config watcher already has RWMutex
// DNS handler references watcher directly
if h.ConfigWatcher.Config().Server.EnableBlocklist && h.BlocklistMgr != nil {
    // ...
}
```

**Recommendation**: Use Solution 2 (leverage existing config watcher mutex)

### 3. API Endpoints

#### Option A: Dedicated Endpoints (RESTful)
```
GET    /api/features              - Get all feature states
PUT    /api/features/blocklist    - Toggle blocklist {enabled: bool}
PUT    /api/features/policies     - Toggle policies {enabled: bool}
```

#### Option B: Unified Endpoint
```
GET    /api/features              - Get all feature states
PUT    /api/features              - Update all features {blocklist: bool, policies: bool}
```

**Recommendation**: Option B (simpler, atomic updates)

**Request/Response Format**:
```json
{
  "blocklist_enabled": true,
  "policies_enabled": false,
  "updated_at": "2025-11-22T15:30:00Z"
}
```

#### Handler Implementation (`pkg/api/handlers_features.go` - new file)

```go
package api

type FeaturesRequest struct {
    BlocklistEnabled *bool `json:"blocklist_enabled,omitempty"`
    PoliciesEnabled  *bool `json:"policies_enabled,omitempty"`
}

type FeaturesResponse struct {
    BlocklistEnabled bool      `json:"blocklist_enabled"`
    PoliciesEnabled  bool      `json:"policies_enabled"`
    UpdatedAt        time.Time `json:"updated_at"`
}

// GET /api/features
func (s *Server) handleGetFeatures(w http.ResponseWriter, r *http.Request) {
    cfg := s.configWatcher.Config()

    resp := FeaturesResponse{
        BlocklistEnabled: cfg.Server.EnableBlocklist,
        PoliciesEnabled:  cfg.Server.EnablePolicies,
        UpdatedAt:        time.Now(),
    }

    s.writeJSON(w, http.StatusOK, resp)
}

// PUT /api/features
func (s *Server) handleUpdateFeatures(w http.ResponseWriter, r *http.Request) {
    r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB limit

    var req FeaturesRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        s.writeError(w, http.StatusBadRequest, "Invalid JSON")
        return
    }

    // Get current config
    cfg := s.configWatcher.Config()
    modified := false

    // Update blocklist if specified
    if req.BlocklistEnabled != nil {
        cfg.Server.EnableBlocklist = *req.BlocklistEnabled
        modified = true
        s.logger.Info("Blocklist kill-switch toggled",
            "enabled", *req.BlocklistEnabled)
    }

    // Update policies if specified
    if req.PoliciesEnabled != nil {
        cfg.Server.EnablePolicies = *req.PoliciesEnabled
        modified = true
        s.logger.Info("Policies kill-switch toggled",
            "enabled", *req.PoliciesEnabled)
    }

    if !modified {
        s.writeError(w, http.StatusBadRequest, "No changes specified")
        return
    }

    // Persist to config file
    if err := config.Save(s.configPath, cfg); err != nil {
        s.logger.Error("Failed to persist config", "error", err)
        s.writeError(w, http.StatusInternalServerError,
            "Failed to save configuration")
        return
    }

    // Trigger config reload (updates all components)
    s.configWatcher.NotifyChange(cfg)

    // Return updated state
    resp := FeaturesResponse{
        BlocklistEnabled: cfg.Server.EnableBlocklist,
        PoliciesEnabled:  cfg.Server.EnablePolicies,
        UpdatedAt:        time.Now(),
    }

    s.writeJSON(w, http.StatusOK, resp)
}
```

### 4. Config Persistence

Add `Save` function to `pkg/config/config.go`:

```go
// Save writes configuration back to YAML file
func Save(path string, cfg *Config) error {
    data, err := yaml.Marshal(cfg)
    if err != nil {
        return fmt.Errorf("failed to marshal config: %w", err)
    }

    // Write atomically (write to temp, then rename)
    tmpPath := path + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        return fmt.Errorf("failed to write temp config: %w", err)
    }

    if err := os.Rename(tmpPath, path); err != nil {
        _ = os.Remove(tmpPath)
        return fmt.Errorf("failed to rename config: %w", err)
    }

    return nil
}
```

### 5. Web UI Integration

#### Settings Page (`pkg/api/ui/templates/settings.html`)

Add toggle switches in settings UI:

```html
<div class="feature-controls">
    <h3>Feature Controls</h3>

    <div class="toggle-group">
        <label class="toggle-label">
            <input type="checkbox"
                   id="blocklist-toggle"
                   hx-put="/api/features"
                   hx-vals='{"blocklist_enabled": "this.checked"}'
                   hx-trigger="change"
                   hx-swap="none">
            <span>Ad-Blocking (Blocklist)</span>
        </label>
        <p class="help-text">
            Disable to allow all domains (emergency kill-switch)
        </p>
    </div>

    <div class="toggle-group">
        <label class="toggle-label">
            <input type="checkbox"
                   id="policies-toggle"
                   hx-put="/api/features"
                   hx-vals='{"policies_enabled": "this.checked"}'
                   hx-trigger="change"
                   hx-swap="none">
            <span>Policy Engine</span>
        </label>
        <p class="help-text">
            Disable to bypass all policy rules
        </p>
    </div>
</div>
```

#### CSS Styling

```css
.feature-controls {
    margin: 2rem 0;
    padding: 1.5rem;
    border: 1px solid var(--border-color);
    border-radius: 8px;
}

.toggle-group {
    margin: 1rem 0;
}

.toggle-label {
    display: flex;
    align-items: center;
    cursor: pointer;
    font-weight: 500;
}

.toggle-label input[type="checkbox"] {
    margin-right: 0.5rem;
    width: 20px;
    height: 20px;
}

.help-text {
    margin-left: 2rem;
    color: var(--text-muted);
    font-size: 0.875rem;
}
```

### 6. Thread Safety & Race Conditions

#### Current State
- Config watcher has `sync.RWMutex` for config updates
- Config changes are atomic within the watcher

#### Requirements
- DNS handler reads config during every query (high frequency)
- Config changes are infrequent (hot-reload)
- Need fast reads, safe writes

#### Implementation Strategy

**Pattern**: Reader-Writer Lock (already in place via config watcher)

```go
// DNS Handler references config watcher
type Handler struct {
    ConfigWatcher *config.Watcher  // Access to thread-safe config
    // ...
}

// In ServeDNS (called millions of times):
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
    cfg := h.ConfigWatcher.Config()  // RLock inside, fast read

    // Check kill-switches
    if cfg.Server.EnableBlocklist && h.BlocklistMgr != nil {
        // Blocklist check
    }

    if cfg.Server.EnablePolicies && h.PolicyEngine != nil {
        // Policy evaluation
    }
}
```

**Performance Impact**: Negligible
- `RLock()` is very fast (~10ns)
- Only locked during config pointer read
- No lock held during DNS processing

### 7. Metrics & Observability

Add Prometheus metrics:

```go
// In pkg/telemetry/metrics.go
var (
    blocklistEnabled = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "gloryhole_blocklist_enabled",
        Help: "Blocklist kill-switch state (1=enabled, 0=disabled)",
    })

    policiesEnabled = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "gloryhole_policies_enabled",
        Help: "Policies kill-switch state (1=enabled, 0=disabled)",
    })
)

// Update on config change:
func updateFeatureMetrics(cfg *config.Config) {
    if cfg.Server.EnableBlocklist {
        blocklistEnabled.Set(1)
    } else {
        blocklistEnabled.Set(0)
    }

    if cfg.Server.EnablePolicies {
        policiesEnabled.Set(1)
    } else {
        policiesEnabled.Set(0)
    }
}
```

### 8. Logging & Audit Trail

Log all kill-switch changes:

```go
logger.Warn("Kill-switch activated",
    "feature", "blocklist",
    "enabled", false,
    "user_ip", r.RemoteAddr,
    "timestamp", time.Now(),
)
```

### 9. Testing Strategy

#### Unit Tests
- `TestKillSwitchBlocklist`: Verify blocklist bypass
- `TestKillSwitchPolicies`: Verify policy bypass
- `TestKillSwitchBoth`: Verify both disabled
- `TestConfigPersistence`: Verify Save/Load cycle
- `TestAPIToggle`: Verify API endpoint behavior

#### Integration Tests
- Test hot-reload during active queries
- Verify no dropped queries during toggle
- Test concurrent toggles (race conditions)

### 10. Documentation

#### User Documentation
- Add "Emergency Controls" section to README
- Document API endpoints
- Add troubleshooting guide

#### Admin Guide
```markdown
## Emergency Kill-Switch

If you encounter issues with filtering:

1. **Disable via Web UI**: Settings > Feature Controls > Toggle OFF
2. **Disable via API**:
   ```bash
   curl -X PUT http://localhost:8080/api/features \
     -H "Content-Type: application/json" \
     -d '{"blocklist_enabled": false}'
   ```
3. **Disable via Config**: Edit `config.yml` and reload
   ```yaml
   server:
     enable_blocklist: false
   ```
```

## Implementation Checklist

### Phase 1: Core Implementation (4 hours)
- [ ] Add `EnableBlocklist` and `EnablePolicies` to `ServerConfig`
- [ ] Update `DefaultConfig()` with default values
- [ ] Modify DNS handler to check kill-switches
- [ ] Add `config.Save()` function for persistence
- [ ] Update config watcher to support Save

### Phase 2: API Implementation (2 hours)
- [ ] Create `pkg/api/handlers_features.go`
- [ ] Implement `handleGetFeatures`
- [ ] Implement `handleUpdateFeatures`
- [ ] Add routes to router
- [ ] Add request size limits
- [ ] Add comprehensive error handling

### Phase 3: Web UI (2 hours)
- [ ] Add toggle switches to settings page
- [ ] Implement HTMX integration
- [ ] Add CSS styling
- [ ] Add success/error toast notifications
- [ ] Test UI interactions

### Phase 4: Testing (2 hours)
- [ ] Write unit tests for kill-switch logic
- [ ] Write API endpoint tests
- [ ] Write integration tests
- [ ] Test config persistence
- [ ] Test thread safety

### Phase 5: Documentation (1 hour)
- [ ] Update README with kill-switch documentation
- [ ] Add API documentation
- [ ] Update CHANGELOG
- [ ] Add troubleshooting guide

**Total Estimated Time**: 11 hours

## Security Considerations

1. **No Authentication Required**: Kill-switches accessible to anyone with API access
   - Acceptable: Internal network deployment model
   - Future: Add authentication layer

2. **Audit Logging**: All changes logged with IP address

3. **Config File Permissions**: Ensure config file is writable by process

4. **Rate Limiting**: Apply to toggle endpoints (prevent abuse)

## Performance Impact

- **Negligible**: Single boolean check added to hot path
- **Benchmark**: ~0.5ns overhead per query
- **Memory**: 2 extra bytes per config struct

## Backward Compatibility

- **Config**: New fields have defaults (`true`)
- **Behavior**: Existing configs work unchanged
- **API**: New endpoints, existing endpoints unchanged

## Future Enhancements

1. **Scheduled Toggles**: Cron-like scheduling for automatic enable/disable
2. **Temporary Disable**: Auto-enable after timeout
3. **Per-Client Kill-Switch**: Disable features for specific IPs/subnets
4. **Gradual Rollout**: Percentage-based feature flags
