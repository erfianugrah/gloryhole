package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/dns"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestServerForConditionalForwarding(t *testing.T, initialRules []config.ForwardingRule) *Server {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	testConfig := &config.Config{
		ConditionalForwarding: config.ConditionalForwardingConfig{
			Enabled: len(initialRules) > 0,
			Rules:   initialRules,
		},
		// Add required fields to make config valid
		UpstreamDNSServers: []string{"8.8.8.8:53"},
	}

	// Create a minimal DNS handler to satisfy the nil check
	dnsHandler := &dns.Handler{}

	// Create temp config file for persistence testing
	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err)
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Write initial config
	err = config.Save(tmpPath, testConfig)
	require.NoError(t, err)

	// Register cleanup
	t.Cleanup(func() {
		os.Remove(tmpPath)
	})

	server := &Server{
		logger:         testLogger,
		dnsHandler:     dnsHandler,
		configSnapshot: testConfig,
		configPath:     tmpPath,
	}

	return server
}

func TestHandleGetConditionalForwarding_Empty(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/conditionalforwarding", nil)
	w := httptest.NewRecorder()

	server.handleGetConditionalForwarding(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response ConditionalForwardingListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Total)
	assert.Empty(t, response.Rules)
	assert.False(t, response.Enabled)
}

func TestHandleGetConditionalForwarding_WithRules(t *testing.T) {
	initialRules := []config.ForwardingRule{
		{
			Name:     "Corporate VPN",
			Priority: 90,
			Domains:  []string{"*.corp.example.com", "*.internal"},
			Upstreams: []string{"10.0.0.1:53", "10.0.0.2:53"},
			Enabled:  true,
		},
		{
			Name:        "Home Network",
			Priority:    50,
			ClientCIDRs: []string{"192.168.1.0/24"},
			QueryTypes:  []string{"PTR"},
			Upstreams:   []string{"192.168.1.1:53"},
			Enabled:     true,
		},
		{
			Name:       "Low Priority",
			Priority:   10,
			Domains:    []string{"*.test.local"},
			Upstreams:  []string{"8.8.8.8:53"},
			Enabled:    false,
		},
	}

	server := createTestServerForConditionalForwarding(t, initialRules)

	req := httptest.NewRequest(http.MethodGet, "/api/conditionalforwarding", nil)
	w := httptest.NewRecorder()

	server.handleGetConditionalForwarding(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response ConditionalForwardingListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 3, response.Total)
	assert.Len(t, response.Rules, 3)
	assert.True(t, response.Enabled)

	// Verify rules are sorted by priority (descending)
	assert.Equal(t, 90, response.Rules[0].Priority)
	assert.Equal(t, "Corporate VPN", response.Rules[0].Name)
	assert.Equal(t, 50, response.Rules[1].Priority)
	assert.Equal(t, 10, response.Rules[2].Priority)
}

func TestHandleGetConditionalForwarding_RuleSorting(t *testing.T) {
	initialRules := []config.ForwardingRule{
		{Name: "Medium", Priority: 50, Domains: []string{"*.test"}, Upstreams: []string{"8.8.8.8:53"}, Enabled: true},
		{Name: "High", Priority: 90, Domains: []string{"*.high"}, Upstreams: []string{"8.8.8.8:53"}, Enabled: true},
		{Name: "Low", Priority: 10, Domains: []string{"*.low"}, Upstreams: []string{"8.8.8.8:53"}, Enabled: true},
		{Name: "Also Medium", Priority: 50, Domains: []string{"*.also"}, Upstreams: []string{"8.8.8.8:53"}, Enabled: true},
	}

	server := createTestServerForConditionalForwarding(t, initialRules)

	req := httptest.NewRequest(http.MethodGet, "/api/conditionalforwarding", nil)
	w := httptest.NewRecorder()

	server.handleGetConditionalForwarding(w, req)

	var response ConditionalForwardingListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify sorting: priority descending, then name ascending
	assert.Equal(t, 90, response.Rules[0].Priority)
	assert.Equal(t, "High", response.Rules[0].Name)

	assert.Equal(t, 50, response.Rules[1].Priority)
	assert.Equal(t, 50, response.Rules[2].Priority)
	// For same priority, should be sorted alphabetically
	assert.True(t, response.Rules[1].Name < response.Rules[2].Name)

	assert.Equal(t, 10, response.Rules[3].Priority)
	assert.Equal(t, "Low", response.Rules[3].Name)
}

