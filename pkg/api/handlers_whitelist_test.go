package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"glory-hole/pkg/config"
	"glory-hole/pkg/dns"
	"glory-hole/pkg/pattern"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestServerForWhitelist(t *testing.T, initialWhitelist []string) (*Server, *dns.Handler) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	dnsHandler := &dns.Handler{
		Whitelist:         atomic.Pointer[map[string]struct{}]{},
		WhitelistPatterns: atomic.Pointer[pattern.Matcher]{},
	}

	// Initialize whitelist if provided
	if len(initialWhitelist) > 0 {
		exactMatches := make(map[string]struct{})
		patterns := make([]string, 0)

		for _, entry := range initialWhitelist {
			if isPattern(entry) {
				patterns = append(patterns, entry)
			} else {
				// Add FQDN format
				if !strings.HasSuffix(entry, ".") {
					entry = entry + "."
				}
				exactMatches[entry] = struct{}{}
			}
		}

		if len(exactMatches) > 0 {
			dnsHandler.Whitelist.Store(&exactMatches)
		}

		if len(patterns) > 0 {
			matcher, err := pattern.NewMatcher(patterns)
			require.NoError(t, err)
			dnsHandler.WhitelistPatterns.Store(matcher)
		}
	}

	testConfig := &config.Config{
		Whitelist: initialWhitelist,
	}

	server := &Server{
		logger:         testLogger,
		dnsHandler:     dnsHandler,
		configSnapshot: testConfig,
	}

	return server, dnsHandler
}

