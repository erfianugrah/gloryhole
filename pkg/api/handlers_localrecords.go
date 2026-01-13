package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"

	"glory-hole/pkg/config"
	"glory-hole/pkg/localrecords"
)

// LocalRecordResponse represents a single local DNS record in API responses
type LocalRecordResponse struct {
	ID         string   `json:"id"`          // Unique identifier (domain:type:index)
	Domain     string   `json:"domain"`
	Type       string   `json:"type"`
	Target     string   `json:"target,omitempty"`
	IPs        []string `json:"ips,omitempty"`
	TxtRecords []string `json:"txt_records,omitempty"`
	TTL        uint32   `json:"ttl"`
	Priority   *uint16  `json:"priority,omitempty"`
	Weight     *uint16  `json:"weight,omitempty"`
	Port       *uint16  `json:"port,omitempty"`
	Wildcard   bool     `json:"wildcard"`
	Enabled    bool     `json:"enabled"`
}

// LocalRecordsListResponse represents the list of local DNS records
type LocalRecordsListResponse struct {
	Records []LocalRecordResponse `json:"records"`
	Total   int                   `json:"total"`
}

// LocalRecordAddRequest represents a request to add a local DNS record
type LocalRecordAddRequest struct {
	Domain     string   `json:"domain"`
	Type       string   `json:"type"`
	Target     string   `json:"target,omitempty"`
	IPs        []string `json:"ips,omitempty"`
	TxtRecords []string `json:"txt_records,omitempty"`
	TTL        uint32   `json:"ttl"`
	Priority   *uint16  `json:"priority,omitempty"`
	Weight     *uint16  `json:"weight,omitempty"`
	Port       *uint16  `json:"port,omitempty"`
	Wildcard   bool     `json:"wildcard"`
}

