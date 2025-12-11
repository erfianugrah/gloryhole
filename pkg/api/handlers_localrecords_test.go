package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"glory-hole/pkg/config"
	"glory-hole/pkg/dns"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestServerForLocalRecords(t *testing.T, initialRecords []config.LocalRecordEntry) *Server {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	testConfig := &config.Config{
		LocalRecords: config.LocalRecordsConfig{
			Enabled: len(initialRecords) > 0,
			Records: initialRecords,
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

func TestHandleGetLocalRecords_Empty(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/localrecords", nil)
	w := httptest.NewRecorder()

	server.handleGetLocalRecords(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response LocalRecordsListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Total)
	assert.Empty(t, response.Records)
}

func TestHandleGetLocalRecords_WithRecords(t *testing.T) {
	initialRecords := []config.LocalRecordEntry{
		{
			Domain: "router.local.",
			Type:   "A",
			IPs:    []string{"192.168.1.1"},
			TTL:    300,
		},
		{
			Domain: "mail.local.",
			Type:   "AAAA",
			IPs:    []string{"2001:db8::1"},
			TTL:    600,
		},
		{
			Domain: "www.local.",
			Type:   "CNAME",
			Target: "router.local.",
			TTL:    300,
		},
	}

	server := createTestServerForLocalRecords(t, initialRecords)

	req := httptest.NewRequest(http.MethodGet, "/api/localrecords", nil)
	w := httptest.NewRecorder()

	server.handleGetLocalRecords(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response LocalRecordsListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 3, response.Total)
	assert.Len(t, response.Records, 3)

	// Verify record types
	typeCount := make(map[string]int)
	for _, record := range response.Records {
		typeCount[record.Type]++
	}
	assert.Equal(t, 1, typeCount["A"])
	assert.Equal(t, 1, typeCount["AAAA"])
	assert.Equal(t, 1, typeCount["CNAME"])
}

func TestHandleAddLocalRecord_ARecord(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	requestBody := LocalRecordAddRequest{
		Domain: "test.local",
		Type:   "A",
		IPs:    []string{"192.168.1.100"},
		TTL:    300,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/localrecords", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddLocalRecord(w, req)

	// Verify operation succeeds
	assert.Equal(t, http.StatusOK, w.Code)
	// Note: Full state verification would require config reload mechanism
}

func TestHandleAddLocalRecord_AAAARecord(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	requestBody := LocalRecordAddRequest{
		Domain: "test.local",
		Type:   "AAAA",
		IPs:    []string{"2001:db8::100"},
		TTL:    600,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/localrecords", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddLocalRecord(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleAddLocalRecord_CNAMERecord(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	requestBody := LocalRecordAddRequest{
		Domain: "www.local",
		Type:   "CNAME",
		Target: "router.local",
		TTL:    300,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/localrecords", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddLocalRecord(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleAddLocalRecord_MultipleIPs(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	requestBody := LocalRecordAddRequest{
		Domain: "server.local",
		Type:   "A",
		IPs:    []string{"192.168.1.100", "192.168.1.101", "192.168.1.102"},
		TTL:    300,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/localrecords", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddLocalRecord(w, req)

	// Verify operation succeeds
	assert.Equal(t, http.StatusOK, w.Code)
	// Note: Full state verification would require config reload mechanism
}

func TestHandleAddLocalRecord_MissingDomain(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	requestBody := LocalRecordAddRequest{
		Domain: "",
		Type:   "A",
		IPs:    []string{"192.168.1.100"},
		TTL:    300,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/localrecords", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddLocalRecord(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "Domain is required")
}

func TestHandleAddLocalRecord_InvalidType(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	requestBody := LocalRecordAddRequest{
		Domain: "test.local",
		Type:   "INVALID",
		IPs:    []string{"192.168.1.100"},
		TTL:    300,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/localrecords", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddLocalRecord(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "Unsupported record type")
}

func TestHandleAddLocalRecord_ARecordWithoutIP(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	requestBody := LocalRecordAddRequest{
		Domain: "test.local",
		Type:   "A",
		IPs:    []string{},
		TTL:    300,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/localrecords", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddLocalRecord(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "IPs are required")
}

func TestHandleAddLocalRecord_CNAMEWithoutTarget(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	requestBody := LocalRecordAddRequest{
		Domain: "www.local",
		Type:   "CNAME",
		Target: "",
		TTL:    300,
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/localrecords", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddLocalRecord(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "Target is required")
}

func TestHandleAddLocalRecord_InvalidJSON(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/localrecords", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddLocalRecord(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleRemoveLocalRecord_Success(t *testing.T) {
	initialRecords := []config.LocalRecordEntry{
		{
			Domain: "router.local.",
			Type:   "A",
			IPs:    []string{"192.168.1.1"},
			TTL:    300,
		},
		{
			Domain: "mail.local.",
			Type:   "A",
			IPs:    []string{"192.168.1.2"},
			TTL:    300,
		},
	}

	server := createTestServerForLocalRecords(t, initialRecords)

	// ID format is "domain:type:index"
	req := httptest.NewRequest(http.MethodDelete, "/api/localrecords/router.local.:A:0", nil)
	req.SetPathValue("id", "router.local.:A:0")
	w := httptest.NewRecorder()

	server.handleRemoveLocalRecord(w, req)

	// Verify operation succeeds
	assert.Equal(t, http.StatusOK, w.Code)
	// Note: Full state verification would require config reload mechanism
}

func TestHandleRemoveLocalRecord_NotFound(t *testing.T) {
	initialRecords := []config.LocalRecordEntry{
		{
			Domain: "router.local.",
			Type:   "A",
			IPs:    []string{"192.168.1.1"},
			TTL:    300,
		},
	}

	server := createTestServerForLocalRecords(t, initialRecords)

	req := httptest.NewRequest(http.MethodDelete, "/api/localrecords/nonexistent.local.:A:0", nil)
	req.SetPathValue("id", "nonexistent.local.:A:0")
	w := httptest.NewRecorder()

	server.handleRemoveLocalRecord(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleRemoveLocalRecord_InvalidIDFormat(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/localrecords/invalid-id", nil)
	req.SetPathValue("id", "invalid-id")
	w := httptest.NewRecorder()

	server.handleRemoveLocalRecord(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "Invalid record ID format")
}

func TestHandleRemoveLocalRecord_MissingID(t *testing.T) {
	server := createTestServerForLocalRecords(t, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/localrecords/", nil)
	req.SetPathValue("id", "")
	w := httptest.NewRecorder()

	server.handleRemoveLocalRecord(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPersistLocalRecordsConfig_NoConfigPath(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		logger:     testLogger,
		configPath: "", // No config path
	}

	err := server.persistLocalRecordsConfig(func(cfg *config.Config) error {
		return nil
	})

	// Should not error when no config path is set (ephemeral mode)
	assert.NoError(t, err)
}

func TestPersistLocalRecordsConfig_NoConfig(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		logger:         testLogger,
		configPath:     "/tmp/test.yaml",
		configSnapshot: nil,
	}

	err := server.persistLocalRecordsConfig(func(cfg *config.Config) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no config available")
}

func TestHandleGetLocalRecords_HTMLResponse(t *testing.T) {
	t.Skip("Skipping HTML response test - requires template initialization")

	initialRecords := []config.LocalRecordEntry{
		{
			Domain: "router.local.",
			Type:   "A",
			IPs:    []string{"192.168.1.1"},
			TTL:    300,
		},
	}

	server := createTestServerForLocalRecords(t, initialRecords)

	req := httptest.NewRequest(http.MethodGet, "/api/localrecords", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	server.handleGetLocalRecords(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Should return HTML when HX-Request header is present
}
