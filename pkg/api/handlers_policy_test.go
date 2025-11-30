package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"glory-hole/pkg/policy"
)

func setupTestAPIServer() *Server {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	policyEngine := policy.NewEngine()

	return New(&Config{
		ListenAddress:    ":8080",
		Storage:          nil,
		BlocklistManager: nil,
		PolicyEngine:     policyEngine,
		Logger:           logger,
		Version:          "test",
	})
}

func TestHandleGetPolicies_Empty(t *testing.T) {
	server := setupTestAPIServer()

	req := httptest.NewRequest("GET", "/api/policies", nil)
	w := httptest.NewRecorder()

	server.handleGetPolicies(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result PolicyListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Expected 0 policies, got %d", result.Total)
	}

	if len(result.Policies) != 0 {
		t.Errorf("Expected empty policies list, got %d items", len(result.Policies))
	}
}

func TestHandleExportPolicies(t *testing.T) {
	server := setupTestAPIServer()
	_ = server.policyEngine.AddRule(&policy.Rule{
		Name:    "Export Rule",
		Logic:   `Domain == "export.com"`,
		Action:  policy.ActionBlock,
		Enabled: true,
	})

	req := httptest.NewRequest("GET", "/api/policies/export", nil)
	w := httptest.NewRecorder()

	server.handleExportPolicies(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "policies-export.json") {
		t.Errorf("Expected content disposition to include filename, got %s", cd)
	}

	var payload PolicyListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("Failed to decode export payload: %v", err)
	}

	if payload.Total != 1 {
		t.Fatalf("Expected 1 policy in export, got %d", payload.Total)
	}
}

