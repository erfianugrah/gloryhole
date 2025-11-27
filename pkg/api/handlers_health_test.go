package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"glory-hole/pkg/policy"
	"glory-hole/pkg/storage"
)

// mockStorageForHealth implements storage.Storage for health check testing
type mockStorageForHealth struct {
	shouldFail bool
}

func (m *mockStorageForHealth) LogQuery(ctx context.Context, query *storage.QueryLog) error {
	return nil
}

func (m *mockStorageForHealth) GetRecentQueries(ctx context.Context, limit, offset int) ([]*storage.QueryLog, error) {
	return nil, nil
}

func (m *mockStorageForHealth) GetQueriesByDomain(ctx context.Context, domain string, limit int) ([]*storage.QueryLog, error) {
	return nil, nil
}

func (m *mockStorageForHealth) GetQueriesByClientIP(ctx context.Context, clientIP string, limit int) ([]*storage.QueryLog, error) {
	return nil, nil
}

func (m *mockStorageForHealth) GetStatistics(ctx context.Context, since time.Time) (*storage.Statistics, error) {
	if m.shouldFail {
		return nil, errors.New("storage unavailable")
	}
	return &storage.Statistics{}, nil
}

func (m *mockStorageForHealth) GetTopDomains(ctx context.Context, limit int, blocked bool) ([]*storage.DomainStats, error) {
	return nil, nil
}

func (m *mockStorageForHealth) GetBlockedCount(ctx context.Context, since time.Time) (int64, error) {
	return 0, nil
}

func (m *mockStorageForHealth) GetQueryCount(ctx context.Context, since time.Time) (int64, error) {
	return 0, nil
}

func (m *mockStorageForHealth) GetQueriesFiltered(ctx context.Context, filter storage.QueryFilter, limit, offset int) ([]*storage.QueryLog, error) {
	return []*storage.QueryLog{}, nil
}

func (m *mockStorageForHealth) GetTimeSeriesStats(ctx context.Context, bucket time.Duration, points int) ([]*storage.TimeSeriesPoint, error) {
	return []*storage.TimeSeriesPoint{}, nil
}

func (m *mockStorageForHealth) GetTraceStatistics(ctx context.Context, since time.Time) (*storage.TraceStatistics, error) {
	return &storage.TraceStatistics{
		Since:    since,
		Until:    time.Now(),
		ByStage:  make(map[string]int64),
		ByAction: make(map[string]int64),
		ByRule:   make(map[string]int64),
		BySource: make(map[string]int64),
	}, nil
}

func (m *mockStorageForHealth) GetQueriesWithTraceFilter(ctx context.Context, filter storage.TraceFilter, limit, offset int) ([]*storage.QueryLog, error) {
	return nil, nil
}

func (m *mockStorageForHealth) GetQueryTypeStats(ctx context.Context, limit int, since time.Time) ([]*storage.QueryTypeStats, error) {
	return []*storage.QueryTypeStats{}, nil
}

func (m *mockStorageForHealth) Cleanup(ctx context.Context, olderThan time.Time) error {
	return nil
}

func (m *mockStorageForHealth) Reset(ctx context.Context) error {
	return nil
}

func (m *mockStorageForHealth) Close() error {
	return nil
}

func (m *mockStorageForHealth) Ping(ctx context.Context) error {
	if m.shouldFail {
		return errors.New("storage unavailable")
	}
	return nil
}

func (m *mockStorageForHealth) GetClientSummaries(ctx context.Context, limit, offset int) ([]*storage.ClientSummary, error) {
	return []*storage.ClientSummary{}, nil
}

func (m *mockStorageForHealth) UpdateClientProfile(ctx context.Context, profile *storage.ClientProfile) error {
	return nil
}

func (m *mockStorageForHealth) GetClientGroups(ctx context.Context) ([]*storage.ClientGroup, error) {
	return []*storage.ClientGroup{}, nil
}

func (m *mockStorageForHealth) UpsertClientGroup(ctx context.Context, group *storage.ClientGroup) error {
	return nil
}

func (m *mockStorageForHealth) DeleteClientGroup(ctx context.Context, name string) error {
	return nil
}

