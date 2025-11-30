package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"glory-hole/pkg/storage"
)

const (
	defaultClientPageSize = 50
	maxClientPageSize     = 500
)

type clientsPageData struct {
	Version string
	Clients []*storage.ClientSummary
	Groups  []*storage.ClientGroup
	Limit   int
	Offset  int
	Page    int
}

type clientUpdateRequest struct {
	DisplayName string `json:"display_name"`
	GroupName   string `json:"group_name"`
	Notes       string `json:"notes"`
}

type clientGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

func (s *Server) handleClientsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.storage == nil {
		http.Error(w, "Storage not available", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	clients, err := s.storage.GetClientSummaries(ctx, defaultClientPageSize, 0)
	if err != nil {
		s.logger.Error("Failed to fetch client summaries", "error", err)
		http.Error(w, "Failed to load clients", http.StatusInternalServerError)
		return
	}

	groups, err := s.storage.GetClientGroups(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch client groups", "error", err)
		groups = []*storage.ClientGroup{}
	}

	data := clientsPageData{
		Version: s.uiVersion(),
		Clients: clients,
		Groups:  groups,
		Limit:   defaultClientPageSize,
		Offset:  0,
		Page:    1,
	}

	if err := clientsTemplate.ExecuteTemplate(w, "clients.html", data); err != nil {
		s.logger.Error("Failed to render clients template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleGetClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	limit := parsePositiveInt(r.URL.Query().Get("limit"), defaultClientPageSize, maxClientPageSize)
	offset := parseNonNegativeInt(r.URL.Query().Get("offset"), 0)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		ctx = storage.WithClientSearch(ctx, search)
	}

	clients, err := s.storage.GetClientSummaries(ctx, limit, offset)
	if err != nil {
		s.logger.Error("Failed to list clients", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to list clients")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"clients": clients,
		"limit":   limit,
		"offset":  offset,
	})
}

func (s *Server) handleUpdateClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	rawID := strings.TrimSpace(r.PathValue("client"))
	if rawID == "" {
		s.writeError(w, http.StatusBadRequest, "Client identifier is required")
		return
	}
	clientID, err := url.PathUnescape(rawID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid client identifier")
		return
	}

	var req clientUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid payload: %v", err))
		return
	}

	profile := &storage.ClientProfile{
		ClientIP:    clientID,
		DisplayName: strings.TrimSpace(req.DisplayName),
		GroupName:   strings.TrimSpace(req.GroupName),
		Notes:       strings.TrimSpace(req.Notes),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.storage.UpdateClientProfile(ctx, profile); err != nil {
		s.logger.Error("Failed to update client profile", "client", clientID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to update client")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *Server) handleGetClientGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	groups, err := s.storage.GetClientGroups(ctx)
	if err != nil {
		s.logger.Error("Failed to list client groups", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to list client groups")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"groups": groups,
	})
}

func (s *Server) handleCreateClientGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	s.upsertClientGroup(w, r, "")
}

func (s *Server) handleUpdateClientGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	groupName := strings.TrimSpace(r.PathValue("group"))
	s.upsertClientGroup(w, r, groupName)
}

func (s *Server) upsertClientGroup(w http.ResponseWriter, r *http.Request, pathName string) {
	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	var req clientGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid payload: %v", err))
		return
	}

	name := strings.TrimSpace(req.Name)
	if pathName != "" {
		name = pathName
	}
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "Group name is required")
		return
	}

	group := &storage.ClientGroup{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Color:       sanitizeColor(req.Color),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.storage.UpsertClientGroup(ctx, group); err != nil {
		s.logger.Error("Failed to upsert client group", "name", name, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to update client group")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *Server) handleDeleteClientGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	name := strings.TrimSpace(r.PathValue("group"))
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "Group name is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.storage.DeleteClientGroup(ctx, name); err != nil {
		if err == storage.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "Group not found")
			return
		}
		s.logger.Error("Failed to delete client group", "name", name, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to delete client group")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func sanitizeColor(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "#") && (len(trimmed) == 4 || len(trimmed) == 7) {
		return trimmed
	}
	return trimmed
}

func parsePositiveInt(raw string, fallback, max int) int {
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return fallback
	}
	if max > 0 && val > max {
		return max
	}
	return val
}

func parseNonNegativeInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val < 0 {
		return fallback
	}
	return val
}