func TestHandleAddPolicy_Success(t *testing.T) {
	server := setupTestAPIServer()

	reqBody := PolicyRequest{
		Name:    "Test Block Rule",
		Logic:   `Domain == "blocked.com"`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/policies", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleAddPolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 201, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result PolicyResponse
	body, _ = io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Name != reqBody.Name {
		t.Errorf("Expected name %q, got %q", reqBody.Name, result.Name)
	}

	if result.Logic != reqBody.Logic {
		t.Errorf("Expected logic %q, got %q", reqBody.Logic, result.Logic)
	}

	if result.Action != reqBody.Action {
		t.Errorf("Expected action %q, got %q", reqBody.Action, result.Action)
	}

	if result.Enabled != reqBody.Enabled {
		t.Errorf("Expected enabled %v, got %v", reqBody.Enabled, result.Enabled)
	}

	if result.ID != 0 {
		t.Errorf("Expected ID 0 for first policy, got %d", result.ID)
	}
}

func TestHandleAddPolicy_WithRedirect(t *testing.T) {
	server := setupTestAPIServer()

	reqBody := PolicyRequest{
		Name:       "Redirect Rule",
		Logic:      `Domain == "redirect.com"`,
		Action:     policy.ActionRedirect,
		ActionData: "192.168.1.100",
		Enabled:    true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/policies", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleAddPolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 201, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result PolicyResponse
	body, _ = io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.ActionData != reqBody.ActionData {
		t.Errorf("Expected action_data %q, got %q", reqBody.ActionData, result.ActionData)
	}
}

func TestHandleAddPolicy_RedirectWithoutActionData(t *testing.T) {
	server := setupTestAPIServer()

	reqBody := PolicyRequest{
		Name:    "Invalid Redirect",
		Logic:   `Domain == "redirect.com"`,
		Action:  policy.ActionRedirect,
		Enabled: true,
		// Missing ActionData
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/policies", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleAddPolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Code != http.StatusBadRequest {
		t.Errorf("Expected error code 400, got %d", errResp.Code)
	}
}

func TestHandleAddPolicy_InvalidAction(t *testing.T) {
	server := setupTestAPIServer()

	reqBody := PolicyRequest{
		Name:    "Invalid Action",
		Logic:   `Domain == "test.com"`,
		Action:  "INVALID",
		Enabled: true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/policies", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleAddPolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleAddPolicy_MissingName(t *testing.T) {
	server := setupTestAPIServer()

	reqBody := PolicyRequest{
		Logic:   `Domain == "test.com"`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/policies", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleAddPolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleGetPolicy_Success(t *testing.T) {
	server := setupTestAPIServer()

	// Add a policy first
	rule := &policy.Rule{
		Name:    "Test Rule",
		Logic:   `Domain == "test.com"`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	_ = server.policyEngine.AddRule(rule)

	// Get the policy
	req := httptest.NewRequest("GET", "/api/policies/0", nil)
	req.SetPathValue("id", "0")
	w := httptest.NewRecorder()

	server.handleGetPolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result PolicyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Name != rule.Name {
		t.Errorf("Expected name %q, got %q", rule.Name, result.Name)
	}

	if result.ID != 0 {
		t.Errorf("Expected ID 0, got %d", result.ID)
	}
}

func TestHandleGetPolicy_NotFound(t *testing.T) {
	server := setupTestAPIServer()

	req := httptest.NewRequest("GET", "/api/policies/999", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	server.handleGetPolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestHandleGetPolicy_InvalidID(t *testing.T) {
	server := setupTestAPIServer()

	req := httptest.NewRequest("GET", "/api/policies/invalid", nil)
	req.SetPathValue("id", "invalid")
	w := httptest.NewRecorder()

	server.handleGetPolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestHandleUpdatePolicy_Success(t *testing.T) {
	server := setupTestAPIServer()

	// Add a policy first
	rule := &policy.Rule{
		Name:    "Original Rule",
		Logic:   `Domain == "old.com"`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	_ = server.policyEngine.AddRule(rule)

	// Update the policy
	updateReq := PolicyRequest{
		Name:    "Updated Rule",
		Logic:   `Domain == "new.com"`,
		Action:  policy.ActionAllow,
		Enabled: false,
	}

	body, _ := json.Marshal(updateReq)
	req := httptest.NewRequest("PUT", "/api/policies/0", bytes.NewReader(body))
	req.SetPathValue("id", "0")
	w := httptest.NewRecorder()

	server.handleUpdatePolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result PolicyResponse
	body, _ = io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Name != updateReq.Name {
		t.Errorf("Expected name %q, got %q", updateReq.Name, result.Name)
	}

	if result.Logic != updateReq.Logic {
		t.Errorf("Expected logic %q, got %q", updateReq.Logic, result.Logic)
	}

	if result.Action != updateReq.Action {
		t.Errorf("Expected action %q, got %q", updateReq.Action, result.Action)
	}

	if result.Enabled != updateReq.Enabled {
		t.Errorf("Expected enabled %v, got %v", updateReq.Enabled, result.Enabled)
	}
}

func TestHandleUpdatePolicy_NotFound(t *testing.T) {
	server := setupTestAPIServer()

	updateReq := PolicyRequest{
		Name:    "Test",
		Logic:   `Domain == "test.com"`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}

	body, _ := json.Marshal(updateReq)
	req := httptest.NewRequest("PUT", "/api/policies/999", bytes.NewReader(body))
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	server.handleUpdatePolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestHandleDeletePolicy_Success(t *testing.T) {
	server := setupTestAPIServer()

	// Add a policy first
	rule := &policy.Rule{
		Name:    "To Delete",
		Logic:   `Domain == "delete.com"`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	_ = server.policyEngine.AddRule(rule)

	// Verify it exists
	if server.policyEngine.Count() != 1 {
		t.Fatalf("Expected 1 rule, got %d", server.policyEngine.Count())
	}

	// Delete the policy
	req := httptest.NewRequest("DELETE", "/api/policies/0", nil)
	req.SetPathValue("id", "0")
	w := httptest.NewRecorder()

	server.handleDeletePolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Verify it's deleted
	if server.policyEngine.Count() != 0 {
		t.Errorf("Expected 0 rules after delete, got %d", server.policyEngine.Count())
	}
}

func TestHandleDeletePolicy_NotFound(t *testing.T) {
	server := setupTestAPIServer()

	req := httptest.NewRequest("DELETE", "/api/policies/999", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	server.handleDeletePolicy(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestHandleGetPolicies_MultiplePolicies(t *testing.T) {
	server := setupTestAPIServer()

	// Add multiple policies
	rules := []*policy.Rule{
		{
			Name:    "Rule 1",
			Logic:   `Domain == "test1.com"`,
			Action:  policy.ActionBlock,
			Enabled: true,
		},
		{
			Name:    "Rule 2",
			Logic:   `Domain == "test2.com"`,
			Action:  policy.ActionAllow,
			Enabled: false,
		},
		{
			Name:       "Rule 3",
			Logic:      `Domain == "test3.com"`,
			Action:     policy.ActionRedirect,
			ActionData: "192.168.1.1",
			Enabled:    true,
		},
	}

	for _, rule := range rules {
		_ = server.policyEngine.AddRule(rule)
	}

	// Get all policies
	req := httptest.NewRequest("GET", "/api/policies", nil)
	w := httptest.NewRecorder()

	server.handleGetPolicies(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result PolicyListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Total != len(rules) {
		t.Errorf("Expected %d policies, got %d", len(rules), result.Total)
	}

	if len(result.Policies) != len(rules) {
		t.Errorf("Expected %d policies in list, got %d", len(rules), len(result.Policies))
	}

	// Verify all policies are present with correct IDs
	for i, rule := range rules {
		if result.Policies[i].ID != i {
			t.Errorf("Expected policy %d to have ID %d, got %d", i, i, result.Policies[i].ID)
		}

		if result.Policies[i].Name != rule.Name {
			t.Errorf("Expected policy %d name %q, got %q", i, rule.Name, result.Policies[i].Name)
		}

		if result.Policies[i].Action != rule.Action {
			t.Errorf("Expected policy %d action %q, got %q", i, rule.Action, result.Policies[i].Action)
		}
	}
}

func TestPolicyAPI_NoPolicyEngine(t *testing.T) {
	// Create server without policy engine
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	server := New(&Config{
		ListenAddress:    ":8080",
		Storage:          nil,
		BlocklistManager: nil,
		PolicyEngine:     nil, // No policy engine
		Logger:           logger,
		Version:          "test",
	})

	tests := []struct {
		handler    func(http.ResponseWriter, *http.Request)
		name       string
		method     string
		path       string
		expectCode int
	}{
		{
			name:       "GET policies",
			method:     "GET",
			path:       "/api/policies",
			handler:    server.handleGetPolicies,
			expectCode: http.StatusOK,
		},
		{
			name:       "POST policy",
			method:     "POST",
			path:       "/api/policies",
			handler:    server.handleAddPolicy,
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "GET policy",
			method:     "GET",
			path:       "/api/policies/0",
			handler:    server.handleGetPolicy,
			expectCode: http.StatusNotFound,
		},
		{
			name:       "PUT policy",
			method:     "PUT",
			path:       "/api/policies/0",
			handler:    server.handleUpdatePolicy,
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "DELETE policy",
			method:     "DELETE",
			path:       "/api/policies/0",
			handler:    server.handleDeletePolicy,
			expectCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.method == "POST" || tt.method == "PUT" {
				reqBody := PolicyRequest{
					Name:    "Test",
					Logic:   `Domain == "test.com"`,
					Action:  policy.ActionBlock,
					Enabled: true,
				}
				body, _ := json.Marshal(reqBody)
				req = httptest.NewRequest(tt.method, tt.path, bytes.NewReader(body))
			}

			if tt.path == "/api/policies/0" {
				req.SetPathValue("id", "0")
			}

			w := httptest.NewRecorder()
			tt.handler(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.expectCode {
				t.Errorf("Expected status %d, got %d", tt.expectCode, resp.StatusCode)
			}
		})
	}
}
