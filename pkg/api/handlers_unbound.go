package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"glory-hole/pkg/storage"
	"glory-hole/pkg/unbound"
)

// --- Response types ---

type unboundStatusResponse struct {
	Enabled    bool          `json:"enabled"`
	Managed    bool          `json:"managed"`
	State      unbound.State `json:"state"`
	Error      string        `json:"error,omitempty"`
	ListenAddr string        `json:"listen_addr,omitempty"`
}

type unboundConfigResponse struct {
	Server       unbound.ServerBlock   `json:"server"`
	ForwardZones []unbound.ForwardZone `json:"forward_zones"`
	StubZones    []unbound.StubZone    `json:"stub_zones"`
}

type forwardZoneRequest struct {
	Name         string   `json:"name"`
	ForwardAddrs []string `json:"forward_addrs"`
	ForwardFirst bool     `json:"forward_first"`
	ForwardTLS   bool     `json:"forward_tls_upstream"`
}

// --- Guard middleware ---

// unboundGuard returns 503 if the Unbound supervisor is nil (disabled).
func (s *Server) unboundGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.unboundSupervisor == nil {
			s.writeError(w, http.StatusServiceUnavailable, "Unbound resolver is not enabled")
			return
		}
		next(w, r)
	}
}

// --- Status & Stats ---

func (s *Server) handleGetUnboundStatus(w http.ResponseWriter, _ *http.Request) {
	cfg := s.currentConfig()

	resp := unboundStatusResponse{
		Enabled: cfg.Unbound.Enabled,
		Managed: cfg.Unbound.Managed,
	}

	if s.unboundSupervisor != nil {
		state, lastErr := s.unboundSupervisor.Status()
		resp.State = state
		resp.ListenAddr = s.unboundSupervisor.ListenAddr()
		if lastErr != nil {
			resp.Error = lastErr.Error()
		}
	} else {
		resp.State = unbound.StateStopped
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetUnboundStats(w http.ResponseWriter, _ *http.Request) {
	stats, err := s.unboundSupervisor.GetStats()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve Unbound stats: "+err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, stats)
}

// --- Config ---

func (s *Server) handleGetUnboundConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := s.getUnboundServerConfig()
	resp := unboundConfigResponse{
		Server:       cfg.Server,
		ForwardZones: cfg.ForwardZones,
		StubZones:    cfg.StubZones,
	}
	if resp.ForwardZones == nil {
		resp.ForwardZones = []unbound.ForwardZone{}
	}
	if resp.StubZones == nil {
		resp.StubZones = []unbound.StubZone{}
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUpdateUnboundServer(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024) // 1MB

	// Read body twice: once as raw map (to detect which fields were sent),
	// once as typed struct (for easy access to values).
	var raw map[string]json.RawMessage
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read body")
		return
	}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	var partial unbound.ServerBlock
	if err := json.Unmarshal(bodyBytes, &partial); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	cfg := s.getUnboundServerConfig()
	mergeServerBlock(&cfg.Server, &partial, raw)

	if err := s.applyUnboundConfig(cfg); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, cfg.Server)
}

// --- Forward Zones ---

func (s *Server) handleGetForwardZones(w http.ResponseWriter, _ *http.Request) {
	cfg := s.getUnboundServerConfig()
	zones := cfg.ForwardZones
	if zones == nil {
		zones = []unbound.ForwardZone{}
	}
	s.writeJSON(w, http.StatusOK, zones)
}

func (s *Server) handleAddForwardZone(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)
	var req forwardZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" || len(req.ForwardAddrs) == 0 {
		s.writeError(w, http.StatusBadRequest, "name and forward_addrs are required")
		return
	}
	req.Name = sanitizeZoneName(req.Name)

	cfg := s.getUnboundServerConfig()

	// Check for duplicate
	for _, z := range cfg.ForwardZones {
		if z.Name == req.Name {
			s.writeError(w, http.StatusConflict, "Forward zone already exists: "+req.Name)
			return
		}
	}

	zone := unbound.ForwardZone{
		Name:         req.Name,
		ForwardAddrs: req.ForwardAddrs,
		ForwardFirst: req.ForwardFirst,
		ForwardTLS:   req.ForwardTLS,
	}
	cfg.ForwardZones = append(cfg.ForwardZones, zone)

	if err := s.applyUnboundConfig(cfg); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, zone)
}

