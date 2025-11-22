package config

import (
	"testing"
)

func TestConditionalForwardingConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ConditionalForwardingConfig
		wantErr bool
	}{
		{
			name: "disabled config - always valid",
			config: ConditionalForwardingConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid rule with domain",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:      "Local domains",
						Priority:  50,
						Domains:   []string{"*.local"},
						Upstreams: []string{"192.168.1.1:53"},
						Enabled:   true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid rule with CIDR",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:        "VPN clients",
						Priority:    50,
						ClientCIDRs: []string{"10.8.0.0/24"},
						Upstreams:   []string{"10.0.0.1:53"},
						Enabled:     true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid rule with query type",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:       "PTR queries",
						Priority:   50,
						QueryTypes: []string{"PTR"},
						Upstreams:  []string{"192.168.1.1:53"},
						Enabled:    true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid rule with multiple upstreams",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:     "Multiple upstreams",
						Priority: 50,
						Domains:  []string{"*.local"},
						Upstreams: []string{
							"192.168.1.1:53",
							"192.168.1.2:53",
							"192.168.1.3:53",
						},
						Enabled: true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - missing name",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:      "",
						Domains:   []string{"*.local"},
						Upstreams: []string{"192.168.1.1:53"},
						Enabled:   true,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid - no upstreams",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:      "No upstreams",
						Domains:   []string{"*.local"},
						Upstreams: []string{},
						Enabled:   true,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid - priority 0 defaults to 50",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:      "Priority defaults",
						Priority:  0,
						Domains:   []string{"*.local"},
						Upstreams: []string{"192.168.1.1:53"},
						Enabled:   true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - priority too high",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:      "Invalid priority",
						Priority:  101,
						Domains:   []string{"*.local"},
						Upstreams: []string{"192.168.1.1:53"},
						Enabled:   true,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid - no matching conditions",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:      "No conditions",
						Priority:  50,
						Upstreams: []string{"192.168.1.1:53"},
						Enabled:   true,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid - priority defaults to 50",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:      "Default priority",
						Domains:   []string{"*.local"},
						Upstreams: []string{"192.168.1.1:53"},
						Enabled:   true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple rules with different priorities",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:      "High priority",
						Priority:  90,
						Domains:   []string{"nas.local"},
						Upstreams: []string{"192.168.1.1:53"},
						Enabled:   true,
					},
					{
						Name:      "Low priority",
						Priority:  10,
						Domains:   []string{"*.local"},
						Upstreams: []string{"192.168.1.2:53"},
						Enabled:   true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "combined rule - all matchers",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:        "Combined rule",
						Priority:    80,
						Domains:     []string{"*.local"},
						ClientCIDRs: []string{"10.8.0.0/24"},
						QueryTypes:  []string{"A", "AAAA"},
						Upstreams:   []string{"192.168.1.1:53"},
						Enabled:     true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "disabled rule is skipped",
			config: ConditionalForwardingConfig{
				Enabled: true,
				Rules: []ForwardingRule{
					{
						Name:      "Disabled rule",
						Priority:  50,
						Domains:   []string{"*.local"},
						Upstreams: []string{"192.168.1.1:53"},
						Enabled:   false,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ConditionalForwardingConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestForwardingRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rule    ForwardingRule
		wantErr bool
	}{
		{
			name: "valid rule",
			rule: ForwardingRule{
				Name:      "Test rule",
				Priority:  50,
				Domains:   []string{"*.local"},
				Upstreams: []string{"192.168.1.1:53"},
				Enabled:   true,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			rule: ForwardingRule{
				Name:      "",
				Domains:   []string{"*.local"},
				Upstreams: []string{"192.168.1.1:53"},
			},
			wantErr: true,
		},
		{
			name: "no upstreams",
			rule: ForwardingRule{
				Name:      "Test",
				Domains:   []string{"*.local"},
				Upstreams: []string{},
			},
			wantErr: true,
		},
		{
			name: "priority auto-defaults to 50",
			rule: ForwardingRule{
				Name:      "Test",
				Priority:  0, // Should default to 50
				Domains:   []string{"*.local"},
				Upstreams: []string{"192.168.1.1:53"},
			},
			wantErr: false,
		},
		{
			name: "no matching conditions",
			rule: ForwardingRule{
				Name:      "Test",
				Priority:  50,
				Upstreams: []string{"192.168.1.1:53"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ForwardingRule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check that priority defaults to 50 if not set
			if err == nil && tt.rule.Priority == 0 {
				if tt.rule.Priority != 50 {
					t.Errorf("ForwardingRule.Validate() should set priority to 50, got %d", tt.rule.Priority)
				}
			}
		})
	}
}
