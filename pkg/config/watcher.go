package config

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches configuration files for changes and reloads them
type Watcher struct {
	path     string
	cfg      *Config
	mu       sync.RWMutex
	watcher  *fsnotify.Watcher
	onChange func(*Config)
	logger   *slog.Logger
}

// NewWatcher creates a new configuration file watcher
func NewWatcher(path string, logger *slog.Logger) (*Watcher, error) {
	// Load initial config
	cfg, err := Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load initial config: %w", err)
	}

	// Create file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Add config file to watcher
	if err := watcher.Add(path); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch config file: %w", err)
	}

	w := &Watcher{
		path:    path,
		cfg:     cfg,
		watcher: watcher,
		logger:  logger,
	}

	return w, nil
}

// Config returns the current configuration (thread-safe)
func (w *Watcher) Config() *Config {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cfg
}

// OnChange registers a callback to be called when config changes
func (w *Watcher) OnChange(fn func(*Config)) {
	w.onChange = fn
}

// Start begins watching the configuration file for changes
func (w *Watcher) Start(ctx context.Context) error {
	w.logger.Info("Starting config file watcher", "path", w.path)

	// Debounce rapid file changes (editors often write multiple times)
	debounceTimer := time.NewTimer(0)
	debounceTimer.Stop()
	const debounceDelay = 100 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Config watcher stopped")
			return w.watcher.Close()

		case event, ok := <-w.watcher.Events:
			if !ok {
				return fmt.Errorf("watcher events channel closed")
			}

			// We care about Write and Create events
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				// Reset debounce timer
				debounceTimer.Reset(debounceDelay)
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return fmt.Errorf("watcher errors channel closed")
			}
			w.logger.Error("Config watcher error", "error", err)

		case <-debounceTimer.C:
			// Reload config after debounce period
			if err := w.reload(); err != nil {
				w.logger.Error("Failed to reload config", "error", err)
			} else {
				w.logger.Info("Config reloaded successfully")
				if w.onChange != nil {
					w.onChange(w.Config())
				}
			}
		}
	}
}

// reload reloads the configuration from file
func (w *Watcher) reload() error {
	// Load new config
	newCfg, err := Load(w.path)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Update current config atomically
	w.mu.Lock()
	w.cfg = newCfg
	w.mu.Unlock()

	return nil
}

// Close stops the watcher
func (w *Watcher) Close() error {
	if w.watcher != nil {
		return w.watcher.Close()
	}
	return nil
}
