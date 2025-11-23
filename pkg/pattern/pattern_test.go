package pattern

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePattern(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		wantType    PatternType
		shouldError bool
	}{
		{
			name:     "exact match",
			pattern:  "example.com",
			wantType: PatternTypeExact,
		},
		{
			name:     "wildcard match",
			pattern:  "*.example.com",
			wantType: PatternTypeWildcard,
		},
		{
			name:     "regex with parentheses",
			pattern:  "(\\.|^)example\\.com$",
			wantType: PatternTypeRegex,
		},
		{
			name:     "regex with brackets",
			pattern:  "^ad[sz]\\..*\\.com$",
			wantType: PatternTypeRegex,
		},
		{
			name:     "regex with pipe",
			pattern:  "^(ads|adz)\\.example\\.com$",
			wantType: PatternTypeRegex,
		},
		{
			name:     "regex with caret",
			pattern:  "^ads\\..*",
			wantType: PatternTypeRegex,
		},
		{
			name:     "regex with dollar",
			pattern:  ".*\\.tracking$",
			wantType: PatternTypeRegex,
		},
		{
			name:        "empty pattern",
			pattern:     "",
			shouldError: true,
		},
		{
			name:        "invalid regex",
			pattern:     "^[unclosed",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, err := ParsePattern(tt.pattern)

			if tt.shouldError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantType, pattern.Type)
			assert.Equal(t, tt.pattern, pattern.Raw)

			// Verify regex is compiled for regex patterns
			if tt.wantType == PatternTypeRegex {
				assert.NotNil(t, pattern.Compiled)
			}
		})
	}
}

func TestPatternMatch(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		domain  string
		match   bool
	}{
		// Exact matches
		{
			name:    "exact match - positive",
			pattern: "example.com",
			domain:  "example.com",
			match:   true,
		},
		{
			name:    "exact match - negative",
			pattern: "example.com",
			domain:  "sub.example.com",
			match:   false,
		},
		{
			name:    "exact match - different domain",
			pattern: "example.com",
			domain:  "other.com",
			match:   false,
		},

		// Wildcard matches
		{
			name:    "wildcard - subdomain match",
			pattern: "*.example.com",
			domain:  "sub.example.com",
			match:   true,
		},
		{
			name:    "wildcard - nested subdomain match",
			pattern: "*.example.com",
			domain:  "deep.sub.example.com",
			match:   true,
		},
		{
			name:    "wildcard - exact domain no match",
			pattern: "*.example.com",
			domain:  "example.com",
			match:   false,
		},
		{
			name:    "wildcard - different domain no match",
			pattern: "*.example.com",
			domain:  "other.com",
			match:   false,
		},

		// Regex matches - Pi-hole style
		{
			name:    "regex pihole - exact domain",
			pattern: "(\\.|^)example\\.com$",
			domain:  "example.com",
			match:   true,
		},
		{
			name:    "regex pihole - subdomain",
			pattern: "(\\.|^)example\\.com$",
			domain:  "sub.example.com",
			match:   true,
		},
		{
			name:    "regex pihole - nested subdomain",
			pattern: "(\\.|^)example\\.com$",
			domain:  "deep.sub.example.com",
			match:   true,
		},
		{
			name:    "regex pihole - no match",
			pattern: "(\\.|^)example\\.com$",
			domain:  "notexample.com",
			match:   false,
		},

		// Regex matches - advanced patterns
		{
			name:    "regex prefix ads",
			pattern: "^ad[sz]\\..*\\.com$",
			domain:  "ads.tracker.com",
			match:   true,
		},
		{
			name:    "regex prefix adz",
			pattern: "^ad[sz]\\..*\\.com$",
			domain:  "adz.network.com",
			match:   true,
		},
		{
			name:    "regex prefix no match",
			pattern: "^ad[sz]\\..*\\.com$",
			domain:  "advert.com",
			match:   false,
		},
		{
			name:    "regex prefix wrong tld",
			pattern: "^ad[sz]\\..*\\.com$",
			domain:  "ads.tracker.net",
			match:   false,
		},

		// Regex matches - contains pattern
		{
			name:    "regex contains tracker",
			pattern: ".*tracker.*",
			domain:  "ads.tracker.com",
			match:   true,
		},
		{
			name:    "regex contains tracker middle",
			pattern: ".*tracker.*",
			domain:  "mytracker.example.com",
			match:   true,
		},
		{
			name:    "regex contains no match",
			pattern: ".*tracker.*",
			domain:  "example.com",
			match:   false,
		},

		// Google Assistant patterns (from user's config)
		{
			name:    "google taskassist exact",
			pattern: "(\\.|^)taskassist-pa\\.clients6\\.google\\.com$",
			domain:  "taskassist-pa.clients6.google.com",
			match:   true,
		},
		{
			name:    "google taskassist subdomain",
			pattern: "(\\.|^)taskassist-pa\\.clients6\\.google\\.com$",
			domain:  "xyz.taskassist-pa.clients6.google.com",
			match:   true,
		},

		// Cloudflare Gateway patterns (from user's config)
		{
			name:    "cloudflare proxy exact",
			pattern: "(\\.|^)proxy\\.cloudflare-gateway\\.com$",
			domain:  "proxy.cloudflare-gateway.com",
			match:   true,
		},
		{
			name:    "cloudflare proxy subdomain",
			pattern: "(\\.|^)proxy\\.cloudflare-gateway\\.com$",
			domain:  "xyz.proxy.cloudflare-gateway.com",
			match:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, err := ParsePattern(tt.pattern)
			require.NoError(t, err)

			got := pattern.Match(tt.domain)
			assert.Equal(t, tt.match, got,
				"Pattern %q vs domain %q: expected %v, got %v",
				tt.pattern, tt.domain, tt.match, got)
		})
	}
}