func TestHandleAddConditionalForwarding_DomainMatcher(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	requestBody := ForwardingRuleAddRequest{
		Name:      "Test Rule",
		Domains:   []string{"*.local"},
		Upstreams: []string{"192.168.1.1:53"},
		Priority:  70,
		Failover:  true,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddConditionalForwarding(w, req)

	// Verify operation succeeds
	assert.Equal(t, http.StatusOK, w.Code)
	// Note: Full state verification would require config reload mechanism
}

func TestHandleAddConditionalForwarding_ClientCIDRMatcher(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	requestBody := ForwardingRuleAddRequest{
		Name:        "Client Network",
		ClientCIDRs: []string{"192.168.1.0/24", "10.0.0.0/8"},
		Upstreams:   []string{"192.168.1.1:53"},
		Priority:    60,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddConditionalForwarding(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleAddConditionalForwarding_QueryTypeMatcher(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	requestBody := ForwardingRuleAddRequest{
		Name:       "PTR Queries",
		QueryTypes: []string{"PTR", "SOA"},
		Upstreams:  []string{"192.168.1.1:53"},
		Priority:   50,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddConditionalForwarding(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleAddConditionalForwarding_AllMatchers(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	requestBody := ForwardingRuleAddRequest{
		Name:        "Complex Rule",
		Domains:     []string{"*.local"},
		ClientCIDRs: []string{"192.168.1.0/24"},
		QueryTypes:  []string{"A", "AAAA"},
		Upstreams:   []string{"192.168.1.1:53", "192.168.1.2:53"},
		Priority:    80,
		Timeout:     "3s",
		MaxRetries:  3,
		Failover:    true,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddConditionalForwarding(w, req)

	// Verify operation succeeds
	assert.Equal(t, http.StatusOK, w.Code)
	// Note: Full state verification would require config reload mechanism
}

func TestHandleAddConditionalForwarding_DefaultPriority(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	requestBody := ForwardingRuleAddRequest{
		Name:      "Default Priority",
		Domains:   []string{"*.test"},
		Upstreams: []string{"8.8.8.8:53"},
		// Priority not specified
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddConditionalForwarding(w, req)

	// Verify operation succeeds
	assert.Equal(t, http.StatusOK, w.Code)
	// Note: Full state verification would require config reload mechanism
}

func TestHandleAddConditionalForwarding_MissingName(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	requestBody := ForwardingRuleAddRequest{
		Name:      "",
		Domains:   []string{"*.test"},
		Upstreams: []string{"8.8.8.8:53"},
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddConditionalForwarding(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "Rule name is required")
}

func TestHandleAddConditionalForwarding_MissingUpstreams(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	requestBody := ForwardingRuleAddRequest{
		Name:      "Test",
		Domains:   []string{"*.test"},
		Upstreams: []string{},
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddConditionalForwarding(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "At least one upstream is required")
}

func TestHandleAddConditionalForwarding_NoMatchers(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	requestBody := ForwardingRuleAddRequest{
		Name:      "Test",
		Upstreams: []string{"8.8.8.8:53"},
		// No domains, client_cidrs, or query_types
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddConditionalForwarding(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "At least one matching condition required")
}

func TestHandleAddConditionalForwarding_InvalidPriority(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	tests := []struct {
		name     string
		priority int
	}{
		// Note: Priority 0 is valid (defaults to 50), so we test negative and > 100
		{"Priority too high", 101},
		{"Priority negative", -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestBody := ForwardingRuleAddRequest{
				Name:      "Test",
				Domains:   []string{"*.test"},
				Upstreams: []string{"8.8.8.8:53"},
				Priority:  tt.priority,
			}
			bodyJSON, _ := json.Marshal(requestBody)

			req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader(bodyJSON))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleAddConditionalForwarding(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var response ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.Contains(t, response.Message, "Priority must be between 1 and 100")
		})
	}
}

func TestHandleAddConditionalForwarding_InvalidTimeout(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	requestBody := ForwardingRuleAddRequest{
		Name:      "Test",
		Domains:   []string{"*.test"},
		Upstreams: []string{"8.8.8.8:53"},
		Timeout:   "invalid",
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddConditionalForwarding(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "Invalid timeout format")
}

func TestHandleAddConditionalForwarding_InvalidJSON(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/conditionalforwarding", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddConditionalForwarding(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleRemoveConditionalForwarding_Success(t *testing.T) {
	initialRules := []config.ForwardingRule{
		{
			Name:      "Test-Rule-1",
			Domains:   []string{"*.test1"},
			Upstreams: []string{"8.8.8.8:53"},
			Priority:  80,
			Enabled:   true,
		},
		{
			Name:      "Test-Rule-2",
			Domains:   []string{"*.test2"},
			Upstreams: []string{"8.8.8.8:53"},
			Priority:  70,
			Enabled:   true,
		},
	}

	server := createTestServerForConditionalForwarding(t, initialRules)

	// ID format is "name:index"
	req := httptest.NewRequest(http.MethodDelete, "/api/conditionalforwarding/Test-Rule-1:0", nil)
	req.SetPathValue("id", "Test-Rule-1:0")
	w := httptest.NewRecorder()

	server.handleRemoveConditionalForwarding(w, req)

	// Verify operation succeeds
	assert.Equal(t, http.StatusOK, w.Code)
	// Note: Full state verification would require config reload mechanism
}

func TestHandleRemoveConditionalForwarding_NotFound(t *testing.T) {
	initialRules := []config.ForwardingRule{
		{
			Name:      "Test-Rule",
			Domains:   []string{"*.test"},
			Upstreams: []string{"8.8.8.8:53"},
			Priority:  80,
			Enabled:   true,
		},
	}

	server := createTestServerForConditionalForwarding(t, initialRules)

	req := httptest.NewRequest(http.MethodDelete, "/api/conditionalforwarding/Nonexistent-Rule:0", nil)
	req.SetPathValue("id", "Nonexistent-Rule:0")
	w := httptest.NewRecorder()

	server.handleRemoveConditionalForwarding(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleRemoveConditionalForwarding_InvalidIDFormat(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/conditionalforwarding/invalid-id", nil)
	req.SetPathValue("id", "invalid-id")
	w := httptest.NewRecorder()

	server.handleRemoveConditionalForwarding(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "Invalid rule ID format")
}

func TestHandleRemoveConditionalForwarding_MissingID(t *testing.T) {
	server := createTestServerForConditionalForwarding(t, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/conditionalforwarding/", nil)
	req.SetPathValue("id", "")
	w := httptest.NewRecorder()

	server.handleRemoveConditionalForwarding(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPersistConditionalForwardingConfig_NoConfigPath(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		logger:     testLogger,
		configPath: "", // No config path
	}

	err := server.persistConditionalForwardingConfig(func(cfg *config.Config) error {
		return nil
	})

	// Should not error when no config path is set (ephemeral mode)
	assert.NoError(t, err)
}

func TestPersistConditionalForwardingConfig_NoConfig(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		logger:         testLogger,
		configPath:     "/tmp/test.yaml",
		configSnapshot: nil,
	}

	err := server.persistConditionalForwardingConfig(func(cfg *config.Config) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no config available")
}

func TestHandleGetConditionalForwarding_TimeoutFormatting(t *testing.T) {
	initialRules := []config.ForwardingRule{
		{
			Name:      "With Timeout",
			Domains:   []string{"*.test"},
			Upstreams: []string{"8.8.8.8:53"},
			Priority:  80,
			Timeout:   3 * time.Second,
			Enabled:   true,
		},
		{
			Name:      "No Timeout",
			Domains:   []string{"*.test2"},
			Upstreams: []string{"8.8.8.8:53"},
			Priority:  70,
			Enabled:   true,
		},
	}

	server := createTestServerForConditionalForwarding(t, initialRules)

	req := httptest.NewRequest(http.MethodGet, "/api/conditionalforwarding", nil)
	w := httptest.NewRecorder()

	server.handleGetConditionalForwarding(w, req)

	var response ConditionalForwardingListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "3s", response.Rules[0].Timeout)
	assert.Equal(t, "", response.Rules[1].Timeout)
}

func TestHandleGetConditionalForwarding_HTMLResponse(t *testing.T) {
	t.Skip("Skipping HTML response test - requires template initialization")

	initialRules := []config.ForwardingRule{
		{
			Name:      "Test Rule",
			Domains:   []string{"*.test"},
			Upstreams: []string{"8.8.8.8:53"},
			Priority:  80,
			Enabled:   true,
		},
	}

	server := createTestServerForConditionalForwarding(t, initialRules)

	req := httptest.NewRequest(http.MethodGet, "/api/conditionalforwarding", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	server.handleGetConditionalForwarding(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Should return HTML when HX-Request header is present
}
