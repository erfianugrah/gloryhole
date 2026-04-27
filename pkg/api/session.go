package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"
)

type sessionManager struct {
	mu       sync.RWMutex
	sessions map[string]sessionData
	ttl      time.Duration
	stopCh   chan struct{}
}

type sessionData struct {
	subject   string
	csrfToken string
	expires   time.Time
}

func newSessionManager(ttl time.Duration) *sessionManager {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	m := &sessionManager{
		sessions: make(map[string]sessionData),
		ttl:      ttl,
		stopCh:   make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

// Create generates a new session token and CSRF token bound to that session.
// Returns sessionToken, csrfToken, expiry.
func (m *sessionManager) Create(subject string) (string, string, time.Time, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	csrfToken, err := generateSessionToken()
	if err != nil {
		return "", "", time.Time{}, err
	}

	expiry := time.Now().Add(m.ttl)

	m.mu.Lock()
	m.sessions[token] = sessionData{subject: subject, csrfToken: csrfToken, expires: expiry}
	m.mu.Unlock()

	return token, csrfToken, expiry, nil
}

func (m *sessionManager) Validate(token string) bool {
	if token == "" {
		return false
	}

	m.mu.RLock()
	session, ok := m.sessions[token]
	m.mu.RUnlock()

	if !ok {
		return false
	}

	if time.Now().After(session.expires) {
		m.mu.Lock()
		delete(m.sessions, token)
		m.mu.Unlock()
		return false
	}

	return true
}

// CSRFToken returns the CSRF token bound to the given session token.
// Returns empty string if the session does not exist or is expired.
func (m *sessionManager) CSRFToken(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return ""
	}
	if time.Now().After(session.expires) {
		return ""
	}
	return session.csrfToken
}

func (m *sessionManager) Revoke(token string) {
	if token == "" {
		return
	}
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

// Rotate looks up the existing session, creates a new session+CSRF token with
// the same subject and a fresh expiry, deletes the old token, and returns the
// new session token, CSRF token, and expiry.
func (m *sessionManager) Rotate(oldToken string) (string, string, time.Time, error) {
	if oldToken == "" {
		return "", "", time.Time{}, errors.New("empty session token")
	}
	m.mu.RLock()
	prev, ok := m.sessions[oldToken]
	m.mu.RUnlock()
	if !ok {
		return "", "", time.Time{}, errors.New("session not found")
	}

	newToken, err := generateSessionToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	newCSRF, err := generateSessionToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	expiry := time.Now().Add(m.ttl)

	m.mu.Lock()
	delete(m.sessions, oldToken)
	m.sessions[newToken] = sessionData{subject: prev.subject, csrfToken: newCSRF, expires: expiry}
	m.mu.Unlock()

	return newToken, newCSRF, expiry, nil
}

func (m *sessionManager) Stop() {
	if m == nil {
		return
	}
	select {
	case <-m.stopCh:
		return
	default:
		close(m.stopCh)
	}
}

func (m *sessionManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopCh:
			return
		}
	}
}

func (m *sessionManager) cleanup() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for token, session := range m.sessions {
		if now.After(session.expires) {
			delete(m.sessions, token)
		}
	}
}

func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// requestIsHTTPS returns true when the connection (or trusted-proxy hop) is HTTPS.
func (s *Server) requestIsHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if r.Header.Get("X-Forwarded-Proto") == "https" && s.isBehindTrustedProxy(r) {
		return true
	}
	return false
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request, subject string) error {
	if s.sessionManager == nil {
		return errors.New("session manager unavailable")
	}
	token, _, expiry, err := s.sessionManager.Create(subject)
	if err != nil {
		return err
	}
	secure := s.requestIsHTTPS(r)
	maxAge := int(time.Until(expiry).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiry,
		MaxAge:   maxAge,
	})
	return nil
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	if s == nil {
		return
	}
	token := s.sessionTokenFromRequest(r)
	if s.sessionManager != nil && token != "" {
		s.sessionManager.Revoke(token)
	}
	secure := s.requestIsHTTPS(r)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) sessionTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		return cookie.Value
	}
	return ""
}

func (s *Server) hasValidSession(r *http.Request) bool {
	if s.sessionManager == nil {
		return false
	}
	token := s.sessionTokenFromRequest(r)
	return s.sessionManager.Validate(token)
}

// validateCSRFToken checks the X-CSRF-Token header against the session-bound CSRF token.
// Constant-time compare. Returns false if either token is empty or they don't match.
func (s *Server) validateCSRFToken(r *http.Request) bool {
	if s == nil || s.sessionManager == nil || r == nil {
		return false
	}
	sessionToken := s.sessionTokenFromRequest(r)
	if sessionToken == "" {
		return false
	}
	expected := s.sessionManager.CSRFToken(sessionToken)
	if expected == "" {
		return false
	}
	provided := r.Header.Get("X-CSRF-Token")
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}
