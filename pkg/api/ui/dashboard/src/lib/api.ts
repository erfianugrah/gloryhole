// ─── Glory-Hole API Client ───────────────────────────────────────────
// Typed fetch wrappers for all /api/* endpoints.

async function apiFetch<T>(
  path: string,
  init?: RequestInit
): Promise<T> {
  const res = await fetch(path, {
    credentials: "same-origin",
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`API ${res.status}: ${text || res.statusText}`);
  }
  // Some endpoints return 204 No Content
  if (res.status === 204) return undefined as T;
  return res.json();
}

// ─── Types ───────────────────────────────────────────────────────────

export interface Stats {
  total_queries: number;
  blocked_queries: number;
  cached_queries: number;
  block_rate: number;
  cache_hit_rate: number;
  avg_response_ms: number;
  cpu_usage_percent: number;
  memory_usage_bytes: number;
  memory_total_bytes: number;
  memory_usage_percent: number;
  temperature_celsius: number;
  temperature_available: boolean;
  period: string;
  timestamp: string;
}

export interface TimeseriesBucket {
  timestamp: string;
  total: number;
  blocked: number;
  cached: number;
  allowed: number;
}

// Raw shape from the Go API
interface TimeseriesBucketRaw {
  timestamp: string;
  total_queries: number;
  blocked_queries: number;
  cached_queries: number;
  avg_response_ms: number;
}

export interface QueryTypeCount {
  type: string;
  count: number;
  percentage: number;
}

// Raw shape from the Go API
interface QueryTypeStatRaw {
  query_type: string;
  total: number;
  blocked: number;
  cached: number;
}

export interface TopDomain {
  domain: string;
  query_count: number;
}

// Raw shape from the Go API
interface TopDomainRaw {
  domain: string;
  queries: number;
  blocked: boolean;
}

export interface QueryLog {
  id: number;
  timestamp: string;
  client_ip: string;
  domain: string;
  query_type: string;
  blocked: boolean;
  cached: boolean;
  dnssec_validated: boolean;
  response_code: number;
  upstream: string;
  upstream_error?: string;
  response_time_ms: number;
  upstream_response_ms: number;
  block_trace?: BlockTraceEntry[];
}

export interface BlockTraceEntry {
  stage: string;
  action: string;
  rule?: string;
  source?: string;
  detail?: string;
  metadata?: Record<string, string>;
}

export interface Policy {
  id: number;
  name: string;
  logic: string;
  action: string;
  action_data?: string;
  enabled: boolean;
}

export interface LocalRecord {
  id: string;
  domain: string;
  type: string;
  value: string;   // UI display value (IP, target, or text)
  ttl: number;
}

export interface LocalRecordCreateRequest {
  domain: string;
  type: string;
  ips?: string[];
  target?: string;
  txt_records?: string[];
  ttl: number;
  priority?: number;
  weight?: number;
  port?: number;
  wildcard?: boolean;
}

export interface ConditionalForwardingRule {
  id: string;
  name: string;
  domains?: string[];
  client_cidrs?: string[];
  query_types?: string[];
  upstreams: string[];
  priority: number;
  timeout?: string;
  max_retries?: number;
  failover: boolean;
  enabled: boolean;
}

export interface ClientSummary {
  client_ip: string;
  display_name: string;
  group_name?: string;
  group_color?: string;
  notes?: string;
  total_queries: number;
  blocked_queries: number;
  nxdomain_queries: number;
  last_seen: string;
  first_seen: string;
}

export interface ClientGroup {
  name: string;
  description: string;
  clients: string[];
}

export interface BlocklistInfo {
  enabled: boolean;
  auto_update: boolean;
  update_interval: string;
  total_domains: number;
  exact_domains: number;
  pattern_stats: Record<string, number>;
  last_updated?: string;
  sources: string[];
}

// Alias for UI convenience
export type BlocklistSource = string;

export interface HealthResponse {
  status: string;
  version: string;
  uptime: string;
  uptime_seconds: number;
}

export interface FeatureState {
  blocklist_enabled: boolean;
  blocklist_temp_disabled: boolean;
  blocklist_disabled_until?: string;
  policies_enabled: boolean;
  policies_temp_disabled: boolean;
  policies_disabled_until?: string;
}

