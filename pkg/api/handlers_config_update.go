package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"glory-hole/pkg/config"
)

const maxConfigPayloadSize = 64 * 1024 // 64KB
const (
	settingsTemplateRateLimit = "settings-rate-limit"
	settingsTemplateTLS       = "settings-tls"
	flashKeyRateLimit         = "rate_limit"
	flashKeyTLS               = "tls"
)

// handleUpdateUpstreams handles PUT /api/config/upstreams
func (s *Server) handleUpdateUpstreams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	cfg, err := s.mutableConfig()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxConfigPayloadSize)
	servers, err := parseUpstreamServers(r)
	if err != nil {
		s.respondConfigValidationError(w, r, settingsTemplateUpstreams, flashKeyUpstreams, err.Error(), cfg, http.StatusBadRequest)
		return
	}

	updated := *cfg
	updated.UpstreamDNSServers = servers
	if !s.persistConfigSection(w, r, &updated, settingsTemplateUpstreams, flashKeyUpstreams, cfg) {
		return
	}

	data := s.newSettingsPageData(cfg)
	s.respondConfigUpdate(w, r, settingsTemplateUpstreams, flashKeyUpstreams, "Upstream DNS servers updated", data)
}

// handleUpdateCache handles PUT /api/config/cache
func (s *Server) handleUpdateCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	cfg, err := s.mutableConfig()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxConfigPayloadSize)
	payload, err := parseCachePayload(r, cfg.Cache)
	if err != nil {
		s.respondConfigValidationError(w, r, settingsTemplateCache, flashKeyCache, err.Error(), cfg, http.StatusBadRequest)
		return
	}

	updated := *cfg
	updated.Cache.Enabled = payload.Enabled
	updated.Cache.MaxEntries = payload.MaxEntries
	updated.Cache.MinTTL = payload.MinTTL
	updated.Cache.MaxTTL = payload.MaxTTL
	updated.Cache.NegativeTTL = payload.NegativeTTL
	updated.Cache.BlockedTTL = payload.BlockedTTL
	updated.Cache.ShardCount = payload.ShardCount

	if !s.persistConfigSection(w, r, &updated, settingsTemplateCache, flashKeyCache, cfg) {
		return
	}

	data := s.newSettingsPageData(cfg)
	s.respondConfigUpdate(w, r, settingsTemplateCache, flashKeyCache, "Cache settings updated", data)
}

// handleUpdateLogging handles PUT /api/config/logging
func (s *Server) handleUpdateLogging(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	cfg, err := s.mutableConfig()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxConfigPayloadSize)
	payload, err := parseLoggingPayload(r, cfg.Logging)
	if err != nil {
		s.respondConfigValidationError(w, r, settingsTemplateLogging, flashKeyLogging, err.Error(), cfg, http.StatusBadRequest)
		return
	}

	updated := *cfg
	updated.Logging.Level = payload.Level
	updated.Logging.Format = payload.Format
	updated.Logging.Output = payload.Output
	updated.Logging.FilePath = payload.FilePath
	updated.Logging.AddSource = payload.AddSource
	updated.Logging.MaxSize = payload.MaxSize
	updated.Logging.MaxBackups = payload.MaxBackups
	updated.Logging.MaxAge = payload.MaxAge

	if !s.persistConfigSection(w, r, &updated, settingsTemplateLogging, flashKeyLogging, cfg) {
		return
	}

	data := s.newSettingsPageData(cfg)
	s.respondConfigUpdate(w, r, settingsTemplateLogging, flashKeyLogging, "Logging settings updated", data)
}

// handleUpdateRateLimit handles PUT /api/config/rate-limit
func (s *Server) handleUpdateRateLimit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	cfg, err := s.mutableConfig()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxConfigPayloadSize)
	payload, err := parseRateLimitPayload(r, cfg.RateLimit)
	if err != nil {
		s.respondConfigValidationError(w, r, settingsTemplateRateLimit, flashKeyRateLimit, err.Error(), cfg, http.StatusBadRequest)
		return
	}

	updated := *cfg
	updated.RateLimit.Enabled = payload.Enabled
	updated.RateLimit.RequestsPerSecond = payload.RequestsPerSecond
	updated.RateLimit.Burst = payload.Burst
	updated.RateLimit.Action = payload.Action
	updated.RateLimit.LogViolations = payload.LogViolations
	updated.RateLimit.CleanupInterval = payload.CleanupInterval
	updated.RateLimit.MaxTrackedClients = payload.MaxTrackedClients
	// Keep overrides/trusted proxies untouched for now

	if !s.persistConfigSection(w, r, &updated, settingsTemplateRateLimit, flashKeyRateLimit, cfg) {
		return
	}

	data := s.newSettingsPageData(cfg)
	s.respondConfigUpdate(w, r, settingsTemplateRateLimit, flashKeyRateLimit, "Rate limiting updated", data)
}

