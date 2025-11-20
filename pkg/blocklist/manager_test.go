package blocklist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
)

func TestNewManager(t *testing.T) {
	cfg := &config.Config{
		Blocklists: []string{},
	}
	logger := logging.NewDefault()

	m := NewManager(cfg, logger)

	if m == nil {
		t.Fatal("Expected manager, got nil")
	}

	if m.cfg == nil {
		t.Error("Expected config to be set")
	}

	if m.logger == nil {
		t.Error("Expected logger to be set")
	}

	if m.downloader == nil {
		t.Error("Expected downloader to be set")
	}

	// Should start with empty blocklist
	if m.Size() != 0 {
		t.Errorf("Expected empty blocklist, got %d domains", m.Size())
	}
}

func TestManager_Update(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts := `0.0.0.0 ads.example.com
0.0.0.0 tracker.example.com
0.0.0.0 malware.example.com
`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(hosts))
	}))
	defer server.Close()

	cfg := &config.Config{
		Blocklists: []string{server.URL},
	}
	logger := logging.NewDefault()
	m := NewManager(cfg, logger)

	ctx := context.Background()
	err := m.Update(ctx)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check size
	if m.Size() != 3 {
		t.Errorf("Expected 3 domains, got %d", m.Size())
	}

	// Check specific domains
	if !m.IsBlocked("ads.example.com.") {
		t.Error("Expected ads.example.com. to be blocked")
	}

	if !m.IsBlocked("tracker.example.com.") {
		t.Error("Expected tracker.example.com. to be blocked")
	}

	if m.IsBlocked("allowed.example.com.") {
		t.Error("Expected allowed.example.com. not to be blocked")
	}
}

func TestManager_Update_NoBlocklists(t *testing.T) {
	cfg := &config.Config{
		Blocklists: []string{},
	}
	logger := logging.NewDefault()
	m := NewManager(cfg, logger)

	ctx := context.Background()
	err := m.Update(ctx)

	// Should not error with empty blocklist
	if err != nil {
		t.Fatalf("Expected no error for empty blocklist, got %v", err)
	}

	if m.Size() != 0 {
		t.Errorf("Expected 0 domains, got %d", m.Size())
	}
}

func TestManager_Update_AtomicReplacement(t *testing.T) {
	// Create test HTTP server
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First update: 2 domains
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("0.0.0.0 ads1.example.com\n0.0.0.0 ads2.example.com\n"))
		} else {
			// Second update: 3 different domains
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("0.0.0.0 ads3.example.com\n0.0.0.0 ads4.example.com\n0.0.0.0 ads5.example.com\n"))
		}
	}))
	defer server.Close()

	cfg := &config.Config{
		Blocklists: []string{server.URL},
	}
	logger := logging.NewDefault()
	m := NewManager(cfg, logger)

	ctx := context.Background()

	// First update
	err := m.Update(ctx)
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}

	if m.Size() != 2 {
		t.Errorf("Expected 2 domains after first update, got %d", m.Size())
	}

	if !m.IsBlocked("ads1.example.com.") {
		t.Error("Expected ads1.example.com. to be blocked after first update")
	}

	// Second update (should atomically replace)
	err = m.Update(ctx)
	if err != nil {
		t.Fatalf("Second update failed: %v", err)
	}

	if m.Size() != 3 {
		t.Errorf("Expected 3 domains after second update, got %d", m.Size())
	}

	// Old domains should no longer be blocked
	if m.IsBlocked("ads1.example.com.") {
		t.Error("Expected ads1.example.com. not to be blocked after second update")
	}

	// New domains should be blocked
	if !m.IsBlocked("ads3.example.com.") {
		t.Error("Expected ads3.example.com. to be blocked after second update")
	}
}

func TestManager_Get(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("0.0.0.0 ads.example.com\n"))
	}))
	defer server.Close()

	cfg := &config.Config{
		Blocklists: []string{server.URL},
	}
	logger := logging.NewDefault()
	m := NewManager(cfg, logger)

	ctx := context.Background()
	m.Update(ctx)

	// Get blocklist pointer
	blocklist := m.Get()

	if blocklist == nil {
		t.Fatal("Expected blocklist pointer, got nil")
	}

	if len(*blocklist) != 1 {
		t.Errorf("Expected 1 domain in blocklist, got %d", len(*blocklist))
	}

	if _, ok := (*blocklist)["ads.example.com."]; !ok {
		t.Error("Expected ads.example.com. to be in blocklist")
	}
}

