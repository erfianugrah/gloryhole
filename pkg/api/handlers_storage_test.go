package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleStorageReset_MethodNotAllowed(t *testing.T) {
	server := New(&Config{ListenAddress: ":0"})

	req := httptest.NewRequest(http.MethodGet, "/api/storage/reset", nil)
	w := httptest.NewRecorder()

	server.handleStorageReset(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleStorageReset_StorageUnavailable(t *testing.T) {
	server := New(&Config{ListenAddress: ":0"})

	req := httptest.NewRequest(http.MethodPost, "/api/storage/reset", strings.NewReader(`{"confirm":"NUKE"}`))
	w := httptest.NewRecorder()

	server.handleStorageReset(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleStorageReset_RequiresConfirmation(t *testing.T) {
	server := New(&Config{ListenAddress: ":0"})
	server.storage = &mockStorage{}

	req := httptest.NewRequest(http.MethodPost, "/api/storage/reset", strings.NewReader(`{"confirm":"nope"}`))
	w := httptest.NewRecorder()

	server.handleStorageReset(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleStorageReset_Success(t *testing.T) {
	server := New(&Config{ListenAddress: ":0"})
	mock := &mockStorage{}
	server.storage = mock

	req := httptest.NewRequest(http.MethodPost, "/api/storage/reset", strings.NewReader(`{"confirm":"NUKE"}`))
	w := httptest.NewRecorder()

	server.handleStorageReset(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !mock.resetCalled {
		t.Fatal("expected Reset to be called")
	}
}

func TestHandleStorageReset_ResetError(t *testing.T) {
	server := New(&Config{ListenAddress: ":0"})
	mock := &mockStorage{resetErr: errors.New("boom")}
	server.storage = mock

	req := httptest.NewRequest(http.MethodPost, "/api/storage/reset", strings.NewReader(`{"confirm":"NUKE"}`))
	w := httptest.NewRecorder()

	server.handleStorageReset(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
