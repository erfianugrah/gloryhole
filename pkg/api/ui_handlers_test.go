package api

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHandleDashboard(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test-1.0.0",
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	server.handleDashboard(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Expected text/html content type, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestHandleDashboard_WrongMethod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()

	server.handleDashboard(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestHandleQueriesPage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test-1.0.0",
	})

	req := httptest.NewRequest("GET", "/queries", nil)
	w := httptest.NewRecorder()

	server.handleQueriesPage(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleQueriesPage_WrongMethod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("DELETE", "/queries", nil)
	w := httptest.NewRecorder()

	server.handleQueriesPage(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestHandlePoliciesPage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test-1.0.0",
	})

	req := httptest.NewRequest("GET", "/policies", nil)
	w := httptest.NewRecorder()

	server.handlePoliciesPage(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleSettingsPage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test-1.0.0",
	})

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()

	server.handleSettingsPage(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Settings") {
		t.Errorf("expected settings page to contain 'Settings'")
	}
}

func TestGetAstroDistFS(t *testing.T) {
	distFS, err := getAstroDistFS()
	if err != nil {
		t.Fatalf("getAstroDistFS() failed: %v", err)
	}

	if distFS == nil {
		t.Fatal("Astro dist FS should not be nil")
	}

	// Verify key Astro build output files exist
	files := []string{"index.html", "queries/index.html", "login/index.html", "favicon.svg"}
	for _, path := range files {
		if _, err := fs.Stat(distFS, path); err != nil {
			t.Errorf("Expected file %s in Astro dist: %v", path, err)
		}
	}
}

func TestServeAstroPage_AllPages(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	pages := []struct {
		name string
		path string
	}{
		{"dashboard", "index.html"},
		{"queries", "queries/index.html"},
		{"policies", "policies/index.html"},
		{"localrecords", "localrecords/index.html"},
		{"forwarding", "forwarding/index.html"},
		{"blocklists", "blocklists/index.html"},
		{"clients", "clients/index.html"},
		{"settings", "settings/index.html"},
		{"login", "login/index.html"},
	}

	for _, p := range pages {
		t.Run(p.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/"+p.name, nil)
			w := httptest.NewRecorder()

			server.serveAstroPage(w, req, p.path)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200 for %s, got %d", p.name, resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
				t.Errorf("Expected text/html for %s, got %s", p.name, ct)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), "<!DOCTYPE html>") {
				t.Errorf("Expected HTML doctype in %s response", p.name)
			}
		})
	}
}

func TestServeAstroPage_NotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	server.serveAstroPage(w, req, "nonexistent/index.html")

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestFormatVersionLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "vdev"},
		{"  ", "vdev"},
		{"1.0.0", "v1.0.0"},
		{"v1.0.0", "v1.0.0"},
		{"V1.0.0", "V1.0.0"},
		{"dev", "vdev"},
	}

	for _, tt := range tests {
		result := formatVersionLabel(tt.input)
		if result != tt.expected {
			t.Errorf("formatVersionLabel(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}
