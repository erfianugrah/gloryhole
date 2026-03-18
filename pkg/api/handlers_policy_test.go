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

	policyEngine := policy.NewEngine(nil)

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

// TestHandleTestPolicy exercises the /api/policies/test endpoint with every
// expression pattern the fixed UI generates, ensuring compile + evaluate work
// through the HTTP layer.
func TestHandleTestPolicy(t *testing.T) {
	server := setupTestAPIServer()

	tests := []struct {
		name       string
		logic      string
		domain     string
		wantStatus int
		wantMatch  bool
	}{
		// Domain operators
		{name: "Domain/equals", logic: `Domain == "ads.example.com"`, domain: "ads.example.com", wantStatus: 200, wantMatch: true},
		{name: "Domain/contains", logic: `DomainMatches(Domain, "example")`, domain: "ads.example.com", wantStatus: 200, wantMatch: true},
		{name: "Domain/starts_with", logic: `DomainStartsWith(Domain, "ads")`, domain: "ads.example.com", wantStatus: 200, wantMatch: true},
		{name: "Domain/ends_with", logic: `DomainEndsWith(Domain, ".example.com")`, domain: "ads.example.com", wantStatus: 200, wantMatch: true},
		{name: "Domain/regex", logic: `DomainRegex(Domain, "^ads\\.")`, domain: "ads.example.com", wantStatus: 200, wantMatch: true},
		{name: "Domain/regex/no-match", logic: `DomainRegex(Domain, "^www\\.")`, domain: "ads.example.com", wantStatus: 200, wantMatch: false},
		// ClientIP — uses IPEquals/IPInCIDR
		{name: "ClientIP/equals", logic: `IPEquals(ClientIP, "127.0.0.1")`, domain: "test.com", wantStatus: 200, wantMatch: true},
		{name: "ClientIP/not-equals", logic: `!IPEquals(ClientIP, "10.0.0.1")`, domain: "test.com", wantStatus: 200, wantMatch: true},
		{name: "ClientIP/CIDR", logic: `IPInCIDR(ClientIP, "127.0.0.0/8")`, domain: "test.com", wantStatus: 200, wantMatch: true},
		// QueryType — uses QueryTypeIn
		{name: "QueryType/equals", logic: `QueryType == "A"`, domain: "test.com", wantStatus: 200, wantMatch: true},
		{name: "QueryType/in", logic: `QueryTypeIn(QueryType, "A", "AAAA")`, domain: "test.com", wantStatus: 200, wantMatch: true},
		// Hour/Weekday — numeric, no quotes
		{name: "Hour/gte", logic: `Hour >= 0`, domain: "test.com", wantStatus: 200, wantMatch: true},
		{name: "Weekday/lte", logic: `Weekday <= 6`, domain: "test.com", wantStatus: 200, wantMatch: true},
		// Groups
		{name: "AND group", logic: `(DomainMatches(Domain, "example") && DomainEndsWith(Domain, ".com"))`, domain: "ads.example.com", wantStatus: 200, wantMatch: true},
		{name: "OR group", logic: `(Domain == "other.com" || DomainMatches(Domain, "example"))`, domain: "ads.example.com", wantStatus: 200, wantMatch: true},
		{name: "NOT group", logic: `!(Domain == "other.com")`, domain: "ads.example.com", wantStatus: 200, wantMatch: true},
		// Cross-field
		{name: "cross-field domain+hour", logic: `(DomainMatches(Domain, "example") && Hour >= 0)`, domain: "ads.example.com", wantStatus: 200, wantMatch: true},
		// Invalid expression
		{name: "compile error", logic: `Hour == "bad"`, domain: "test.com", wantStatus: 400, wantMatch: false},
		// Empty inputs
		{name: "empty logic", logic: ``, domain: "test.com", wantStatus: 400, wantMatch: false},
		{name: "empty domain", logic: `true`, domain: "", wantStatus: 400, wantMatch: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]string{"logic": tt.logic, "domain": tt.domain}
			body, _ := json.Marshal(payload)
			req := httptest.NewRequest("POST", "/api/policies/test", bytes.NewReader(body))
			w := httptest.NewRecorder()

			server.handleTestPolicy(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, resp.StatusCode, string(respBody))
			}

			if tt.wantStatus == 200 {
				var result map[string]any
				json.NewDecoder(resp.Body).Decode(&result)
				matched, _ := result["matched"].(bool)
				if matched != tt.wantMatch {
					t.Errorf("expected matched=%v, got %v", tt.wantMatch, matched)
				}
			}
		})
	}
}

