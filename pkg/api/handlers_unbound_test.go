package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"glory-hole/pkg/config"
	"glory-hole/pkg/unbound"
)

// newUnboundTestServer creates a test server with an unbound supervisor mock.
// The supervisor is not started (no real Unbound process), but has config set.
func newUnboundTestServer(t *testing.T, withSupervisor bool) *Server {
	t.Helper()

	cfg := config.LoadWithDefaults()
	cfg.Unbound.Enabled = withSupervisor
	cfg.Unbound.Managed = true
	cfg.Unbound.ListenPort = 5353
	cfg.Unbound.ControlSocket = "/var/run/unbound/control.sock"

	var sup *unbound.Supervisor
	if withSupervisor {
		sup = unbound.NewSupervisor(&cfg.Unbound, nil)
		// Set default config so getUnboundServerConfig returns it
		sup.SetServerConfig(unbound.DefaultServerConfig(5353, "/var/run/unbound/control.sock"))
	}

	return New(&Config{
		ListenAddress:     ":0",
		InitialConfig:     cfg,
		UnboundSupervisor: sup,
	})
}

func TestHandleGetUnboundStatus_Enabled(t *testing.T) {
	s := newUnboundTestServer(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/unbound/status", nil)
	w := httptest.NewRecorder()

	s.handleGetUnboundStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp unboundStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if !resp.Enabled {
		t.Error("expected enabled=true")
	}
	if !resp.Managed {
		t.Error("expected managed=true")
	}
	// Supervisor not started, so state should be stopped
	if resp.State != unbound.StateStopped {
		t.Errorf("expected state 'stopped', got %q", resp.State)
	}
}

func TestHandleGetUnboundStatus_Disabled(t *testing.T) {
	s := newUnboundTestServer(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/unbound/status", nil)
	w := httptest.NewRecorder()

	s.handleGetUnboundStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp unboundStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Enabled {
		t.Error("expected enabled=false")
	}
	if resp.State != unbound.StateStopped {
		t.Errorf("expected state 'stopped', got %q", resp.State)
	}
}

func TestUnboundGuard_Returns503WhenDisabled(t *testing.T) {
	s := newUnboundTestServer(t, false)

	handler := s.unboundGuard(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/unbound/stats", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestUnboundGuard_PassesThroughWhenEnabled(t *testing.T) {
	s := newUnboundTestServer(t, true)

	called := false
	handler := s.unboundGuard(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/unbound/stats", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if !called {
		t.Error("expected handler to be called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetUnboundConfig(t *testing.T) {
	s := newUnboundTestServer(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/unbound/config", nil)
	w := httptest.NewRecorder()

	s.handleGetUnboundConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp unboundConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Server.MsgCacheSize != "32m" {
		t.Errorf("expected msg_cache_size '32m', got %q", resp.Server.MsgCacheSize)
	}
	if resp.Server.NumThreads != 2 {
		t.Errorf("expected 2 threads, got %d", resp.Server.NumThreads)
	}
	if !resp.Server.HardenGlue {
		t.Error("expected harden_glue=true")
	}
	if resp.ForwardZones == nil {
		t.Error("expected non-nil forward_zones array")
	}
}

func TestHandleGetForwardZones_Empty(t *testing.T) {
	s := newUnboundTestServer(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/unbound/forward-zones", nil)
	w := httptest.NewRecorder()

	s.handleGetForwardZones(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Errorf("expected empty array '[]', got %q", body)
	}
}

func TestHandleAddForwardZone_Validation(t *testing.T) {
	s := newUnboundTestServer(t, true)

	// Missing required fields
	req := httptest.NewRequest(http.MethodPost, "/api/unbound/forward-zones",
		strings.NewReader(`{"name":"","forward_addrs":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleAddForwardZone(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty name, got %d", w.Code)
	}
}

func TestHandleDeleteForwardZone_NotFound(t *testing.T) {
	s := newUnboundTestServer(t, true)

	req := httptest.NewRequest(http.MethodDelete, "/api/unbound/forward-zones/nonexistent", nil)
	req.SetPathValue("name", "nonexistent")
	w := httptest.NewRecorder()

	s.handleDeleteForwardZone(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestMergeServerBlock(t *testing.T) {
	base := &unbound.ServerBlock{
		MsgCacheSize:   "32m",
		RRSetCacheSize: "64m",
		NumThreads:     2,
		Verbosity:      1,
		HardenGlue:     true,
	}

	partial := &unbound.ServerBlock{
		MsgCacheSize: "128m",
		NumThreads:   4,
	}

	mergeServerBlock(base, partial)

	if base.MsgCacheSize != "128m" {
		t.Errorf("expected merged msg_cache_size '128m', got %q", base.MsgCacheSize)
	}
	if base.NumThreads != 4 {
		t.Errorf("expected merged num_threads 4, got %d", base.NumThreads)
	}
	// Unchanged fields
	if base.RRSetCacheSize != "64m" {
		t.Errorf("expected unchanged rrset_cache_size '64m', got %q", base.RRSetCacheSize)
	}
	if base.Verbosity != 1 {
		t.Errorf("expected unchanged verbosity 1, got %d", base.Verbosity)
	}
}
