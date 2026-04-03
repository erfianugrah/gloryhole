package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"glory-hole/pkg/policy"
	"glory-hole/pkg/storage"
)

// PolicyResponse represents a policy rule in API responses.
// ID is a stable auto-increment integer from SQLite (not an array index).
type PolicyResponse struct {
	Name       string `json:"name"`
	Logic      string `json:"logic"`
	Action     string `json:"action"`
	ActionData string `json:"action_data,omitempty"`
	ID         int64  `json:"id"`
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

// ─── Helpers ────────────────────────────────────────────────────────

func policyRuleToResponse(r *storage.PolicyRule) PolicyResponse {
	return PolicyResponse{
		ID:         r.ID,
		Name:       r.Name,
		Logic:      r.Logic,
		Action:     r.Action,
		ActionData: r.ActionData,
		Enabled:    r.Enabled,
	}
}

// loadPolicyResponses reads policy rules from SQLite (or falls back to
// the in-memory engine when storage is unavailable, e.g. in tests).
func (s *Server) loadPolicyResponses(ctx context.Context) ([]PolicyResponse, error) {
	if s.storage != nil {
		rules, err := s.storage.GetPolicyRules(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]PolicyResponse, 0, len(rules))
		for _, r := range rules {
			out = append(out, policyRuleToResponse(r))
		}
		return out, nil
	}
	// Fallback: read from in-memory engine (tests, no-storage mode)
	if s.policyEngine != nil {
		rules := s.policyEngine.GetRules()
		out := make([]PolicyResponse, 0, len(rules))
		for i, r := range rules {
			out = append(out, PolicyResponse{
				ID: int64(i), Name: r.Name, Logic: r.Logic,
				Action: r.Action, ActionData: r.ActionData, Enabled: r.Enabled,
			})
		}
		return out, nil
	}
	return []PolicyResponse{}, nil
}

// rebuildPolicyEngine reloads ALL rules from SQLite into the in-memory
// policy engine. Called after every create/update/delete so the engine
// stays in sync with the database.
func (s *Server) rebuildPolicyEngine(ctx context.Context) error {
	if s.policyEngine == nil {
		return nil
	}
	rules, err := s.storage.GetPolicyRules(ctx)
	if err != nil {
		return fmt.Errorf("load rules from DB: %w", err)
	}

	s.policyEngine.Clear()
	for _, r := range rules {
		rule := &policy.Rule{
			Name:       r.Name,
			Logic:      r.Logic,
			Action:     r.Action,
			ActionData: r.ActionData,
			Enabled:    r.Enabled,
		}
		if err := s.policyEngine.AddRule(rule); err != nil {
			s.logger.Error("Failed to compile policy rule from DB",
				"id", r.ID, "name", r.Name, "error", err)
			// Continue loading other rules
		}
	}
	return nil
}

// ─── Handlers ───────────────────────────────────────────────────────

// handleGetPolicies returns all policies from SQLite.
func (s *Server) handleGetPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := s.loadPolicyResponses(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to load policies: "+err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, PolicyListResponse{
		Policies: policies,
		Total:    len(policies),
	})
}

// handleGetPolicy returns a specific policy by ID.
func (s *Server) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid policy ID")
		return
	}

	policies, loadErr := s.loadPolicyResponses(r.Context())
	if loadErr != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to load policies")
		return
	}

	for _, p := range policies {
		if p.ID == id {
			s.writeJSON(w, http.StatusOK, p)
			return
		}
	}
	s.writeError(w, http.StatusNotFound, "Policy not found")
}

