package unbound

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Patterns for validating unbound config field values.
// These prevent template injection via newlines or embedded directives.
var (
	validNetblock    = regexp.MustCompile(`^[0-9a-fA-F.:/%]+$`)
	validACLAction   = regexp.MustCompile(`^(allow|deny|refuse|allow_snoop|allow_setrd|allow_cookie|deny_non_local|refuse_non_local)$`)
	validDomainName  = regexp.MustCompile(`^[a-zA-Z0-9._-]+\.?$`)
	validAddr        = regexp.MustCompile(`^[0-9a-fA-F.:@#%]+$`) // IP:port, IP@port, IP#name
	validCacheSize   = regexp.MustCompile(`^[0-9]+[kmgKMG]?$`)
	validPrivateAddr = regexp.MustCompile(`^[0-9a-fA-F.:/%]+$`)
)

// sanitizeField rejects values that could inject unbound.conf directives.
// All values embedded in the text/template must pass through this.
func sanitizeField(name, value string, pattern *regexp.Regexp) error {
	if strings.ContainsAny(value, "\n\r\"\\") {
		return fmt.Errorf("%s contains illegal characters (newline, quote, or backslash)", name)
	}
	if len(value) > 512 {
		return fmt.Errorf("%s exceeds maximum length (512)", name)
	}
	if pattern != nil && !pattern.MatchString(value) {
		return fmt.Errorf("%s contains invalid characters: %q", name, value)
	}
	return nil
}

// SanitizeConfig validates all user-supplied string fields in the config
// before they are rendered into unbound.conf via text/template.
// This MUST be called before WriteConfig.
func SanitizeConfig(cfg *UnboundServerConfig) error {
	// Access control entries
	for i, acl := range cfg.Server.AccessControl {
		if err := sanitizeField(fmt.Sprintf("access_control[%d].netblock", i), acl.Netblock, validNetblock); err != nil {
			return err
		}
		if err := sanitizeField(fmt.Sprintf("access_control[%d].action", i), acl.Action, validACLAction); err != nil {
			return err
		}
	}

	// Domain insecure entries
	for i, d := range cfg.Server.DomainInsecure {
		if err := sanitizeField(fmt.Sprintf("domain_insecure[%d]", i), d, validDomainName); err != nil {
			return err
		}
	}

	// Private addresses
	for i, p := range cfg.Server.PrivateAddress {
		if err := sanitizeField(fmt.Sprintf("private_address[%d]", i), p, validPrivateAddr); err != nil {
			return err
		}
	}

	// Forward zones
	for i, fz := range cfg.ForwardZones {
		if err := sanitizeField(fmt.Sprintf("forward_zone[%d].name", i), fz.Name, validDomainName); err != nil {
			return err
		}
		for j, addr := range fz.ForwardAddrs {
			if err := sanitizeField(fmt.Sprintf("forward_zone[%d].forward_addr[%d]", i, j), addr, validAddr); err != nil {
				return err
			}
		}
	}

	// Stub zones
	for i, sz := range cfg.StubZones {
		if err := sanitizeField(fmt.Sprintf("stub_zone[%d].name", i), sz.Name, validDomainName); err != nil {
			return err
		}
		for j, addr := range sz.StubAddrs {
			if err := sanitizeField(fmt.Sprintf("stub_zone[%d].stub_addr[%d]", i, j), addr, validAddr); err != nil {
				return err
			}
		}
	}

	// Cache sizes (string fields that go into template)
	if cfg.Server.MsgCacheSize != "" {
		if err := sanitizeField("msg_cache_size", cfg.Server.MsgCacheSize, validCacheSize); err != nil {
			return err
		}
	}
	if cfg.Server.RRSetCacheSize != "" {
		if err := sanitizeField("rrset_cache_size", cfg.Server.RRSetCacheSize, validCacheSize); err != nil {
			return err
		}
	}
	if cfg.Server.KeyCacheSize != "" {
		if err := sanitizeField("key_cache_size", cfg.Server.KeyCacheSize, validCacheSize); err != nil {
			return err
		}
	}

	return nil
}

// Validate writes the config to a temp file and runs unbound-checkconf.
// Returns nil if valid, or an error with the checkconf output.
func Validate(cfg *UnboundServerConfig, checkconfBin string) error {
	tmp, err := os.CreateTemp("", "unbound-*.conf")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if writeErr := WriteConfig(cfg, tmp.Name()); writeErr != nil {
		return fmt.Errorf("write temp config: %w", writeErr)
	}

	out, err := exec.Command(checkconfBin, tmp.Name()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("unbound-checkconf: %s", strings.TrimSpace(string(out)))
	}

	return nil
}