func TestHandleGetWhitelist_Empty(t *testing.T) {
	server, _ := createTestServerForWhitelist(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/whitelist", nil)
	w := httptest.NewRecorder()

	server.handleGetWhitelist(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response WhitelistResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Total)
	assert.Empty(t, response.Entries)
}

func TestHandleGetWhitelist_WithEntries(t *testing.T) {
	initialWhitelist := []string{
		"example.com",
		"test.org",
		"*.cdn.example.com",
		"^.*\\.googleapis\\.com$",
	}

	server, _ := createTestServerForWhitelist(t, initialWhitelist)

	req := httptest.NewRequest(http.MethodGet, "/api/whitelist", nil)
	w := httptest.NewRecorder()

	server.handleGetWhitelist(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response WhitelistResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 4, response.Total)
	assert.Len(t, response.Entries, 4)

	// Verify exact and pattern detection
	exactCount := 0
	patternCount := 0
	for _, entry := range response.Entries {
		if entry.Pattern {
			patternCount++
		} else {
			exactCount++
		}
	}

	assert.Equal(t, 2, exactCount, "Should have 2 exact matches")
	assert.Equal(t, 2, patternCount, "Should have 2 patterns")
}

func TestHandleGetWhitelist_HTMLResponse(t *testing.T) {
	t.Skip("Skipping HTML response test - requires template initialization")

	server, _ := createTestServerForWhitelist(t, []string{"example.com"})

	req := httptest.NewRequest(http.MethodGet, "/api/whitelist", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	server.handleGetWhitelist(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Should return HTML when HX-Request header is present
	// We can't fully test template rendering without initializing templates,
	// but we can verify the handler tries to render HTML
}

func TestHandleAddWhitelist_SingleExactDomain(t *testing.T) {
	server, dnsHandler := createTestServerForWhitelist(t, nil)

	requestBody := WhitelistAddRequest{
		Domains: []string{"example.com"},
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/whitelist", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddWhitelist(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify domain was added to runtime
	wl := dnsHandler.Whitelist.Load()
	require.NotNil(t, wl)

	_, exists := (*wl)["example.com."]
	assert.True(t, exists, "Domain should be in whitelist with trailing dot")
}

func TestHandleAddWhitelist_WildcardPattern(t *testing.T) {
	server, _ := createTestServerForWhitelist(t, nil)

	requestBody := WhitelistAddRequest{
		Domains: []string{"*.cdn.example.com"},
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/whitelist", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddWhitelist(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleAddWhitelist_RegexPattern(t *testing.T) {
	server, _ := createTestServerForWhitelist(t, nil)

	requestBody := WhitelistAddRequest{
		Domains: []string{"^.*\\.googleapis\\.com$"},
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/whitelist", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddWhitelist(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleAddWhitelist_InvalidPattern(t *testing.T) {
	server, _ := createTestServerForWhitelist(t, nil)

	// Invalid regex pattern
	requestBody := WhitelistAddRequest{
		Domains: []string{"^[invalid(regex$"},
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/whitelist", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddWhitelist(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, "Invalid pattern")
}

func TestHandleAddWhitelist_EmptyDomains(t *testing.T) {
	server, _ := createTestServerForWhitelist(t, nil)

	requestBody := WhitelistAddRequest{
		Domains: []string{},
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/whitelist", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddWhitelist(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAddWhitelist_MixedExactAndPatterns(t *testing.T) {
	server, dnsHandler := createTestServerForWhitelist(t, nil)

	requestBody := WhitelistAddRequest{
		Domains: []string{
			"example.com",
			"test.org",
			"*.cdn.example.com",
			"^.*\\.googleapis\\.com$",
		},
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/whitelist", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddWhitelist(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify exact matches were added
	wl := dnsHandler.Whitelist.Load()
	require.NotNil(t, wl)
	_, exists := (*wl)["example.com."]
	assert.True(t, exists)
	_, exists = (*wl)["test.org."]
	assert.True(t, exists)
}

func TestHandleAddWhitelist_NoDNSHandler(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		logger:     testLogger,
		dnsHandler: nil, // No DNS handler
	}

	requestBody := WhitelistAddRequest{
		Domains: []string{"example.com"},
	}
	bodyJSON, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/api/whitelist", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleAddWhitelist(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleRemoveWhitelist_ExactDomain(t *testing.T) {
	server, dnsHandler := createTestServerForWhitelist(t, []string{"example.com", "test.org"})

	// Create a request with path value
	req := httptest.NewRequest(http.MethodDelete, "/api/whitelist/example.com", nil)
	req.SetPathValue("domain", "example.com")
	w := httptest.NewRecorder()

	server.handleRemoveWhitelist(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify domain was removed
	wl := dnsHandler.Whitelist.Load()
	require.NotNil(t, wl)
	_, exists := (*wl)["example.com."]
	assert.False(t, exists, "Domain should be removed from whitelist")

	// Verify other domain still exists
	_, exists = (*wl)["test.org."]
	assert.True(t, exists, "Other domain should still be in whitelist")
}

func TestHandleRemoveWhitelist_Pattern(t *testing.T) {
	server, _ := createTestServerForWhitelist(t, []string{"*.cdn.example.com"})

	req := httptest.NewRequest(http.MethodDelete, "/api/whitelist/*.cdn.example.com", nil)
	req.SetPathValue("domain", "*.cdn.example.com")
	w := httptest.NewRecorder()

	server.handleRemoveWhitelist(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleRemoveWhitelist_NotFound(t *testing.T) {
	server, _ := createTestServerForWhitelist(t, []string{"example.com"})

	req := httptest.NewRequest(http.MethodDelete, "/api/whitelist/nonexistent.com", nil)
	req.SetPathValue("domain", "nonexistent.com")
	w := httptest.NewRecorder()

	server.handleRemoveWhitelist(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleRemoveWhitelist_MissingDomain(t *testing.T) {
	server, _ := createTestServerForWhitelist(t, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/whitelist/", nil)
	req.SetPathValue("domain", "")
	w := httptest.NewRecorder()

	server.handleRemoveWhitelist(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleBulkImportWhitelist_ValidInput(t *testing.T) {
	server, dnsHandler := createTestServerForWhitelist(t, nil)

	bulkInput := `example.com
test.org
# This is a comment
*.cdn.example.com

^.*\.googleapis\.com$
// Another comment style
another-domain.net`

	req := httptest.NewRequest(http.MethodPost, "/api/whitelist/bulk", strings.NewReader(bulkInput))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	server.handleBulkImportWhitelist(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify domains were added (excluding comments)
	wl := dnsHandler.Whitelist.Load()
	require.NotNil(t, wl)

	// Should have exact matches
	_, exists := (*wl)["example.com."]
	assert.True(t, exists)
	_, exists = (*wl)["test.org."]
	assert.True(t, exists)
	_, exists = (*wl)["another-domain.net."]
	assert.True(t, exists)
}

func TestHandleBulkImportWhitelist_EmptyInput(t *testing.T) {
	server, _ := createTestServerForWhitelist(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/whitelist/bulk", strings.NewReader(""))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	server.handleBulkImportWhitelist(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleBulkImportWhitelist_OnlyComments(t *testing.T) {
	server, _ := createTestServerForWhitelist(t, nil)

	bulkInput := `# Comment 1
// Comment 2
# Comment 3`

	req := httptest.NewRequest(http.MethodPost, "/api/whitelist/bulk", strings.NewReader(bulkInput))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	server.handleBulkImportWhitelist(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestIsPattern_ExactDomains(t *testing.T) {
	assert.False(t, isPattern("example.com"))
	assert.False(t, isPattern("test.org"))
	assert.False(t, isPattern("sub.domain.example.com"))
}

func TestIsPattern_Wildcards(t *testing.T) {
	assert.True(t, isPattern("*.example.com"))
	assert.True(t, isPattern("*.cdn.*.example.com"))
}

func TestIsPattern_Regex(t *testing.T) {
	assert.True(t, isPattern("^.*\\.example\\.com$"))
	assert.True(t, isPattern("(foo|bar)\\.example\\.com"))
	assert.True(t, isPattern("[a-z]+\\.example\\.com"))
	assert.True(t, isPattern("test\\.example\\.com$"))
}

func TestPersistWhitelistConfig_NoConfigPath(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		logger:     testLogger,
		configPath: "", // No config path
	}

	err := server.persistWhitelistConfig(func(cfg *config.Config) error {
		return nil
	})

	// Should not error when no config path is set (ephemeral mode)
	assert.NoError(t, err)
}

func TestPersistWhitelistConfig_NoConfig(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := &Server{
		logger:         testLogger,
		configPath:     "/tmp/test.yaml",
		configSnapshot: nil,
		configWatcher:  nil,
	}

	err := server.persistWhitelistConfig(func(cfg *config.Config) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no config available")
}