// handleAddPolicy creates a new policy in SQLite and rebuilds the engine.
func (s *Server) handleAddPolicy(w http.ResponseWriter, r *http.Request) {
	if s.policyEngine == nil {
		s.writeError(w, http.StatusBadRequest, "Policy engine not configured - enable policies in config to add rules")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
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

	if err := validatePolicyRequest(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate expression compiles before persisting
	testRule := &policy.Rule{
		Name: req.Name, Logic: req.Logic, Action: req.Action,
		ActionData: req.ActionData, Enabled: req.Enabled,
	}
	testEngine := policy.NewEngine(nil)
	if err := testEngine.AddRule(testRule); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Failed to compile expression: %v", err))
		return
	}

	var newID int64

	if s.storage != nil {
		// Determine sort_order (append at end)
		existing, _ := s.storage.GetPolicyRules(r.Context())
		sortOrder := len(existing)

		dbRule := &storage.PolicyRule{
			Name:       req.Name,
			Logic:      req.Logic,
			Action:     req.Action,
			ActionData: req.ActionData,
			Enabled:    req.Enabled,
			SortOrder:  sortOrder,
		}
		var createErr error
		newID, createErr = s.storage.CreatePolicyRule(r.Context(), dbRule)
		if createErr != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save policy: %v", createErr))
			return
		}

		// Rebuild the in-memory engine from DB
		if err := s.rebuildPolicyEngine(r.Context()); err != nil {
			s.logger.Error("Failed to rebuild policy engine after add", "error", err)
		}
	} else {
		// No storage — just add to in-memory engine (tests)
		if err := s.policyEngine.AddRule(testRule); err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add policy: %v", err))
			return
		}
		newID = int64(s.policyEngine.Count() - 1)
	}

	s.logger.Info("Policy added via API", "id", newID, "name", req.Name, "action", req.Action)

	s.writeJSON(w, http.StatusCreated, PolicyResponse{
		ID:         newID,
		Name:       req.Name,
		Logic:      req.Logic,
		Action:     req.Action,
		ActionData: req.ActionData,
		Enabled:    req.Enabled,
	})
}

// handleUpdatePolicy updates an existing policy in SQLite and rebuilds the engine.
func (s *Server) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	if s.policyEngine == nil {
		s.writeError(w, http.StatusBadRequest, "Policy engine not configured")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid policy ID")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
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

	if err := validatePolicyRequest(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate expression compiles
	testRule := &policy.Rule{
		Name: req.Name, Logic: req.Logic, Action: req.Action,
		ActionData: req.ActionData, Enabled: req.Enabled,
	}
	testEngine := policy.NewEngine(nil)
	if err := testEngine.AddRule(testRule); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Failed to compile expression: %v", err))
		return
	}

	if s.storage != nil {
		// Update in SQLite
		dbRule := &storage.PolicyRule{
			Name:       req.Name,
			Logic:      req.Logic,
			Action:     req.Action,
			ActionData: req.ActionData,
			Enabled:    req.Enabled,
		}
		if err := s.storage.UpdatePolicyRule(r.Context(), id, dbRule); err != nil {
			s.writeError(w, http.StatusNotFound, fmt.Sprintf("Failed to update policy: %v", err))
			return
		}
		if err := s.rebuildPolicyEngine(r.Context()); err != nil {
			s.logger.Error("Failed to rebuild policy engine after update", "error", err)
		}
	} else if s.policyEngine != nil {
		// No storage — update in-memory engine directly (tests)
		rules := s.policyEngine.GetRules()
		if int(id) < 0 || int(id) >= len(rules) {
			s.writeError(w, http.StatusNotFound, "Policy not found")
			return
		}
		if err := s.policyEngine.UpdateRule(int(id), testRule); err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Failed to update policy: %v", err))
			return
		}
	}

	s.logger.Info("Policy updated via API", "id", id, "name", req.Name, "action", req.Action)

	s.writeJSON(w, http.StatusOK, PolicyResponse{
		ID:         id,
		Name:       req.Name,
		Logic:      req.Logic,
		Action:     req.Action,
		ActionData: req.ActionData,
		Enabled:    req.Enabled,
	})
}

// handleDeletePolicy deletes a policy from SQLite and rebuilds the engine.
func (s *Server) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid policy ID")
		return
	}

	var ruleName string

	if s.storage != nil {
		// Look up rule name from DB
		rules, _ := s.storage.GetPolicyRules(r.Context())
		for _, r := range rules {
			if r.ID == id {
				ruleName = r.Name
				break
			}
		}
		if ruleName == "" {
			s.writeError(w, http.StatusNotFound, "Policy not found")
			return
		}

		if err := s.storage.DeletePolicyRule(r.Context(), id); err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete policy: %v", err))
			return
		}
		if err := s.rebuildPolicyEngine(r.Context()); err != nil {
			s.logger.Error("Failed to rebuild policy engine after delete", "error", err)
		}
	} else if s.policyEngine != nil {
		// No storage — delete from in-memory engine (tests)
		rules := s.policyEngine.GetRules()
		idx := int(id)
		if idx < 0 || idx >= len(rules) {
			s.writeError(w, http.StatusNotFound, "Policy not found")
			return
		}
		ruleName = rules[idx].Name
		s.policyEngine.RemoveRule(ruleName)
	} else {
		s.writeError(w, http.StatusNotFound, "Policy not found")
		return
	}

	s.logger.Info("Policy deleted via API", "id", id, "name", ruleName)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Policy deleted successfully",
		"id":      id,
		"name":    ruleName,
	})
}

