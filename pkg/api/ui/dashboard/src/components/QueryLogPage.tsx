import { useState, useEffect, useCallback, useRef } from "react";
import { Search, X, ChevronRight, ChevronsDownUp, RefreshCw } from "lucide-react";
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
import type { QueryLog, QueryFilter } from "@/lib/api";
import { fetchQueries } from "@/lib/api";

// ─── Helpers ────────────────────────────────────────────────────────

const RCODE_NAMES: Record<number, string> = {
  0: "NOERROR",
  1: "FORMERR",
  2: "SERVFAIL",
  3: "NXDOMAIN",
  4: "NOTIMP",
  5: "REFUSED",
};

function statusBadge(q: QueryLog) {
  if (q.blocked) {
    return <Badge className="bg-gh-red/20 text-gh-red border-gh-red/30">Blocked</Badge>;
  }
  if (q.cached) {
    return <Badge className="bg-gh-blue/20 text-gh-blue border-gh-blue/30">Cached</Badge>;
  }
  if (q.response_code === 2) {
    return <Badge className="bg-gh-red/20 text-gh-red border-gh-red/30">SERVFAIL</Badge>;
  }
  if (q.response_code === 5) {
    return <Badge className="bg-gh-peach/20 text-gh-peach border-gh-peach/30">REFUSED</Badge>;
  }
  if (q.response_code === 3) {
    return <Badge className="bg-gh-peach/20 text-gh-peach border-gh-peach/30">NXDOMAIN</Badge>;
  }
  if (q.response_code !== 0 && q.response_code != null) {
    const name = RCODE_NAMES[q.response_code] ?? `RCODE ${q.response_code}`;
    return <Badge className="bg-gh-peach/20 text-gh-peach border-gh-peach/30">{name}</Badge>;
  }
  return <Badge className="bg-gh-green/20 text-gh-green border-gh-green/30">Allowed</Badge>;
}

function formatTime(ts: string): string {
  const d = new Date(ts);
  return d.toLocaleTimeString("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function formatDate(ts: string): string {
  const d = new Date(ts);
  return d.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
  });
}

const STATUS_OPTIONS = [
  { value: "all", label: "All" },
  { value: "allowed", label: "Allowed" },
  { value: "blocked", label: "Blocked" },
  { value: "cached", label: "Cached" },
] as const;

const TIME_RANGES = [
  { value: "1h", label: "Last 1h" },
  { value: "6h", label: "Last 6h" },
  { value: "24h", label: "Last 24h" },
  { value: "168h", label: "Last 7d" },
] as const;

const AUTO_REFRESH_OPTIONS = [
  { value: 0, label: "Off" },
  { value: 5, label: "5s" },
  { value: 10, label: "10s" },
  { value: 30, label: "30s" },
  { value: 60, label: "60s" },
] as const;

// ─── Component ──────────────────────────────────────────────────────

