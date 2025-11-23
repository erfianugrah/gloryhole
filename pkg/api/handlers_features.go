package api

import (
	"encoding/json"
	"net/http"
	"time"

	"glory-hole/pkg/config"
)

// FeaturesRequest represents a request to update feature kill-switches
type FeaturesRequest struct {
	BlocklistEnabled *bool `json:"blocklist_enabled,omitempty"`
	PoliciesEnabled  *bool `json:"policies_enabled,omitempty"`
}

// FeaturesResponse represents the current state of feature kill-switches
type FeaturesResponse struct {
	BlocklistDisabledUntil   *time.Time `json:"blocklist_disabled_until,omitempty"` // When it will auto-re-enable
	PoliciesDisabledUntil    *time.Time `json:"policies_disabled_until,omitempty"`  // When it will auto-re-enable
	UpdatedAt                time.Time  `json:"updated_at"`
	BlocklistEnabled         bool       `json:"blocklist_enabled"`          // Permanent setting from config
	PoliciesEnabled          bool       `json:"policies_enabled"`           // Permanent setting from config
	BlocklistTemporarilyDisabled bool   `json:"blocklist_temp_disabled"`    // Temporary disable state
	PoliciesTemporarilyDisabled  bool   `json:"policies_temp_disabled"`     // Temporary disable state
}

// DisableRequest represents a request to temporarily disable a feature
type DisableRequest struct {
	Duration int `json:"duration"` // Duration in seconds (0 = indefinite)
}

// handleGetFeatures returns the current state of feature kill-switches
// GET /api/features
func (s *Server) handleGetFeatures(w http.ResponseWriter, r *http.Request) {
	// Only allow GET
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		s.writeError(w, http.StatusMethodNotAllowed, "Only GET is allowed")
		return
	}

	// Get current config
	cfg := s.configWatcher.Config()

	// Get temporary disable status
	blocklistTempDisabled, blocklistUntil, policiesTempDisabled, policiesUntil := s.killSwitch.GetStatus()

	resp := FeaturesResponse{
		UpdatedAt:                    time.Now(),
		BlocklistEnabled:             cfg.Server.EnableBlocklist,
		PoliciesEnabled:              cfg.Server.EnablePolicies,
		BlocklistTemporarilyDisabled: blocklistTempDisabled,
		PoliciesTemporarilyDisabled:  policiesTempDisabled,
	}

	// Only set until times if temporarily disabled
	if blocklistTempDisabled {
		resp.BlocklistDisabledUntil = &blocklistUntil
	}
	if policiesTempDisabled {
		resp.PoliciesDisabledUntil = &policiesUntil
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleUpdateFeatures updates feature kill-switches
// PUT /api/features
func (s *Server) handleUpdateFeatures(w http.ResponseWriter, r *http.Request) {
	// Only allow PUT
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", "PUT")
		s.writeError(w, http.StatusMethodNotAllowed, "Only PUT is allowed")
		return
	}

	// Limit request body size to prevent abuse
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB limit

	// Parse request
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
			"enabled", *req.BlocklistEnabled,
			"client", r.RemoteAddr)
	}

	// Update policies if specified
	if req.PoliciesEnabled != nil {
		cfg.Server.EnablePolicies = *req.PoliciesEnabled
		modified = true
		s.logger.Info("Policies kill-switch toggled",
			"enabled", *req.PoliciesEnabled,
			"client", r.RemoteAddr)
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
	// This will call the OnChange callback registered in main.go
	// which updates the config reference used by DNS handler
	s.logger.Info("Configuration saved, changes will take effect immediately")

	// Return updated state
	resp := FeaturesResponse{
		UpdatedAt:        time.Now(),
		BlocklistEnabled: cfg.Server.EnableBlocklist,
		PoliciesEnabled:  cfg.Server.EnablePolicies,
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleDisableBlocklist temporarily disables the blocklist for a specified duration
// POST /api/features/blocklist/disable
func (s *Server) handleDisableBlocklist(w http.ResponseWriter, r *http.Request) {
	// Only allow POST
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		s.writeError(w, http.StatusMethodNotAllowed, "Only POST is allowed")
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB limit

	// Parse request
	var req DisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate duration (0 = indefinite, max 24 hours)
	if req.Duration < 0 || req.Duration > 86400 {
		s.writeError(w, http.StatusBadRequest, "Duration must be between 0 and 86400 seconds (24 hours)")
		return
	}

	// Disable for specified duration
	var until time.Time
	if req.Duration == 0 {
		// Indefinite disable (1 year)
		until = s.killSwitch.DisableBlocklistFor(365 * 24 * time.Hour)
	} else {
		until = s.killSwitch.DisableBlocklistFor(time.Duration(req.Duration) * time.Second)
	}

	resp := map[string]interface{}{
		"disabled_until": until,
		"duration":       req.Duration,
		"message":        "Blocklist temporarily disabled",
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleEnableBlocklist immediately re-enables the blocklist (cancels temporary disable)
// POST /api/features/blocklist/enable
func (s *Server) handleEnableBlocklist(w http.ResponseWriter, r *http.Request) {
	// Only allow POST
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		s.writeError(w, http.StatusMethodNotAllowed, "Only POST is allowed")
		return
	}

	s.killSwitch.EnableBlocklist()

	resp := map[string]interface{}{
		"message": "Blocklist re-enabled",
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleDisablePolicies temporarily disables policies for a specified duration
// POST /api/features/policies/disable
func (s *Server) handleDisablePolicies(w http.ResponseWriter, r *http.Request) {
	// Only allow POST
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		s.writeError(w, http.StatusMethodNotAllowed, "Only POST is allowed")
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB limit

	// Parse request
	var req DisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate duration (0 = indefinite, max 24 hours)
	if req.Duration < 0 || req.Duration > 86400 {
		s.writeError(w, http.StatusBadRequest, "Duration must be between 0 and 86400 seconds (24 hours)")
		return
	}

	// Disable for specified duration
	var until time.Time
	if req.Duration == 0 {
		// Indefinite disable (1 year)
		until = s.killSwitch.DisablePoliciesFor(365 * 24 * time.Hour)
	} else {
		until = s.killSwitch.DisablePoliciesFor(time.Duration(req.Duration) * time.Second)
	}

	resp := map[string]interface{}{
		"disabled_until": until,
		"duration":       req.Duration,
		"message":        "Policies temporarily disabled",
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleEnablePolicies immediately re-enables policies (cancels temporary disable)
// POST /api/features/policies/enable
func (s *Server) handleEnablePolicies(w http.ResponseWriter, r *http.Request) {
	// Only allow POST
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		s.writeError(w, http.StatusMethodNotAllowed, "Only POST is allowed")
		return
	}

	s.killSwitch.EnablePolicies()

	resp := map[string]interface{}{
		"message": "Policies re-enabled",
	}

	s.writeJSON(w, http.StatusOK, resp)
}
