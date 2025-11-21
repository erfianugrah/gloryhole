package localrecords

import (
	"net"
	"testing"
)

func TestIsValidDomain(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		want    bool
	}{
		{
			name:   "valid simple domain",
			domain: "example.com",
			want:   true,
		},
		{
			name:   "valid subdomain",
			domain: "www.example.com",
			want:   true,
		},
		{
			name:   "valid with trailing dot",
			domain: "example.com.",
			want:   true,
		},
		{
			name:   "valid wildcard",
			domain: "*.example.com",
			want:   true,
		},
		{
			name:   "valid single label",
			domain: "localhost",
			want:   true,
		},
		{
			name:   "valid with hyphens",
			domain: "my-server.example.com",
			want:   true,
		},
		{
			name:   "valid with numbers",
			domain: "server123.example.com",
			want:   true,
		},
		{
			name:   "empty domain",
			domain: "",
			want:   false,
		},
		{
			name:   "too long domain (>253)",
			domain: "a." + string(make([]byte, 300)),
			want:   false,
		},
		{
			name:   "starts with dot",
			domain: ".example.com",
			want:   false,
		},
		{
			name:   "label too long (>63)",
			domain: string(make([]byte, 64)) + ".example.com",
			want:   false,
		},
		{
			name:   "empty label",
			domain: "example..com",
			want:   false,
		},
		{
			name:   "hyphen at start of label",
			domain: "-example.com",
			want:   false,
		},
		{
			name:   "hyphen at end of label",
			domain: "example-.com",
			want:   false,
		},
		{
			name:   "invalid character",
			domain: "exam_ple.com",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidDomain(tt.domain)
			if got != tt.want {
				t.Errorf("isValidDomain(%q) = %v, want %v", tt.domain, got, tt.want)
			}
		})
	}
}

func TestIsAlphanumeric(t *testing.T) {
	tests := []struct {
		name string
		c    byte
		want bool
	}{
		{name: "lowercase a", c: 'a', want: true},
		{name: "lowercase z", c: 'z', want: true},
		{name: "uppercase A", c: 'A', want: true},
		{name: "uppercase Z", c: 'Z', want: true},
		{name: "digit 0", c: '0', want: true},
		{name: "digit 9", c: '9', want: true},
		{name: "hyphen", c: '-', want: false},
		{name: "underscore", c: '_', want: false},
		{name: "dot", c: '.', want: false},
		{name: "space", c: ' ', want: false},
		{name: "asterisk", c: '*', want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAlphanumeric(tt.c)
			if got != tt.want {
				t.Errorf("isAlphanumeric(%c) = %v, want %v", tt.c, got, tt.want)
			}
		})
	}
}

func TestParseIP(t *testing.T) {
	tests := []struct {
		name    string
		ipStr   string
		wantErr bool
	}{
		{
			name:    "valid IPv4",
			ipStr:   "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "valid IPv6",
			ipStr:   "fe80::1",
			wantErr: false,
		},
		{
			name:    "valid IPv6 full",
			ipStr:   "2001:0db8:0000:0000:0000:0000:0000:0001",
			wantErr: false,
		},
		{
			name:    "invalid IP",
			ipStr:   "invalid",
			wantErr: true,
		},
		{
			name:    "empty string",
			ipStr:   "",
			wantErr: true,
		},
		{
			name:    "invalid IPv4 (out of range)",
			ipStr:   "256.1.1.1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIP(tt.ipStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIP(%q) error = %v, wantErr %v", tt.ipStr, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Errorf("parseIP(%q) returned nil IP without error", tt.ipStr)
			}
		})
	}
}

