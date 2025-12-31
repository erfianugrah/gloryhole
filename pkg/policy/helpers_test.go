package policy

import (
	"testing"
)

func TestDomainRegex(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		pattern  string
		expected bool
		wantErr  bool
	}{
		{
			name:     "match subdomain pattern",
			domain:   "www.example.com",
			pattern:  `^www\..+\.com$`,
			expected: true,
		},
		{
			name:     "match any google domain",
			domain:   "mail.google.com",
			pattern:  `.*\.google\.com$`,
			expected: true,
		},
		{
			name:     "no match",
			domain:   "facebook.com",
			pattern:  `^.*\.google\.com$`,
			expected: false,
		},
		{
			name:     "match numeric subdomains",
			domain:   "cdn123.example.com",
			pattern:  `^cdn\d+\.`,
			expected: true,
		},
		{
			name:     "invalid regex",
			domain:   "test.com",
			pattern:  `[invalid(`,
			expected: false,
			wantErr:  true,
		},
		{
			name:     "case insensitive",
			domain:   "WWW.EXAMPLE.COM",
			pattern:  `^www\.example\.com$`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DomainRegex(tt.domain, tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("DomainRegex() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("DomainRegex(%q, %q) = %v, want %v", tt.domain, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestDomainLevelCount(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		expected int
	}{
		{
			name:     "simple domain",
			domain:   "example.com",
			expected: 2,
		},
		{
			name:     "subdomain",
			domain:   "www.example.com",
			expected: 3,
		},
		{
			name:     "deep subdomain",
			domain:   "cdn.assets.www.example.com",
			expected: 5,
		},
		{
			name:     "single level",
			domain:   "localhost",
			expected: 1,
		},
		{
			name:     "trailing dot",
			domain:   "example.com.",
			expected: 2,
		},
		{
			name:     "empty domain",
			domain:   "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DomainLevelCount(tt.domain)
			if result != tt.expected {
				t.Errorf("DomainLevelCount(%q) = %d, want %d", tt.domain, result, tt.expected)
			}
		})
	}
}

func TestIPEquals(t *testing.T) {
	tests := []struct {
		name     string
		ip1      string
		ip2      string
		expected bool
	}{
		{
			name:     "same IPv4",
			ip1:      "192.168.1.1",
			ip2:      "192.168.1.1",
			expected: true,
		},
		{
			name:     "different IPv4",
			ip1:      "192.168.1.1",
			ip2:      "192.168.1.2",
			expected: false,
		},
		{
			name:     "same IPv6",
			ip1:      "2001:db8::1",
			ip2:      "2001:db8::1",
			expected: true,
		},
		{
			name:     "different IPv6",
			ip1:      "2001:db8::1",
			ip2:      "2001:db8::2",
			expected: false,
		},
		{
			name:     "IPv6 full vs compressed",
			ip1:      "2001:0db8:0000:0000:0000:0000:0000:0001",
			ip2:      "2001:db8::1",
			expected: true,
		},
		{
			name:     "invalid IP1",
			ip1:      "invalid",
			ip2:      "192.168.1.1",
			expected: false,
		},
		{
			name:     "invalid IP2",
			ip1:      "192.168.1.1",
			ip2:      "invalid",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IPEquals(tt.ip1, tt.ip2)
			if result != tt.expected {
				t.Errorf("IPEquals(%q, %q) = %v, want %v", tt.ip1, tt.ip2, result, tt.expected)
			}
		})
	}
}

func TestQueryTypeIn(t *testing.T) {
	tests := []struct {
		name      string
		queryType string
		types     []string
		expected  bool
	}{
		{
			name:      "A in list",
			queryType: "A",
			types:     []string{"A", "AAAA", "CNAME"},
			expected:  true,
		},
		{
			name:      "not in list",
			queryType: "MX",
			types:     []string{"A", "AAAA", "CNAME"},
			expected:  false,
		},
		{
			name:      "case insensitive match",
			queryType: "aaaa",
			types:     []string{"A", "AAAA", "CNAME"},
			expected:  true,
		},
		{
			name:      "single type match",
			queryType: "A",
			types:     []string{"A"},
			expected:  true,
		},
		{
			name:      "single type no match",
			queryType: "AAAA",
			types:     []string{"A"},
			expected:  false,
		},
		{
			name:      "empty list",
			queryType: "A",
			types:     []string{},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := QueryTypeIn(tt.queryType, tt.types...)
			if result != tt.expected {
				t.Errorf("QueryTypeIn(%q, %v) = %v, want %v", tt.queryType, tt.types, result, tt.expected)
			}
		})
	}
}

func TestIsWeekend(t *testing.T) {
	tests := []struct {
		name     string
		weekday  int
		expected bool
	}{
		{name: "Sunday", weekday: 0, expected: true},
		{name: "Monday", weekday: 1, expected: false},
		{name: "Tuesday", weekday: 2, expected: false},
		{name: "Wednesday", weekday: 3, expected: false},
		{name: "Thursday", weekday: 4, expected: false},
		{name: "Friday", weekday: 5, expected: false},
		{name: "Saturday", weekday: 6, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWeekend(tt.weekday)
			if result != tt.expected {
				t.Errorf("IsWeekend(%d) = %v, want %v", tt.weekday, result, tt.expected)
			}
		})
	}
}

