package dns

import (
	"testing"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/pattern"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWhitelistPatternIntegration tests whitelist pattern matching in the DNS handler
func TestWhitelistPatternIntegration(t *testing.T) {
	// Create handler
	handler := NewHandler()
	logger, err := logging.New(&config.LoggingConfig{
		Level:  "info",
		Format: "json",
	})
	require.NoError(t, err)
	handler.Logger = logger

	// Create a simple forwarder for testing
	fw := &forwarder.Forwarder{}
	handler.SetForwarder(fw)

	// Set up blocklist manager that blocks everything
	cfg := &config.Config{
		Blocklists: []string{}, // Empty for this test
	}
	manager := blocklist.NewManager(cfg, handler.Logger, nil)

	// Manually add some blocked domains
	blockedDomains := map[string]struct{}{
		"ads.example.com":     {},
		"tracker.example.com": {},
		"cdn1.example.com":    {},
		"cdn2.example.com":    {},
		"cdn3.example.com":    {},
	}
	manager.Get() // Initialize the pointer
	*manager.Get() = blockedDomains

	handler.SetBlocklistManager(manager)

	// Set up whitelist patterns
	whitelistPatterns := []string{
		"*.cdn.example.com",                    // Wildcard: should NOT match cdn1.example.com
		"(\\.|^)taskassist.*\\.google\\.com$", // Regex: Pi-hole style pattern
	}

	matcher, err := pattern.NewMatcher(whitelistPatterns)
	require.NoError(t, err)
	handler.WhitelistPatterns.Store(matcher)

	// Set up exact whitelist entries
	handler.Whitelist["tracker.example.com"] = struct{}{} // Exact match whitelist

	tests := []struct {
		name          string
		domain        string
		shouldBlock   bool
		blockReason   string
	}{
		{
			name:        "blocked domain - no whitelist match",
			domain:      "ads.example.com",
			shouldBlock: true,
			blockReason: "blocked and not whitelisted",
		},
		{
			name:        "blocked domain - whitelisted by exact match",
			domain:      "tracker.example.com",
			shouldBlock: false,
			blockReason: "whitelisted by exact match",
		},
		{
			name:        "blocked domain - NOT whitelisted by wildcard (cdn1 != *.cdn)",
			domain:      "cdn1.example.com",
			shouldBlock: true,
			blockReason: "*.cdn.example.com doesn't match cdn1.example.com",
		},
		{
			name:        "wildcard pattern match",
			domain:      "foo.cdn.example.com",
			shouldBlock: false,
			blockReason: "should match *.cdn.example.com pattern",
		},
		{
			name:        "regex pattern match - exact",
			domain:      "taskassist-pa.google.com",
			shouldBlock: false,
			blockReason: "should match regex pattern",
		},
		{
			name:        "regex pattern match - subdomain",
			domain:      "xyz.taskassist-pa.google.com",
			shouldBlock: false,
			blockReason: "should match regex pattern with subdomain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Manually check if domain would be blocked
			blocked := handler.BlocklistManager.IsBlocked(tt.domain)

			// Check whitelist (exact)
			_, whitelistedExact := handler.Whitelist[tt.domain]

			// Check whitelist (pattern)
			whitelistedPattern := false
			patterns := handler.WhitelistPatterns.Load()
			if patterns != nil {
				whitelistedPattern = patterns.Match(tt.domain)
			}

			whitelisted := whitelistedExact || whitelistedPattern

			if whitelisted {
				blocked = false
			}

			if tt.shouldBlock {
				assert.True(t, blocked,
					"%s: expected domain to be blocked (reason: %s)", tt.name, tt.blockReason)
			} else {
				assert.False(t, blocked,
					"%s: expected domain to NOT be blocked (reason: %s)", tt.name, tt.blockReason)
			}
		})
	}
}

// TestBlocklistManagerPatternIntegration tests blocklist manager pattern support
func TestBlocklistManagerPatternIntegration(t *testing.T) {
	cfg := &config.Config{}
	logger, err := logging.New(&config.LoggingConfig{
		Level:  "info",
		Format: "json",
	})
	require.NoError(t, err)

	manager := blocklist.NewManager(cfg, logger, nil)

	// Set up pattern-based blocklist
	patterns := []string{
		"*.ads.example.com",           // Wildcard
		"^ad[sz]\\..*\\.com$",         // Regex
		"(\\.|^)tracker\\.com$",       // Pi-hole style regex
	}

	err = manager.SetPatterns(patterns)
	require.NoError(t, err)

	tests := []struct {
		domain  string
		blocked bool
	}{
		// Wildcard matches
		{"foo.ads.example.com", true},
		{"bar.ads.example.com", true},
		{"ads.example.com", true}, // Also matches regex ^ad[sz]\\..*\\.com$

		// Regex prefix matches
		{"ads.tracker.com", true},
		{"adz.network.com", true},
		{"advert.com", false},

		// Pi-hole style regex
		{"tracker.com", true},
		{"sub.tracker.com", true},
		{"nottracker.com", false},

		// Wildcard base domain test (use different pattern)
		{"cdn.example.org", false}, // Doesn't match any pattern
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			blocked := manager.IsBlocked(tt.domain)
			assert.Equal(t, tt.blocked, blocked,
				"Domain %q: expected blocked=%v, got blocked=%v",
				tt.domain, tt.blocked, blocked)
		})
	}

	// Check stats
	stats := manager.Stats()
	assert.Equal(t, 0, stats["exact"], "Should have 0 exact matches")
	assert.Equal(t, 1, stats["pattern_wildcard"], "Should have 1 wildcard pattern")
	assert.Equal(t, 2, stats["pattern_regex"], "Should have 2 regex patterns")
	assert.Equal(t, 3, stats["total"], "Should have 3 total patterns")
}
