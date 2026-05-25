package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/storage"
)

// newTestStorage spins up a fresh SQLite storage backed by an isolated tempdir
// file (NOT in-memory: we need the migrator to be able to round-trip through
// the same DB across "restarts").
func newTestStorage(t *testing.T) (storage.Storage, string) {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "gh.db")
	def := storage.DefaultConfig()
	def.SQLite.Path = dbPath
	stor, err := storage.NewSQLiteStorage(&def, nil)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	t.Cleanup(func() {
		_ = stor.Close()
	})
	return stor, dbPath
}

// writeConfigFile writes cfg as YAML so the migrator's persist step has a
// real file to overwrite.
func writeConfigFile(t *testing.T, cfg *config.Config) string {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("config.Save: %v", err)
	}
	return path
}

func countAllowMigratedRules(t *testing.T, stor storage.Storage) int {
	t.Helper()
	rules, err := stor.GetPolicyRules(context.Background())
	if err != nil {
		t.Fatalf("GetPolicyRules: %v", err)
	}
	n := 0
	for _, r := range rules {
		if r.Action == "ALLOW" && r.Enabled {
			n++
		}
	}
	return n
}

// TestMigrateWhitelistToPolicies_Idempotent_AcrossRestarts is the regression
// test for CC-1: pre-v0.26 the migrator wrote ALLOW rows on every boot when
// the YAML still had whitelist: entries. Three guards in v0.26: sentinel,
// UNIQUE(name) constraint, YAML persist. This test exercises all three.
func TestMigrateWhitelistToPolicies_Idempotent_AcrossRestarts(t *testing.T) {
	stor, _ := newTestStorage(t)
	logger := logging.NewDefault()

	cfg := &config.Config{
		Whitelist: []string{"foo.com", "*.bar.org", "(baz|qux)\\.example"},
	}
	cfgPath := writeConfigFile(t, cfg)

	// === Boot 1: full migration runs ===
	if !migrateWhitelistToPolicies(cfg, stor, cfgPath, logger) {
		t.Fatal("first boot: expected migration to report changes")
	}
	got := countAllowMigratedRules(t, stor)
	if got != 3 {
		t.Fatalf("first boot: want 3 ALLOW rules, got %d", got)
	}
	if cfg.Whitelist != nil {
		t.Errorf("first boot: cfg.Whitelist must be nil after migration, got %v", cfg.Whitelist)
	}

	// === Verify Guard 3 (YAML persist) ===
	reloaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load after migration: %v", err)
	}
	if len(reloaded.Whitelist) != 0 {
		t.Errorf("YAML persist failed: whitelist still has %d entries on disk: %v", len(reloaded.Whitelist), reloaded.Whitelist)
	}

	// === Verify Guard 1 (sentinel set) ===
	marker, err := stor.GetDynamicConfig(context.Background(), whitelistMigratedSentinel)
	if err != nil {
		t.Fatalf("GetDynamicConfig sentinel: %v", err)
	}
	if marker == "" {
		t.Error("sentinel not set after successful migration")
	}

	// === Boot 2: simulate user manually re-adding whitelist: in YAML ===
	cfg2, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load before boot 2: %v", err)
	}
	cfg2.Whitelist = []string{"foo.com", "new-entry.com"} // re-added by user

	// Sentinel should short-circuit; NO new ALLOW rules added.
	if migrateWhitelistToPolicies(cfg2, stor, cfgPath, logger) {
		t.Error("second boot: sentinel should have prevented migration; got changes=true")
	}
	got2 := countAllowMigratedRules(t, stor)
	if got2 != 3 {
		t.Errorf("second boot: ALLOW rule count must stay at 3 (sentinel guard), got %d", got2)
	}
	if cfg2.Whitelist != nil {
		t.Errorf("second boot: cfg.Whitelist must still be nilled in memory, got %v", cfg2.Whitelist)
	}
}