// handleUpdateTLS handles PUT /api/config/tls
func (s *Server) handleUpdateTLS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	cfg, err := s.mutableConfig()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxConfigPayloadSize)
	payload, err := parseTLSPayload(r, cfg.Server)
	if err != nil {
		s.respondConfigValidationError(w, r, settingsTemplateTLS, flashKeyTLS, err.Error(), cfg, http.StatusBadRequest)
		return
	}

	updated := *cfg
	updated.Server.DotEnabled = payload.DotEnabled
	updated.Server.DotAddress = payload.DotAddress
	updated.Server.TLS = payload.TLS

	if !s.persistConfigSection(w, r, &updated, settingsTemplateTLS, flashKeyTLS, cfg) {
		return
	}

	data := s.newSettingsPageData(cfg)
	s.respondConfigUpdate(w, r, settingsTemplateTLS, flashKeyTLS, "DoT/TLS settings updated", data)
}

func parseUpstreamServers(r *http.Request) ([]string, error) {
	type request struct {
		Servers []string `json:"servers"`
		Text    string   `json:"servers_text"`
	}

	var req request
	if isJSONContent(r) {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, fmt.Errorf("invalid JSON payload: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return nil, fmt.Errorf("invalid form payload: %w", err)
		}
		req.Text = r.FormValue("servers")
		if rawList := r.Form["servers[]"]; len(rawList) > 0 {
			req.Servers = append(req.Servers, rawList...)
		}
	}

	candidates := make([]string, 0, len(req.Servers)+4)
	for _, s := range req.Servers {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			candidates = append(candidates, trimmed)
		}
	}
	candidates = append(candidates, splitList(req.Text)...)

	if len(candidates) == 0 {
		return nil, fmt.Errorf("at least one upstream DNS server is required")
	}

	seen := make(map[string]struct{})
	result := make([]string, 0, len(candidates))
	for _, entry := range candidates {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, ":") {
			return nil, fmt.Errorf("server %q must include a port (e.g., 1.1.1.1:53)", entry)
		}
		if _, _, err := net.SplitHostPort(entry); err != nil {
			return nil, fmt.Errorf("invalid upstream server %q: %w", entry, err)
		}
		lower := strings.ToLower(entry)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		result = append(result, entry)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("at least one upstream DNS server is required")
	}

	return result, nil
}

type cachePayload struct {
	Enabled     bool
	MaxEntries  int
	MinTTL      time.Duration
	MaxTTL      time.Duration
	NegativeTTL time.Duration
	BlockedTTL  time.Duration
	ShardCount  int
}