func TestInTimeRange(t *testing.T) {
	tests := []struct {
		name        string
		hour        int
		minute      int
		startHour   int
		startMinute int
		endHour     int
		endMinute   int
		expected    bool
	}{
		{
			name: "within range - middle",
			hour: 10, minute: 30,
			startHour: 9, startMinute: 0,
			endHour: 17, endMinute: 0,
			expected: true,
		},
		{
			name: "within range - start boundary",
			hour: 9, minute: 0,
			startHour: 9, startMinute: 0,
			endHour: 17, endMinute: 0,
			expected: true,
		},
		{
			name: "within range - end boundary",
			hour: 17, minute: 0,
			startHour: 9, startMinute: 0,
			endHour: 17, endMinute: 0,
			expected: true,
		},
		{
			name: "before range",
			hour: 8, minute: 59,
			startHour: 9, startMinute: 0,
			endHour: 17, endMinute: 0,
			expected: false,
		},
		{
			name: "after range",
			hour: 17, minute: 1,
			startHour: 9, startMinute: 0,
			endHour: 17, endMinute: 0,
			expected: false,
		},
		{
			name: "overnight range - in first part",
			hour: 23, minute: 30,
			startHour: 22, startMinute: 0,
			endHour: 6, endMinute: 0,
			expected: true,
		},
		{
			name: "overnight range - in second part",
			hour: 2, minute: 0,
			startHour: 22, startMinute: 0,
			endHour: 6, endMinute: 0,
			expected: true,
		},
		{
			name: "overnight range - outside",
			hour: 12, minute: 0,
			startHour: 22, startMinute: 0,
			endHour: 6, endMinute: 0,
			expected: false,
		},
		{
			name: "exact minute match",
			hour: 14, minute: 30,
			startHour: 14, startMinute: 30,
			endHour: 15, endMinute: 30,
			expected: true,
		},
		{
			name: "one minute before",
			hour: 14, minute: 29,
			startHour: 14, startMinute: 30,
			endHour: 15, endMinute: 30,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InTimeRange(
				tt.hour, tt.minute,
				tt.startHour, tt.startMinute,
				tt.endHour, tt.endMinute,
			)
			if result != tt.expected {
				t.Errorf("InTimeRange(%d:%02d, %d:%02d-%d:%02d) = %v, want %v",
					tt.hour, tt.minute,
					tt.startHour, tt.startMinute,
					tt.endHour, tt.endMinute,
					result, tt.expected)
			}
		})
	}
}

// Test helper functions in expressions
func TestHelperFunctionsInExpressions(t *testing.T) {
	engine := NewEngine(nil)

	tests := []struct {
		rule     *Rule
		name     string
		context  Context
		expected bool
	}{
		{
			name: "DomainRegex in expression",
			rule: &Rule{
				Name:    "Test Regex",
				Logic:   `DomainRegex(Domain, "^cdn\\d+\\.example\\.com$")`,
				Action:  ActionBlock,
				Enabled: true,
			},
			context: Context{
				Domain: "cdn123.example.com",
			},
			expected: true,
		},
		{
			name: "DomainLevelCount in expression",
			rule: &Rule{
				Name:    "Test Level Count",
				Logic:   `DomainLevelCount(Domain) >= 4`,
				Action:  ActionBlock,
				Enabled: true,
			},
			context: Context{
				Domain: "www.cdn.example.com",
			},
			expected: true,
		},
		{
			name: "IPEquals in expression",
			rule: &Rule{
				Name:    "Test IP Equals",
				Logic:   `IPEquals(ClientIP, "192.168.1.1")`,
				Action:  ActionBlock,
				Enabled: true,
			},
			context: Context{
				ClientIP: "192.168.1.1",
			},
			expected: true,
		},
		{
			name: "QueryTypeIn in expression",
			rule: &Rule{
				Name:    "Test Query Type",
				Logic:   `QueryTypeIn(QueryType, "A", "AAAA")`,
				Action:  ActionBlock,
				Enabled: true,
			},
			context: Context{
				QueryType: "A",
			},
			expected: true,
		},
		{
			name: "IsWeekend in expression",
			rule: &Rule{
				Name:    "Test Weekend",
				Logic:   `IsWeekend(Weekday)`,
				Action:  ActionBlock,
				Enabled: true,
			},
			context: Context{
				Weekday: 0, // Sunday
			},
			expected: true,
		},
		{
			name: "InTimeRange in expression",
			rule: &Rule{
				Name:    "Test Time Range",
				Logic:   `InTimeRange(Hour, Minute, 9, 0, 17, 0)`,
				Action:  ActionBlock,
				Enabled: true,
			},
			context: Context{
				Hour:   10,
				Minute: 30,
			},
			expected: true,
		},
		{
			name: "Complex expression with multiple helpers",
			rule: &Rule{
				Name:    "Complex Rule",
				Logic:   `DomainLevelCount(Domain) >= 3 && QueryTypeIn(QueryType, "A", "AAAA") && !IsWeekend(Weekday)`,
				Action:  ActionBlock,
				Enabled: true,
			},
			context: Context{
				Domain:    "www.example.com",
				QueryType: "A",
				Weekday:   1, // Monday
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Add rule to engine
			if err := engine.AddRule(tt.rule); err != nil {
				t.Fatalf("Failed to add rule: %v", err)
			}

			// Evaluate
			matched, _ := engine.Evaluate(tt.context)

			if matched != tt.expected {
				t.Errorf("Evaluate() = %v, want %v", matched, tt.expected)
			}

			// Clean up
			engine.Clear()
		})
	}
}