// TestMigrateWhitelistToPolicies_UniqueConstraintCatchesDuplicates verifies
// Guard 2: even WITHOUT the sentinel (e.g. db wiped but YAML preserved), the
// UNIQUE(name) index on policy_rules prevents duplicate row accumulation.
func TestMigrateWhitelistToPolicies_UniqueConstraintCatchesDuplicates(t *testing.T) {
	stor, _ := newTestStorage(t)
	logger := logging.NewDefault()

	cfg := &config.Config{Whitelist: []string{"unique.com"}}
	cfgPath := writeConfigFile(t, cfg)

	// First migration succeeds.
	migrateWhitelistToPolicies(cfg, stor, cfgPath, logger)
	if got := countAllowMigratedRules(t, stor); got != 1 {
		t.Fatalf("after first migrate: want 1 rule, got %d", got)
	}

	// Manually wipe the sentinel to simulate a backup-restore where the SQLite
	// row was reverted but the policy_rules table preserved.
	if err := stor.SetDynamicConfig(context.Background(), whitelistMigratedSentinel, ""); err != nil {
		t.Fatalf("clear sentinel: %v", err)
	}

	// Re-add the YAML entry (simulate user edit) and try to migrate again.
	cfg.Whitelist = []string{"unique.com"}
	migrateWhitelistToPolicies(cfg, stor, cfgPath, logger)

	// UNIQUE(name) must have caught the duplicate. Still exactly 1 ALLOW rule.
	if got := countAllowMigratedRules(t, stor); got != 1 {
		t.Errorf("UNIQUE constraint should have prevented duplicate; got %d rules", got)
	}
}

// TestMigrateWhitelistToPolicies_NoStorage_AppendsToConfig covers the
// no-storage code path (used for first-boot YAML seed) — sentinel is skipped,
// entries land in cfg.Policy.Rules.
func TestMigrateWhitelistToPolicies_NoStorage_AppendsToConfig(t *testing.T) {
	logger := logging.NewDefault()
	cfg := &config.Config{Whitelist: []string{"a.com", "b.com"}}
	cfgPath := writeConfigFile(t, cfg)

	if !migrateWhitelistToPolicies(cfg, nil, cfgPath, logger) {
		t.Fatal("expected migration to run with no storage")
	}
	if len(cfg.Policy.Rules) != 2 {
		t.Errorf("want 2 policy rules in cfg.Policy.Rules, got %d", len(cfg.Policy.Rules))
	}
	if cfg.Whitelist != nil {
		t.Error("cfg.Whitelist must be nil after migration")
	}
}

// TestMigrateWhitelistToPolicies_PersistFailureNonFatal ensures a read-only
// config path doesn't cause a panic / abort — the sentinel + UNIQUE index
// keep idempotency even when YAML can't be rewritten.
func TestMigrateWhitelistToPolicies_PersistFailureNonFatal(t *testing.T) {
	stor, _ := newTestStorage(t)
	logger := logging.NewDefault()

	cfg := &config.Config{Whitelist: []string{"readonly.com"}}

	// Point at a path under a non-existent parent dir so config.Save fails.
	bogusPath := filepath.Join(t.TempDir(), "no-such-dir", "config.yml")

	// Must not panic, must not return an error.
	got := migrateWhitelistToPolicies(cfg, stor, bogusPath, logger)
	if !got {
		t.Error("migration should still report changes even if YAML persist fails")
	}
	if countAllowMigratedRules(t, stor) != 1 {
		t.Error("rule should land in DB even when YAML persist fails")
	}
	// Verify nothing got written
	if _, err := os.Stat(bogusPath); !os.IsNotExist(err) {
		t.Error("bogus path should not have been created")
	}
}

func countForwardMigratedRules(t *testing.T, stor storage.Storage) []*storage.PolicyRule {
	t.Helper()
	rules, err := stor.GetPolicyRules(context.Background())
	if err != nil {
		t.Fatalf("GetPolicyRules: %v", err)
	}
	out := make([]*storage.PolicyRule, 0)
	for _, r := range rules {
		if r.Action == "FORWARD" {
			out = append(out, r)
		}
	}
	return out
}

