package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"glory-hole/pkg/config"
)

// TestHandleGetFeatures tests GET /api/features endpoint
func TestHandleGetFeatures(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	cfg := config.LoadWithDefaults()
	cfg.Server.EnableBlocklist = true
	cfg.Server.EnablePolicies = false

	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	// Create config watcher
	watcher, err := config.NewWatcher(configPath, nil)
	if err != nil {
		t.Fatalf("failed to create config watcher: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":8080",
		ConfigWatcher: watcher,
		ConfigPath:    configPath,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/features", nil)
	w := httptest.NewRecorder()

	server.handleGetFeatures(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response FeaturesResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.BlocklistEnabled {
		t.Error("expected blocklist to be enabled")
	}

	if response.PoliciesEnabled {
		t.Error("expected policies to be disabled")
	}

	if response.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

// TestHandleGetFeatures_MethodNotAllowed tests that non-GET methods are rejected
func TestHandleGetFeatures_MethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")
	cfg := config.LoadWithDefaults()
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	watcher, err := config.NewWatcher(configPath, nil)
	if err != nil {
		t.Fatalf("failed to create config watcher: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":8080",
		ConfigWatcher: watcher,
		ConfigPath:    configPath,
	})

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/features", nil)
		w := httptest.NewRecorder()

		server.handleGetFeatures(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected status 405, got %d", method, w.Code)
		}
	}
}

// TestHandleUpdateFeatures_EnableBlocklist tests enabling blocklist via API
func TestHandleUpdateFeatures_EnableBlocklist(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	cfg := config.LoadWithDefaults()
	cfg.Server.EnableBlocklist = false
	cfg.Server.EnablePolicies = true

	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	watcher, err := config.NewWatcher(configPath, nil)
	if err != nil {
		t.Fatalf("failed to create config watcher: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":8080",
		ConfigWatcher: watcher,
		ConfigPath:    configPath,
	})

	enabled := true
	reqBody := FeaturesRequest{
		BlocklistEnabled: &enabled,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/features", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleUpdateFeatures(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response FeaturesResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.BlocklistEnabled {
		t.Error("expected blocklist to be enabled")
	}

	// Verify config was persisted
	loadedCfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !loadedCfg.Server.EnableBlocklist {
		t.Error("expected blocklist to be enabled in persisted config")
	}
}

// TestHandleUpdateFeatures_DisablePolicies tests disabling policies via API
func TestHandleUpdateFeatures_DisablePolicies(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	cfg := config.LoadWithDefaults()
	cfg.Server.EnableBlocklist = true
	cfg.Server.EnablePolicies = true

	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	watcher, err := config.NewWatcher(configPath, nil)
	if err != nil {
		t.Fatalf("failed to create config watcher: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":8080",
		ConfigWatcher: watcher,
		ConfigPath:    configPath,
	})

	disabled := false
	reqBody := FeaturesRequest{
		PoliciesEnabled: &disabled,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/features", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleUpdateFeatures(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response FeaturesResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.PoliciesEnabled {
		t.Error("expected policies to be disabled")
	}

	// Verify config was persisted
	loadedCfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loadedCfg.Server.EnablePolicies {
		t.Error("expected policies to be disabled in persisted config")
	}
}

// TestHandleUpdateFeatures_UpdateBoth tests updating both kill-switches simultaneously
// Note: At least one feature must remain enabled due to backward compatibility logic
func TestHandleUpdateFeatures_UpdateBoth(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	cfg := config.LoadWithDefaults()
	cfg.Server.EnableBlocklist = false
	cfg.Server.EnablePolicies = false

	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	watcher, err := config.NewWatcher(configPath, nil)
	if err != nil {
		t.Fatalf("failed to create config watcher: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":8080",
		ConfigWatcher: watcher,
		ConfigPath:    configPath,
	})

	// Enable both (from disabled state)
	blocklistEnabled := true
	policiesEnabled := true
	reqBody := FeaturesRequest{
		BlocklistEnabled: &blocklistEnabled,
		PoliciesEnabled:  &policiesEnabled,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/features", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleUpdateFeatures(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response FeaturesResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.BlocklistEnabled {
		t.Error("expected blocklist to be enabled")
	}

	if !response.PoliciesEnabled {
		t.Error("expected policies to be enabled")
	}

	// Verify config was persisted
	loadedCfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !loadedCfg.Server.EnableBlocklist {
		t.Error("expected blocklist to be enabled in persisted config")
	}

	if !loadedCfg.Server.EnablePolicies {
		t.Error("expected policies to be enabled in persisted config")
	}
}

// TestHandleUpdateFeatures_NoChanges tests that empty request returns error
func TestHandleUpdateFeatures_NoChanges(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	cfg := config.LoadWithDefaults()
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	watcher, err := config.NewWatcher(configPath, nil)
	if err != nil {
		t.Fatalf("failed to create config watcher: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":8080",
		ConfigWatcher: watcher,
		ConfigPath:    configPath,
	})

	reqBody := FeaturesRequest{}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/features", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleUpdateFeatures(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestHandleUpdateFeatures_InvalidJSON tests that invalid JSON returns error
func TestHandleUpdateFeatures_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	cfg := config.LoadWithDefaults()
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	watcher, err := config.NewWatcher(configPath, nil)
	if err != nil {
		t.Fatalf("failed to create config watcher: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":8080",
		ConfigWatcher: watcher,
		ConfigPath:    configPath,
	})

	req := httptest.NewRequest(http.MethodPut, "/api/features", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleUpdateFeatures(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestHandleUpdateFeatures_MethodNotAllowed tests that non-PUT methods are rejected
func TestHandleUpdateFeatures_MethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	cfg := config.LoadWithDefaults()
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	watcher, err := config.NewWatcher(configPath, nil)
	if err != nil {
		t.Fatalf("failed to create config watcher: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":8080",
		ConfigWatcher: watcher,
		ConfigPath:    configPath,
	})

	methods := []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/features", nil)
		w := httptest.NewRecorder()

		server.handleUpdateFeatures(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected status 405, got %d", method, w.Code)
		}
	}
}

// TestHandleUpdateFeatures_ReadOnlyConfig tests handling when config cannot be written
func TestHandleUpdateFeatures_ReadOnlyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	cfg := config.LoadWithDefaults()
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	// Make config read-only
	if err := os.Chmod(configPath, 0444); err != nil {
		t.Fatalf("failed to make config read-only: %v", err)
	}
	// Make directory read-only to prevent temp file creation
	if err := os.Chmod(tmpDir, 0555); err != nil {
		t.Fatalf("failed to make directory read-only: %v", err)
	}
	defer os.Chmod(tmpDir, 0755) // Restore for cleanup

	watcher, err := config.NewWatcher(configPath, nil)
	if err != nil {
		t.Fatalf("failed to create config watcher: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":8080",
		ConfigWatcher: watcher,
		ConfigPath:    configPath,
	})

	enabled := true
	reqBody := FeaturesRequest{
		BlocklistEnabled: &enabled,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/features", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleUpdateFeatures(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

// TestHandleUpdateFeatures_LargePayload tests request body size limit
func TestHandleUpdateFeatures_LargePayload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	cfg := config.LoadWithDefaults()
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	watcher, err := config.NewWatcher(configPath, nil)
	if err != nil {
		t.Fatalf("failed to create config watcher: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":8080",
		ConfigWatcher: watcher,
		ConfigPath:    configPath,
	})

	// Create a payload larger than 1MB
	largePayload := make([]byte, 2*1024*1024)
	for i := range largePayload {
		largePayload[i] = 'a'
	}

	req := httptest.NewRequest(http.MethodPut, "/api/features", bytes.NewReader(largePayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleUpdateFeatures(w, req)

	// Should fail due to size limit
	if w.Code == http.StatusOK {
		t.Error("expected request to fail due to size limit")
	}
}
