package api

import (
	"html/template"
	"net"
	"net/http"
	"strings"
)

// blockPageTemplate is a self-contained HTML page served to browsers when
// a blocked domain resolves to the glory-hole server's IP.
var blockPageTemplate = template.Must(template.New("block").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Blocked - Glory-Hole DNS</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", sans-serif;
    background: #0a0a0f;
    color: #e4e4e7;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    padding: 1rem;
  }
  .card {
    background: #18181b;
    border: 1px solid #27272a;
    border-radius: 12px;
    padding: 2.5rem;
    max-width: 480px;
    width: 100%;
    text-align: center;
  }
  .icon {
    width: 48px;
    height: 48px;
    margin: 0 auto 1.5rem;
    background: rgba(239, 68, 68, 0.15);
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .icon svg { width: 24px; height: 24px; color: #ef4444; }
  h1 {
    font-size: 1.25rem;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }
  .domain {
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
    font-size: 0.875rem;
    background: #27272a;
    border: 1px solid #3f3f46;
    border-radius: 6px;
    padding: 0.5rem 1rem;
    margin: 1rem 0;
    word-break: break-all;
    color: #ef4444;
  }
  .desc {
    font-size: 0.8125rem;
    color: #a1a1aa;
    line-height: 1.5;
  }
  .footer {
    margin-top: 1.5rem;
    padding-top: 1rem;
    border-top: 1px solid #27272a;
    font-size: 0.75rem;
    color: #52525b;
  }
  .footer a { color: #71717a; text-decoration: none; }
  .footer a:hover { color: #a1a1aa; }
</style>
</head>
<body>
<div class="card">
  <div class="icon">
    <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" />
    </svg>
  </div>
  <h1>Domain Blocked</h1>
  <div class="domain">{{.Domain}}</div>
  <p class="desc">
    This domain has been blocked by your DNS server.
    If you believe this is a mistake, contact your network administrator.
  </p>
  <div class="footer">
    Protected by <a href="/">Glory-Hole DNS</a>
  </div>
</div>
</body>
</html>`))

// blockPageData holds the template context for the block page.
type blockPageData struct {
	Domain string
}

// blockPageMiddleware intercepts requests to unrecognized hosts and serves
// the block page instead. This catches browsers visiting blocked domains
// that resolve to the glory-hole server's IP.
func (s *Server) blockPageMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		// Strip port
		if idx := strings.LastIndex(host, ":"); idx != -1 {
			host = host[:idx]
		}

		// Allow requests to the server's own addresses (dashboard)
		// localhost, 127.0.0.1, and the configured listen address pass through
		if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
			next.ServeHTTP(w, r)
			return
		}

		// Allow API requests (they use the correct host or IP)
		if strings.HasPrefix(r.URL.Path, "/api/") ||
			strings.HasPrefix(r.URL.Path, "/_astro/") ||
			r.URL.Path == "/dns-query" ||
			r.URL.Path == "/health" ||
			r.URL.Path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if it looks like a blocked domain (not an IP address)
		if net.ParseIP(host) != nil {
			next.ServeHTTP(w, r)
			return
		}

		// Host is a domain name that resolved to us -- it's likely blocked
		s.handleBlockPage(w, r)
	})
}

// handleBlockPage serves the block page for any HTTP request whose Host header
// doesn't match the glory-hole dashboard. This is the page browsers see when a
// blocked domain resolves to the glory-hole server's own IP.
func (s *Server) handleBlockPage(w http.ResponseWriter, r *http.Request) {
	domain := r.Host
	// Strip port if present
	if idx := strings.LastIndex(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusForbidden)

	if err := blockPageTemplate.Execute(w, blockPageData{Domain: domain}); err != nil {
		s.logger.Error("Failed to render block page", "error", err)
	}
}
