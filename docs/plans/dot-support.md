# DoT/DoH Hardening & Android Private DNS Support

## Goal
Provide first-class DNS-over-TLS (DoT) support so Android “Private DNS” can use Gloryhole directly, and harden existing DoH to production readiness (TLS, auth, docs).

## Scope
- Add configurable DoT listener (port 853 by default) with certificate handling.
- Keep DoH at `/dns-query`, but document TLS termination and recommended proxying.
- Minimal auth story: respect existing API auth for DoH; DoT remains unauthenticated (DNS standard).
- Config, metrics, and graceful lifecycle support for new listener.

## Out of Scope
- DoH3/HTTP3, DNSSEC validation, split-horizon routing changes, dynamic cert acquisition UX beyond basic autocert.

## Deliverables
- Code: DoT listener with TLS config (file-based PEM; optional autocert), lifecycle wiring, metrics, and tests.
- Docs: Android Private DNS how-to, config samples, security notes, reverse-proxy guidance for DoH.
- Tests: integration test hitting DoT over TLS with self-signed cert; existing suites kept green.

## Work Plan
- [ ] Config: add `server.dot_enabled`, `server.dot_address` (default `":853"`), `server.tls_cert_file`, `server.tls_key_file`, `server.tls_autocert` block (optional), validation rules.
- [ ] Server wiring: create TLS-enabled miekg/dns server (`Net: "tcp-tls"`, ALPN `dot`); start/stop alongside UDP/TCP; share handler/metrics.
- [ ] Metrics: expose DoT listener status and connection/error counters (extend telemetry where needed).
- [ ] Tests: integration test using `dns.Client{Net: "tcp-tls", TLSConfig: InsecureSkipVerify}` hitting a self-signed DoT listener on an ephemeral port.
- [ ] Docs: update configuration guide + README + quickstart; add Android Private DNS walkthrough; note DoH TLS termination options and firewall ports.
- [ ] Release notes: mention DoT addition and config changes.

## Risks & Mitigations
- Cert management complexity → provide simple PEM path first; autocert optional and disabled by default.
- Port conflicts on 853 → make address configurable and document binding requirements.
- Performance impact of TLS → keep reuse of handler and cache; benchmark if time permits.

## Testing Strategy
- Unit/integration: self-signed DoT listener test, DoH path still passes existing suites.
- Manual: curl with `dnscloudflare-doh`/`cloudflared` for DoH; `kdig -d @host -p 853 +tls-ca` for DoT; Android Private DNS on a hostname with valid cert.
