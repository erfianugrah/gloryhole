package api

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var authBypassPaths = map[string]struct{}{
	"/health":     {},
	"/ready":      {},
	"/api/health": {},
	"/login":      {},
	"/logout":     {},
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	if s == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthRequired(r) {
			next.ServeHTTP(w, r)
			return
		}

		if s.authorizeRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		if s.shouldRedirectToLogin(r) {
			http.Redirect(w, r, buildLoginRedirectURL(r), http.StatusFound)
			return
		}

		if s.hasBasicCredentials() && strings.HasPrefix(r.URL.Path, "/api/") && hasBasicAttempt(r) {
			// Only prompt when client explicitly attempted Basic auth; avoid browser pop-ups for UI/HTMX
			w.Header().Set("WWW-Authenticate", `Basic realm="Glory-Hole", charset="UTF-8"`)
		}
		s.writeError(w, http.StatusUnauthorized, "Unauthorized")
	})
}

// hasBasicAttempt returns true if the incoming request already attempted Basic auth.
func hasBasicAttempt(r *http.Request) bool {
	if r == nil {
		return false
	}
	auth := r.Header.Get("Authorization")
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(auth)), "basic ")
}

func (s *Server) isAuthRequired(r *http.Request) bool {
	s.authMu.RLock()
	enabled := s.authEnabled
	s.authMu.RUnlock()

	if !enabled {
		return false
	}

	if r.Method == http.MethodOptions {
		return false
	}

	if _, ok := authBypassPaths[r.URL.Path]; ok {
		return false
	}

	if strings.HasPrefix(r.URL.Path, "/static/") {
		return false
	}

	return true
}

func (s *Server) hasBasicCredentials() bool {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	return s.basicUser != "" && (s.basicPass != "" || s.passwordHash != "")
}

func (s *Server) authorizeRequest(r *http.Request) bool {
	s.authMu.RLock()
	apiKey := s.apiKey
	header := s.authHeader
	username := s.basicUser
	password := s.basicPass
	passwordHash := s.passwordHash
	s.authMu.RUnlock()

	if s.hasValidSession(r) {
		return true
	}

	// Try API key authentication
	if apiKey != "" {
		if token := extractAPIKey(r, header); token != "" {
			if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) == 1 {
				return true
			}
		}
	}

	// Try Basic Auth (username/password or username/passwordHash)
	if username != "" {
		if user, pass, ok := r.BasicAuth(); ok {
			return matchBasicCredentials(user, pass, username, password, passwordHash)
		}
	}

	return false
}

func extractAPIKey(r *http.Request, header string) string {
	value := strings.TrimSpace(r.Header.Get(header))
	if value == "" && !strings.EqualFold(header, "Authorization") {
		value = strings.TrimSpace(r.Header.Get("Authorization"))
	}
	if value == "" {
		return ""
	}

	parts := strings.Fields(value)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return ""
}

func matchBasicCredentials(user, pass, expectedUser, expectedPass, expectedHash string) bool {
	if expectedUser == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(user), []byte(expectedUser)) != 1 {
		return false
	}
	if expectedHash != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(expectedHash), []byte(pass)); err == nil {
			return true
		}
		return false
	}
	if expectedPass != "" {
		if subtle.ConstantTimeCompare([]byte(pass), []byte(expectedPass)) == 1 {
			return true
		}
	}
	return false
}

func acceptsHTML(r *http.Request) bool {
	accept := strings.ToLower(r.Header.Get("Accept"))
	if accept == "" {
		return true
	}
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func (s *Server) shouldRedirectToLogin(r *http.Request) bool {
	if r == nil {
		return false
	}
	if s == nil {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return acceptsHTML(r)
}

func buildLoginRedirectURL(r *http.Request) string {
	if r == nil {
		return "/login"
	}
	next := sanitizeRedirectTarget(r.URL.RequestURI())
	if next == "" {
		next = "/"
	}
	values := url.Values{}
	values.Set("next", next)
	return "/login?" + values.Encode()
}

func (s *Server) validateAPIKeyInput(candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	s.authMu.RLock()
	apiKey := s.apiKey
	s.authMu.RUnlock()
	if apiKey == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(apiKey)) == 1
}

func (s *Server) validateUserPasswordInput(user, pass string) bool {
	username, password, passwordHash := s.basicCredentials()
	return matchBasicCredentials(user, pass, username, password, passwordHash)
}

func (s *Server) basicCredentials() (string, string, string) {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	return s.basicUser, s.basicPass, s.passwordHash
}

func sanitizeRedirectTarget(next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return "/"
	}
	if !strings.HasPrefix(next, "/") {
		return "/"
	}
	if strings.HasPrefix(next, "//") {
		return "/"
	}
	return next
}

func (s *Server) isAuthenticationEnabled() bool {
	if s == nil {
		return false
	}
	s.authMu.RLock()
	enabled := s.authEnabled
	s.authMu.RUnlock()
	return enabled
}