// handleExportPolicies returns all policies as a downloadable JSON file.
func (s *Server) handleExportPolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	policies, err := s.loadPolicyResponses(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to load policies")
		return
	}

	payload := struct {
		Policies []PolicyResponse `json:"policies"`
		Total    int              `json:"total"`
	}{
		Policies: policies,
		Total:    len(policies),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="policies-export.json"`)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.Error("Failed to export policies", "error", err)
	}
}

type policyTestRequest struct {
	Logic     string `json:"logic"`
	Domain    string `json:"domain"`
	ClientIP  string `json:"client_ip"`
	QueryType string `json:"query_type"`
}

func (s *Server) handleTestPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req policyTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid payload: %v", err))
		return
	}

	req.Logic = strings.TrimSpace(req.Logic)
	if req.Logic == "" {
		s.writeError(w, http.StatusBadRequest, "Logic expression is required")
		return
	}

	req.Domain = strings.TrimSpace(req.Domain)
	if req.Domain == "" {
		s.writeError(w, http.StatusBadRequest, "Domain sample is required")
		return
	}

	req.ClientIP = strings.TrimSpace(req.ClientIP)
	if req.ClientIP == "" {
		req.ClientIP = "127.0.0.1"
	}

	if net.ParseIP(req.ClientIP) == nil {
		s.writeError(w, http.StatusBadRequest, "Client IP must be a valid address")
		return
	}

	req.QueryType = strings.ToUpper(strings.TrimSpace(req.QueryType))
	if req.QueryType == "" {
		req.QueryType = "A"
	}

	engine := policy.NewEngine(nil)
	rule := &policy.Rule{
		Name:    "tester",
		Logic:   req.Logic,
		Action:  policy.ActionBlock,
		Enabled: true,
	}

	if err := engine.AddRule(rule); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Failed to compile rule: %v", err))
		return
	}

	now := time.Now()
	pCtx := policy.Context{
		Time:      now,
		Domain:    req.Domain,
		ClientIP:  req.ClientIP,
		QueryType: req.QueryType,
		Hour:      now.Hour(),
		Minute:    now.Minute(),
		Day:       now.Day(),
		Month:     int(now.Month()),
		Weekday:   int(now.Weekday()),
	}

	matched, _ := engine.Evaluate(pCtx)
	s.writeJSON(w, http.StatusOK, map[string]any{
		"matched": matched,
	})
}

// ─── Validation ─────────────────────────────────────────────────────

func validatePolicyRequest(req *PolicyRequest) error {
	if req.Name == "" {
		return fmt.Errorf("policy name is required")
	}
	if req.Logic == "" {
		return fmt.Errorf("policy logic is required")
	}
	if req.Action == "" {
		return fmt.Errorf("policy action is required")
	}

	validActions := map[string]bool{
		policy.ActionBlock: true, policy.ActionAllow: true,
		policy.ActionRedirect: true, policy.ActionForward: true,
	}
	if !validActions[req.Action] {
		return fmt.Errorf("invalid action: %s (must be BLOCK, ALLOW, REDIRECT, or FORWARD)", req.Action)
	}

	if req.Action == policy.ActionRedirect && req.ActionData == "" {
		return fmt.Errorf("action_data (redirect IP) is required for REDIRECT action")
	}
	if req.Action == policy.ActionForward {
		if req.ActionData == "" {
			return fmt.Errorf("action_data (upstream DNS servers) is required for FORWARD action")
		}
		if _, err := policy.ParseUpstreams(req.ActionData); err != nil {
			return fmt.Errorf("invalid upstream format: %v", err)
		}
	}
	return nil
}
