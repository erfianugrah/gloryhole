package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

	for _, path := range []string{"/health", "/ready", "/api/health", "/dns-query"} {
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

	token, _, _, err := s.sessionManager.Create("tester")
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

// TestAuthMiddleware_CSRF_SessionRequiresToken ensures mutating /api/* calls
// authenticated by session cookie are rejected without a valid X-CSRF-Token.
func TestAuthMiddleware_CSRF_SessionRequiresToken(t *testing.T) {
	cfg := config.LoadWithDefaults()
	cfg.Auth.Enabled = true
	cfg.Auth.APIKey = "secret"
	s := &Server{logger: testLogger(), sessionManager: newSessionManager(time.Hour)}
	s.applyAuthConfig(cfg.Auth)

	token, csrf, _, err := s.sessionManager.Create("tester")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	middleware := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 1. Missing CSRF header -> 403
	req := httptest.NewRequest(http.MethodPost, "/api/policies", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	res := httptest.NewRecorder()
	middleware.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without CSRF token, got %d", res.Code)
	}

	// 2. Wrong CSRF header -> 403
	req2 := httptest.NewRequest(http.MethodPost, "/api/policies", nil)
	req2.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	req2.Header.Set("X-CSRF-Token", "wrong-token")
	res2 := httptest.NewRecorder()
	middleware.ServeHTTP(res2, req2)
	if res2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 with wrong CSRF token, got %d", res2.Code)
	}

	// 3. Correct CSRF header -> 200
	req3 := httptest.NewRequest(http.MethodPost, "/api/policies", nil)
	req3.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	req3.Header.Set("X-CSRF-Token", csrf)
	res3 := httptest.NewRecorder()
	middleware.ServeHTTP(res3, req3)
	if res3.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid CSRF token, got %d", res3.Code)
	}

	// 4. GET requests don't need CSRF
	req4 := httptest.NewRequest(http.MethodGet, "/api/policies", nil)
	req4.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	res4 := httptest.NewRecorder()
	middleware.ServeHTTP(res4, req4)
	if res4.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET without CSRF, got %d", res4.Code)
	}

	// 5. The legacy X-Requested-With header alone is no longer sufficient.
	req5 := httptest.NewRequest(http.MethodPost, "/api/policies", nil)
	req5.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	req5.Header.Set("X-Requested-With", "XMLHttpRequest")
	res5 := httptest.NewRecorder()
	middleware.ServeHTTP(res5, req5)
	if res5.Code != http.StatusForbidden {
		t.Fatalf("expected 403 with only X-Requested-With (legacy), got %d", res5.Code)
	}
}

// TestAuthMiddleware_CSRF_APIKeyExempt ensures Bearer / API-key auth is exempt
// from CSRF checks (browsers don't auto-send Authorization headers).
func TestAuthMiddleware_CSRF_APIKeyExempt(t *testing.T) {
	cfg := config.LoadWithDefaults()
	cfg.Auth.Enabled = true
	cfg.Auth.APIKey = "secret"
	s := &Server{logger: testLogger(), sessionManager: newSessionManager(time.Hour)}
	s.applyAuthConfig(cfg.Auth)

	middleware := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/policies", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()
	middleware.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for API-key POST without CSRF, got %d", res.Code)
	}
}

// TestSessionManager_Rotate ensures the old token is invalidated and a new
// session+CSRF pair is issued with the same subject.
func TestSessionManager_Rotate(t *testing.T) {
	m := newSessionManager(time.Hour)
	defer m.Stop()

	oldToken, oldCSRF, _, err := m.Create("alice")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newToken, newCSRF, _, err := m.Rotate(oldToken)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if newToken == oldToken {
		t.Fatal("rotated token must differ from old")
	}
	if newCSRF == oldCSRF {
		t.Fatal("rotated CSRF must differ from old")
	}
	if m.Validate(oldToken) {
		t.Fatal("old token should be invalid after rotate")
	}
	if !m.Validate(newToken) {
		t.Fatal("new token should be valid after rotate")
	}
	if got := m.CSRFToken(newToken); got != newCSRF {
		t.Fatalf("CSRFToken mismatch: got %q want %q", got, newCSRF)
	}

	// Rotating an unknown token should error
	if _, _, _, err := m.Rotate("nope"); err == nil {
		t.Fatal("expected error rotating unknown token")
	}
}

// TestCSRFTokenEndpoint exercises GET /api/csrf-token end-to-end through the
// auth middleware.
func TestCSRFTokenEndpoint(t *testing.T) {
	cfg := config.LoadWithDefaults()
	cfg.Auth.Enabled = true
	cfg.Auth.APIKey = "secret"
	s := &Server{logger: testLogger(), sessionManager: newSessionManager(time.Hour)}
	s.applyAuthConfig(cfg.Auth)

	token, csrf, _, err := s.sessionManager.Create("tester")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/csrf-token", s.handleCSRFToken)
	handler := s.authMiddleware(mux)

	// Without session cookie -> 401 (auth middleware rejects)
	req := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", res.Code)
	}

	// With valid session cookie -> 200 + token JSON
	req2 := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	req2.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	res2 := httptest.NewRecorder()
	handler.ServeHTTP(res2, req2)
	if res2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", res2.Code, res2.Body.String())
	}
	body := res2.Body.String()
	if !strings.Contains(body, csrf) {
		t.Fatalf("expected response to contain CSRF token; got %s", body)
	}
	if cc := res2.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("expected Cache-Control: no-store, got %q", cc)
	}

	// API-key callers should also be able to fetch (no session required for auth,
	// but our handler returns 401 because no session = no CSRF binding).
	req3 := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	req3.Header.Set("Authorization", "Bearer secret")
	res3 := httptest.NewRecorder()
	handler.ServeHTTP(res3, req3)
	if res3.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for API-key caller (no session), got %d", res3.Code)
	}
}

// TestSessionCookieSameSiteStrict ensures created cookies use SameSite=Strict.
func TestSessionCookieSameSiteStrict(t *testing.T) {
	s := &Server{logger: testLogger(), sessionManager: newSessionManager(time.Hour)}

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	res := httptest.NewRecorder()
	if err := s.createSession(res, req, "tester"); err != nil {
		t.Fatalf("createSession: %v", err)
	}
	cookies := res.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			found = true
			if c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("expected SameSite=Strict, got %v", c.SameSite)
			}
			if !c.HttpOnly {
				t.Fatal("expected HttpOnly cookie")
			}
		}
	}
	if !found {
		t.Fatal("session cookie not set")
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
}
