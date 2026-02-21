package api

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestAcmeHostsChanged(t *testing.T) {
	tests := []struct {
		name string
		old  []string
		new  []string
		want bool
	}{
		{"identical", []string{"a.com"}, []string{"a.com"}, false},
		{"case insensitive match", []string{"A.COM"}, []string{"a.com"}, false},
		{"reordered same", []string{"b.com", "a.com"}, []string{"a.com", "b.com"}, false},
		{"different host", []string{"old.com"}, []string{"new.com"}, true},
		{"added host", []string{"a.com"}, []string{"a.com", "b.com"}, true},
		{"removed host", []string{"a.com", "b.com"}, []string{"a.com"}, true},
		{"both empty", []string{}, []string{}, false},
		{"old empty", []string{}, []string{"a.com"}, true},
		{"new empty", []string{"a.com"}, []string{}, true},
		{"both nil", nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acmeHostsChanged(tt.old, tt.new); got != tt.want {
				t.Errorf("acmeHostsChanged(%v, %v) = %v, want %v", tt.old, tt.new, got, tt.want)
			}
		})
	}
}

func TestPurgeACMECache(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create fake cached files.
	for _, name := range []string{"cert.pem", "key.pem"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	purgeACMECache(dir, logger)

	for _, name := range []string{"cert.pem", "key.pem"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, got err: %v", name, err)
		}
	}
}

func TestPurgeACMECacheNoDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	// Should not panic on empty or nonexistent dir.
	purgeACMECache("", logger)
	purgeACMECache("/nonexistent/path/xyz", logger)
}