// TestMigrateConditionalForwardingToPolicies_FullShape exercises every matcher
// dimension at once (domains incl. wildcard + exact, client_cidrs, query_types)
// and asserts the synthesized DSL expression is correct.
func TestMigrateConditionalForwardingToPolicies_FullShape(t *testing.T) {
	stor, _ := newTestStorage(t)
	logger := logging.NewDefault()

	cfg := &config.Config{
		ConditionalForwarding: config.ConditionalForwardingConfig{
			Enabled: true,
			Rules: []config.ForwardingRule{
				{
					Name:        "corp",
					Domains:     []string{"*.corp.example.com", "internal.example.com"},
					ClientCIDRs: []string{"10.0.0.0/8"},
					QueryTypes:  []string{"A", "AAAA"},
					Upstreams:   []string{"10.0.0.53:53", "10.0.0.54:53"},
					Priority:    80,
					Enabled:     true,
				},
			},
		},
	}
	cfgPath := writeConfigFile(t, cfg)

	if !migrateConditionalForwardingToPolicies(cfg, stor, cfgPath, logger) {
		t.Fatal("expected migration to report changes")
	}

	rules := countForwardMigratedRules(t, stor)
	if len(rules) != 1 {
		t.Fatalf("want 1 FORWARD rule, got %d", len(rules))
	}
	r := rules[0]
	if r.Name != "Forward corp (migrated)" {
		t.Errorf("name: got %q", r.Name)
	}
	if r.Action != "FORWARD" {
		t.Errorf("action: %q", r.Action)
	}
	if r.ActionData != "10.0.0.53:53,10.0.0.54:53" {
		t.Errorf("action_data: %q", r.ActionData)
	}
	expected := `((Domain == "corp.example.com" || DomainEndsWith(Domain, ".corp.example.com")) || Domain == "internal.example.com") && IPInCIDR(ClientIP, "10.0.0.0/8") && QueryTypeIn(QueryType, "A", "AAAA")`
	if r.Logic != expected {
		t.Errorf("logic mismatch:\n  got:  %s\n  want: %s", r.Logic, expected)
	}
	// Priority 80 inverts to SortOrder 1000 + (100-80) = 1020
	if r.SortOrder != 1020 {
		t.Errorf("sort_order: want 1020, got %d", r.SortOrder)
	}

	if cfg.ConditionalForwarding.Enabled || len(cfg.ConditionalForwarding.Rules) != 0 {
		t.Errorf("CF should be drained: enabled=%v rules=%d", cfg.ConditionalForwarding.Enabled, len(cfg.ConditionalForwarding.Rules))
	}
	reloaded, _ := config.Load(cfgPath)
	if reloaded.ConditionalForwarding.Enabled || len(reloaded.ConditionalForwarding.Rules) != 0 {
		t.Errorf("YAML still has CF block: enabled=%v rules=%d", reloaded.ConditionalForwarding.Enabled, len(reloaded.ConditionalForwarding.Rules))
	}
}

func TestMigrateConditionalForwardingToPolicies_Idempotent_AcrossRestarts(t *testing.T) {
	stor, _ := newTestStorage(t)
	logger := logging.NewDefault()

	cfg := &config.Config{
		ConditionalForwarding: config.ConditionalForwardingConfig{
			Enabled: true,
			Rules: []config.ForwardingRule{
				{Name: "a", Domains: []string{"a.test"}, Upstreams: []string{"1.1.1.1:53"}, Priority: 50, Enabled: true},
				{Name: "b", Domains: []string{"b.test"}, Upstreams: []string{"1.1.1.1:53"}, Priority: 50, Enabled: true},
			},
		},
	}
	cfgPath := writeConfigFile(t, cfg)

	migrateConditionalForwardingToPolicies(cfg, stor, cfgPath, logger)
	if got := len(countForwardMigratedRules(t, stor)); got != 2 {
		t.Fatalf("first boot: want 2, got %d", got)
	}

	cfg2, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg2.ConditionalForwarding = config.ConditionalForwardingConfig{
		Enabled: true,
		Rules: []config.ForwardingRule{
			{Name: "a", Domains: []string{"a.test"}, Upstreams: []string{"1.1.1.1:53"}, Priority: 50, Enabled: true},
			{Name: "c", Domains: []string{"c.test"}, Upstreams: []string{"1.1.1.1:53"}, Priority: 50, Enabled: true},
		},
	}

	if migrateConditionalForwardingToPolicies(cfg2, stor, cfgPath, logger) {
		t.Error("second boot: sentinel should have prevented migration")
	}
	if got := len(countForwardMigratedRules(t, stor)); got != 2 {
		t.Errorf("second boot: rule count must stay at 2 (sentinel guard), got %d", got)
	}
}

