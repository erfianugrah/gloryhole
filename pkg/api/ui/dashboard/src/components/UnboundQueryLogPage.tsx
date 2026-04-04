import { useState, useEffect, useCallback, useRef } from "react";
import { Search, X, ChevronRight, RefreshCw } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";
import { T } from "@/lib/typography";
import { TablePagination } from "./TablePagination";
import type { UnboundQueryLog, UnboundQueryFilter, UnboundQueryStatsResponse } from "@/lib/api";
import { fetchUnboundQueries, fetchUnboundQueryStats } from "@/lib/api";

// ─── Helpers ────────────────────────────────────────────────────────

function messageTypeBadge(type: string) {
  switch (type) {
    case "CLIENT_QUERY":
      return <Badge className="bg-gh-blue/20 text-gh-blue border-gh-blue/30 text-[10px]">CQ</Badge>;
    case "CLIENT_RESPONSE":
      return <Badge className="bg-gh-green/20 text-gh-green border-gh-green/30 text-[10px]">CR</Badge>;
    case "RESOLVER_QUERY":
      return <Badge className="bg-gh-peach/20 text-gh-peach border-gh-peach/30 text-[10px]">RQ</Badge>;
    case "RESOLVER_RESPONSE":
      return <Badge className="bg-gh-mauve/20 text-gh-mauve border-gh-mauve/30 text-[10px]">RR</Badge>;
    default:
      return <Badge variant="outline" className="text-[10px]">{type}</Badge>;
  }
}

function rcodeBadge(rcode?: string) {
  if (!rcode) return null;
  if (rcode === "NOERROR") {
    return <Badge className="bg-gh-green/20 text-gh-green border-gh-green/30 text-[10px]">{rcode}</Badge>;
  }
  if (rcode === "NXDOMAIN") {
    return <Badge className="bg-gh-peach/20 text-gh-peach border-gh-peach/30 text-[10px]">{rcode}</Badge>;
  }
  if (rcode === "SERVFAIL") {
    return <Badge className="bg-gh-red/20 text-gh-red border-gh-red/30 text-[10px]">{rcode}</Badge>;
  }
  return <Badge variant="outline" className="text-[10px]">{rcode}</Badge>;
}

function sourceBadge(cached: boolean) {
  if (cached) {
    return <Badge className="bg-gh-green/20 text-gh-green border-gh-green/30 text-[10px]">Cache</Badge>;
  }
  return <Badge className="bg-gh-peach/20 text-gh-peach border-gh-peach/30 text-[10px]">Recursive</Badge>;
}

function formatTime(ts: string) {
  try {
    const d = new Date(ts);
    return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  } catch {
    return ts;
  }
}

function formatDate(ts: string) {
  try {
    const d = new Date(ts);
    return d.toLocaleDateString([], { month: "short", day: "numeric" });
  } catch {
    return "";
  }
}

// ─── Stats Card ─────────────────────────────────────────────────────

function StatsBar({ stats }: { stats: UnboundQueryStatsResponse | null }) {
  if (!stats) return null;

  return (
    <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-6 gap-3 mb-4">
      <StatItem label="Total Queries" value={stats.total_queries.toLocaleString()} />
      <StatItem label="Cache Hits" value={stats.cache_hits.toLocaleString()} />
      <StatItem label="Cache Hit Rate" value={`${stats.cache_hit_rate.toFixed(1)}%`} />
      <StatItem label="Recursive" value={stats.recursive_queries.toLocaleString()} />
      <StatItem label="Avg Recursive" value={`${stats.avg_recursive_ms.toFixed(1)}ms`} />
      <StatItem label="DNSSEC Validated" value={`${stats.dnssec_validated_pct.toFixed(1)}%`} />
    </div>
  );
}

function StatItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-gh-800 p-3">
      <div className={T.statLabel}>{label}</div>
      <div className="text-lg font-bold tabular-nums font-data">{value}</div>
    </div>
  );
}

// ─── Main Component ─────────────────────────────────────────────────

