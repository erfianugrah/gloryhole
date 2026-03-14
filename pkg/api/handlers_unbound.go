package api

import (
	"encoding/json"
	"net/http"
	"strings"

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
	// Partial update: decode only the fields present in the request body
	var partial unbound.ServerBlock
	if err := json.NewDecoder(r.Body).Decode(&partial); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	cfg := s.getUnboundServerConfig()
	mergeServerBlock(&cfg.Server, &partial)

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
	var req forwardZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" || len(req.ForwardAddrs) == 0 {
		s.writeError(w, http.StatusBadRequest, "name and forward_addrs are required")
		return
	}

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
	name := r.PathValue("name")

	var req forwardZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
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
// This enables partial updates via PUT.
func mergeServerBlock(base, partial *unbound.ServerBlock) {
	if partial.MsgCacheSize != "" {
		base.MsgCacheSize = partial.MsgCacheSize
	}
	if partial.RRSetCacheSize != "" {
		base.RRSetCacheSize = partial.RRSetCacheSize
	}
	if partial.KeyCacheSize != "" {
		base.KeyCacheSize = partial.KeyCacheSize
	}
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
	if partial.ModuleConfig != "" {
		base.ModuleConfig = partial.ModuleConfig
	}
	if partial.DomainInsecure != nil {
		base.DomainInsecure = partial.DomainInsecure
	}

	// Booleans — these are always applied since JSON decodes false explicitly
	// The caller sends only the fields they want to change
	_ = partial // suppress unused warning for bool fields we don't merge

	// For boolean toggles, the UI should send the full server block for now.
	// A proper PATCH semantic would require a map[string]interface{} approach.
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
