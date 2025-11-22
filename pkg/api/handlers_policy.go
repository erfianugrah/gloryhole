package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"glory-hole/pkg/policy"
)

// PolicyResponse represents a policy rule in API responses
type PolicyResponse struct {
	Name       string `json:"name"`
	Logic      string `json:"logic"`
	Action     string `json:"action"`
	ActionData string `json:"action_data,omitempty"`
	ID         int    `json:"id"`
	Enabled    bool   `json:"enabled"`
}

// PolicyListResponse represents the list of policies
type PolicyListResponse struct {
	Policies []PolicyResponse `json:"policies"`
	Total    int              `json:"total"`
}

// PolicyRequest represents a request to add/update a policy
type PolicyRequest struct {
	Name       string `json:"name"`
	Logic      string `json:"logic"`
	Action     string `json:"action"`
	ActionData string `json:"action_data,omitempty"`
	Enabled    bool   `json:"enabled"`
}

// handleGetPolicies returns all policies
func (s *Server) handleGetPolicies(w http.ResponseWriter, r *http.Request) {
	if s.policyEngine == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Policy engine not configured")
		return
	}

	rules := s.policyEngine.GetRules()
	policies := make([]PolicyResponse, 0, len(rules))

	for i, rule := range rules {
		policies = append(policies, PolicyResponse{
			ID:         i,
			Name:       rule.Name,
			Logic:      rule.Logic,
			Action:     rule.Action,
			ActionData: rule.ActionData,
			Enabled:    rule.Enabled,
		})
	}

	// Check if request wants HTML (from HTMX or browser)
	if r.Header.Get("HX-Request") == "true" || r.Header.Get("Accept") == "text/html" {
		// Return HTML partial
		data := struct {
			Policies []PolicyResponse
		}{
			Policies: policies,
		}

		if err := templates.ExecuteTemplate(w, "policies_partial.html", data); err != nil {
			s.logger.Error("Failed to render policies partial", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Return JSON
	s.writeJSON(w, http.StatusOK, PolicyListResponse{
		Policies: policies,
		Total:    len(policies),
	})
}

// handleGetPolicy returns a specific policy by ID
func (s *Server) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	if s.policyEngine == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Policy engine not configured")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid policy ID")
		return
	}

	rules := s.policyEngine.GetRules()
	if id < 0 || id >= len(rules) {
		s.writeError(w, http.StatusNotFound, "Policy not found")
		return
	}

	rule := rules[id]
	s.writeJSON(w, http.StatusOK, PolicyResponse{
		ID:         id,
		Name:       rule.Name,
		Logic:      rule.Logic,
		Action:     rule.Action,
		ActionData: rule.ActionData,
		Enabled:    rule.Enabled,
	})
}

// handleAddPolicy adds a new policy
func (s *Server) handleAddPolicy(w http.ResponseWriter, r *http.Request) {
	if s.policyEngine == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Policy engine not configured")
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req PolicyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Validate request
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "Policy name is required")
		return
	}
	if req.Logic == "" {
		s.writeError(w, http.StatusBadRequest, "Policy logic is required")
		return
	}
	if req.Action == "" {
		s.writeError(w, http.StatusBadRequest, "Policy action is required")
		return
	}

	// Validate action
	if req.Action != policy.ActionBlock && req.Action != policy.ActionAllow && req.Action != policy.ActionRedirect {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid action: %s (must be BLOCK, ALLOW, or REDIRECT)", req.Action))
		return
	}

	// If REDIRECT, require action_data
	if req.Action == policy.ActionRedirect && req.ActionData == "" {
		s.writeError(w, http.StatusBadRequest, "action_data (redirect IP) is required for REDIRECT action")
		return
	}

	// Create and add rule
	rule := &policy.Rule{
		Name:       req.Name,
		Logic:      req.Logic,
		Action:     req.Action,
		ActionData: req.ActionData,
		Enabled:    req.Enabled,
	}

	if err := s.policyEngine.AddRule(rule); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Failed to add policy: %v", err))
		return
	}

	s.logger.Info("Policy added via API",
		"name", req.Name,
		"action", req.Action,
		"enabled", req.Enabled)

	// Return the created policy
	rules := s.policyEngine.GetRules()
	newID := len(rules) - 1

	s.writeJSON(w, http.StatusCreated, PolicyResponse{
		ID:         newID,
		Name:       rule.Name,
		Logic:      rule.Logic,
		Action:     rule.Action,
		ActionData: rule.ActionData,
		Enabled:    rule.Enabled,
	})
}

// handleUpdatePolicy updates an existing policy
func (s *Server) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	if s.policyEngine == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Policy engine not configured")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid policy ID")
		return
	}

	// Get existing rule
	rules := s.policyEngine.GetRules()
	if id < 0 || id >= len(rules) {
		s.writeError(w, http.StatusNotFound, "Policy not found")
		return
	}

	existingRule := rules[id]

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req PolicyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Validate request
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "Policy name is required")
		return
	}
	if req.Logic == "" {
		s.writeError(w, http.StatusBadRequest, "Policy logic is required")
		return
	}
	if req.Action == "" {
		s.writeError(w, http.StatusBadRequest, "Policy action is required")
		return
	}

	// Validate action
	if req.Action != policy.ActionBlock && req.Action != policy.ActionAllow && req.Action != policy.ActionRedirect {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid action: %s", req.Action))
		return
	}

	// If REDIRECT, require action_data
	if req.Action == policy.ActionRedirect && req.ActionData == "" {
		s.writeError(w, http.StatusBadRequest, "action_data (redirect IP) is required for REDIRECT action")
		return
	}

	// Remove old rule and add new one
	// Note: This preserves the order by removing and adding in sequence
	s.policyEngine.RemoveRule(existingRule.Name)

	newRule := &policy.Rule{
		Name:       req.Name,
		Logic:      req.Logic,
		Action:     req.Action,
		ActionData: req.ActionData,
		Enabled:    req.Enabled,
	}

	if err := s.policyEngine.AddRule(newRule); err != nil {
		// Try to restore the old rule
		_ = s.policyEngine.AddRule(existingRule)
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Failed to update policy: %v", err))
		return
	}

	s.logger.Info("Policy updated via API",
		"id", id,
		"name", req.Name,
		"action", req.Action)

	s.writeJSON(w, http.StatusOK, PolicyResponse{
		ID:         id,
		Name:       newRule.Name,
		Logic:      newRule.Logic,
		Action:     newRule.Action,
		ActionData: newRule.ActionData,
		Enabled:    newRule.Enabled,
	})
}

// handleDeletePolicy deletes a policy
func (s *Server) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	if s.policyEngine == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Policy engine not configured")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid policy ID")
		return
	}

	// Get existing rule
	rules := s.policyEngine.GetRules()
	if id < 0 || id >= len(rules) {
		s.writeError(w, http.StatusNotFound, "Policy not found")
		return
	}

	rule := rules[id]

	// Remove the rule
	if !s.policyEngine.RemoveRule(rule.Name) {
		s.writeError(w, http.StatusInternalServerError, "Failed to remove policy")
		return
	}

	s.logger.Info("Policy deleted via API",
		"id", id,
		"name", rule.Name)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Policy deleted successfully",
		"id":      id,
		"name":    rule.Name,
	})
}
