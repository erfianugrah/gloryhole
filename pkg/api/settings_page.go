package api

import (
	"bytes"
	"errors"
	"net/http"

	"glory-hole/pkg/config"
)

const (
	settingsTemplateUpstreams = "settings-upstreams"
	settingsTemplateCache     = "settings-cache"
	settingsTemplateLogging   = "settings-logging"

	flashKeyUpstreams = "upstreams"
	flashKeyCache     = "cache"
	flashKeyLogging   = "logging"
)

// SettingsPageData is the view-model for the Settings page and HTMX partials.
type SettingsPageData struct {
	Version    string
	Config     ConfigResponse
	ConfigPath string
	Flash      map[string]string
	Errors     map[string]string
}

func (s *Server) newSettingsPageData(cfg *config.Config) *SettingsPageData {
	return &SettingsPageData{
		Version:    s.version,
		Config:     convertConfigResponse(cfg),
		ConfigPath: s.configPath,
	}
}

func (s *Server) renderSettingsPartial(w http.ResponseWriter, tmpl string, data *SettingsPageData, status int) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, tmpl, data); err != nil {
		s.logger.Error("Failed to render settings partial", "template", tmpl, "error", err)
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) respondConfigUpdate(w http.ResponseWriter, r *http.Request, tmpl, flashKey, message string, data *SettingsPageData) {
	if isHTMXRequest(r) && tmpl != "" {
		if message != "" {
			if data.Flash == nil {
				data.Flash = make(map[string]string)
			}
			data.Flash[flashKey] = message
		}
		s.renderSettingsPartial(w, tmpl, data, http.StatusOK)
		return
	}

	response := ConfigUpdateResponse{
		Status:  statusOK,
		Message: message,
		Config:  data.Config,
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) respondConfigValidationError(w http.ResponseWriter, r *http.Request, tmpl, errorKey, message string, cfg *config.Config, status int) {
	if isHTMXRequest(r) && tmpl != "" {
		data := s.newSettingsPageData(cfg)
		if data.Errors == nil {
			data.Errors = make(map[string]string)
		}
		data.Errors[errorKey] = message
		s.renderSettingsPartial(w, tmpl, data, status)
		return
	}

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

func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
