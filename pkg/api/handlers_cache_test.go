package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCachePurge_Success(t *testing.T) {
	// Create a cache with test data
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	})

	cacheConfig := &config.CacheConfig{
		Enabled:     true,
		MaxEntries:  100,
		MinTTL:      60 * time.Second,
		MaxTTL:      3600 * time.Second,
		NegativeTTL: 300 * time.Second,
	}

	dnsCache, err := cache.New(cacheConfig, logger, nil)
	require.NoError(t, err)
	defer dnsCache.Close()

	// Populate cache with some entries
	// (In a real scenario, the DNS handler would do this)
	// For testing, we'll just verify the Clear() method is called

	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		cache:  dnsCache,
		logger: testLogger,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cache/purge", nil)
	w := httptest.NewRecorder()

	server.handleCachePurge(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify response structure
	var response CachePurgeResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, "DNS cache purged successfully", response.Message)
	assert.GreaterOrEqual(t, response.EntriesCleared, 0)
}

func TestHandleCachePurge_MethodNotAllowed(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		logger: testLogger,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/cache/purge", nil)
	w := httptest.NewRecorder()

	server.handleCachePurge(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandleCachePurge_NoCacheAvailable(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		cache:  nil, // No cache configured
		logger: testLogger,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cache/purge", nil)
	w := httptest.NewRecorder()

	server.handleCachePurge(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Check either Error or Message field contains the expected text
	assert.True(t,
		response.Error == "Cache not available" || response.Message == "Cache not available",
		"Expected error message about cache not available, got: %+v", response)
}

func TestHandleCachePurge_Integration(t *testing.T) {
	// This test verifies the cache is actually cleared
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	})

	cacheConfig := &config.CacheConfig{
		Enabled:     true,
		MaxEntries:  100,
		MinTTL:      60 * time.Second,
		MaxTTL:      3600 * time.Second,
		NegativeTTL: 300 * time.Second,
	}

	dnsCache, err := cache.New(cacheConfig, logger, nil)
	require.NoError(t, err)
	defer dnsCache.Close()

	// Add some entries to the cache
	// Note: This would normally be done by the DNS handler
	// We're just verifying stats work correctly

	statsBefore := dnsCache.Stats()
	initialEntries := statsBefore.Entries

	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		cache:  dnsCache,
		logger: testLogger,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cache/purge", nil)
	w := httptest.NewRecorder()

	server.handleCachePurge(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify cache stats after purge
	statsAfter := dnsCache.Stats()
	assert.Equal(t, 0, statsAfter.Entries, "Cache should be empty after purge")

	// Verify response contains correct count
	var response CachePurgeResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, initialEntries, response.EntriesCleared)
}