/** Effective state: enabled in config AND not temporarily disabled. */
export function isBlocklistActive(f: FeatureState | null): boolean {
  if (!f) return true;
  return f.blocklist_enabled && !f.blocklist_temp_disabled;
}

export function isPoliciesActive(f: FeatureState | null): boolean {
  if (!f) return true;
  return f.policies_enabled && !f.policies_temp_disabled;
}

export interface ConfigResponse {
  server: {
    listen_address: string;
    web_ui_address: string;
    tcp_enabled: boolean;
    udp_enabled: boolean;
    enable_blocklist: boolean;
    enable_policies: boolean;
    decision_trace: boolean;
    dot_enabled: boolean;
    dot_address: string;
    cors_allowed_origins: string[];
    tls: {
      cert_file: string;
      key_file: string;
      autocert: Record<string, unknown>;
      acme: { enabled: boolean; hosts: string[]; email?: string; cache_dir?: string };
    };
  };
  cache: {
    enabled: boolean;
    max_entries: number;
    min_ttl: string;
    max_ttl: string;
    negative_ttl: string;
    blocked_ttl: string;
    shard_count: number;
  };
  block_page?: { enabled: boolean; block_ip: string };
  logging: { level: string; format: string; output: string; file_path?: string;
             add_source?: boolean; max_size?: number; max_backups?: number; max_age?: number };
  upstream_dns_servers: string[];
  blocklists: string[];
  whitelist: string[];
}

// ─── Statistics ──────────────────────────────────────────────────────

export function fetchStats(since = "24h"): Promise<Stats> {
  return apiFetch<Stats>(`/api/stats?since=${since}`);
}

export async function fetchTimeseries(
  since = "24h",
  buckets = 24
): Promise<TimeseriesBucket[]> {
  // Go handler expects ?period=hour|day|week&points=N
  // period sets the bucket size: hour=1h buckets, day=24h buckets, week=7d buckets
  // "Last 24h" → 24 x 1h buckets; "Last 7d" → 7 x 24h buckets
  const period = since === "7d" ? "day" : "hour";
  const res = await apiFetch<{ data: TimeseriesBucketRaw[] }>(
    `/api/stats/timeseries?period=${period}&points=${buckets}`
  );
  return (res.data ?? []).map((d) => ({
    timestamp: d.timestamp,
    total: d.total_queries,
    blocked: d.blocked_queries,
    cached: d.cached_queries,
    allowed: d.total_queries - d.blocked_queries - d.cached_queries,
  }));
}

export async function fetchQueryTypes(since = "24h"): Promise<QueryTypeCount[]> {
  const res = await apiFetch<{ types: QueryTypeStatRaw[] }>(`/api/stats/query-types?since=${since}`);
  const raw = res.types ?? [];
  const total = raw.reduce((sum, t) => sum + t.total, 0);
  return raw.map((t) => ({
    type: t.query_type,
    count: t.total,
    percentage: total > 0 ? (t.total / total) * 100 : 0,
  }));
}

export async function fetchTopDomains(
  limit = 10,
  blocked = false,
  since?: string
): Promise<TopDomain[]> {
  let url = `/api/top-domains?limit=${limit}&blocked=${blocked}`;
  if (since) url += `&since=${since}`;
  const res = await apiFetch<{ domains: TopDomainRaw[] }>(url);
  return (res.domains ?? []).map((d) => ({ domain: d.domain, query_count: d.queries }));
}

// ─── Queries ─────────────────────────────────────────────────────────

export interface QueryFilter {
  limit?: number;
  offset?: number;
  status?: string;
  query_type?: string;
  client?: string;
  domain?: string;
  since?: string;
}

export async function fetchQueries(filter: QueryFilter = {}): Promise<QueryLog[]> {
  const params = new URLSearchParams();
  if (filter.limit) params.set("limit", String(filter.limit));
  if (filter.offset) params.set("offset", String(filter.offset));
  if (filter.status) params.set("status", filter.status);
  if (filter.query_type) params.set("type", filter.query_type);  // Go reads "type"
  if (filter.client) params.set("client", filter.client);
  if (filter.domain) params.set("domain", filter.domain);
  if (filter.since) {
    // Go reads "start" as an ISO timestamp, convert duration like "24h" to absolute time
    const ms = filter.since.endsWith("h")
      ? parseInt(filter.since) * 3600000
      : filter.since.endsWith("d")
        ? parseInt(filter.since) * 86400000
        : 86400000;
    params.set("start", new Date(Date.now() - ms).toISOString());
  }
  const res = await apiFetch<{ queries: QueryLog[] }>(`/api/queries?${params}`);
  return res.queries ?? [];
}

