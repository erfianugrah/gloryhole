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

		clientIP := s.clientIPFromRequest(r)
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

func (s *Server) clientIPFromRequest(r *http.Request) string {
	// Extract the actual remote address first
	remoteAddr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}

	// Only trust X-Forwarded-For/X-Real-IP if the request comes from a trusted proxy
	// Default behavior: DO NOT trust client-controlled headers (secure by default)
	if len(s.trustedProxies) > 0 {
		remoteIP := net.ParseIP(remoteAddr)
		if remoteIP != nil {
			// Check if remote IP is in any trusted proxy CIDR
			trusted := false
			for _, network := range s.trustedProxies {
				if network.Contains(remoteIP) {
					trusted = true
					break
				}
			}

			// Only use headers if request is from a trusted proxy
			if trusted {
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
			}
		}
	}

	// Fall back to remote address (untrusted proxy or no headers)
	return remoteAddr
}