export function QueryLogPage() {
  const [queries, setQueries] = useState<QueryLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [range, setRange] = useState("24h");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);

  // Multi-row expansion
  const [expandedIds, setExpandedIds] = useState<Set<number>>(new Set());

  // Auto-refresh
  const [refreshInterval, setRefreshInterval] = useState(5);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const toggleExpanded = (id: number) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const loadData = useCallback(async () => {
    try {
      const filter: QueryFilter = {
        limit: pageSize,
        offset: (page - 1) * pageSize,
        since: range,
      };
      if (statusFilter !== "all") filter.status = statusFilter;
      if (search.trim()) filter.domain = search.trim();

      const data = await fetchQueries(filter);
      setQueries(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load queries");
    } finally {
      setLoading(false);
    }
  }, [search, statusFilter, range, page, pageSize]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // Auto-refresh interval
  useEffect(() => {
    if (intervalRef.current) clearInterval(intervalRef.current);
    if (refreshInterval > 0) {
      intervalRef.current = setInterval(loadData, refreshInterval * 1000);
    }
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [loadData, refreshInterval]);

  // Reset page when filters change
  useEffect(() => {
    setPage(1);
  }, [search, statusFilter, range]);

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h2 className={T.pageTitle}>Query Log</h2>
        <p className={T.pageDescription}>DNS query history with filtering and search</p>
      </div>

      {error && (
        <div className="rounded-lg border border-gh-red/30 bg-gh-red/10 px-4 py-3 text-sm text-gh-red">
          {error}
        </div>
      )}

      {/* Filters */}
      <Card>
        <CardContent className="p-4">
          <div className="flex flex-wrap items-center gap-3">
            {/* Search */}
            <div className="relative flex-1 min-w-[200px]">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search domain..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9 font-data"
              />
              {search && (
                <button
                  onClick={() => setSearch("")}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </div>

            {/* Status filter */}
            <div className="flex rounded-lg bg-muted p-1">
              {STATUS_OPTIONS.map((opt) => (
                <button
                  key={opt.value}
                  onClick={() => setStatusFilter(opt.value)}
                  className={cn(
                    "rounded-md px-3 py-1 text-xs font-medium transition-all",
                    statusFilter === opt.value
                      ? "bg-background text-foreground shadow"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  {opt.label}
                </button>
              ))}
            </div>

            {/* Time range */}
            <Select value={range} onValueChange={setRange}>
              <SelectTrigger className="w-[120px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TIME_RANGES.map((r) => (
                  <SelectItem key={r.value} value={r.value}>
                    {r.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {/* Auto-refresh selector + manual refresh */}
            <div className="ml-auto flex items-center gap-2">
              <span className={T.formLabel}>Refresh</span>
              <div className="flex rounded-md border border-border">
                {AUTO_REFRESH_OPTIONS.map((opt, idx) => (
                  <button
                    key={opt.value}
                    onClick={() => setRefreshInterval(opt.value)}
                    className={cn(
                      "px-2.5 py-1 text-xs font-data transition-colors",
                      refreshInterval === opt.value
                        ? "bg-gh-green/20 text-gh-green"
                        : "text-muted-foreground hover:text-foreground hover:bg-muted",
                      idx !== 0 && "border-l border-border",
                    )}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
              <button
                onClick={() => loadData()}
                disabled={loading}
                className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50"
              >
                <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
                Refresh
              </button>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Table */}
      <Card>
        {loading && queries.length === 0 ? (
          <CardContent className="p-4 space-y-2">
            {Array.from({ length: 10 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </CardContent>
        ) : (
          <>
            {/* Collapse-all header */}
            {expandedIds.size > 0 && (
              <CardHeader className="py-2 px-4">
                <div className="flex items-center justify-end">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 text-xs text-muted-foreground hover:text-foreground"
                    onClick={() => setExpandedIds(new Set())}
                    title="Collapse all expanded rows"
                  >
                    <ChevronsDownUp className="h-3 w-3 mr-1" />
                    Collapse ({expandedIds.size})
                  </Button>
                </div>
              </CardHeader>
            )}
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[30px]"></TableHead>
                  <TableHead className="w-[100px]">Time</TableHead>
                  <TableHead className="w-[120px]">Client</TableHead>
                  <TableHead>Domain</TableHead>
                  <TableHead className="w-[60px]">Type</TableHead>
                  <TableHead className="w-[90px]">Status</TableHead>
                  <TableHead className="w-[80px] text-right">Latency</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {queries.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} className="text-center py-8">
                      <span className={T.mutedSm}>No queries found</span>
                    </TableCell>
                  </TableRow>
                ) : (
                  queries.map((q, i) => {
                    const isExpanded = expandedIds.has(i);
                    return (
                      <>
                        <TableRow
                          key={i}
                          className="cursor-pointer"
                          onClick={() => toggleExpanded(i)}
                        >
                          <TableCell className="w-6 px-2">
                            <ChevronRight
                              className={cn(
                                "h-3.5 w-3.5 text-muted-foreground transition-transform duration-150",
                                isExpanded && "rotate-90",
                              )}
                            />
                          </TableCell>
                          <TableCell>
                            <div className={T.tableCellMono}>
                              {formatTime(q.timestamp)}
                            </div>
                            <div className={cn(T.muted, "text-[10px]")}>
                              {formatDate(q.timestamp)}
                            </div>
                          </TableCell>
                          <TableCell className={T.tableCellMono}>
                            {q.client_ip}
                          </TableCell>
                          <TableCell className={cn(T.tableCellMono, "max-w-[300px] truncate")}>
                            {q.domain}
                          </TableCell>
                          <TableCell>
                            <Badge variant="outline" className="text-[10px]">
                              {q.query_type}
                            </Badge>
                          </TableCell>
                          <TableCell>{statusBadge(q)}</TableCell>
                          <TableCell className={T.tableCellNumeric}>
                            {q.response_time_ms.toFixed(1)}ms
                          </TableCell>
                        </TableRow>
                        {isExpanded && (
                          <TableRow key={`${i}-detail`}>
                            <TableCell colSpan={7} className="bg-gh-950/50 px-6 py-4">
                              <QueryDetail query={q} />
                            </TableCell>
                          </TableRow>
                        )}
                      </>
                    );
                  })
                )}
              </TableBody>
            </Table>

            <TablePagination
              page={page}
              totalPages={Math.max(1, Math.ceil(queries.length > 0 ? 1000 : 0 / pageSize))}
              pageSize={pageSize}
              pageSizeOptions={[25, 50, 100]}
              hasPrev={page > 1}
              hasNext={queries.length === pageSize}
              onPageChange={setPage}
              onPageSizeChange={(s) => { setPageSize(s); setPage(1); }}
            />
          </>
        )}
      </Card>
    </div>
  );
}

// ─── Query Detail ───────────────────────────────────────────────────

function QueryDetail({ query }: { query: QueryLog }) {
  const rcodeName = RCODE_NAMES[query.response_code] ?? `RCODE ${query.response_code}`;

  return (
    <div className="grid gap-4 md:grid-cols-2 text-xs">
      <div className="space-y-2">
        <DetailRow label="Domain" value={query.domain} mono />
        <DetailRow label="Query Type" value={query.query_type} />
        <DetailRow label="Client" value={query.client_ip} mono />
        <DetailRow
          label="Response Code"
          value={`${query.response_code} (${rcodeName})`}
          className={query.response_code !== 0 ? "text-gh-red" : ""}
        />
      </div>
      <div className="space-y-2">
        <DetailRow label="Upstream" value={query.upstream || "N/A"} mono />
        <DetailRow label="Response Time" value={`${query.response_time_ms.toFixed(2)}ms`} />
        <DetailRow label="Upstream Time" value={`${query.upstream_response_ms.toFixed(2)}ms`} />
        {query.blocked && <DetailRow label="Status" value="Blocked" className="text-gh-red" />}
        {query.cached && <DetailRow label="Status" value="Cached" className="text-gh-blue" />}
        <DetailRow label="DNSSEC" value={query.dnssec_validated ? "Validated" : "No"} className={query.dnssec_validated ? "text-gh-green" : ""} />
        {query.upstream_error && (
          <DetailRow label="Upstream Error" value={query.upstream_error} className="text-gh-red" />
        )}
      </div>

      {/* Block Trace */}
      {query.block_trace && query.block_trace.length > 0 && (
        <div className="md:col-span-2 mt-2">
          <div className={cn(T.sectionLabel, "mb-2")}>Decision Trace</div>
          <div className="space-y-1">
            {query.block_trace.map((entry, i) => (
              <div
                key={i}
                className="flex items-center gap-2 rounded-md bg-gh-800 px-3 py-1.5 font-data text-xs"
              >
                <Badge variant="outline" className="text-[10px]">
                  {entry.stage}
                </Badge>
                <Badge className={
                  entry.action === "block" || entry.action === "BLOCK"
                    ? "bg-gh-red/20 text-gh-red border-gh-red/30 text-[10px]"
                    : "bg-gh-green/20 text-gh-green border-gh-green/30 text-[10px]"
                }>
                  {entry.action}
                </Badge>
                {entry.rule && <span className="text-muted-foreground">{entry.rule}</span>}
                {entry.source && <span className="text-gh-red">[{entry.source}]</span>}
                {entry.detail && <span className="text-muted-foreground/60">{entry.detail}</span>}
              </div>
            ))}
          </div>
        </div>
      )}
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