func parseCachePayload(r *http.Request, current config.CacheConfig) (cachePayload, error) {
	type request struct {
		Enabled     *bool  `json:"enabled,omitempty"`
		MaxEntries  *int   `json:"max_entries,omitempty"`
		MinTTL      string `json:"min_ttl"`
		MaxTTL      string `json:"max_ttl"`
		NegativeTTL string `json:"negative_ttl"`
		BlockedTTL  string `json:"blocked_ttl"`
		ShardCount  *int   `json:"shard_count,omitempty"`
	}

	var req request
	if isJSONContent(r) {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return cachePayload{}, fmt.Errorf("invalid JSON payload: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return cachePayload{}, fmt.Errorf("invalid form payload: %w", err)
		}
		if val := r.FormValue("enabled"); val != "" {
			enabled := parseCheckbox(val)
			req.Enabled = &enabled
		} else {
			disabled := false
			req.Enabled = &disabled
		}
		req.MinTTL = r.FormValue("min_ttl")
		req.MaxTTL = r.FormValue("max_ttl")
		req.NegativeTTL = r.FormValue("negative_ttl")
		req.BlockedTTL = r.FormValue("blocked_ttl")

		maxEntriesRaw := strings.TrimSpace(r.FormValue("max_entries"))
		if maxEntriesRaw == "" {
			return cachePayload{}, fmt.Errorf("max_entries is required")
		}
		maxEntries, err := strconv.Atoi(maxEntriesRaw)
		if err != nil {
			return cachePayload{}, fmt.Errorf("max_entries must be a positive integer")
		}
		req.MaxEntries = &maxEntries

		shardsRaw := strings.TrimSpace(r.FormValue("shard_count"))
		if shardsRaw == "" {
			return cachePayload{}, fmt.Errorf("shard_count is required")
		}
		shardCount, err := strconv.Atoi(shardsRaw)
		if err != nil {
			return cachePayload{}, fmt.Errorf("shard_count must be a non-negative integer")
		}
		req.ShardCount = &shardCount
	}

	payload := cachePayload{
		Enabled: current.Enabled,
	}
	if req.Enabled != nil {
		payload.Enabled = *req.Enabled
	}

	if req.MaxEntries == nil {
		return cachePayload{}, fmt.Errorf("max_entries is required")
	}
	if *req.MaxEntries <= 0 {
		return cachePayload{}, fmt.Errorf("max_entries must be greater than zero")
	}
	payload.MaxEntries = *req.MaxEntries

	minTTL, err := parseDurationField(req.MinTTL, "min_ttl")
	if err != nil {
		return cachePayload{}, err
	}
	maxTTL, err := parseDurationField(req.MaxTTL, "max_ttl")
	if err != nil {
		return cachePayload{}, err
	}
	if maxTTL < minTTL {
		return cachePayload{}, fmt.Errorf("max_ttl must be greater than or equal to min_ttl")
	}
	negTTL, err := parseDurationField(req.NegativeTTL, "negative_ttl")
	if err != nil {
		return cachePayload{}, err
	}
	blockedTTL, err := parseDurationField(req.BlockedTTL, "blocked_ttl")
	if err != nil {
		return cachePayload{}, err
	}

	payload.MinTTL = minTTL
	payload.MaxTTL = maxTTL
	payload.NegativeTTL = negTTL
	payload.BlockedTTL = blockedTTL

	if req.ShardCount == nil {
		return cachePayload{}, fmt.Errorf("shard_count is required")
	}
	if *req.ShardCount < 0 {
		return cachePayload{}, fmt.Errorf("shard_count cannot be negative")
	}
	payload.ShardCount = *req.ShardCount

	return payload, nil
}

type rateLimitPayload struct {
	Enabled           bool
	RequestsPerSecond float64
	Burst             int
	Action            config.RateLimitAction
	LogViolations     bool
	CleanupInterval   time.Duration
	MaxTrackedClients int
}