// ─── Policies ────────────────────────────────────────────────────────

export async function fetchPolicies(): Promise<Policy[]> {
  const res = await apiFetch<{ policies: Policy[] }>("/api/policies");
  return res.policies ?? [];
}

export function createPolicy(
  policy: Omit<Policy, "id">
): Promise<Policy> {
  return apiFetch<Policy>("/api/policies", {
    method: "POST",
    body: JSON.stringify(policy),
  });
}

export function updatePolicy(
  id: number,
  policy: Partial<Policy>
): Promise<Policy> {
  return apiFetch<Policy>(`/api/policies/${id}`, {
    method: "PUT",
    body: JSON.stringify(policy),
  });
}

export function deletePolicy(id: number): Promise<void> {
  return apiFetch<void>(`/api/policies/${id}`, { method: "DELETE" });
}

export function testPolicy(
  logic: string,
  domain: string,
  clientIP?: string,
  queryType?: string
): Promise<{ matched: boolean }> {
  return apiFetch(`/api/policies/test`, {
    method: "POST",
    body: JSON.stringify({ logic, domain, client_ip: clientIP, query_type: queryType }),
  });
}

export async function exportPolicies(): Promise<Policy[]> {
  const res = await apiFetch<{ policies: Policy[]; total: number }>("/api/policies/export");
  return res.policies ?? [];
}

// ─── Local Records ───────────────────────────────────────────────────

interface LocalRecordRaw {
  id: string;
  domain: string;
  type: string;
  ips?: string[];
  target?: string;
  txt_records?: string[];
  ttl: number;
}

export async function fetchLocalRecords(): Promise<LocalRecord[]> {
  const res = await apiFetch<{ records: LocalRecordRaw[] }>("/api/localrecords");
  return (res.records ?? []).map((r) => ({
    id: r.id,
    domain: r.domain,
    type: r.type,
    ttl: r.ttl,
    value: r.ips?.join(", ") ?? r.target ?? r.txt_records?.join("; ") ?? "",
  }));
}

export function createLocalRecord(
  record: LocalRecordCreateRequest
): Promise<LocalRecord> {
  return apiFetch<LocalRecord>("/api/localrecords", {
    method: "POST",
    body: JSON.stringify(record),
  });
}

export function deleteLocalRecord(id: string): Promise<void> {
  return apiFetch<void>(`/api/localrecords/${id}`, { method: "DELETE" });
}

// ─── Conditional Forwarding ──────────────────────────────────────────

export async function fetchForwardingRules(): Promise<ConditionalForwardingRule[]> {
  const res = await apiFetch<{ rules: ConditionalForwardingRule[] }>("/api/conditionalforwarding");
  return res.rules ?? [];
}

export function createForwardingRule(
  rule: Omit<ConditionalForwardingRule, "id">
): Promise<ConditionalForwardingRule> {
  return apiFetch<ConditionalForwardingRule>("/api/conditionalforwarding", {
    method: "POST",
    body: JSON.stringify(rule),
  });
}

export function deleteForwardingRule(id: string): Promise<void> {
  return apiFetch<void>(`/api/conditionalforwarding/${id}`, {
    method: "DELETE",
  });
}

// ─── Clients ─────────────────────────────────────────────────────────

export async function fetchClients(
  limit = 50,
  offset = 0,
  search?: string
): Promise<ClientSummary[]> {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  if (search) params.set("search", search);
  const res = await apiFetch<{ clients: ClientSummary[] }>(`/api/clients?${params}`);
  return res.clients ?? [];
}

