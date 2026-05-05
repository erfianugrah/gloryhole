package api

import (
	"errors"
	"fmt"
	"net/http"

	"glory-hole/pkg/config"
)

// SettingsPageData is the view-model for settings configuration endpoints.
type SettingsPageData struct {
	Version    string
	Config     ConfigResponse
	ConfigPath string
}

func (s *Server) newSettingsPageData(cfg *config.Config) *SettingsPageData {
	return &SettingsPageData{
		Version:    s.uiVersion(),
		Config:     convertConfigResponse(cfg),
		ConfigPath: s.configPath,
	}
}

func (s *Server) respondConfigUpdate(w http.ResponseWriter, _ *http.Request, _, _, message string, data *SettingsPageData) {
	response := ConfigUpdateResponse{
		Status:  statusOK,
		Message: message,
		Config:  data.Config,
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) respondConfigValidationError(w http.ResponseWriter, _ *http.Request, _, _, message string, _ *config.Config, status int) {
	s.writeError(w, status, message)
}

func (s *Server) mutableConfig() (*config.Config, error) {
	if s.configPath == "" {
		return nil, errors.New("configuration path not set")
	}
	cfg := s.currentConfig()
	if cfg == nil {
		return nil, errors.New("configuration not available")
	}
	return cfg, nil
}

func (s *Server) persistConfigSection(w http.ResponseWriter, r *http.Request, updated *config.Config, tmpl, errorKey string, current *config.Config) bool {
	if s.configPath == "" {
		s.respondConfigValidationError(
			w, r, tmpl, errorKey,
			"Configuration path is not set; settings are read-only in this deployment",
			current,
			http.StatusServiceUnavailable,
		)
		return false
	}

	if err := config.Save(s.configPath, updated); err != nil {
		s.logger.Error("Failed to save configuration", "error", err)
		s.respondConfigValidationError(
			w, r, tmpl, errorKey,
			fmt.Sprintf("Failed to save configuration: %v", err),
			current,
			http.StatusInternalServerError,
		)
		return false
	}

	// Trigger immediate reload so the watcher picks up the new config
	// without waiting for fsnotify. Don't mutate *current in-place — it
	// points to the shared config.Watcher struct and concurrent readers
	// could observe torn slice headers.
	if s.configWatcher != nil {
		if err := s.configWatcher.Reload(); err != nil {
			s.logger.Error("Failed to reload config after save", "error", err)
		}
	}

	return true
}
