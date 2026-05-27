package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"glory-hole/pkg/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClientStorage implements just enough of storage.Storage to drive the
// two mutation handlers covered by the invalidation hook.
type fakeClientStorage struct {
	storage.NoOpStorage
}

func (f *fakeClientStorage) UpdateClientProfile(_ context.Context, _ *storage.ClientProfile) error {
	return nil
}

func (f *fakeClientStorage) DeleteClientGroup(_ context.Context, _ string) error {
	return nil
}

// TestClientGroupInvalidation_FiresOnProfileUpdate asserts the resolver
// reload hook is invoked after a successful PUT /api/clients/{client}.
// The hook is critical for runtime correctness: without it, group-membership
// changes wouldn't take effect until the next restart.
func TestClientGroupInvalidation_FiresOnProfileUpdate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	server := &Server{
		logger:  logger,
		storage: &fakeClientStorage{},
	}

	var hookCount atomic.Int32
	server.SetClientGroupReloader(func(_ context.Context) error {
		hookCount.Add(1)
		return nil
	})

	body, _ := json.Marshal(clientUpdateRequest{
		DisplayName: "kid-laptop",
		GroupName:   "kids",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/clients/10.0.0.50", bytes.NewReader(body))
	req.SetPathValue("client", "10.0.0.50")
	w := httptest.NewRecorder()

	server.handleUpdateClient(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int32(1), hookCount.Load(), "reload hook must fire exactly once on profile update")
}

// TestClientGroupInvalidation_FiresOnGroupDelete asserts the hook fires
// after a successful DELETE /api/clients/groups/{group}. Group deletion
// cascades to client_profiles (group_name SET NULL via FK), so the cache
// MUST be rebuilt or stale rules will keep blocking/allowing.
func TestClientGroupInvalidation_FiresOnGroupDelete(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	server := &Server{
		logger:  logger,
		storage: &fakeClientStorage{},
	}

	var hookCount atomic.Int32
	server.SetClientGroupReloader(func(_ context.Context) error {
		hookCount.Add(1)
		return nil
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/clients/groups/kids", nil)
	req.SetPathValue("group", "kids")
	w := httptest.NewRecorder()

	server.handleDeleteClientGroup(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int32(1), hookCount.Load(), "reload hook must fire exactly once on group delete")
}

// TestClientGroupInvalidation_NoHookConfigured ensures the handler stays
// safe (no panic, no error) when SetClientGroupReloader was never called.
// Tests run without resolver wiring; this must continue to work.
func TestClientGroupInvalidation_NoHookConfigured(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	server := &Server{
		logger:  logger,
		storage: &fakeClientStorage{},
		// clientGroupReload deliberately unset
	}

	body, _ := json.Marshal(clientUpdateRequest{GroupName: "kids"})
	req := httptest.NewRequest(http.MethodPut, "/api/clients/10.0.0.50", bytes.NewReader(body))
	req.SetPathValue("client", "10.0.0.50")
	w := httptest.NewRecorder()

	server.handleUpdateClient(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// TestClientGroupInvalidation_ReloadErrorDoesNotBreakHandler asserts that
// when the reload hook returns a non-nil error, the handler still returns
// 200 OK — the failure is logged at ERROR and swallowed. Without this
// guarantee a transient DB hiccup during reload would 500 the update path
// even though the underlying mutation already committed.
func TestClientGroupInvalidation_ReloadErrorDoesNotBreakHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))
	server := &Server{
		logger:  logger,
		storage: &fakeClientStorage{},
	}

	var hookCount atomic.Int32
	server.SetClientGroupReloader(func(_ context.Context) error {
		hookCount.Add(1)
		return errors.New("simulated reload failure")
	})

	body, _ := json.Marshal(clientUpdateRequest{GroupName: "kids"})
	req := httptest.NewRequest(http.MethodPut, "/api/clients/10.0.0.50", bytes.NewReader(body))
	req.SetPathValue("client", "10.0.0.50")
	w := httptest.NewRecorder()

	server.handleUpdateClient(w, req)

	require.Equal(t, http.StatusOK, w.Code, "handler must return 200 even when reload hook errors")
	assert.Equal(t, int32(1), hookCount.Load(), "hook must still fire on the error path")
}

// TestClientGroupInvalidation_GroupDeleteReloadErrorDoesNotBreakHandler is
// the DELETE counterpart of the error-path test — the cascade-NULL update
// is already committed by the time the reload fires, so a reload error
// must not bubble back to the caller.
func TestClientGroupInvalidation_GroupDeleteReloadErrorDoesNotBreakHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))
	server := &Server{
		logger:  logger,
		storage: &fakeClientStorage{},
	}

	var hookCount atomic.Int32
	server.SetClientGroupReloader(func(_ context.Context) error {
		hookCount.Add(1)
		return errors.New("simulated reload failure")
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/clients/groups/kids", nil)
	req.SetPathValue("group", "kids")
	w := httptest.NewRecorder()

	server.handleDeleteClientGroup(w, req)

	require.Equal(t, http.StatusOK, w.Code, "delete handler must return 200 even when reload hook errors")
	assert.Equal(t, int32(1), hookCount.Load(), "hook must still fire on the error path")
}
