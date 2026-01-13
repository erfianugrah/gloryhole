package dns

import (
	"strings"
	"testing"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ensureTrailingDot(domain string) string {
	if domain == "" || strings.HasSuffix(domain, ".") {
		return domain
	}
	return domain + "."
}

// TestBlocklistManagerPatternIntegration tests blocklist manager pattern support
func TestBlocklistManagerPatternIntegration(t *testing.T) {
	cfg := &config.Config{}
	logger, err := logging.New(&config.LoggingConfig{
		Level:  "info",
		Format: "json",
	})
	require.NoError(t, err)

	manager := blocklist.NewManager(cfg, logger, nil, nil)

	// Set up pattern-based blocklist
	patterns := []string{
		"*.ads.example.com",     // Wildcard
		"^ad[sz]\\..*\\.com$",   // Regex
		"(\\.|^)tracker\\.com$", // Pi-hole style regex
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