func (s *Server) handleUpdateForwardZone(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)
	name := sanitizeZoneName(r.PathValue("name"))

	var req forwardZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}
	if req.Name != "" {
		req.Name = sanitizeZoneName(req.Name)
	}

	cfg := s.getUnboundServerConfig()

	found := false
	for i, z := range cfg.ForwardZones {
		if z.Name == name {
			cfg.ForwardZones[i] = unbound.ForwardZone{
				Name:         name,
				ForwardAddrs: req.ForwardAddrs,
				ForwardFirst: req.ForwardFirst,
				ForwardTLS:   req.ForwardTLS,
			}
			found = true
			break
		}
	}

	if !found {
		s.writeError(w, http.StatusNotFound, "Forward zone not found: "+name)
		return
	}

	if err := s.applyUnboundConfig(cfg); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, cfg.ForwardZones)
}

func (s *Server) handleDeleteForwardZone(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	cfg := s.getUnboundServerConfig()

	found := false
	for i, z := range cfg.ForwardZones {
		if z.Name == name {
			cfg.ForwardZones = append(cfg.ForwardZones[:i], cfg.ForwardZones[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		s.writeError(w, http.StatusNotFound, "Forward zone not found: "+name)
		return
	}

	if err := s.applyUnboundConfig(cfg); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Actions ---

func (s *Server) handleUnboundReload(w http.ResponseWriter, _ *http.Request) {
	if err := s.unboundSupervisor.Reload(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Reload failed: "+err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "reloaded"})
}

func (s *Server) handleUnboundFlushCache(w http.ResponseWriter, _ *http.Request) {
	if err := s.unboundSupervisor.FlushCache(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Cache flush failed: "+err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "flushed"})
}

// --- Helpers ---

// getUnboundServerConfig returns the current Unbound config from the supervisor,
// or defaults if the supervisor has no config set.
func (s *Server) getUnboundServerConfig() *unbound.UnboundServerConfig {
	if s.unboundSupervisor != nil {
		if cfg := s.unboundSupervisor.ServerConfig(); cfg != nil {
			return cfg
		}
	}
	// Fallback to defaults
	appCfg := s.currentConfig()
	return unbound.DefaultServerConfig(appCfg.Unbound.ListenPort, appCfg.Unbound.ControlSocket)
}

// applyUnboundConfig validates the config, writes it, reloads Unbound,
// and updates the in-memory config on the supervisor.
func (s *Server) applyUnboundConfig(cfg *unbound.UnboundServerConfig) error {
	appCfg := s.currentConfig()

	// Validate config via unbound-checkconf before writing the live file.
	// This catches semantic errors (bad addrs, conflicting zones) that pure
	// field sanitization can't detect.
	if s.unboundSupervisor != nil {
		if checkconfBin := s.unboundSupervisor.CheckconfBin(); checkconfBin != "" {
			if err := unbound.Validate(cfg, checkconfBin); err != nil {
				return err
			}
		}
	}

	// Write the config file
	if err := unbound.WriteConfig(cfg, appCfg.Unbound.ConfigPath); err != nil {
		return err
	}

	// Reload Unbound to pick up changes
	if s.unboundSupervisor != nil {
		if err := s.unboundSupervisor.Reload(); err != nil {
			return err
		}
		// Update in-memory config
		s.unboundSupervisor.SetServerConfig(cfg)
	}

	return nil
}

// mergeServerBlock applies non-zero fields from partial onto base.
// The raw map is used to detect which fields were actually sent in the request,
// allowing boolean fields (where false is a valid value) to be properly merged.
func mergeServerBlock(base, partial *unbound.ServerBlock, raw map[string]json.RawMessage) {
	// String fields: non-empty means it was set
	if partial.MsgCacheSize != "" {
		base.MsgCacheSize = partial.MsgCacheSize
	}
	if partial.RRSetCacheSize != "" {
		base.RRSetCacheSize = partial.RRSetCacheSize
	}
	if partial.KeyCacheSize != "" {
		base.KeyCacheSize = partial.KeyCacheSize
	}
	if partial.ModuleConfig != "" {
		base.ModuleConfig = partial.ModuleConfig
	}

	// Numeric fields: non-zero means it was set
	if partial.CacheMaxTTL != 0 {
		base.CacheMaxTTL = partial.CacheMaxTTL
	}
	if partial.CacheMinTTL != 0 {
		base.CacheMinTTL = partial.CacheMinTTL
	}
	if partial.CacheMaxNegTTL != 0 {
		base.CacheMaxNegTTL = partial.CacheMaxNegTTL
	}
	if partial.NumThreads != 0 {
		base.NumThreads = partial.NumThreads
	}
	if partial.Verbosity != 0 {
		base.Verbosity = partial.Verbosity
	}
	if partial.ServeExpiredTTL != 0 {
		base.ServeExpiredTTL = partial.ServeExpiredTTL
	}

	// Slice fields
	if partial.DomainInsecure != nil {
		base.DomainInsecure = partial.DomainInsecure
	}

	// Boolean fields: use the raw map to detect if the field was actually sent.
	// This distinguishes "user sent false" from "user didn't send this field".
	mergeBool := func(key string, dst *bool, src bool) {
		if _, ok := raw[key]; ok {
			*dst = src
		}
	}
	mergeBool("harden_glue", &base.HardenGlue, partial.HardenGlue)
	mergeBool("harden_dnssec_stripped", &base.HardenDNSSEC, partial.HardenDNSSEC)
	mergeBool("harden_below_nxdomain", &base.HardenBelowNX, partial.HardenBelowNX)
	mergeBool("harden_algo_downgrade", &base.HardenAlgoDown, partial.HardenAlgoDown)
	mergeBool("qname_minimisation", &base.QnameMin, partial.QnameMin)
	mergeBool("aggressive_nsec", &base.AggressiveNSEC, partial.AggressiveNSEC)
	mergeBool("serve_expired", &base.ServeExpired, partial.ServeExpired)
	mergeBool("prefetch", &base.Prefetch, partial.Prefetch)
	mergeBool("prefetch_key", &base.PrefetchKey, partial.PrefetchKey)
	mergeBool("log_queries", &base.LogQueries, partial.LogQueries)
	mergeBool("log_replies", &base.LogReplies, partial.LogReplies)
	mergeBool("log_servfail", &base.LogServfail, partial.LogServfail)
	mergeBool("hide_identity", &base.HideIdentity, partial.HideIdentity)
	mergeBool("hide_version", &base.HideVersion, partial.HideVersion)
	mergeBool("minimal_responses", &base.MinimalResponses, partial.MinimalResponses)
	mergeBool("extended_statistics", &base.ExtendedStatistics, partial.ExtendedStatistics)
	mergeBool("statistics_cumulative", &base.StatisticsCumulative, partial.StatisticsCumulative)
	mergeBool("so_reuseport", &base.SoReusePort, partial.SoReusePort)
}

// writeError helper for compatibility — check if the method exists, otherwise inline it
func (s *Server) writeUnboundError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   http.StatusText(code),
		"message": msg,
		"code":    code,
	})
}

// sanitizeZoneName ensures zone names end with a dot
func sanitizeZoneName(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}

// --- Unbound Query Log (dnstap) ---

// handleGetUnboundQueries handles GET /api/unbound/queries
func (s *Server) handleGetUnboundQueries(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	limit := 100
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 1000 {
		limit = l
	}
	offset := 0
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	filter := storage.UnboundQueryFilter{
		Domain:      r.URL.Query().Get("domain"),
		QueryType:   r.URL.Query().Get("type"),
		MessageType: r.URL.Query().Get("message_type"),
		RCode:       r.URL.Query().Get("rcode"),
		Start:       r.URL.Query().Get("start"),
		End:         r.URL.Query().Get("end"),
	}

	if cached := r.URL.Query().Get("cached"); cached != "" {
		v := cached == "true" || cached == "1"
		filter.Cached = &v
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	queries, err := s.storage.GetUnboundQueries(ctx, filter, limit, offset)
	if err != nil {
		s.logger.Error("Failed to get Unbound queries", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve Unbound queries")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"queries": queries,
		"total":   len(queries),
		"limit":   limit,
		"offset":  offset,
	})
}

// handleGetUnboundQueryStats handles GET /api/unbound/query-stats
func (s *Server) handleGetUnboundQueryStats(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	since := time.Now().Add(-24 * time.Hour) // Default: last 24 hours
	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		if t, err := time.Parse(time.RFC3339, sinceParam); err == nil {
			since = t
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := s.storage.GetUnboundQueryStats(ctx, since)
	if err != nil {
		s.logger.Error("Failed to get Unbound query stats", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve Unbound query stats")
		return
	}

	s.writeJSON(w, http.StatusOK, stats)
}
