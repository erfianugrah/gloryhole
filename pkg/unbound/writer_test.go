package unbound

import (
	"strings"
	"testing"
)

func TestDefaultConfigRenders(t *testing.T) {
	cfg := DefaultServerConfig(5353, "/var/run/unbound/control.sock")

	out, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig failed: %v", err)
	}

	// Verify key directives are present
	checks := []string{
		"interface: 127.0.0.1",
		"port: 5353",
		"do-daemonize: no",
		"chroot: \"\"",
		"directory: \"/etc/unbound\"",
		"module-config: \"validator iterator\"",
		"harden-glue: yes",
		"harden-dnssec-stripped: yes",
		"harden-below-nxdomain: yes",
		"qname-minimisation: yes",
		"aggressive-nsec: yes",
		"num-threads: 1",
		"edns-buffer-size: 1232",
		"serve-expired: yes",
		"prefetch: yes",
		"hide-identity: yes",
		"hide-version: yes",
		"msg-cache-size: 4m",
		"rrset-cache-size: 8m",
		"key-cache-size: 4m",
		"access-control: 127.0.0.1/32 allow",
		"access-control: 0.0.0.0/0 refuse",
		"control-enable: yes",
		"control-interface: /var/run/unbound/control.sock",
		"control-use-cert: no",
		"auto-trust-anchor-file: /etc/unbound/root.key",
		"root-hints: /etc/unbound/root.hints",
		"tls-cert-bundle: /etc/ssl/certs/ca-certificates.crt",
		"private-address: 192.168.0.0/16",
		"private-address: 10.0.0.0/8",
		"extended-statistics: yes",
		"log-servfail: yes",
		"log-queries: no",
		"verbosity: 1",
	}

	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("missing directive: %q", check)
		}
	}
}

func TestForwardZoneRenders(t *testing.T) {
	cfg := DefaultServerConfig(5353, "/var/run/unbound/control.sock")
	cfg.ForwardZones = []ForwardZone{
		{
			Name:         "example.com",
			ForwardAddrs: []string{"10.0.0.1", "10.0.0.2"},
			ForwardFirst: true,
		},
		{
			Name:         "secure.example.com",
			ForwardAddrs: []string{"1.1.1.1@853"},
			ForwardTLS:   true,
		},
	}

	out, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig failed: %v", err)
	}

	checks := []string{
		"forward-zone:",
		`name: "example.com"`,
		"forward-addr: 10.0.0.1",
		"forward-addr: 10.0.0.2",
		"forward-first: yes",
		`name: "secure.example.com"`,
		"forward-addr: 1.1.1.1@853",
		"forward-tls-upstream: yes",
	}

	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("missing forward zone directive: %q", check)
		}
	}
}

func TestStubZoneRenders(t *testing.T) {
	cfg := DefaultServerConfig(5353, "/var/run/unbound/control.sock")
	cfg.StubZones = []StubZone{
		{
			Name:      "internal.corp.",
			StubAddrs: []string{"192.168.1.1"},
			StubPrime: true,
		},
	}

	out, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig failed: %v", err)
	}

	checks := []string{
		"stub-zone:",
		`name: "internal.corp."`,
		"stub-addr: 192.168.1.1",
		"stub-prime: yes",
	}

	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("missing stub zone directive: %q", check)
		}
	}
}

func TestDomainInsecureRenders(t *testing.T) {
	cfg := DefaultServerConfig(5353, "/var/run/unbound/control.sock")
	cfg.Server.DomainInsecure = []string{"nas.local.", "home.arpa."}

	out, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig failed: %v", err)
	}

	checks := []string{
		`domain-insecure: "nas.local."`,
		`domain-insecure: "home.arpa."`,
	}

	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("missing domain-insecure: %q", check)
		}
	}
}

func TestBoolYesNo(t *testing.T) {
	cfg := DefaultServerConfig(5353, "/var/run/unbound/control.sock")

	// Disable some features
	cfg.Server.HardenGlue = false
	cfg.Server.Prefetch = false

	out, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig failed: %v", err)
	}

	if !strings.Contains(out, "harden-glue: no") {
		t.Error("expected harden-glue: no")
	}
	if !strings.Contains(out, "prefetch: no") {
		t.Error("expected prefetch: no")
	}
}

func TestOptionalFieldsOmitted(t *testing.T) {
	cfg := DefaultServerConfig(5353, "/var/run/unbound/control.sock")

	// CacheMaxTTL defaults to 0 — should be omitted
	cfg.Server.CacheMaxTTL = 0

	out, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig failed: %v", err)
	}

	if strings.Contains(out, "cache-max-ttl:") {
		t.Error("cache-max-ttl should be omitted when 0")
	}

	// Set it and verify it appears
	cfg.Server.CacheMaxTTL = 86400
	out, err = RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig failed: %v", err)
	}

	if !strings.Contains(out, "cache-max-ttl: 86400") {
		t.Error("expected cache-max-ttl: 86400")
	}
}
