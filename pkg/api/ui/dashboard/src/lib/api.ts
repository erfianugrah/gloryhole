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

export interface QueryTypeCount {
  type: string;
  count: number;
  percentage: number;
}

export interface TopDomain {
  domain: string;
  query_count: number;
}

export interface QueryLog {
  timestamp: string;
  client_ip: string;
  domain: string;
  query_type: string;
  blocked: boolean;
  cached: boolean;
  response_code: number;
  upstream: string;
  response_time_ms: number;
  upstream_time_ms: number;
  block_trace?: BlockTraceEntry[];
}

export interface BlockTraceEntry {
  source: string;
  rule: string;
  list_name?: string;
}

export interface Policy {
  id: string;
  name: string;
  expression: string;
  action: string;
  enabled: boolean;
  priority: number;
  client_filter?: string;
  group_filter?: string;
  description?: string;
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
  domain: string;
  upstreams: string[];
}

export interface ClientSummary {
  ip: string;
  name: string;
  group: string;
  query_count: number;
  blocked_count: number;
  last_seen: string;
}

export interface ClientGroup {
  name: string;
  description: string;
  clients: string[];
}

export interface BlocklistSource {
  url: string;
  name: string;
  enabled: boolean;
  domain_count: number;
  last_updated: string;
  last_status: string;
}

export interface BlocklistInfo {
  sources: BlocklistSource[];
  total_domains: number;
  last_refresh: string;
}

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

export function fetchTimeseries(
  since = "24h",
  buckets = 24
): Promise<TimeseriesBucket[]> {
  return apiFetch<TimeseriesBucket[]>(
    `/api/stats/timeseries?since=${since}&buckets=${buckets}`
  );
}

export function fetchQueryTypes(since = "24h"): Promise<QueryTypeCount[]> {
  return apiFetch<QueryTypeCount[]>(`/api/stats/query-types?since=${since}`);
}

export function fetchTopDomains(
  limit = 10,
  blocked = false,
  since?: string
): Promise<TopDomain[]> {
  let url = `/api/top-domains?limit=${limit}&blocked=${blocked}`;
  if (since) url += `&since=${since}`;
  return apiFetch<TopDomain[]>(url);
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

export function fetchQueries(filter: QueryFilter = {}): Promise<QueryLog[]> {
  const params = new URLSearchParams();
  if (filter.limit) params.set("limit", String(filter.limit));
  if (filter.offset) params.set("offset", String(filter.offset));
  if (filter.status) params.set("status", filter.status);
  if (filter.query_type) params.set("query_type", filter.query_type);
  if (filter.client) params.set("client", filter.client);
  if (filter.domain) params.set("domain", filter.domain);
  if (filter.since) params.set("since", filter.since);
  return apiFetch<QueryLog[]>(`/api/queries?${params}`);
}

// ─── Policies ────────────────────────────────────────────────────────

export function fetchPolicies(): Promise<Policy[]> {
  return apiFetch<Policy[]>("/api/policies");
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
  id: string,
  policy: Partial<Policy>
): Promise<Policy> {
  return apiFetch<Policy>(`/api/policies/${id}`, {
    method: "PUT",
    body: JSON.stringify(policy),
  });
}

export function deletePolicy(id: string): Promise<void> {
  return apiFetch<void>(`/api/policies/${id}`, { method: "DELETE" });
}

export function testPolicy(expression: string): Promise<{ valid: boolean; error?: string }> {
  return apiFetch(`/api/policies/test`, {
    method: "POST",
    body: JSON.stringify({ expression }),
  });
}

export function exportPolicies(): Promise<Policy[]> {
  return apiFetch<Policy[]>("/api/policies/export");
}

// ─── Local Records ───────────────────────────────────────────────────

export function fetchLocalRecords(): Promise<LocalRecord[]> {
  return apiFetch<LocalRecord[]>("/api/localrecords");
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

export function fetchForwardingRules(): Promise<ConditionalForwardingRule[]> {
  return apiFetch<ConditionalForwardingRule[]>("/api/conditionalforwarding");
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

export function fetchClients(
  limit = 50,
  offset = 0,
  search?: string
): Promise<ClientSummary[]> {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  if (search) params.set("search", search);
  return apiFetch<ClientSummary[]>(`/api/clients?${params}`);
}

export function updateClient(
  client: string,
  data: { name?: string; group?: string }
): Promise<void> {
  return apiFetch<void>(`/api/clients/${encodeURIComponent(client)}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export function fetchClientGroups(): Promise<ClientGroup[]> {
  return apiFetch<ClientGroup[]>("/api/client-groups");
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