// TestHandleLiveness tests the liveness probe endpoint
func TestHandleLiveness(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleLiveness(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response LivenessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "alive" {
		t.Errorf("expected status 'alive', got %s", response.Status)
	}
}

// TestHandleLiveness_MethodNotAllowed tests that non-GET methods are rejected
func TestHandleLiveness_MethodNotAllowed(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/health", nil)
		w := httptest.NewRecorder()

		server.handleLiveness(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected status 405, got %d", method, w.Code)
		}
	}
}

// TestHandleReadiness tests the readiness probe endpoint
func TestHandleReadiness(t *testing.T) {
	mock := &mockStorageForHealth{shouldFail: false}
	policyEngine := policy.NewEngine()

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
		PolicyEngine:  policyEngine,
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.handleReadiness(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "ready" {
		t.Errorf("expected status 'ready', got %s", response.Status)
	}

	// Check that all components are reported
	if _, ok := response.Checks["storage"]; !ok {
		t.Error("expected storage check in response")
	}

	if _, ok := response.Checks["policy_engine"]; !ok {
		t.Error("expected policy_engine check in response")
	}

	// Storage should be OK
	if response.Checks["storage"] != "ok" {
		t.Errorf("expected storage status 'ok', got %s", response.Checks["storage"])
	}

	// Policy engine should be OK
	if response.Checks["policy_engine"] != "ok" {
		t.Errorf("expected policy_engine status 'ok', got %s", response.Checks["policy_engine"])
	}

	// Blocklist should be reported as not configured
	if response.Checks["blocklist"] != "not_configured" {
		t.Errorf("expected blocklist status 'not_configured', got %s", response.Checks["blocklist"])
	}
}

// TestHandleReadiness_StorageDegraded tests readiness when storage is degraded
func TestHandleReadiness_StorageDegraded(t *testing.T) {
	mock := &mockStorageForHealth{shouldFail: true}

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.handleReadiness(w, req)

	// Should still be ready (degraded storage is acceptable)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "ready" {
		t.Errorf("expected status 'ready', got %s", response.Status)
	}

	// Storage should be reported as degraded
	if response.Checks["storage"] != "degraded" {
		t.Errorf("expected storage status 'degraded', got %s", response.Checks["storage"])
	}
}

// TestHandleReadiness_NoStorage tests readiness when storage is not configured
func TestHandleReadiness_NoStorage(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.handleReadiness(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "ready" {
		t.Errorf("expected status 'ready', got %s", response.Status)
	}

	// Storage should be reported as not configured
	if response.Checks["storage"] != "not_configured" {
		t.Errorf("expected storage status 'not_configured', got %s", response.Checks["storage"])
	}
}

// TestHandleReadiness_NoBlocklist tests readiness when blocklist is not configured
func TestHandleReadiness_NoBlocklist(t *testing.T) {
	server := New(&Config{
		ListenAddress:    ":8080",
		BlocklistManager: nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.handleReadiness(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should still be ready even without blocklist
	if response.Status != "ready" {
		t.Errorf("expected status 'ready', got %s", response.Status)
	}

	// Blocklist should be reported as not configured
	if response.Checks["blocklist"] != "not_configured" {
		t.Errorf("expected blocklist status 'not_configured', got %s", response.Checks["blocklist"])
	}
}

// TestHandleReadiness_NoComponents tests readiness when no optional components configured
func TestHandleReadiness_NoComponents(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.handleReadiness(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "ready" {
		t.Errorf("expected status 'ready', got %s", response.Status)
	}

	// All components should be reported as not configured
	if response.Checks["storage"] != "not_configured" {
		t.Errorf("expected storage 'not_configured', got %s", response.Checks["storage"])
	}
	if response.Checks["blocklist"] != "not_configured" {
		t.Errorf("expected blocklist 'not_configured', got %s", response.Checks["blocklist"])
	}
	if response.Checks["policy_engine"] != "not_configured" {
		t.Errorf("expected policy_engine 'not_configured', got %s", response.Checks["policy_engine"])
	}
}

// TestHandleReadiness_MethodNotAllowed tests that non-GET methods are rejected
func TestHandleReadiness_MethodNotAllowed(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/ready", nil)
		w := httptest.NewRecorder()

		server.handleReadiness(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected status 405, got %d", method, w.Code)
		}
	}
}