func parseRateLimitPayload(r *http.Request, current config.RateLimitConfig) (rateLimitPayload, error) {
	type request struct {
		Enabled           *bool    `json:"enabled,omitempty"`
		RequestsPerSecond *float64 `json:"requests_per_second,omitempty"`
		Burst             *int     `json:"burst,omitempty"`
		Action            string   `json:"on_exceed"`
		LogViolations     *bool    `json:"log_violations,omitempty"`
		CleanupInterval   string   `json:"cleanup_interval"`
		MaxTrackedClients *int     `json:"max_tracked_clients,omitempty"`
	}

	var req request
	if isJSONContent(r) {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return rateLimitPayload{}, fmt.Errorf("invalid JSON payload: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return rateLimitPayload{}, fmt.Errorf("invalid form payload: %w", err)
		}
		if val := r.FormValue("enabled"); val != "" {
			enabled := parseCheckbox(val)
			req.Enabled = &enabled
		}
		if val := r.FormValue("requests_per_second"); val != "" {
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				req.RequestsPerSecond = &f
			} else {
				return rateLimitPayload{}, fmt.Errorf("invalid requests_per_second: %v", err)
			}
		}
		if val := r.FormValue("burst"); val != "" {
			if i, err := strconv.Atoi(val); err == nil {
				req.Burst = &i
			} else {
				return rateLimitPayload{}, fmt.Errorf("invalid burst: %v", err)
			}
		}
		if val := r.FormValue("on_exceed"); val != "" {
			req.Action = val
		}
		if val := r.FormValue("log_violations"); val != "" {
			lv := parseCheckbox(val)
			req.LogViolations = &lv
		}
		if val := r.FormValue("cleanup_interval"); val != "" {
			req.CleanupInterval = val
		}
		if val := r.FormValue("max_tracked_clients"); val != "" {
			if i, err := strconv.Atoi(val); err == nil {
				req.MaxTrackedClients = &i
			} else {
				return rateLimitPayload{}, fmt.Errorf("invalid max_tracked_clients: %v", err)
			}
		}
	}

	result := rateLimitPayload{
		Enabled:           current.Enabled,
		RequestsPerSecond: current.RequestsPerSecond,
		Burst:             current.Burst,
		Action:            current.Action,
		LogViolations:     current.LogViolations,
		CleanupInterval:   current.CleanupInterval,
		MaxTrackedClients: current.MaxTrackedClients,
	}

	if req.Enabled != nil {
		result.Enabled = *req.Enabled
	}
	if req.RequestsPerSecond != nil {
		result.RequestsPerSecond = *req.RequestsPerSecond
	}
	if req.Burst != nil {
		result.Burst = *req.Burst
	}
	if strings.TrimSpace(req.Action) != "" {
		act := config.RateLimitAction(strings.ToLower(req.Action))
		if act != config.RateLimitActionDrop && act != config.RateLimitActionNXDOMAIN && act != "nxdomain" {
			return rateLimitPayload{}, fmt.Errorf("on_exceed must be 'drop' or 'nxdomain'")
		}
		if act == "nxdomain" {
			act = config.RateLimitActionNXDOMAIN
		}
		result.Action = act
	}
	if req.LogViolations != nil {
		result.LogViolations = *req.LogViolations
	}
	if strings.TrimSpace(req.CleanupInterval) != "" {
		d, err := time.ParseDuration(req.CleanupInterval)
		if err != nil {
			return rateLimitPayload{}, fmt.Errorf("invalid cleanup_interval: %v", err)
		}
		result.CleanupInterval = d
	}
	if req.MaxTrackedClients != nil {
		result.MaxTrackedClients = *req.MaxTrackedClients
	}

	if result.RequestsPerSecond <= 0 {
		return rateLimitPayload{}, fmt.Errorf("requests_per_second must be > 0")
	}
	if result.Burst <= 0 {
		result.Burst = int(result.RequestsPerSecond)
	}

	return result, nil
}

type tlsPayload struct {
	DotEnabled bool
	DotAddress string
	TLS        config.TLSConfig
}

