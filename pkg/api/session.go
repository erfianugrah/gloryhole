package api

import (
	"crypto/rand"
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
}

type sessionData struct {
	subject string
	expires time.Time
}

func newSessionManager(ttl time.Duration) *sessionManager {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &sessionManager{
		sessions: make(map[string]sessionData),
		ttl:      ttl,
	}
}

func (m *sessionManager) Create(subject string) (string, time.Time, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", time.Time{}, err
	}

	expiry := time.Now().Add(m.ttl)

	m.mu.Lock()
	m.sessions[token] = sessionData{subject: subject, expires: expiry}
	m.mu.Unlock()

	return token, expiry, nil
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

func (m *sessionManager) Revoke(token string) {
	if token == "" {
		return
	}
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request, subject string) error {
	if s.sessionManager == nil {
		return errors.New("session manager unavailable")
	}
	token, expiry, err := s.sessionManager.Create(subject)
	if err != nil {
		return err
	}
	secure := r != nil && r.TLS != nil
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
		SameSite: http.SameSiteLaxMode,
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
	secure := r != nil && r.TLS != nil
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
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
