# DNS Library v2 Migration (Future Task)

**Status**: Deferred until Phase 1 complete
**Priority**: Medium
**Effort**: 4-6 hours

## Background

The DNS library has moved from GitHub to Codeberg and released v2 with significant breaking changes:
- **v1**: `github.com/miekg/dns` (currently using v1.1.68)
- **v2**: `codeberg.org/miekg/dns` (v0.5.26)

## Why Deferred?

1. **Phase 1 Stability**: We have working, tested code with v1
2. **Breaking Changes**: v2 has significant API changes requiring updates to ~8 files
3. **Risk vs Reward**: No immediate benefit to migration during active development
4. **Testing Required**: Full integration testing needed after migration

## Breaking Changes in v2

### 1. Question Section Changed
- **v1**: `msg.Question[0].Name`, `msg.Question[0].Qtype`
- **v2**: Question is now `[]RR` - need to extract from RR header

### 2. Handler Signature
- **v1**: `func ServeDNS(w dns.ResponseWriter, r *dns.Msg)`
- **v2**: `func ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg)`

### 3. Message Creation
- **v1**: `msg.SetQuestion()`, `msg.SetEdns0()`
- **v2**: `dns.NewMsg()`, direct field access

### 4. TTL Field
- **v1**: `rr.Header().Ttl` (lowercase)
- **v2**: `rr.Header().TTL` (uppercase)

### 5. Client API
- **v1**: `client.Exchange()`, `Net` field, `Timeout` field
- **v2**: Different client structure

### 6. Utility Functions
- **v1**: `dns.Fqdn()`
- **v2**: May be removed or renamed

## Files Requiring Updates

1. `pkg/dns/server.go` - Handler signature + Question access
2. `pkg/dns/server_impl.go` - Handler interface
3. `pkg/dns/handler_test.go` - Test signatures
4. `pkg/cache/cache.go` - Question field access
5. `pkg/cache/cache_test.go` - Test updates
6. `pkg/forwarder/forwarder.go` - Client API + Question access
7. `pkg/forwarder/forwarder_test.go` - Test updates
8. `pkg/blocklist/blocklist.go` - dns.Fqdn() usage

## Migration Checklist

### Preparation
- [ ] Read full migration guide: https://codeberg.org/miekg/dns/src/branch/main/README-diff-with-v1.md
- [ ] Create feature branch: `git checkout -b feature/dns-v2-migration`
- [ ] Backup current working state

### Code Updates
- [ ] Update all imports: `github.com/miekg/dns` → `codeberg.org/miekg/dns`
- [ ] Update handler signatures to include `context.Context`
- [ ] Update Question access to use `[]RR` format
- [ ] Update TTL access (Ttl → TTL)
- [ ] Update Client usage and configuration
- [ ] Replace `dns.Fqdn()` calls
- [ ] Update message creation calls

### Testing
- [ ] Run all unit tests: `go test ./...`
- [ ] Run integration tests
- [ ] Manual DNS query testing
- [ ] Performance benchmarks
- [ ] Cache functionality verification
- [ ] Forwarder retry/failover testing
- [ ] Blocklist functionality testing

### Documentation
- [ ] Update README.md with new dependency
- [ ] Update ARCHITECTURE.md if needed
- [ ] Update any API examples
- [ ] Document v2 migration in CHANGELOG

## Migration Script (Draft)

```bash
#!/bin/bash
# migrate-to-dns-v2.sh

echo "Starting miekg/dns v2 migration..."

# 1. Update imports
find . -name "*.go" -type f -exec sed -i 's|github.com/miekg/dns|codeberg.org/miekg/dns|g' {} \;

# 2. Update dependencies
go get codeberg.org/miekg/dns@latest
go mod tidy

# 3. Build and identify errors
go build ./... 2>&1 | tee migration-errors.log

echo "Check migration-errors.log for required code changes"
echo "Manual updates needed for:"
echo "  - Handler signatures (add context.Context)"
echo "  - Question field access"
echo "  - TTL field name (uppercase)"
echo "  - Client API changes"
```

## Testing Strategy

1. **Unit Tests First**: Ensure all package-level tests pass
2. **Integration Tests**: Test DNS query flow end-to-end
3. **Performance**: Compare benchmarks with v1 baseline
4. **Production Simulation**: Run with test blocklists and high query load

## Rollback Plan

If migration encounters issues:

```bash
# Revert imports
find . -name "*.go" -type f -exec sed -i 's|codeberg.org/miekg/dns|github.com/miekg/dns|g' {} \;

# Restore v1
go mod edit -droprequire=codeberg.org/miekg/dns
go get github.com/miekg/dns@v1.1.68
go mod tidy
```

## Benefits of v2

1. **Official Source**: Maintained on Codeberg (author's preferred platform)
2. **Better API**: Cleaner message structure
3. **Context Support**: Better cancellation and timeout handling
4. **Future Features**: New features will only be in v2

## Risks

1. **Regression Risk**: Breaking working code
2. **Testing Burden**: Need comprehensive testing
3. **Dependencies**: Other packages may not have migrated yet

## Recommendation

**Defer until**:
- Phase 1 is 100% complete
- All features tested and stable
- Ready for a major version bump (v0.5.0 → v0.6.0)

**When ready**:
- Allocate dedicated time (4-6 hours)
- Create feature branch
- Follow checklist above
- Full test suite before merging

---

**Created**: 2025-11-21
**Author**: Glory-Hole Development Team
**Status**: Reference document for future migration