// TestHandleTestPolicy_RealDomains simulates clicking "Test" in the UI for
// real-world policies: sentry.io, adobe.io, adobe.com.  Each request goes
// through the full HTTP handler → policy.NewEngine → compile → evaluate path.
func TestHandleTestPolicy_RealDomains(t *testing.T) {
	server := setupTestAPIServer()

	tests := []struct {
		name      string
		logic     string
		domain    string
		wantMatch bool
	}{
		// Adobe OR — user picks "contains" for both, OR group
		{"Adobe OR / creativecloud.adobe.com", `(DomainMatches(Domain, "adobe.io") || DomainMatches(Domain, "adobe.com"))`, "creativecloud.adobe.com", true},
		{"Adobe OR / api.adobe.io", `(DomainMatches(Domain, "adobe.io") || DomainMatches(Domain, "adobe.com"))`, "api.adobe.io", true},
		{"Adobe OR / google.com", `(DomainMatches(Domain, "adobe.io") || DomainMatches(Domain, "adobe.com"))`, "google.com", false},

		// Adobe AND — wrong, but verify it behaves correctly (both must match)
		{"Adobe AND / creativecloud.adobe.com", `(DomainMatches(Domain, "adobe.io") && DomainMatches(Domain, "adobe.com"))`, "creativecloud.adobe.com", false},

		// Sentry contains
		{"Sentry / o123456.ingest.sentry.io", `DomainMatches(Domain, "sentry.io")`, "o123456.ingest.sentry.io", true},
		{"Sentry / sentry.io", `DomainMatches(Domain, "sentry.io")`, "sentry.io", true},
		{"Sentry / sentry-cdn.com", `DomainMatches(Domain, "sentry.io")`, "browser.sentry-cdn.com", false},

		// Sentry ends_with (UI prepends dot)
		{"Sentry ends_with / api.sentry.io", `DomainEndsWith(Domain, ".sentry.io")`, "api.sentry.io", true},
		{"Sentry ends_with / sentry.io bare", `DomainEndsWith(Domain, ".sentry.io")`, "sentry.io", false},

		// Combined Adobe + Sentry
		{"Combined / adobe.com", `(DomainMatches(Domain, "adobe.io") || DomainMatches(Domain, "adobe.com") || DomainMatches(Domain, "sentry.io"))`, "exchange.adobe.com", true},
		{"Combined / sentry.io", `(DomainMatches(Domain, "adobe.io") || DomainMatches(Domain, "adobe.com") || DomainMatches(Domain, "sentry.io"))`, "o123456.ingest.sentry.io", true},
		{"Combined / unrelated", `(DomainMatches(Domain, "adobe.io") || DomainMatches(Domain, "adobe.com") || DomainMatches(Domain, "sentry.io"))`, "github.com", false},

		// Regex for sentry subdomains
		{"Sentry regex / subdomain", `DomainRegex(Domain, ".*\\.sentry\\.io$")`, "o123456.ingest.sentry.io", true},
		{"Sentry regex / bare", `DomainRegex(Domain, ".*\\.sentry\\.io$")`, "sentry.io", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]string{"logic": tt.logic, "domain": tt.domain}
			body, _ := json.Marshal(payload)
			req := httptest.NewRequest("POST", "/api/policies/test", bytes.NewReader(body))
			w := httptest.NewRecorder()

			server.handleTestPolicy(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
			}

			var result map[string]any
			json.NewDecoder(resp.Body).Decode(&result)
			matched, _ := result["matched"].(bool)
			if matched != tt.wantMatch {
				t.Errorf("expected matched=%v, got %v\n  logic: %s\n  domain: %s", tt.wantMatch, matched, tt.logic, tt.domain)
			}
		})
	}
}

func TestPolicyAPI_EnableEngineAfterStart(t *testing.T) {
	// Start with no engine
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	server := New(&Config{
		ListenAddress:    ":8080",
		Storage:          nil,
		BlocklistManager: nil,
		PolicyEngine:     nil,
		Logger:           logger,
		Version:          "test",
	})

	// Add policy should fail while engine is nil
	body := PolicyRequest{Name: "rule1", Logic: `Domain == "a.com"`, Action: policy.ActionBlock, Enabled: true}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/policies", bytes.NewReader(buf))
	w := httptest.NewRecorder()
	server.handleAddPolicy(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when engine is nil, got %d", w.Result().StatusCode)
	}

	// Hot-plug engine and retry
	engine := policy.NewEngine(nil)
	server.SetPolicyEngine(engine)

	w2 := httptest.NewRecorder()
	server.handleAddPolicy(w2, req)
	if w2.Result().StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(w2.Result().Body)
		t.Fatalf("expected 201 after engine set, got %d: %s", w2.Result().StatusCode, string(bodyBytes))
	}

	if engine.Count() != 1 {
		t.Fatalf("expected 1 rule in engine after add, got %d", engine.Count())
	}
}
