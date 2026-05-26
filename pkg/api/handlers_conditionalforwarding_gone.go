package api

import (
	"encoding/json"
	"net/http"
)

// handleConditionalForwardingGone returns 410 Gone for any request hitting
// /api/conditionalforwarding* routes.
//
// The Conditional Forwarding subsystem was deprecated in v0.26 (rules
// migrated automatically to Policy FORWARD) and removed in v0.27. The
// 410-Gone stub gives third-party tooling pinned to the old URL a clear
// migration target. Slated for full removal in v0.28+.
//
// See docs/plans/2026-05-25-v026-policy-consolidation.md §1 and
// docs/plans/2026-05-26-v027-cf-deletion-and-clientgroups.md.
func (s *Server) handleConditionalForwardingGone(w http.ResponseWriter, r *http.Request) {
	if s.logger != nil {
		s.logger.InfoContext(r.Context(), "conditional forwarding endpoint hit after removal — returning 410",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":         "gone",
		"message":       "Conditional Forwarding has been removed. Use Policy rules with action=FORWARD instead.",
		"migrate_to":    "/api/policies",
		"removed_in":    "0.27.0",
		"deprecated_in": "0.26.0",
	})
}