func parseTLSPayload(r *http.Request, current config.ServerConfig) (tlsPayload, error) {
	type request struct {
		DotEnabled *bool                  `json:"dot_enabled,omitempty"`
		DotAddress string                 `json:"dot_address"`
		CertFile   string                 `json:"cert_file"`
		KeyFile    string                 `json:"key_file"`
		Autocert   *config.AutocertConfig `json:"autocert,omitempty"`
		ACME       *config.ACMEConfig     `json:"acme,omitempty"`
	}

	var req request
	if isJSONContent(r) {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return tlsPayload{}, fmt.Errorf("invalid JSON payload: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return tlsPayload{}, fmt.Errorf("invalid form payload: %w", err)
		}
		if val := r.FormValue("dot_enabled"); val != "" {
			enabled := parseCheckbox(val)
			req.DotEnabled = &enabled
		}
		req.DotAddress = r.FormValue("dot_address")
		req.CertFile = r.FormValue("cert_file")
		req.KeyFile = r.FormValue("key_file")

		if r.Form.Has("autocert_enabled") || r.Form.Has("autocert_hosts") {
			ac := current.TLS.Autocert
			ac.Enabled = parseCheckbox(r.FormValue("autocert_enabled"))
			if val := strings.TrimSpace(r.FormValue("autocert_hosts")); val != "" {
				ac.Hosts = splitList(val)
			}
			if val := strings.TrimSpace(r.FormValue("autocert_cache_dir")); val != "" {
				ac.CacheDir = val
			}
			if val := strings.TrimSpace(r.FormValue("autocert_email")); val != "" {
				ac.Email = val
			}
			if val := strings.TrimSpace(r.FormValue("autocert_http01_address")); val != "" {
				ac.HTTP01Address = val
			}
			req.Autocert = &ac
		}

		if r.Form.Has("acme_enabled") || r.Form.Has("acme_hosts") {
			acme := current.TLS.ACME
			acme.Enabled = parseCheckbox(r.FormValue("acme_enabled"))
			if val := strings.TrimSpace(r.FormValue("acme_dns_provider")); val != "" {
				acme.DNSProvider = val
			}
			if val := strings.TrimSpace(r.FormValue("acme_upstreams")); val != "" {
				acme.Upstreams = splitList(val)
			}
			if val := strings.TrimSpace(r.FormValue("acme_hosts")); val != "" {
				acme.Hosts = splitList(val)
			}
			if val := strings.TrimSpace(r.FormValue("acme_cache_dir")); val != "" {
				acme.CacheDir = val
			}
			if val := strings.TrimSpace(r.FormValue("acme_email")); val != "" {
				acme.Email = val
			}
			if val := strings.TrimSpace(r.FormValue("acme_renew_before")); val != "" {
				if d, err := time.ParseDuration(val); err == nil {
					acme.RenewBefore = d
				}
			}
			if val := strings.TrimSpace(r.FormValue("acme_cf_api_token")); val != "" {
				acme.Cloudflare.APIToken = val
			}
			if val := strings.TrimSpace(r.FormValue("acme_cf_zone_id")); val != "" {
				acme.Cloudflare.ZoneID = val
			}
			if val := strings.TrimSpace(r.FormValue("acme_cf_ttl")); val != "" {
				if ttl, err := strconv.Atoi(val); err == nil {
					acme.Cloudflare.TTL = ttl
				}
			}
			if val := strings.TrimSpace(r.FormValue("acme_cf_propagation_timeout")); val != "" {
				if d, err := time.ParseDuration(val); err == nil {
					acme.Cloudflare.PropagationTimeout = d
				}
			}
			if val := strings.TrimSpace(r.FormValue("acme_cf_polling_interval")); val != "" {
				if d, err := time.ParseDuration(val); err == nil {
					acme.Cloudflare.PollingInterval = d
				}
			}
			if r.Form.Has("acme_cf_skip_auth_check") {
				acme.Cloudflare.SkipAuthNSCheck = parseCheckbox(r.FormValue("acme_cf_skip_auth_check"))
			}
			req.ACME = &acme
		}
	}

	result := tlsPayload{
		DotEnabled: current.DotEnabled,
		DotAddress: current.DotAddress,
		TLS:        current.TLS,
	}

	if req.DotEnabled != nil {
		result.DotEnabled = *req.DotEnabled
	}
	if strings.TrimSpace(req.DotAddress) != "" {
		result.DotAddress = req.DotAddress
	}
	if strings.TrimSpace(req.CertFile) != "" {
		result.TLS.CertFile = req.CertFile
	}
	if strings.TrimSpace(req.KeyFile) != "" {
		result.TLS.KeyFile = req.KeyFile
	}
	if req.Autocert != nil {
		result.TLS.Autocert = *req.Autocert
	}
	if req.ACME != nil {
		result.TLS.ACME = *req.ACME
	}

	return result, nil
}

type loggingPayload struct {
	Level      string
	Format     string
	Output     string
	FilePath   string
	AddSource  bool
	MaxSize    int
	MaxBackups int
	MaxAge     int
}

