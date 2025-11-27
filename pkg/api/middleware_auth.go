package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

var authBypassPaths = map[string]struct{}{
	"/health":     {},
	"/ready":      {},
	"/api/health": {},
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

		if s.hasBasicCredentials() {
			w.Header().Set("WWW-Authenticate", `Basic realm="Glory-Hole", charset="UTF-8"`)
		}
		s.writeError(w, http.StatusUnauthorized, "Unauthorized")
	})
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

	return true
}

func (s *Server) hasBasicCredentials() bool {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	return s.basicUser != "" && s.basicPass != ""
}

func (s *Server) authorizeRequest(r *http.Request) bool {
	s.authMu.RLock()
	apiKey := s.apiKey
	header := s.authHeader
	username := s.basicUser
	password := s.basicPass
	s.authMu.RUnlock()

	if apiKey != "" {
		if token := extractAPIKey(r, header); token != "" {
			if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) == 1 {
				return true
			}
		}
	}

	if username != "" && password != "" {
		if user, pass, ok := r.BasicAuth(); ok {
			if subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1 &&
				subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1 {
				return true
			}
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
