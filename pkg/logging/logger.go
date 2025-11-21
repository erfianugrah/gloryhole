package logging

import (
	"context"
	"io"
	"log/slog"
	"os"

	"glory-hole/pkg/config"
)

// Logger wraps slog.Logger with Glory-Hole specific functionality
type Logger struct {
	*slog.Logger
	cfg *config.LoggingConfig
}

// New creates a new logger from configuration
func New(cfg *config.LoggingConfig) (*Logger, error) {
	// Determine output writer
	var output io.Writer
	switch cfg.Output {
	case "stdout":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	case "file":
		// For file output, we'll use a simple file writer
		// In production, you might want to use lumberjack for rotation
		f, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, err
		}
		output = f
	default:
		output = os.Stdout
	}

	// Parse log level
	level := parseLevel(cfg.Level)

	// Create handler based on format
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
		// Add source location if configured (useful for debugging but adds ~1-2μs per log)
		// Recommended: true for development/debug, false for production
		AddSource: cfg.AddSource,
	}

	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(output, opts)
	case "text":
		handler = slog.NewTextHandler(output, opts)
	default:
		handler = slog.NewTextHandler(output, opts)
	}

	logger := slog.New(handler)

	return &Logger{
		Logger: logger,
		cfg:    cfg,
	}, nil
}

// NewDefault creates a logger with sensible defaults (info level, text format, stdout)
func NewDefault() *Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: false, // Default to false for performance
	})
	return &Logger{
		Logger: slog.New(handler),
		cfg: &config.LoggingConfig{
			Level:     "info",
			Format:    "text",
			Output:    "stdout",
			AddSource: false,
		},
	}
}

// WithContext adds context to the logger
func (l *Logger) WithContext(ctx context.Context) *Logger {
	return &Logger{
		Logger: l.Logger.With(),
		cfg:    l.cfg,
	}
}

// WithFields creates a new logger with additional fields
func (l *Logger) WithFields(fields map[string]any) *Logger {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{
		Logger: l.Logger.With(args...),
		cfg:    l.cfg,
	}
}

// WithField creates a new logger with an additional field
func (l *Logger) WithField(key string, value any) *Logger {
	return &Logger{
		Logger: l.Logger.With(key, value),
		cfg:    l.cfg,
	}
}

// parseLevel converts string level to slog.Level
func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Global logger instance
var global *Logger

func init() {
	global = NewDefault()
}

// SetGlobal sets the global logger
func SetGlobal(logger *Logger) {
	global = logger
	slog.SetDefault(logger.Logger)
}

// Global returns the global logger
func Global() *Logger {
	return global
}

// Convenience functions that use the global logger

// Debug logs a debug message
func Debug(msg string, args ...any) {
	global.Debug(msg, args...)
}

// Info logs an info message
func Info(msg string, args ...any) {
	global.Info(msg, args...)
}

// Warn logs a warning message
func Warn(msg string, args ...any) {
	global.Warn(msg, args...)
}

// Error logs an error message
func Error(msg string, args ...any) {
	global.Error(msg, args...)
}

// DebugContext logs a debug message with context
func DebugContext(ctx context.Context, msg string, args ...any) {
	global.DebugContext(ctx, msg, args...)
}

// InfoContext logs an info message with context
func InfoContext(ctx context.Context, msg string, args ...any) {
	global.InfoContext(ctx, msg, args...)
}

// WarnContext logs a warning message with context
func WarnContext(ctx context.Context, msg string, args ...any) {
	global.WarnContext(ctx, msg, args...)
}

// ErrorContext logs an error message with context
func ErrorContext(ctx context.Context, msg string, args ...any) {
	global.ErrorContext(ctx, msg, args...)
}