func TestParseIPs(t *testing.T) {
	tests := []struct {
		name    string
		ips     []string
		wantErr bool
		wantLen int
	}{
		{
			name:    "valid IPv4 list",
			ips:     []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"},
			wantErr: false,
			wantLen: 3,
		},
		{
			name:    "valid IPv6 list",
			ips:     []string{"fe80::1", "fe80::2"},
			wantErr: false,
			wantLen: 2,
		},
		{
			name:    "mixed IPv4 and IPv6",
			ips:     []string{"192.168.1.1", "fe80::1"},
			wantErr: false,
			wantLen: 2,
		},
		{
			name:    "empty list",
			ips:     []string{},
			wantErr: false,
			wantLen: 0,
		},
		{
			name:    "one invalid IP",
			ips:     []string{"192.168.1.1", "invalid", "192.168.1.2"},
			wantErr: true,
		},
		{
			name:    "all invalid",
			ips:     []string{"invalid1", "invalid2"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIPs(tt.ips)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIPs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantLen {
				t.Errorf("parseIPs() returned %d IPs, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestValidateRecord_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		record  *LocalRecord
		wantErr error
	}{
		{
			name:    "nil record",
			record:  nil,
			wantErr: ErrInvalidRecord,
		},
		{
			name: "empty domain",
			record: &LocalRecord{
				Domain: "",
				Type:   RecordTypeA,
				IPs:    []net.IP{net.ParseIP("192.168.1.1")},
			},
			wantErr: ErrInvalidDomain,
		},
		{
			name: "A record with no IPs",
			record: &LocalRecord{
				Domain: "test.local",
				Type:   RecordTypeA,
				IPs:    []net.IP{},
			},
			wantErr: ErrNoIPs,
		},
		{
			name: "A record with IPv6 (invalid)",
			record: &LocalRecord{
				Domain: "test.local",
				Type:   RecordTypeA,
				IPs:    []net.IP{net.ParseIP("fe80::1")},
			},
			wantErr: ErrInvalidIP,
		},
		{
			name: "AAAA record with no IPs",
			record: &LocalRecord{
				Domain: "test.local",
				Type:   RecordTypeAAAA,
				IPs:    []net.IP{},
			},
			wantErr: ErrNoIPs,
		},
		{
			name: "AAAA record with IPv4 (invalid)",
			record: &LocalRecord{
				Domain: "test.local",
				Type:   RecordTypeAAAA,
				IPs:    []net.IP{net.ParseIP("192.168.1.1").To4()},
			},
			wantErr: ErrInvalidIP,
		},
		{
			name: "CNAME record with empty target",
			record: &LocalRecord{
				Domain: "alias.local",
				Type:   RecordTypeCNAME,
				Target: "",
			},
			wantErr: ErrEmptyTarget,
		},
		{
			name: "MX record with empty target",
			record: &LocalRecord{
				Domain: "mail.local",
				Type:   RecordTypeMX,
				Target: "",
			},
			wantErr: ErrEmptyTarget,
		},
		{
			name: "SRV record with empty target",
			record: &LocalRecord{
				Domain: "_service._tcp.local",
				Type:   RecordTypeSRV,
				Target: "",
			},
			wantErr: ErrEmptyTarget,
		},
		{
			name: "SRV record with no port",
			record: &LocalRecord{
				Domain: "_service._tcp.local",
				Type:   RecordTypeSRV,
				Target: "server.local",
				Port:   0,
			},
			wantErr: ErrInvalidRecord,
		},
		{
			name: "TXT record with empty target",
			record: &LocalRecord{
				Domain: "txt.local",
				Type:   RecordTypeTXT,
				Target: "",
			},
			wantErr: ErrEmptyTarget,
		},
		{
			name: "PTR record with empty target",
			record: &LocalRecord{
				Domain: "1.1.168.192.in-addr.arpa",
				Type:   RecordTypePTR,
				Target: "",
			},
			wantErr: ErrEmptyTarget,
		},
		{
			name: "invalid record type",
			record: &LocalRecord{
				Domain: "test.local",
				Type:   "INVALID",
			},
			wantErr: ErrInvalidRecord,
		},
		{
			name: "valid A record",
			record: &LocalRecord{
				Domain: "test.local",
				Type:   RecordTypeA,
				IPs:    []net.IP{net.ParseIP("192.168.1.1").To4()},
			},
			wantErr: nil,
		},
		{
			name: "valid AAAA record",
			record: &LocalRecord{
				Domain: "test.local",
				Type:   RecordTypeAAAA,
				IPs:    []net.IP{net.ParseIP("fe80::1")},
			},
			wantErr: nil,
		},
		{
			name: "valid CNAME record",
			record: &LocalRecord{
				Domain: "alias.local",
				Type:   RecordTypeCNAME,
				Target: "target.local",
			},
			wantErr: nil,
		},
		{
			name: "valid MX record",
			record: &LocalRecord{
				Domain:   "mail.local",
				Type:     RecordTypeMX,
				Target:   "mx.local",
				Priority: 10,
			},
			wantErr: nil,
		},
		{
			name: "valid SRV record",
			record: &LocalRecord{
				Domain:   "_service._tcp.local",
				Type:     RecordTypeSRV,
				Target:   "server.local",
				Port:     8080,
				Priority: 10,
				Weight:   100,
			},
			wantErr: nil,
		},
		{
			name: "valid TXT record",
			record: &LocalRecord{
				Domain: "txt.local",
				Type:   RecordTypeTXT,
				Target: "v=spf1 include:example.com ~all",
			},
			wantErr: nil,
		},
		{
			name: "valid PTR record",
			record: &LocalRecord{
				Domain: "1.1.168.192.in-addr.arpa",
				Type:   RecordTypePTR,
				Target: "server.local",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRecord(tt.record)
			if err != tt.wantErr {
				t.Errorf("validateRecord() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
