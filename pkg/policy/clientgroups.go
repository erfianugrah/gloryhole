package policy

import (
	"context"
	"sync/atomic"

	"glory-hole/pkg/storage"
)

// ClientGroupResolver answers "is this IP in this group?" — backed by an
// in-memory cache that is rebuilt when the source-of-truth (SQLite) changes.
//
// Implementations MUST be safe for concurrent reads from the DNS hot path.
// IsInGroup MUST NOT block on I/O.
//
// See docs/designs/client-groups-as-policy-input.md for the full design and
// docs/plans/2026-05-26-v027-cf-deletion-and-clientgroups.md §3 for the
// release plan.
type ClientGroupResolver interface {
	IsInGroup(clientIP, groupName string) bool
}

// noopResolver is the default until SetClientGroupResolver is called.
// All membership queries return false. Rules using InClientGroup() in their
// logic compile and evaluate cleanly on systems that haven't wired up a
// resolver — no compile errors, no runtime errors, no skip-rule semantics.
type noopResolver struct{}

func (noopResolver) IsInGroup(_, _ string) bool { return false }

// resolver is a package-level atomic pointer to the active resolver. Reads
// from the DNS hot path are lock-free single loads; writes (Reload, swap)
// are single pointer stores. Mirrors the BlocklistManager.current pattern.
//
// Why package-level rather than per-engine: compileRuleLogic is a free
// function and Rule.Compile() runs before the rule is attached to any
// Engine instance. A package-level pointer is the minimum-surface change
// and matches the existing IPInCIDR / DomainMatches helpers, which are also
// package-level free functions.
var resolver atomic.Pointer[ClientGroupResolver]

func init() {
	var n ClientGroupResolver = noopResolver{}
	resolver.Store(&n)
}

// SetClientGroupResolver installs the resolver used by InClientGroup().
// Call once at engine init after building the SQLiteResolver. Subsequent
// calls atomically swap the resolver — useful for tests.
func SetClientGroupResolver(r ClientGroupResolver) {
	if r == nil {
		var n ClientGroupResolver = noopResolver{}
		resolver.Store(&n)
		return
	}
	resolver.Store(&r)
}

// InClientGroup is the DSL primitive registered with expr-lang as a function
// callable from rule logic: `InClientGroup(ClientIP, "kids")`. It loads the
// active resolver via a single atomic pointer load and delegates the actual
// membership check.
func InClientGroup(clientIP, groupName string) bool {
	return (*resolver.Load()).IsInGroup(clientIP, groupName)
}

// SQLiteResolver builds and serves the IP → groups cache from SQLite's
// client_profiles table. Reload is cheap (single query, single map build)
// and runs at engine init plus whenever the API mutates the underlying
// table.
type SQLiteResolver struct {
	storage storage.Storage
	cache   atomic.Pointer[map[string]map[string]struct{}]
}

// NewSQLiteResolver constructs a resolver bound to a Storage. Call Reload
// once before installing it via SetClientGroupResolver.
func NewSQLiteResolver(s storage.Storage) *SQLiteResolver {
	return &SQLiteResolver{storage: s}
}

// Reload rebuilds the cache from a fresh ListClientProfiles query. Safe to
// call concurrently with IsInGroup — the swap is a single atomic store.
//
// A profile with empty GroupName contributes nothing to the cache; the IP
// is treated as unprofiled-for-groups (IsInGroup returns false for every
// group name).
//
// Future schema migration: if client_profiles.group_name graduates to a
// many-to-many join table, only this function changes — the cache shape
// already supports multi-group.
func (r *SQLiteResolver) Reload(ctx context.Context) error {
	if r == nil || r.storage == nil {
		empty := make(map[string]map[string]struct{})
		r.cache.Store(&empty)
		return nil
	}

	profiles, err := r.storage.ListClientProfiles(ctx)
	if err != nil {
		return err
	}

	m := make(map[string]map[string]struct{}, len(profiles))
	for _, p := range profiles {
		if p == nil || p.GroupName == "" {
			continue
		}
		groups, ok := m[p.ClientIP]
		if !ok {
			groups = make(map[string]struct{}, 1)
			m[p.ClientIP] = groups
		}
		groups[p.GroupName] = struct{}{}
	}
	r.cache.Store(&m)
	return nil
}

// IsInGroup returns true if the given IP has been assigned to the given
// group via the API. Lock-free single atomic load + two map lookups.
func (r *SQLiteResolver) IsInGroup(clientIP, groupName string) bool {
	if r == nil {
		return false
	}
	m := r.cache.Load()
	if m == nil {
		return false
	}
	groups, ok := (*m)[clientIP]
	if !ok {
		return false
	}
	_, ok = groups[groupName]
	return ok
}
