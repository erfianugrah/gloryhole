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
  response_code: number;
  upstream: string;
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
  value: string;
  ttl: number;
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
  blocklist_disabled_until?: string;
  policies_enabled: boolean;
  policies_disabled_until?: string;
}

export interface ConfigResponse {
  server: Record<string, unknown>;
  dns: Record<string, unknown>;
  blocklist: Record<string, unknown>;
  cache: Record<string, unknown>;
  logging: Record<string, unknown>;
  auth: Record<string, unknown>;
  tls?: Record<string, unknown>;
}

// ─── Statistics ──────────────────────────────────────────────────────

export function fetchStats(since = "24h"): Promise<Stats> {
  return apiFetch<Stats>(`/api/stats?since=${since}`);
}

export async function fetchTimeseries(
  since = "24h",
  buckets = 24
): Promise<TimeseriesBucket[]> {
  const res = await apiFetch<{ data: TimeseriesBucketRaw[] }>(
    `/api/stats/timeseries?since=${since}&buckets=${buckets}`
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
  if (filter.query_type) params.set("query_type", filter.query_type);
  if (filter.client) params.set("client", filter.client);
  if (filter.domain) params.set("domain", filter.domain);
  if (filter.since) params.set("since", filter.since);
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

export async function fetchLocalRecords(): Promise<LocalRecord[]> {
  const res = await apiFetch<{ records: LocalRecord[] }>("/api/localrecords");
  return res.records ?? [];
}

export function createLocalRecord(
  record: Omit<LocalRecord, "id">
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

export function disableBlocklist(duration?: string): Promise<void> {
  return apiFetch<void>("/api/features/blocklist/disable", {
    method: "POST",
    body: JSON.stringify({ duration }),
  });
}

export function enableBlocklist(): Promise<void> {
  return apiFetch<void>("/api/features/blocklist/enable", { method: "POST" });
}

export function disablePolicies(duration?: string): Promise<void> {
  return apiFetch<void>("/api/features/policies/disable", {
    method: "POST",
    body: JSON.stringify({ duration }),
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
    body: JSON.stringify({ upstreams }),
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

// ─── System ──────────────────────────────────────────────────────────

export function fetchHealth(): Promise<HealthResponse> {
  return apiFetch<HealthResponse>("/api/health");
}

export function purgeCache(): Promise<void> {
  return apiFetch<void>("/api/cache/purge", { method: "POST" });
}

export function resetStorage(): Promise<void> {
  return apiFetch<void>("/api/storage/reset", { method: "POST" });
}
