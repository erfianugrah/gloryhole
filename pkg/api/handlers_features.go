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
	UpdatedAt        time.Time `json:"updated_at"`
	BlocklistEnabled bool      `json:"blocklist_enabled"`
	PoliciesEnabled  bool      `json:"policies_enabled"`
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

	resp := FeaturesResponse{
		UpdatedAt:        time.Now(),
		BlocklistEnabled: cfg.Server.EnableBlocklist,
		PoliciesEnabled:  cfg.Server.EnablePolicies,
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
