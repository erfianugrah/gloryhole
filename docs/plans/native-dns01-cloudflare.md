# Native Cloudflare DNS-01 ACME for DoT/DoH

## Goal
Embed first-class DNS-01 (Cloudflare) certificate issuance/renewal so Gloryhole can auto-manage TLS for DoT/DoH without external tools.

## Scope
- Add Cloudflare DNS-01 provider support (API token-based) for ACME issuance and renewal.
- Persist issued certs/keys to cache dir; hot-reload TLS for DoT/DoH on renewal.
- Minimal dependencies: reuse lego ACME library to avoid reinventing challenge logic.
- Secrets via env vars preferred; config keys allowed but discouraged.

## Non-goals
- Multi-provider DNS-01 support (only Cloudflare).
- UI for certificate management.
- DNS-01 for wildcard with multiple providers (only CF).

## Config Sketch
```yaml
server:
  dot_enabled: true
  dot_address: ":853"
  tls:
    acme:
      enabled: true
      dns_provider: "cloudflare"
      cache_dir: "./.cache/acme"
      email: "you@example.com"
      hosts: ["dot.example.com"]
      renew_before: "720h"   # 30 days
      cf:
        api_token: ""        # optional; prefer env CF_DNS_API_TOKEN
```

## Implementation Plan
- [ ] Config: add `tls.acme` block (dns_provider, hosts, email, cache_dir, renew_before, cf token).
- [ ] Secrets: load CF token from env `CF_DNS_API_TOKEN` with config override; warn if empty.
- [ ] ACME client: wrap lego client with Cloudflare DNS solver; store issued PEMs in cache_dir.
- [ ] TLS wiring: on startup, obtain or load certs; build tls.Config; start DoT/DoH.
- [ ] Renewal loop: ticker checks time-to-expiry; renew if <= renew_before; hot-reload tls.Config and servers.
- [ ] Shutdown: stop renewal loop cleanly.
- [ ] Metrics/logging: emit issuance/renewal success/failure counters and succinct logs.
- [ ] Tests: unit for config/env resolution; integration (self-signed ACME stub) is heavy—use mocked DNS solver path and fake storage. Add renewal trigger test with short expiry.
- [ ] Docs: update configuration guide and README with native CF DNS-01 steps and env var usage; note precedence over existing HTTP-01 autocert.

## Risks & Mitigations
- Token leakage: recommend env; avoid logging token; mask in errors.
- Renewal outages: cache last good cert; fall back to existing PEMs on failure.
- Clock skew: log but proceed with conservative renew window.

## Dependencies
- `github.com/go-acme/lego/v4` (already used indirectly in many projects; add to go.mod)