export function UnboundQueryLogPage() {
  const [queries, setQueries] = useState<UnboundQueryLog[]>([]);
  const [stats, setStats] = useState<UnboundQueryStatsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [msgTypeFilter, setMsgTypeFilter] = useState("CLIENT_RESPONSE");
  const [timeRange, setTimeRange] = useState("24h");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [refreshInterval, setRefreshInterval] = useState(0);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const loadData = useCallback(async () => {
    try {
      const filter: UnboundQueryFilter = {
        limit: pageSize,
        offset: (page - 1) * pageSize,
        since: timeRange,
        message_type: msgTypeFilter === "ALL" ? undefined : msgTypeFilter,
        domain: search || undefined,
      };

      const [q, s] = await Promise.all([
        fetchUnboundQueries(filter),
        fetchUnboundQueryStats(),
      ]);
      setQueries(q);
      setStats(s);
    } catch (err) {
      console.error("Failed to fetch Unbound queries:", err);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, timeRange, msgTypeFilter, search]);

  useEffect(() => { loadData(); }, [loadData]);

  useEffect(() => {
    if (timerRef.current) clearInterval(timerRef.current);
    if (refreshInterval > 0) {
      timerRef.current = setInterval(loadData, refreshInterval * 1000);
    }
    return () => { if (timerRef.current) clearInterval(timerRef.current); };
  }, [refreshInterval, loadData]);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className={T.pageTitle}>Unbound Query Log</h1>
          <p className={T.pageDescription}>
            DNS resolution events captured via dnstap from the Unbound recursive resolver.
          </p>
        </div>
      </div>

      <StatsBar stats={stats} />

      <Card>
        <CardHeader className="pb-3">
          <div className="flex flex-wrap items-center gap-2">
            {/* Search */}
            <div className="relative flex-1 min-w-[200px]">
              <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
              <Input
                placeholder="Search domain..."
                value={search}
                onChange={(e) => { setSearch(e.target.value); setPage(1); }}
                className="pl-8 h-8 text-xs"
              />
              {search && (
                <button onClick={() => { setSearch(""); setPage(1); }} className="absolute right-2.5 top-2.5">
                  <X className="h-3.5 w-3.5 text-muted-foreground hover:text-foreground" />
                </button>
              )}
            </div>

            {/* Message type filter */}
            <Select value={msgTypeFilter} onValueChange={(v) => { setMsgTypeFilter(v); setPage(1); }}>
              <SelectTrigger className="h-8 w-[160px] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="CLIENT_RESPONSE">Client Response</SelectItem>
                <SelectItem value="CLIENT_QUERY">Client Query</SelectItem>
                <SelectItem value="RESOLVER_QUERY">Resolver Query</SelectItem>
                <SelectItem value="RESOLVER_RESPONSE">Resolver Response</SelectItem>
                <SelectItem value="ALL">All Types</SelectItem>
              </SelectContent>
            </Select>

            {/* Time range */}
            <Select value={timeRange} onValueChange={(v) => { setTimeRange(v); setPage(1); }}>
              <SelectTrigger className="h-8 w-[90px] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="1h">1h</SelectItem>
                <SelectItem value="6h">6h</SelectItem>
                <SelectItem value="24h">24h</SelectItem>
                <SelectItem value="7d">7d</SelectItem>
              </SelectContent>
            </Select>

            {/* Auto-refresh */}
            <div className="flex items-center gap-1">
              {[0, 5, 10, 30].map((s) => (
                <Button
                  key={s}
                  variant={refreshInterval === s ? "default" : "outline"}
                  size="sm"
                  className="h-8 px-2 text-xs"
                  onClick={() => setRefreshInterval(s)}
                >
                  {s === 0 ? "Off" : `${s}s`}
                </Button>
              ))}
            </div>

            <Button variant="outline" size="sm" className="h-8" onClick={loadData}>
              <RefreshCw className="h-3.5 w-3.5" />
            </Button>
          </div>
        </CardHeader>

        <CardContent className="p-0">
          {loading ? (
            <div className="p-4 space-y-2">
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : queries.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground text-sm">
              No Unbound queries found. Queries will appear here when Unbound processes DNS requests via dnstap.
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[30px]" />
                    <TableHead className="w-[100px]">Time</TableHead>
                    <TableHead>Domain</TableHead>
                    <TableHead className="w-[50px]">Type</TableHead>
                    <TableHead className="w-[50px]">Msg</TableHead>
                    <TableHead className="w-[80px]">Result</TableHead>
                    <TableHead className="w-[80px]">Source</TableHead>
                    <TableHead className="w-[60px]">DNSSEC</TableHead>
                    <TableHead className="w-[80px] text-right">Duration</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {queries.map((q) => (
                    <QueryRow
                      key={q.id}
                      query={q}
                      expanded={expandedId === q.id}
                      onToggle={() => setExpandedId(expandedId === q.id ? null : q.id)}
                    />
                  ))}
                </TableBody>
              </Table>
              <TablePagination
                page={page}
                pageSize={pageSize}
                pageSizeOptions={[25, 50, 100]}
                hasPrev={page > 1}
                hasNext={queries.length === pageSize}
                onPageChange={setPage}
                onPageSizeChange={(s) => { setPageSize(s); setPage(1); }}
              />
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// ─── Row Component ──────────────────────────────────────────────────

function QueryRow({
  query,
  expanded,
  onToggle,
}: {
  query: UnboundQueryLog;
  expanded: boolean;
  onToggle: () => void;
}) {
  const isResponse = query.message_type.includes("RESPONSE");

  return (
    <>
      <TableRow
        className="cursor-pointer hover:bg-gh-800/50 transition-colors"
        onClick={onToggle}
      >
        <TableCell className="py-2 px-2">
          <ChevronRight
            className={cn(
              "h-3.5 w-3.5 text-muted-foreground transition-transform",
              expanded && "rotate-90"
            )}
          />
        </TableCell>
        <TableCell className="py-2">
          <div className={T.tableCellMono}>{formatTime(query.timestamp)}</div>
          <div className={T.muted}>{formatDate(query.timestamp)}</div>
        </TableCell>
        <TableCell className="py-2">
          <span className={cn(T.tableCellMono, "truncate block max-w-[300px]")}>
            {query.domain}
          </span>
        </TableCell>
        <TableCell className="py-2">
          <Badge variant="outline" className="text-[10px]">{query.query_type}</Badge>
        </TableCell>
        <TableCell className="py-2">
          {messageTypeBadge(query.message_type)}
        </TableCell>
        <TableCell className="py-2">
          {isResponse ? rcodeBadge(query.response_code) : <span className={T.muted}>-</span>}
        </TableCell>
        <TableCell className="py-2">
          {isResponse ? sourceBadge(query.cached_in_unbound) : <span className={T.muted}>-</span>}
        </TableCell>
        <TableCell className="py-2">
          {isResponse ? (
            query.dnssec_validated
              ? <span className="text-gh-green text-xs">Yes</span>
              : <span className="text-muted-foreground text-xs">No</span>
          ) : (
            <span className={T.muted}>-</span>
          )}
        </TableCell>
        <TableCell className={cn(T.tableCellNumeric, "py-2")}>
          {isResponse && query.duration_ms ? `${query.duration_ms.toFixed(2)}ms` : "-"}
        </TableCell>
      </TableRow>

      {expanded && (
        <TableRow>
          <TableCell colSpan={9} className="bg-gh-900/50 px-6 py-4">
            <QueryDetail query={query} />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

// ─── Detail View ────────────────────────────────────────────────────

function QueryDetail({ query }: { query: UnboundQueryLog }) {
  return (
    <div className="grid gap-4 md:grid-cols-2 text-xs">
      <div className="space-y-2">
        <DetailRow label="Domain" value={query.domain} mono />
        <DetailRow label="Query Type" value={query.query_type} />
        <DetailRow label="Message Type" value={query.message_type} />
        <DetailRow label="Client IP" value={query.client_ip} mono />
        {query.server_ip && (
          <DetailRow label="Server IP" value={query.server_ip} mono />
        )}
      </div>
      <div className="space-y-2">
        {query.response_code && (
          <DetailRow label="Response Code" value={query.response_code} />
        )}
        {query.duration_ms != null && query.duration_ms > 0 && (
          <DetailRow label="Duration" value={`${query.duration_ms.toFixed(3)}ms`} />
        )}
        {query.answer_count != null && query.answer_count > 0 && (
          <DetailRow label="Answer Count" value={String(query.answer_count)} />
        )}
        {query.response_size != null && query.response_size > 0 && (
          <DetailRow label="Response Size" value={`${query.response_size} bytes`} />
        )}
        <DetailRow
          label="Source"
          value={query.cached_in_unbound ? "Unbound Cache" : "Recursive Resolution"}
          className={query.cached_in_unbound ? "text-gh-green" : "text-gh-peach"}
        />
        <DetailRow
          label="DNSSEC"
          value={query.dnssec_validated ? "Validated" : "No"}
          className={query.dnssec_validated ? "text-gh-green" : ""}
        />
      </div>
    </div>
  );
}

function DetailRow({
  label,
  value,
  mono,
  className,
}: {
  label: string;
  value: string;
  mono?: boolean;
  className?: string;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className={T.formLabel}>{label}</span>
      <span className={cn(mono ? "font-data" : "", className)}>{value}</span>
    </div>
  );
}
