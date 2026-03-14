package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:ui/static
var staticFS embed.FS

func formatVersionLabel(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return "vdev"
	}
	if strings.HasPrefix(strings.ToLower(v), "v") {
		return v
	}
	return "v" + v
}

func (s *Server) uiVersion() string {
	return formatVersionLabel(s.version)
}

// getAstroDistFS returns the Astro build output filesystem (ui/static/dist).
func getAstroDistFS() (fs.FS, error) {
	return fs.Sub(staticFS, "ui/static/dist")
}

// serveAstroPage serves a pre-rendered Astro HTML page from the embedded dist/ directory.
// For the root page pass pagePath = "index.html"; for sub-pages pass e.g. "queries/index.html".
func (s *Server) serveAstroPage(w http.ResponseWriter, r *http.Request, pagePath string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	distFS, err := getAstroDistFS()
	if err != nil {
		s.logger.Error("Failed to access Astro dist filesystem", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data, err := fs.ReadFile(distFS, pagePath)
	if err != nil {
		s.logger.Error("Failed to read Astro page", "path", pagePath, "error", err)
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleDashboard serves the main dashboard page (Astro pre-rendered).
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	s.serveAstroPage(w, r, "index.html")
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.isAuthenticationEnabled() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	next := sanitizeRedirectTarget(r.URL.Query().Get("next"))
	if s.hasValidSession(r) {
		http.Redirect(w, r, next, http.StatusFound)
		return
	}
	s.serveAstroPage(w, r, "login/index.html")
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.isAuthenticationEnabled() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=Invalid+form+submission", http.StatusSeeOther)
		return
	}
	next := sanitizeRedirectTarget(r.FormValue("next"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	var subject string
	switch {
	case apiKey != "" && s.validateAPIKeyInput(apiKey):
		subject = "api-key"
	case username != "" && password != "" && s.validateUserPasswordInput(username, password):
		subject = username
	default:
		http.Redirect(w, r, "/login?error=Invalid+credentials&next="+next, http.StatusSeeOther)
		return
	}

	// Prevent session fixation: revoke any existing session before creating new one
	s.revokeSession(w, r)

	if err := s.createSession(w, r, subject); err != nil {
		s.logger.Error("failed to create session", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.revokeSession(w, r)
	redirectTo := sanitizeRedirectTarget(r.FormValue("next"))
	if redirectTo == "/" {
		redirectTo = "/login"
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

// handleQueriesPage serves the queries log page (Astro pre-rendered).
func (s *Server) handleQueriesPage(w http.ResponseWriter, r *http.Request) {
	s.serveAstroPage(w, r, "queries/index.html")
}

// handlePoliciesPage serves the policies management page (Astro pre-rendered).
func (s *Server) handlePoliciesPage(w http.ResponseWriter, r *http.Request) {
	s.serveAstroPage(w, r, "policies/index.html")
}

// handleLocalRecordsPage serves the local DNS records management page (Astro pre-rendered).
func (s *Server) handleLocalRecordsPage(w http.ResponseWriter, r *http.Request) {
	s.serveAstroPage(w, r, "localrecords/index.html")
}

// handleConditionalForwardingPage serves the conditional forwarding management page (Astro pre-rendered).
func (s *Server) handleConditionalForwardingPage(w http.ResponseWriter, r *http.Request) {
	s.serveAstroPage(w, r, "forwarding/index.html")
}

// handleSettingsPage serves the settings/configuration page (Astro pre-rendered).
func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	s.serveAstroPage(w, r, "settings/index.html")
}

// handleResolverPage serves the Unbound resolver overview page (Astro pre-rendered).
func (s *Server) handleResolverPage(w http.ResponseWriter, r *http.Request) {
	s.serveAstroPage(w, r, "resolver/index.html")
}

// handleResolverSettingsPage serves the resolver settings page (Astro pre-rendered).
func (s *Server) handleResolverSettingsPage(w http.ResponseWriter, r *http.Request) {
	s.serveAstroPage(w, r, "resolver/settings/index.html")
}

// handleResolverZonesPage serves the resolver zones page (Astro pre-rendered).
func (s *Server) handleResolverZonesPage(w http.ResponseWriter, r *http.Request) {
	s.serveAstroPage(w, r, "resolver/zones/index.html")
}
