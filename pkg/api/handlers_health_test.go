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

func (m *mockStorageForHealth) Cleanup(ctx context.Context, olderThan time.Time) error {
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

// TestHandleHealthz tests the Kubernetes liveness probe endpoint
func TestHandleHealthz(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	server.handleHealthz(w, req)

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

// TestHandleHealthz_MethodNotAllowed tests that non-GET methods are rejected
func TestHandleHealthz_MethodNotAllowed(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/healthz", nil)
		w := httptest.NewRecorder()

		server.handleHealthz(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected status 405, got %d", method, w.Code)
		}
	}
}

// TestHandleReadyz tests the Kubernetes readiness probe endpoint
func TestHandleReadyz(t *testing.T) {
	mock := &mockStorageForHealth{shouldFail: false}
	policyEngine := policy.NewEngine()

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
		PolicyEngine:  policyEngine,
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	server.handleReadyz(w, req)

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

// TestHandleReadyz_StorageDegraded tests readiness when storage is degraded
func TestHandleReadyz_StorageDegraded(t *testing.T) {
	mock := &mockStorageForHealth{shouldFail: true}

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	server.handleReadyz(w, req)

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

// TestHandleReadyz_NoStorage tests readiness when storage is not configured
func TestHandleReadyz_NoStorage(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	server.handleReadyz(w, req)

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

// TestHandleReadyz_NoBlocklist tests readiness when blocklist is not configured
func TestHandleReadyz_NoBlocklist(t *testing.T) {
	server := New(&Config{
		ListenAddress:    ":8080",
		BlocklistManager: nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	server.handleReadyz(w, req)

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

// TestHandleReadyz_NoComponents tests readiness when no optional components configured
func TestHandleReadyz_NoComponents(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	server.handleReadyz(w, req)

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

// TestHandleReadyz_MethodNotAllowed tests that non-GET methods are rejected
func TestHandleReadyz_MethodNotAllowed(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/readyz", nil)
		w := httptest.NewRecorder()

		server.handleReadyz(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected status 405, got %d", method, w.Code)
		}
	}
}