func TestMatcher(t *testing.T) {
	patterns := []string{
		// Exact
		"example.com",
		"test.com",

		// Wildcard
		"*.cdn.example.com",
		"*.ads.example.com",

		// Regex
		"(\\.|^)taskassist-pa\\.clients6\\.google\\.com$",
		"^ad[sz]\\..*\\.com$",
	}

	matcher, err := NewMatcher(patterns)
	require.NoError(t, err)

	tests := []struct {
		domain string
		match  bool
	}{
		// Exact matches
		{"example.com", true},
		{"test.com", true},
		{"other.com", false},

		// Wildcard matches
		{"foo.cdn.example.com", true},
		{"bar.ads.example.com", true},
		{"cdn.example.com", false}, // Wildcard doesn't match base

		// Regex matches
		{"taskassist-pa.clients6.google.com", true},
		{"xyz.taskassist-pa.clients6.google.com", true},
		{"ads.tracker.com", true},
		{"adz.network.com", true},

		// No matches
		{"nomatch.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := matcher.Match(tt.domain)
			assert.Equal(t, tt.match, got,
				"Domain %q: expected %v, got %v", tt.domain, tt.match, got)
		})
	}
}

func TestMatcherStats(t *testing.T) {
	patterns := []string{
		"example.com",
		"test.com",
		"*.cdn.example.com",
		"^ad[sz]\\..*\\.com$",
	}

	matcher, err := NewMatcher(patterns)
	require.NoError(t, err)

	stats := matcher.Stats()
	assert.Equal(t, 2, stats["exact"])
	assert.Equal(t, 1, stats["wildcard"])
	assert.Equal(t, 1, stats["regex"])
	assert.Equal(t, 4, stats["total"])
}

func TestMatcherInvalidPattern(t *testing.T) {
	patterns := []string{
		"example.com",
		"^[unclosed", // Invalid regex
	}

	_, err := NewMatcher(patterns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regex")
}

func TestPatternTypeString(t *testing.T) {
	tests := []struct {
		want string
		pt   PatternType
	}{
		{"exact", PatternTypeExact},
		{"wildcard", PatternTypeWildcard},
		{"regex", PatternTypeRegex},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.pt.String())
		})
	}
}

func TestPatternString(t *testing.T) {
	pattern := &Pattern{
		Raw:  "example.com",
		Type: PatternTypeExact,
	}

	assert.Equal(t, "exact(example.com)", pattern.String())
}

// Benchmark tests
func BenchmarkPatternMatch(b *testing.B) {
	benchmarks := []struct {
		name    string
		pattern string
		domain  string
	}{
		{
			name:    "exact",
			pattern: "example.com",
			domain:  "example.com",
		},
		{
			name:    "wildcard",
			pattern: "*.example.com",
			domain:  "sub.example.com",
		},
		{
			name:    "regex-pihole",
			pattern: "(\\.|^)example\\.com$",
			domain:  "sub.example.com",
		},
		{
			name:    "regex-complex",
			pattern: "^ad[sz]\\..*\\.com$",
			domain:  "ads.tracker.com",
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			pattern, err := ParsePattern(bm.pattern)
			require.NoError(b, err)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pattern.Match(bm.domain)
			}
		})
	}
}

func BenchmarkMatcherMatch(b *testing.B) {
	patterns := []string{
		"example.com",
		"test.com",
		"blocked.com",
		"*.cdn.example.com",
		"*.ads.example.com",
		"(\\.|^)taskassist-pa\\.clients6\\.google\\.com$",
		"^ad[sz]\\..*\\.com$",
		".*tracker.*",
	}

	matcher, err := NewMatcher(patterns)
	require.NoError(b, err)

	testDomains := []string{
		"example.com",           // Exact hit
		"foo.cdn.example.com",   // Wildcard hit
		"ads.tracker.com",       // Regex hit
		"nomatch.com",           // Miss (tries all tiers)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, domain := range testDomains {
			matcher.Match(domain)
		}
	}
}