export function updateClient(
  client: string,
  data: { display_name?: string; group_name?: string; notes?: string }
): Promise<void> {
  return apiFetch<void>(`/api/clients/${encodeURIComponent(client)}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export async function fetchClientGroups(): Promise<ClientGroup[]> {
  const res = await apiFetch<{ groups: ClientGroup[] }>("/api/client-groups");
  return res.groups ?? [];
}

export function createClientGroup(group: ClientGroup): Promise<ClientGroup> {
  return apiFetch<ClientGroup>("/api/client-groups", {
    method: "POST",
    body: JSON.stringify(group),
  });
}

export function updateClientGroup(
  name: string,
  group: Partial<ClientGroup>
): Promise<ClientGroup> {
  return apiFetch<ClientGroup>(
    `/api/client-groups/${encodeURIComponent(name)}`,
    { method: "PUT", body: JSON.stringify(group) }
  );
}

export function deleteClientGroup(name: string): Promise<void> {
  return apiFetch<void>(`/api/client-groups/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

// ─── Blocklists ──────────────────────────────────────────────────────

export function fetchBlocklists(): Promise<BlocklistInfo> {
  return apiFetch<BlocklistInfo>("/api/blocklists");
}

export function reloadBlocklists(): Promise<void> {
  return apiFetch<void>("/api/blocklist/reload", { method: "POST" });
}

export function checkDomain(
  domain: string
): Promise<{ blocked: boolean; source?: string }> {
  return apiFetch(`/api/blocklists/check?domain=${encodeURIComponent(domain)}`);
}

export function updateBlocklistSources(
  sources: string[]
): Promise<{ status: string; message: string; sources: string[] }> {
  return apiFetch("/api/config/blocklists", {
    method: "PUT",
    body: JSON.stringify({ sources }),
  });
}

// ─── Features ────────────────────────────────────────────────────────

export function fetchFeatures(): Promise<FeatureState> {
  return apiFetch<FeatureState>("/api/features");
}

export function updateFeatures(state: Partial<FeatureState>): Promise<void> {
  return apiFetch<void>("/api/features", {
    method: "PUT",
    body: JSON.stringify(state),
  });
}

/** Convert a human duration like "5m", "1h", "24h" to seconds for the Go API. */
function durationToSeconds(dur?: string): number {
  if (!dur) return 0; // indefinite
  const match = dur.match(/^(\d+)(m|h)$/);
  if (!match) return 0;
  const n = parseInt(match[1], 10);
  return match[2] === "h" ? n * 3600 : n * 60;
}

export function disableBlocklist(duration?: string): Promise<void> {
  return apiFetch<void>("/api/features/blocklist/disable", {
    method: "POST",
    body: JSON.stringify({ duration: durationToSeconds(duration) }),
  });
}

export function enableBlocklist(): Promise<void> {
  return apiFetch<void>("/api/features/blocklist/enable", { method: "POST" });
}

export function disablePolicies(duration?: string): Promise<void> {
  return apiFetch<void>("/api/features/policies/disable", {
    method: "POST",
    body: JSON.stringify({ duration: durationToSeconds(duration) }),
  });
}

export function enablePolicies(): Promise<void> {
  return apiFetch<void>("/api/features/policies/enable", { method: "POST" });
}

// ─── Config ──────────────────────────────────────────────────────────

export function fetchConfig(): Promise<ConfigResponse> {
  return apiFetch<ConfigResponse>("/api/config");
}

export function updateUpstreams(
  upstreams: string[]
): Promise<void> {
  return apiFetch<void>("/api/config/upstreams", {
    method: "PUT",
    body: JSON.stringify({ servers: upstreams }),
  });
}

export function updateCacheConfig(
  config: Record<string, unknown>
): Promise<void> {
  return apiFetch<void>("/api/config/cache", {
    method: "PUT",
    body: JSON.stringify(config),
  });
}

export function updateLoggingConfig(
  config: Record<string, unknown>
): Promise<void> {
  return apiFetch<void>("/api/config/logging", {
    method: "PUT",
    body: JSON.stringify(config),
  });
}

export function updateTLSConfig(
  config: Record<string, unknown>
): Promise<void> {
  return apiFetch<void>("/api/config/tls", {
    method: "PUT",
    body: JSON.stringify(config),
  });
}

// ─── Block Page ──────────────────────────────────────────────────

export interface BlockPageConfig {
  enabled: boolean;
  block_ip: string;
}

export function updateBlockPageConfig(
  config: BlockPageConfig
): Promise<void> {
  return apiFetch<void>("/api/config/block-page", {
    method: "PUT",
    body: JSON.stringify(config),
  });
}

// ─── System ──────────────────────────────────────────────────────────

export function fetchHealth(): Promise<HealthResponse> {
  return apiFetch<HealthResponse>("/api/health");
}

export function purgeCache(): Promise<void> {
  return apiFetch<void>("/api/cache/purge", { method: "POST" });
}

export function resetStorage(): Promise<void> {
  return apiFetch<void>("/api/storage/reset", {
    method: "POST",
    body: JSON.stringify({ confirm: "NUKE" }),
  });
}

// ─── Unbound Resolver ────────────────────────────────────────────────

export interface UnboundStatus {
  enabled: boolean;
  managed: boolean;
  state: "stopped" | "starting" | "running" | "degraded" | "failed";
  error?: string;
  listen_addr?: string;
}

export interface UnboundStats {
  total_queries: number;
  cache_hits: number;
  cache_miss: number;
  cache_hit_rate: number;
  avg_recursion_ms: number;
  msg_cache_count: number;
  rrset_cache_count: number;
  mem_total_bytes: number;
  uptime_seconds: number;
  query_types: Record<string, number>;
  response_codes: Record<string, number>;
}

export interface UnboundServerBlock {
  interface: string;
  port: number;
  msg_cache_size: string;
  rrset_cache_size: string;
  key_cache_size: string;
  cache_min_ttl: number;
  cache_max_negative_ttl: number;
  module_config: string;
  harden_glue: boolean;
  harden_dnssec_stripped: boolean;
  harden_below_nxdomain: boolean;
  harden_algo_downgrade: boolean;
  qname_minimisation: boolean;
  aggressive_nsec: boolean;
  num_threads: number;
  edns_buffer_size: number;
  serve_expired: boolean;
  serve_expired_ttl: number;
  prefetch: boolean;
  prefetch_key: boolean;
  verbosity: number;
  log_queries: boolean;
  log_replies: boolean;
  log_servfail: boolean;
  hide_identity: boolean;
  hide_version: boolean;
  minimal_responses: boolean;
  extended_statistics: boolean;
}

export interface UnboundForwardZone {
  name: string;
  forward_addrs: string[];
  forward_first?: boolean;
  forward_tls_upstream?: boolean;
}

export interface UnboundConfig {
  server: UnboundServerBlock;
  forward_zones: UnboundForwardZone[];
  stub_zones: Array<{ name: string; stub_addrs: string[] }>;
}

export function fetchUnboundStatus(): Promise<UnboundStatus> {
  return apiFetch<UnboundStatus>("/api/unbound/status");
}

export function fetchUnboundStats(): Promise<UnboundStats> {
  return apiFetch<UnboundStats>("/api/unbound/stats");
}

export function fetchUnboundConfig(): Promise<UnboundConfig> {
  return apiFetch<UnboundConfig>("/api/unbound/config");
}

export function updateUnboundServer(
  config: Partial<UnboundServerBlock>
): Promise<UnboundServerBlock> {
  return apiFetch<UnboundServerBlock>("/api/unbound/config/server", {
    method: "PUT",
    body: JSON.stringify(config),
  });
}

export function fetchForwardZones(): Promise<UnboundForwardZone[]> {
  return apiFetch<UnboundForwardZone[]>("/api/unbound/forward-zones");
}

export function createForwardZone(
  zone: UnboundForwardZone
): Promise<UnboundForwardZone> {
  return apiFetch<UnboundForwardZone>("/api/unbound/forward-zones", {
    method: "POST",
    body: JSON.stringify(zone),
  });
}

export function updateForwardZone(
  name: string,
  zone: Partial<UnboundForwardZone>
): Promise<UnboundForwardZone[]> {
  return apiFetch<UnboundForwardZone[]>(
    `/api/unbound/forward-zones/${encodeURIComponent(name)}`,
    { method: "PUT", body: JSON.stringify(zone) }
  );
}

export function deleteForwardZone(name: string): Promise<void> {
  return apiFetch<void>(
    `/api/unbound/forward-zones/${encodeURIComponent(name)}`,
    { method: "DELETE" }
  );
}

export function reloadUnbound(): Promise<void> {
  return apiFetch<void>("/api/unbound/reload", { method: "POST" });
}

export function flushUnboundCache(): Promise<void> {
  return apiFetch<void>("/api/unbound/flush-cache", { method: "POST" });
}
