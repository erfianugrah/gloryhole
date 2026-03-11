import { useState, useEffect, useCallback } from "react";
import {
  AreaChart,
  Area,
  PieChart,
  Pie,
  Cell,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from "recharts";
import {
  Activity,
  ShieldBan,
  Database,
  Timer,
  Cpu,
  HardDrive,
  Thermometer,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn, STATUS_COLORS, CHART_PALETTE, CHART_TOOLTIP_STYLE } from "@/lib/utils";
import { T } from "@/lib/typography";
import type {
  Stats,
  TimeseriesBucket,
  QueryTypeCount,
  TopDomain,
} from "@/lib/api";
import {
  fetchStats,
  fetchTimeseries,
  fetchQueryTypes,
  fetchTopDomains,
} from "@/lib/api";

// ─── Helpers ────────────────────────────────────────────────────────

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

function formatPercent(n: number): string {
  return `${n.toFixed(1)}%`;
}

function formatMs(n: number): string {
  if (n < 1) return "<1ms";
  return `${n.toFixed(1)}ms`;
}

function formatBytes(bytes: number): string {
  if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`;
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(0)} MB`;
  return `${(bytes / 1_024).toFixed(0)} KB`;
}

function formatTimeShort(ts: string): string {
  const d = new Date(ts);
  return d.toLocaleTimeString("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

const TIME_RANGES = [
  { value: "1h", label: "Last 1h", buckets: 12 },
  { value: "6h", label: "Last 6h", buckets: 12 },
  { value: "24h", label: "Last 24h", buckets: 24 },
  { value: "168h", label: "Last 7d", buckets: 28 },
] as const;

// ─── Component ──────────────────────────────────────────────────────

export function DashboardOverview() {
  const [range, setRange] = useState("24h");
  const [stats, setStats] = useState<Stats | null>(null);
  const [timeseries, setTimeseries] = useState<TimeseriesBucket[]>([]);
  const [queryTypes, setQueryTypes] = useState<QueryTypeCount[]>([]);
  const [topAllowed, setTopAllowed] = useState<TopDomain[]>([]);
  const [topBlocked, setTopBlocked] = useState<TopDomain[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const buckets = TIME_RANGES.find((r) => r.value === range)?.buckets ?? 24;

  const loadData = useCallback(async () => {
    try {
      const [s, ts, qt, ta, tb] = await Promise.all([
        fetchStats(range),
        fetchTimeseries(range, buckets),
        fetchQueryTypes(range),
        fetchTopDomains(10, false, range),
        fetchTopDomains(10, true, range),
      ]);
      setStats(s);
      setTimeseries(ts);
      setQueryTypes(qt);
      setTopAllowed(ta);
      setTopBlocked(tb);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load data");
    } finally {
      setLoading(false);
    }
  }, [range, buckets]);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 10_000);
    return () => clearInterval(interval);
  }, [loadData]);

  if (loading && !stats) {
    return <DashboardSkeleton />;
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className={T.pageTitle}>Dashboard</h2>
          <p className={T.pageDescription}>DNS sinkhole overview</p>
        </div>
        <Select value={range} onValueChange={setRange}>
          <SelectTrigger className="w-[140px]">
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
      </div>

      {error && (
        <div className="rounded-lg border border-gh-pink/30 bg-gh-pink/10 px-4 py-3 text-sm text-gh-pink">
          {error}
        </div>
      )}

      {/* Stat Cards */}
      {stats && (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <StatCard
            icon={<Activity className="h-4 w-4" />}
            label="Total Queries"
            value={formatNumber(stats.total_queries)}
            color="text-gh-green"
          />
          <StatCard
            icon={<ShieldBan className="h-4 w-4" />}
            label="Blocked"
            value={formatNumber(stats.blocked_queries)}
            sub={formatPercent(stats.block_rate)}
            color="text-gh-pink"
          />
          <StatCard
            icon={<Database className="h-4 w-4" />}
            label="Cached"
            value={formatNumber(stats.cached_queries)}
            sub={formatPercent(stats.cache_hit_rate)}
            color="text-gh-blue"
          />
          <StatCard
            icon={<Timer className="h-4 w-4" />}
            label="Avg Response"
            value={formatMs(stats.avg_response_ms)}
            color="text-gh-yellow"
          />
        </div>
      )}

      {/* System Metrics */}
      {stats && (
        <div className="grid gap-4 md:grid-cols-3">
          <StatCard
            icon={<Cpu className="h-4 w-4" />}
            label="CPU"
            value={formatPercent(stats.cpu_usage_percent)}
            color="text-gh-cyan"
          />
          <StatCard
            icon={<HardDrive className="h-4 w-4" />}
            label="Memory"
            value={formatBytes(stats.memory_usage_bytes)}
            sub={`${formatPercent(stats.memory_usage_percent)} of ${formatBytes(stats.memory_total_bytes)}`}
            color="text-gh-orange"
          />
          {stats.temperature_available && (
            <StatCard
              icon={<Thermometer className="h-4 w-4" />}
              label="Temperature"
              value={`${stats.temperature_celsius.toFixed(1)}°C`}
              color={
                stats.temperature_celsius > 70
                  ? "text-gh-pink"
                  : stats.temperature_celsius > 50
                    ? "text-gh-yellow"
                    : "text-gh-green"
              }
            />
          )}
        </div>
      )}

      {/* Traffic Chart */}
      {timeseries.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className={T.cardTitle}>Traffic Over Time</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="h-[260px]">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={timeseries}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#2a2f52" />
                  <XAxis
                    dataKey="timestamp"
                    tickFormatter={formatTimeShort}
                    tick={{ fontSize: T.chartAxisTick, fill: "#8888aa" }}
                    stroke="#2a2f52"
                  />
                  <YAxis
                    tick={{ fontSize: T.chartAxisTick, fill: "#8888aa" }}
                    stroke="#2a2f52"
                  />
                  <Tooltip
                    {...CHART_TOOLTIP_STYLE}
                    labelFormatter={(v) => formatTimeShort(String(v))}
                  />
                  <Area
                    type="monotone"
                    dataKey="allowed"
                    stackId="1"
                    stroke={STATUS_COLORS.allowed}
                    fill={STATUS_COLORS.allowed}
                    fillOpacity={0.3}
                  />
                  <Area
                    type="monotone"
                    dataKey="cached"
                    stackId="1"
                    stroke={STATUS_COLORS.cached}
                    fill={STATUS_COLORS.cached}
                    fillOpacity={0.3}
                  />
                  <Area
                    type="monotone"
                    dataKey="blocked"
                    stackId="1"
                    stroke={STATUS_COLORS.blocked}
                    fill={STATUS_COLORS.blocked}
                    fillOpacity={0.3}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
            <div className="mt-2 flex items-center justify-center gap-4">
              <LegendItem color={STATUS_COLORS.allowed} label="Allowed" />
              <LegendItem color={STATUS_COLORS.cached} label="Cached" />
              <LegendItem color={STATUS_COLORS.blocked} label="Blocked" />
            </div>
          </CardContent>
        </Card>
      )}

      {/* Distribution Row */}
      <div className="grid gap-4 lg:grid-cols-3">
        {/* Query Types Pie */}
        {queryTypes.length > 0 && (
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className={T.cardTitle}>Query Types</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="h-[200px]">
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie
                      data={queryTypes}
                      dataKey="count"
                      nameKey="type"
                      cx="50%"
                      cy="50%"
                      innerRadius={50}
                      outerRadius={80}
                      paddingAngle={2}
                    >
                      {queryTypes.map((_, i) => (
                        <Cell
                          key={i}
                          fill={CHART_PALETTE[i % CHART_PALETTE.length]}
                        />
                      ))}
                    </Pie>
                    <Tooltip {...CHART_TOOLTIP_STYLE} />
                  </PieChart>
                </ResponsiveContainer>
              </div>
              <div className="mt-2 flex flex-wrap items-center justify-center gap-3">
                {queryTypes.slice(0, 6).map((qt, i) => (
                  <LegendItem
                    key={qt.type}
                    color={CHART_PALETTE[i % CHART_PALETTE.length]}
                    label={`${qt.type} (${qt.percentage.toFixed(0)}%)`}
                  />
                ))}
              </div>
            </CardContent>
          </Card>
        )}

        {/* Top Allowed */}
        {topAllowed.length > 0 && (
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className={T.cardTitle}>Top Allowed</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="h-[200px]">
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart
                    data={topAllowed.slice(0, 8)}
                    layout="vertical"
                    margin={{ left: 0, right: 16 }}
                  >
                    <CartesianGrid
                      strokeDasharray="3 3"
                      stroke="#2a2f52"
                      horizontal={false}
                    />
                    <XAxis
                      type="number"
                      tick={{ fontSize: T.chartAxisTick, fill: "#8888aa" }}
                      stroke="#2a2f52"
                    />
                    <YAxis
                      type="category"
                      dataKey="domain"
                      width={120}
                      tick={{ fontSize: 9, fill: "#8888aa" }}
                      stroke="#2a2f52"
                    />
                    <Tooltip {...CHART_TOOLTIP_STYLE} />
                    <Bar dataKey="query_count" fill={STATUS_COLORS.allowed} radius={[0, 4, 4, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Top Blocked */}
        {topBlocked.length > 0 && (
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className={T.cardTitle}>Top Blocked</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="h-[200px]">
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart
                    data={topBlocked.slice(0, 8)}
                    layout="vertical"
                    margin={{ left: 0, right: 16 }}
                  >
                    <CartesianGrid
                      strokeDasharray="3 3"
                      stroke="#2a2f52"
                      horizontal={false}
                    />
                    <XAxis
                      type="number"
                      tick={{ fontSize: T.chartAxisTick, fill: "#8888aa" }}
                      stroke="#2a2f52"
                    />
                    <YAxis
                      type="category"
                      dataKey="domain"
                      width={120}
                      tick={{ fontSize: 9, fill: "#8888aa" }}
                      stroke="#2a2f52"
                    />
                    <Tooltip {...CHART_TOOLTIP_STYLE} />
                    <Bar dataKey="query_count" fill={STATUS_COLORS.blocked} radius={[0, 4, 4, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}

// ─── Sub-components ─────────────────────────────────────────────────

function StatCard({
  icon,
  label,
  value,
  sub,
  color = "text-foreground",
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  sub?: string;
  color?: string;
}) {
  return (
    <Card className="animate-fade-in-up">
      <CardContent className="p-4">
        <div className="flex items-center justify-between">
          <span className={T.statLabelUpper}>{label}</span>
          <span className="text-muted-foreground">{icon}</span>
        </div>
        <div className={cn(T.statValue, color, "mt-2")}>{value}</div>
        {sub && <div className={cn(T.muted, "mt-1")}>{sub}</div>}
      </CardContent>
    </Card>
  );
}

function LegendItem({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-1.5">
      <span
        className="h-2.5 w-2.5 rounded-full"
        style={{ backgroundColor: color }}
      />
      <span className={T.muted}>{label}</span>
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-4 w-48" />
        </div>
        <Skeleton className="h-9 w-[140px]" />
      </div>
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-24 rounded-xl" />
        ))}
      </div>
      <Skeleton className="h-[320px] rounded-xl" />
      <div className="grid gap-4 lg:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-[280px] rounded-xl" />
        ))}
      </div>
    </div>
  );
}