func TestManager_IsBlocked(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts := `0.0.0.0 ads.example.com
0.0.0.0 tracker.example.com
`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(hosts))
	}))
	defer server.Close()

	cfg := &config.Config{
		Blocklists: []string{server.URL},
	}
	logger := logging.NewDefault()
	m := NewManager(cfg, logger)

	ctx := context.Background()
	m.Update(ctx)

	// Test blocked domains
	if !m.IsBlocked("ads.example.com.") {
		t.Error("Expected ads.example.com. to be blocked")
	}

	if !m.IsBlocked("tracker.example.com.") {
		t.Error("Expected tracker.example.com. to be blocked")
	}

	// Test allowed domain
	if m.IsBlocked("allowed.example.com.") {
		t.Error("Expected allowed.example.com. not to be blocked")
	}

	// Test empty domain
	if m.IsBlocked("") {
		t.Error("Expected empty domain not to be blocked")
	}
}

func TestManager_Size(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts := `0.0.0.0 ads1.example.com
0.0.0.0 ads2.example.com
0.0.0.0 ads3.example.com
0.0.0.0 ads4.example.com
0.0.0.0 ads5.example.com
`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(hosts))
	}))
	defer server.Close()

	cfg := &config.Config{
		Blocklists: []string{server.URL},
	}
	logger := logging.NewDefault()
	m := NewManager(cfg, logger)

	// Before update
	if m.Size() != 0 {
		t.Errorf("Expected size 0 before update, got %d", m.Size())
	}

	// After update
	ctx := context.Background()
	m.Update(ctx)

	if m.Size() != 5 {
		t.Errorf("Expected size 5 after update, got %d", m.Size())
	}
}

func TestManager_StartStop(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("0.0.0.0 ads.example.com\n"))
	}))
	defer server.Close()

	cfg := &config.Config{
		Blocklists:           []string{server.URL},
		AutoUpdateBlocklists: false, // Disable auto-update for this test
	}
	logger := logging.NewDefault()
	m := NewManager(cfg, logger)

	ctx := context.Background()

	// Start manager
	err := m.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Should have downloaded blocklist
	if m.Size() == 0 {
		t.Error("Expected blocklist to be downloaded on start")
	}

	// Stop manager
	m.Stop()

	// Should be able to start again
	err = m.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to restart manager: %v", err)
	}

	m.Stop()
}

func TestManager_AutoUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping auto-update test in short mode")
	}

	// Create test HTTP server
	updateCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		updateCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("0.0.0.0 ads.example.com\n"))
	}))
	defer server.Close()

	cfg := &config.Config{
		Blocklists:           []string{server.URL},
		AutoUpdateBlocklists: true,
		UpdateInterval:       500 * time.Millisecond, // Short interval for testing
	}
	logger := logging.NewDefault()
	m := NewManager(cfg, logger)

	ctx := context.Background()

	// Start manager (triggers initial download)
	err := m.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer m.Stop()

	initialCount := updateCount

	// Wait for at least one auto-update
	time.Sleep(1 * time.Second)

	// Should have performed additional update(s)
	if updateCount <= initialCount {
		t.Errorf("Expected at least %d updates, got %d", initialCount+1, updateCount)
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts := `0.0.0.0 ads.example.com
0.0.0.0 tracker.example.com
`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(hosts))
	}))
	defer server.Close()

	cfg := &config.Config{
		Blocklists: []string{server.URL},
	}
	logger := logging.NewDefault()
	m := NewManager(cfg, logger)

	ctx := context.Background()
	m.Update(ctx)

	// Spawn multiple goroutines reading from blocklist
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = m.IsBlocked("ads.example.com.")
				_ = m.Size()
				_ = m.Get()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// No data races should occur
}
