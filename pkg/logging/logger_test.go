package logging

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"glory-hole/pkg/config"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.LoggingConfig
		wantErr bool
	}{
		{
			name: "text format stdout",
			cfg: &config.LoggingConfig{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			},
			wantErr: false,
		},
		{
			name: "json format stderr",
			cfg: &config.LoggingConfig{
				Level:  "debug",
				Format: "json",
				Output: "stderr",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && logger == nil {
				t.Error("New() returned nil logger")
			}
		})
	}
}

func TestNewDefault(t *testing.T) {
	logger := NewDefault()
	if logger == nil {
		t.Fatal("NewDefault() returned nil")
	}
	if logger.cfg.Level != "info" {
		t.Errorf("Expected default level info, got %s", logger.cfg.Level)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		level string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo}, // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := parseLevel(tt.level)
			if got != tt.want {
				t.Errorf("parseLevel(%s) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestWithField(t *testing.T) {
	logger := NewDefault()
	newLogger := logger.WithField("test_key", "test_value")

	if newLogger == nil {
		t.Fatal("WithField() returned nil")
	}
	if newLogger == logger {
		t.Error("WithField() should return a new logger instance")
	}
}

func TestWithFields(t *testing.T) {
	logger := NewDefault()
	fields := map[string]any{
		"key1": "value1",
		"key2": 42,
	}
	newLogger := logger.WithFields(fields)

	if newLogger == nil {
		t.Fatal("WithFields() returned nil")
	}
	if newLogger == logger {
		t.Error("WithFields() should return a new logger instance")
	}
}

func TestGlobalLogger(t *testing.T) {
	// Test that global logger exists
	globalLogger := Global()
	if globalLogger == nil {
		t.Fatal("Global() returned nil")
	}

	// Test setting global logger
	newLogger := NewDefault()
	SetGlobal(newLogger)

	if Global() != newLogger {
		t.Error("SetGlobal() did not update global logger")
	}
}

func TestLoggingOutput(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Create a custom handler that writes to buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := &Logger{
		Logger: slog.New(handler),
		cfg: &config.LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		},
	}

	// Log a message
	logger.Info("test message", "key", "value")

	// Check that output contains our message
	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Log output doesn't contain message. Got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Log output doesn't contain key-value pair. Got: %s", output)
	}
}

func TestContextLogging(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := &Logger{
		Logger: slog.New(handler),
		cfg: &config.LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		},
	}

	ctx := context.Background()
	logger.InfoContext(ctx, "context message")

	output := buf.String()
	if !strings.Contains(output, "context message") {
		t.Errorf("Context log output doesn't contain message. Got: %s", output)
	}
}

func TestFileOutput(t *testing.T) {
	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "test-log-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	_ = tmpfile.Close()

	cfg := &config.LoggingConfig{
		Level:    "info",
		Format:   "text",
		Output:   "file",
		FilePath: tmpfile.Name(),
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger with file output: %v", err)
	}

	logger.Info("test file message")

	// Read the file and check content
	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test file message") {
		t.Errorf("Log file doesn't contain message. Got: %s", string(content))
	}
}
