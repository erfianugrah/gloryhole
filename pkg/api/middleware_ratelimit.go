package api

import (
	"net"
	"net/http"
	"strings"
)

// rateLimitMiddleware enforces per-IP limits on the HTTP API.
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	if s.rateLimiter == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			// Skip limiter for CORS preflight
			next.ServeHTTP(w, r)
			return
		}

		clientIP := clientIPFromRequest(r)
		allowed, limited, _, label := s.rateLimiter.Allow(clientIP)
		if allowed {
			next.ServeHTTP(w, r)
			return
		}

		if limited && s.rateLimiter.LogViolations() {
			s.logger.Warn("HTTP request rate limited",
				"client_ip", clientIP,
				"path", r.URL.Path,
				"label", label,
			)
		}

		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
	})
}

func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for _, part := range parts {
			if ip := strings.TrimSpace(part); ip != "" {
				return ip
			}
		}
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}

	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}

	return r.RemoteAddr
}