// handleGetLocalRecords returns all local DNS records
func (s *Server) handleGetLocalRecords(w http.ResponseWriter, r *http.Request) {
	records := make([]LocalRecordResponse, 0)

	// Get local records from config (source of truth)
	cfg := s.currentConfig()
	if cfg != nil && cfg.LocalRecords.Enabled {
		for idx, entry := range cfg.LocalRecords.Records {
			// Generate unique ID
			id := fmt.Sprintf("%s:%s:%d", entry.Domain, entry.Type, idx)

			records = append(records, LocalRecordResponse{
				ID:         id,
				Domain:     entry.Domain,
				Type:       entry.Type,
				Target:     entry.Target,
				IPs:        entry.IPs,
				TxtRecords: entry.TxtRecords,
				TTL:        entry.TTL,
				Priority:   entry.Priority,
				Weight:     entry.Weight,
				Port:       entry.Port,
				Wildcard:   entry.Wildcard,
				Enabled:    true, // All records in config are considered enabled
			})
		}
	}

	// Sort records by domain, then type
	sort.Slice(records, func(i, j int) bool {
		if records[i].Domain != records[j].Domain {
			return records[i].Domain < records[j].Domain
		}
		return records[i].Type < records[j].Type
	})

	// Check if request wants HTML (from HTMX or browser)
	if r.Header.Get("HX-Request") == "true" || r.Header.Get("Accept") == "text/html" {
		// Return HTML partial
		data := struct {
			Records []LocalRecordResponse
		}{
			Records: records,
		}

		if err := localRecordsPartialTemplate.Execute(w, data); err != nil {
			s.logger.Error("Failed to render local records partial", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Return JSON
	s.writeJSON(w, http.StatusOK, LocalRecordsListResponse{
		Records: records,
		Total:   len(records),
	})
}

// handleAddLocalRecord adds a new local DNS record
func (s *Server) handleAddLocalRecord(w http.ResponseWriter, r *http.Request) {
	if s.dnsHandler == nil {
		s.writeError(w, http.StatusInternalServerError, "DNS handler not configured")
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var req LocalRecordAddRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate required fields
	if req.Domain == "" {
		s.writeError(w, http.StatusBadRequest, "Domain is required")
		return
	}
	if req.Type == "" {
		s.writeError(w, http.StatusBadRequest, "Record type is required")
		return
	}
	if req.TTL == 0 {
		req.TTL = 300 // Default TTL of 5 minutes
	}

	// Validate record type and required fields
	switch strings.ToUpper(req.Type) {
	case "A", "AAAA":
		if len(req.IPs) == 0 {
			s.writeError(w, http.StatusBadRequest, "IPs are required for A/AAAA records")
			return
		}
		// Validate IP addresses
		for _, ipStr := range req.IPs {
			if net.ParseIP(ipStr) == nil {
				s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid IP address: %s", ipStr))
				return
			}
		}
	case "CNAME", "PTR":
		if req.Target == "" {
			s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Target is required for %s records", req.Type))
			return
		}
	case "MX":
		if req.Target == "" {
			s.writeError(w, http.StatusBadRequest, "Target is required for MX records")
			return
		}
		if req.Priority == nil {
			defaultPriority := uint16(10)
			req.Priority = &defaultPriority
		}
	case "SRV":
		if req.Target == "" {
			s.writeError(w, http.StatusBadRequest, "Target is required for SRV records")
			return
		}
		if req.Priority == nil || req.Weight == nil || req.Port == nil {
			s.writeError(w, http.StatusBadRequest, "Priority, weight, and port are required for SRV records")
			return
		}
	case "TXT":
		if len(req.TxtRecords) == 0 {
			s.writeError(w, http.StatusBadRequest, "TXT records are required for TXT type")
			return
		}
	default:
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Unsupported record type: %s", req.Type))
		return
	}

	// Create config entry
	entry := config.LocalRecordEntry{
		Domain:     req.Domain,
		Type:       strings.ToUpper(req.Type),
		Target:     req.Target,
		IPs:        req.IPs,
		TxtRecords: req.TxtRecords,
		TTL:        req.TTL,
		Priority:   req.Priority,
		Weight:     req.Weight,
		Port:       req.Port,
		Wildcard:   req.Wildcard,
	}

	// Persist to config
	if err := s.persistLocalRecordsConfig(func(cfg *config.Config) error {
		cfg.LocalRecords.Enabled = true
		cfg.LocalRecords.Records = append(cfg.LocalRecords.Records, entry)
		return nil
	}); err != nil {
		s.logger.Error("Failed to persist local record to config", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to save record")
		return
	}

	// Reload local records in DNS handler
	if err := s.reloadLocalRecords(); err != nil {
		s.logger.Error("Failed to reload local records", "error", err)
		// Don't fail the request - config was saved successfully
	}

	// Log the action
	s.logger.Info("Added local DNS record",
		"domain", req.Domain,
		"type", req.Type,
		"user", "admin") // TODO: Get actual user from session

	// Return updated list
	s.handleGetLocalRecords(w, r)
}

// handleRemoveLocalRecord removes a local DNS record
func (s *Server) handleRemoveLocalRecord(w http.ResponseWriter, r *http.Request) {
	if s.dnsHandler == nil {
		s.writeError(w, http.StatusInternalServerError, "DNS handler not configured")
		return
	}

	recordID := r.PathValue("id")
	if recordID == "" {
		s.writeError(w, http.StatusBadRequest, "Record ID parameter required")
		return
	}

	// Parse record ID (format: domain:type:index)
	parts := strings.Split(recordID, ":")
	if len(parts) != 3 {
		s.writeError(w, http.StatusBadRequest, "Invalid record ID format")
		return
	}

	domain := parts[0]
	recordType := parts[1]

	removed := false

	// Persist to config - remove from config list
	if err := s.persistLocalRecordsConfig(func(cfg *config.Config) error {
		newRecords := make([]config.LocalRecordEntry, 0, len(cfg.LocalRecords.Records))
		for _, record := range cfg.LocalRecords.Records {
			// Match by domain and type
			if record.Domain == domain && record.Type == recordType {
				// Skip this record (remove it)
				removed = true
				continue
			}
			newRecords = append(newRecords, record)
		}

		if !removed {
			return fmt.Errorf("record not found")
		}

		cfg.LocalRecords.Records = newRecords
		return nil
	}); err != nil {
		s.logger.Error("Failed to persist local records to config", "error", err)
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, "Record not found")
		} else {
			s.writeError(w, http.StatusInternalServerError, "Failed to remove record")
		}
		return
	}

	// Reload local records in DNS handler
	if err := s.reloadLocalRecords(); err != nil {
		s.logger.Error("Failed to reload local records", "error", err)
	}

	// Log the action
	s.logger.Info("Removed local DNS record",
		"domain", domain,
		"type", recordType,
		"user", "admin") // TODO: Get actual user from session

	// Return updated list
	s.handleGetLocalRecords(w, r)
}

// persistLocalRecordsConfig persists local records changes to config file
func (s *Server) persistLocalRecordsConfig(mutator func(cfg *config.Config) error) error {
	if s.configPath == "" {
		// In tests or ephemeral runs without a config file, keep records in-memory only.
		return nil
	}
	cfg := s.currentConfig()
	if cfg == nil {
		return fmt.Errorf("no config available")
	}

	// Clone config
	cloned, err := cfg.Clone()
	if err != nil {
		return fmt.Errorf("failed to clone config: %w", err)
	}

	// Apply mutation
	if err := mutator(cloned); err != nil {
		return err
	}

	// Validate
	if err := cloned.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Write to disk
	if err := config.Save(s.configPath, cloned); err != nil {
		return err
	}

	// Force reload config from disk to update in-memory cache
	// (fsnotify may not work reliably in all environments like WSL)
	if s.configWatcher != nil {
		if err := s.configWatcher.Reload(); err != nil {
			s.logger.Warn("Failed to reload config after persist", "error", err)
			// Don't fail - the file was saved successfully
		}
	}

	return nil
}

// reloadLocalRecords reloads local records from config into DNS handler
func (s *Server) reloadLocalRecords() error {
	cfg := s.currentConfig()
	if cfg == nil {
		return fmt.Errorf("no config available")
	}

	if !cfg.LocalRecords.Enabled || len(cfg.LocalRecords.Records) == 0 {
		// Disable local records
		s.dnsHandler.SetLocalRecords(nil)
		return nil
	}

	// Create new manager and populate from config
	mgr := localrecords.NewManager()
	for _, entry := range cfg.LocalRecords.Records {
		// Parse IPs
		ips := make([]net.IP, 0, len(entry.IPs))
		for _, ipStr := range entry.IPs {
			if ip := net.ParseIP(ipStr); ip != nil {
				ips = append(ips, ip)
			}
		}

		record := &localrecords.LocalRecord{
			Domain:     entry.Domain,
			Type:       localrecords.RecordType(entry.Type),
			Target:     entry.Target,
			IPs:        ips,
			TxtRecords: entry.TxtRecords,
			TTL:        entry.TTL,
			Wildcard:   entry.Wildcard,
			Enabled:    true,
		}

		// Set optional fields
		if entry.Priority != nil {
			record.Priority = *entry.Priority
		}
		if entry.Weight != nil {
			record.Weight = *entry.Weight
		}
		if entry.Port != nil {
			record.Port = *entry.Port
		}

		if err := mgr.AddRecord(record); err != nil {
			s.logger.Error("Failed to add local record", "error", err, "domain", entry.Domain, "type", entry.Type)
			continue
		}
	}

	// Update DNS handler
	s.dnsHandler.SetLocalRecords(mgr)
	return nil
}
