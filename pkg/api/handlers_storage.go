package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type storageResetRequest struct {
	Confirm string `json:"confirm"`
}

// handleStorageReset handles POST /api/storage/reset
// Requires the caller to send {"confirm":"NUKE"} to avoid accidental wipes.
func (s *Server) handleStorageReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	var req storageResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if !strings.EqualFold(strings.TrimSpace(req.Confirm), "nuke") {
		s.writeError(w, http.StatusBadRequest, "Confirmation phrase mismatch")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := s.storage.Reset(ctx); err != nil {
		s.logger.Error("Failed to reset storage", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to reset storage")
		return
	}

	s.logger.Warn("Storage reset invoked; all query data deleted")

	response := StorageResetResponse{
		Status:  statusOK,
		Message: "All query data deleted",
	}

	s.writeJSON(w, http.StatusOK, response)
}
