package api

import (
	"net/http"
)

// handleCSRFToken returns the CSRF token bound to the caller's session.
// Auth is enforced by the auth middleware (route is registered as auth-required).
// The token is then sent on subsequent mutating requests via the X-CSRF-Token header.
func (s *Server) handleCSRFToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Require a session cookie — API key / Basic auth callers don't need CSRF
	// tokens (they don't suffer CSRF), so this endpoint is session-only.
	if s.sessionManager == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Session manager unavailable")
		return
	}
	token := s.sessionTokenFromRequest(r)
	if token == "" {
		s.writeError(w, http.StatusUnauthorized, "No active session")
		return
	}
	csrf := s.sessionManager.CSRFToken(token)
	if csrf == "" {
		s.writeError(w, http.StatusUnauthorized, "No active session")
		return
	}

	// Disable caching — the token must always come from the live session store.
	w.Header().Set("Cache-Control", "no-store")

	s.writeJSON(w, http.StatusOK, map[string]string{"token": csrf})
}