func TestMigrateConditionalForwardingToPolicies_RejectsEmptyMatchers(t *testing.T) {
	stor, _ := newTestStorage(t)
	logger := logging.NewDefault()

	cfg := &config.Config{
		ConditionalForwarding: config.ConditionalForwardingConfig{
			Enabled: true,
			Rules: []config.ForwardingRule{
				{Name: "no-matchers", Upstreams: []string{"1.1.1.1:53"}, Priority: 50, Enabled: true},
				{Name: "valid", Domains: []string{"x.test"}, Upstreams: []string{"1.1.1.1:53"}, Priority: 50, Enabled: true},
			},
		},
	}
	cfgPath := writeConfigFile(t, cfg)

	migrateConditionalForwardingToPolicies(cfg, stor, cfgPath, logger)
	rules := countForwardMigratedRules(t, stor)
	if len(rules) != 1 {
		t.Fatalf("want 1 valid rule (no-matchers skipped), got %d", len(rules))
	}
	if rules[0].Name != "Forward valid (migrated)" {
		t.Errorf("wrong rule survived: %q", rules[0].Name)
	}
}

func TestMigrateConditionalForwardingToPolicies_DisabledRulesSkipped(t *testing.T) {
	stor, _ := newTestStorage(t)
	logger := logging.NewDefault()

	cfg := &config.Config{
		ConditionalForwarding: config.ConditionalForwardingConfig{
			Enabled: true,
			Rules: []config.ForwardingRule{
				{Name: "off", Domains: []string{"x.test"}, Upstreams: []string{"1.1.1.1:53"}, Priority: 50, Enabled: false},
				{Name: "on", Domains: []string{"y.test"}, Upstreams: []string{"1.1.1.1:53"}, Priority: 50, Enabled: true},
			},
		},
	}
	cfgPath := writeConfigFile(t, cfg)
	migrateConditionalForwardingToPolicies(cfg, stor, cfgPath, logger)

	rules := countForwardMigratedRules(t, stor)
	if len(rules) != 1 || rules[0].Name != "Forward on (migrated)" {
		t.Errorf("only enabled rule should migrate, got %v", rules)
	}
}

func TestBuildPolicyLogicFromCFRule_Permutations(t *testing.T) {
	tests := []struct {
		name string
		rule config.ForwardingRule
		want string
	}{
		{
			name: "single exact domain",
			rule: config.ForwardingRule{Domains: []string{"example.com"}},
			want: `Domain == "example.com"`,
		},
		{
			name: "wildcard domain",
			rule: config.ForwardingRule{Domains: []string{"*.example.com"}},
			want: `(Domain == "example.com" || DomainEndsWith(Domain, ".example.com"))`,
		},
		{
			name: "regex-like domain",
			rule: config.ForwardingRule{Domains: []string{`(foo|bar)\.test`}},
			want: `DomainMatches(Domain, "(foo|bar)\\.test")`,
		},
		{
			name: "two domains OR-joined",
			rule: config.ForwardingRule{Domains: []string{"a.test", "b.test"}},
			want: `(Domain == "a.test" || Domain == "b.test")`,
		},
		{
			name: "single CIDR",
			rule: config.ForwardingRule{ClientCIDRs: []string{"10.0.0.0/8"}},
			want: `IPInCIDR(ClientIP, "10.0.0.0/8")`,
		},
		{
			name: "qtype only",
			rule: config.ForwardingRule{QueryTypes: []string{"A"}},
			want: `QueryTypeIn(QueryType, "A")`,
		},
		{
			name: "all three matchers AND-joined",
			rule: config.ForwardingRule{
				Domains:     []string{"x.test"},
				ClientCIDRs: []string{"10.0.0.0/8"},
				QueryTypes:  []string{"A", "AAAA"},
			},
			want: `Domain == "x.test" && IPInCIDR(ClientIP, "10.0.0.0/8") && QueryTypeIn(QueryType, "A", "AAAA")`,
		},
		{
			name: "no matchers returns empty",
			rule: config.ForwardingRule{},
			want: ``,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPolicyLogicFromCFRule(tc.rule)
			if got != tc.want {
				t.Errorf("\n  got:  %s\n  want: %s", got, tc.want)
			}
		})
	}
}
