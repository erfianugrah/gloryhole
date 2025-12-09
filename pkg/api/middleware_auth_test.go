package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"glory-hole/pkg/config"
)

func TestAuthMiddleware_Disabled(t *testing.T) {
	s := &Server{logger: testLogger()}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	called := false
	middleware := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	middleware.ServeHTTP(res, req)

	if !called {
		t.Fatalf("expected next handler to be called")
	}
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
}

func TestAuthMiddleware_APIKey(t *testing.T) {
	cfg := config.LoadWithDefaults()
	cfg.Auth.Enabled = true
	cfg.Auth.APIKey = "secret"
	s := &Server{logger: testLogger()}
	s.applyAuthConfig(cfg.Auth)

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	res := httptest.NewRecorder()

	middleware := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	middleware.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req2.Header.Set("Authorization", "Bearer secret")
	res2 := httptest.NewRecorder()
	middleware.ServeHTTP(res2, req2)
	if res2.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid token, got %d", res2.Code)
	}
}

func TestAuthMiddleware_Basic(t *testing.T) {
	cfg := config.LoadWithDefaults()
	cfg.Auth.Enabled = true
	cfg.Auth.Username = "admin"
	cfg.Auth.Password = "pass"
	s := &Server{logger: testLogger()}
	s.applyAuthConfig(cfg.Auth)

	req := httptest.NewRequest(http.MethodGet, "/api/queries", nil)
	res := httptest.NewRecorder()

	middleware := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	middleware.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
	if res.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("expected no WWW-Authenticate header when no basic attempt present")
	}

	// Wrong basic credentials should emit challenge
	badReq := httptest.NewRequest(http.MethodGet, "/api/queries", nil)
	badReq.SetBasicAuth("admin", "wrong")
	badRes := httptest.NewRecorder()
	middleware.ServeHTTP(badRes, badReq)
	if badRes.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad basic auth, got %d", badRes.Code)
	}
	if badRes.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("expected WWW-Authenticate header for failed basic auth attempt")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/queries", nil)
	req2.SetBasicAuth("admin", "pass")
	res2 := httptest.NewRecorder()
	middleware.ServeHTTP(res2, req2)
	if res2.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid basic auth, got %d", res2.Code)
	}
}

func TestAuthMiddleware_BypassPaths(t *testing.T) {
	cfg := config.LoadWithDefaults()
	cfg.Auth.Enabled = true
	cfg.Auth.APIKey = "secret"
	s := &Server{logger: testLogger()}
	s.applyAuthConfig(cfg.Auth)

	for _, path := range []string{"/health", "/ready", "/api/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		called := false
		middleware := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		middleware.ServeHTTP(res, req)
		if !called || res.Code != http.StatusOK {
			t.Fatalf("expected bypass for %s", path)
		}
	}
}

func TestAuthMiddleware_RedirectsToLogin(t *testing.T) {
	cfg := config.LoadWithDefaults()
	cfg.Auth.Enabled = true
	cfg.Auth.APIKey = "secret"
	s := &Server{logger: testLogger()}
	s.applyAuthConfig(cfg.Auth)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	res := httptest.NewRecorder()
	middleware := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	middleware.ServeHTTP(res, req)
	if res.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", res.Code)
	}
	if location := res.Header().Get("Location"); location != "/login?next=%2F" {
		t.Fatalf("unexpected redirect location: %s", location)
	}
}

func TestAuthMiddleware_SessionCookie(t *testing.T) {
	cfg := config.LoadWithDefaults()
	cfg.Auth.Enabled = true
	cfg.Auth.APIKey = "secret"
	s := &Server{logger: testLogger(), sessionManager: newSessionManager(time.Hour)}
	s.applyAuthConfig(cfg.Auth)

	token, _, err := s.sessionManager.Create("tester")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	res := httptest.NewRecorder()
	middleware := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	middleware.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid session, got %d", res.Code)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
}