func parseLoggingPayload(r *http.Request, current config.LoggingConfig) (loggingPayload, error) {
	type request struct {
		Level      string `json:"level"`
		Format     string `json:"format"`
		Output     string `json:"output"`
		FilePath   string `json:"file_path"`
		AddSource  *bool  `json:"add_source,omitempty"`
		MaxSize    *int   `json:"max_size,omitempty"`
		MaxBackups *int   `json:"max_backups,omitempty"`
		MaxAge     *int   `json:"max_age,omitempty"`
	}

	var req request
	if isJSONContent(r) {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return loggingPayload{}, fmt.Errorf("invalid JSON payload: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return loggingPayload{}, fmt.Errorf("invalid form payload: %w", err)
		}
		req.Level = r.FormValue("level")
		req.Format = r.FormValue("format")
		req.Output = r.FormValue("output")
		req.FilePath = r.FormValue("file_path")
		if val := r.FormValue("add_source"); val != "" {
			checked := parseCheckbox(val)
			req.AddSource = &checked
		} else {
			disabled := false
			req.AddSource = &disabled
		}
		maxSizeRaw := strings.TrimSpace(r.FormValue("max_size"))
		if maxSizeRaw == "" {
			return loggingPayload{}, fmt.Errorf("max_size is required")
		}
		maxSize, err := strconv.Atoi(maxSizeRaw)
		if err != nil {
			return loggingPayload{}, fmt.Errorf("max_size must be a positive integer")
		}
		req.MaxSize = &maxSize

		maxBackupsRaw := strings.TrimSpace(r.FormValue("max_backups"))
		if maxBackupsRaw == "" {
			return loggingPayload{}, fmt.Errorf("max_backups is required")
		}
		maxBackups, err := strconv.Atoi(maxBackupsRaw)
		if err != nil {
			return loggingPayload{}, fmt.Errorf("max_backups must be an integer")
		}
		req.MaxBackups = &maxBackups

		maxAgeRaw := strings.TrimSpace(r.FormValue("max_age"))
		if maxAgeRaw == "" {
			return loggingPayload{}, fmt.Errorf("max_age is required")
		}
		maxAge, err := strconv.Atoi(maxAgeRaw)
		if err != nil {
			return loggingPayload{}, fmt.Errorf("max_age must be an integer")
		}
		req.MaxAge = &maxAge
	}

	payload := loggingPayload{
		Level:      strings.ToLower(strings.TrimSpace(req.Level)),
		Format:     strings.ToLower(strings.TrimSpace(req.Format)),
		Output:     strings.ToLower(strings.TrimSpace(req.Output)),
		FilePath:   strings.TrimSpace(req.FilePath),
		AddSource:  current.AddSource,
		MaxSize:    current.MaxSize,
		MaxBackups: current.MaxBackups,
		MaxAge:     current.MaxAge,
	}

	if req.AddSource != nil {
		payload.AddSource = *req.AddSource
	}
	if req.MaxSize == nil {
		return loggingPayload{}, fmt.Errorf("max_size is required")
	}
	if *req.MaxSize <= 0 {
		return loggingPayload{}, fmt.Errorf("max_size must be greater than zero")
	}
	payload.MaxSize = *req.MaxSize

	if req.MaxBackups == nil {
		return loggingPayload{}, fmt.Errorf("max_backups is required")
	}
	payload.MaxBackups = *req.MaxBackups

	if req.MaxAge == nil {
		return loggingPayload{}, fmt.Errorf("max_age is required")
	}
	payload.MaxAge = *req.MaxAge

	if payload.Level == "" {
		return loggingPayload{}, fmt.Errorf("level is required")
	}
	switch payload.Level {
	case "debug", "info", "warn", "error":
	default:
		return loggingPayload{}, fmt.Errorf("invalid log level %q", payload.Level)
	}

	if payload.Format == "" {
		return loggingPayload{}, fmt.Errorf("format is required")
	}
	if payload.Format != "json" && payload.Format != "text" {
		return loggingPayload{}, fmt.Errorf("format must be 'json' or 'text'")
	}

	if payload.Output == "" {
		return loggingPayload{}, fmt.Errorf("output is required")
	}
	switch payload.Output {
	case "stdout", "stderr", "file":
	default:
		return loggingPayload{}, fmt.Errorf("output must be stdout, stderr, or file")
	}

	if payload.Output == "file" && payload.FilePath == "" {
		return loggingPayload{}, fmt.Errorf("file_path is required when output is 'file'")
	}

	if payload.MaxBackups < 0 {
		return loggingPayload{}, fmt.Errorf("max_backups cannot be negative")
	}
	if payload.MaxAge < 0 {
		return loggingPayload{}, fmt.Errorf("max_age cannot be negative")
	}

	return payload, nil
}

func parseDurationField(value, field string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("%s is required", field)
	}

	if isNumeric(value) {
		value += "s"
	}

	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration (e.g., 30s, 5m, 1h)", field)
	}
	return d, nil
}

func splitList(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == ',' || r == ';'
	})
	results := make([]string, 0, len(fields))
	for _, f := range fields {
		if trimmed := strings.TrimSpace(f); trimmed != "" {
			results = append(results, trimmed)
		}
	}
	return results
}

func parseCheckbox(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isJSONContent(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json")
}

func (s *Server) persistConfigSection(w http.ResponseWriter, r *http.Request, updated *config.Config, tmpl, errorKey string, current *config.Config) bool {
	if s.configPath == "" {
		s.respondConfigValidationError(
			w, r, tmpl, errorKey,
			"Configuration path is not set; settings are read-only in this deployment",
			current,
			http.StatusServiceUnavailable,
		)
		return false
	}

	if err := config.Save(s.configPath, updated); err != nil {
		s.logger.Error("Failed to save configuration", "error", err)
		status := http.StatusInternalServerError
		if isHTMXRequest(r) {
			status = http.StatusOK
		}
		s.respondConfigValidationError(
			w, r, tmpl, errorKey,
			fmt.Sprintf("Failed to save configuration: %v", err),
			current,
			status,
		)
		return false
	}

	*current = *updated
	return true
}
