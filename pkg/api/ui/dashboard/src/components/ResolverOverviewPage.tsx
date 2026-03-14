import { useState, useEffect, useCallback } from "react";
import { RefreshCw, Trash2, Server, Shield, Gauge, Clock } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { T } from "@/lib/typography";
import type { UnboundStatus, UnboundStats } from "@/lib/api";
import {
  fetchUnboundStatus,
  fetchUnboundStats,
  reloadUnbound,
  flushUnboundCache,
} from "@/lib/api";

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatBytes(bytes: number): string {
  if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`;
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
}

function stateColor(state: string): string {
  switch (state) {
    case "running": return "text-gh-green";
    case "degraded": return "text-gh-yellow";
    case "failed": return "text-destructive";
    default: return "text-muted-foreground";
  }
}

export function ResolverOverviewPage() {
  const [status, setStatus] = useState<UnboundStatus | null>(null);
  const [stats, setStats] = useState<UnboundStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [reloading, setReloading] = useState(false);
  const [flushing, setFlushing] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const s = await fetchUnboundStatus();
      setStatus(s);
      if (s.state === "running") {
        const st = await fetchUnboundStats();
        setStats(st);
      }
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load resolver status");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 10_000);
    return () => clearInterval(interval);
  }, [loadData]);

  function showSuccess(msg: string) {
    setSuccess(msg);
    setTimeout(() => setSuccess(null), 3000);
  }

  async function handleReload() {
    setReloading(true);
    try {
      await reloadUnbound();
      showSuccess("Resolver config reloaded");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Reload failed");
    } finally {
      setReloading(false);
    }
  }

  async function handleFlush() {
    setFlushing(true);
    try {
      await flushUnboundCache();
      showSuccess("Resolver cache flushed");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Flush failed");
    } finally {
      setFlushing(false);
    }
  }

  if (!status?.enabled) {
    return (
      <div className="space-y-6">
        <div>
          <h2 className={T.pageTitle}>Resolver</h2>
          <p className={T.pageDescription}>Unbound recursive resolver</p>
        </div>
        <Card>
          <CardContent className="py-12 text-center">
            <Server className="h-10 w-10 mx-auto text-muted-foreground mb-3" />
            <p className="text-sm text-muted-foreground">
              The Unbound resolver is not enabled. Set{" "}
              <code className="text-xs font-data bg-muted px-1 py-0.5 rounded">
                unbound.enabled: true
              </code>{" "}
              in your config.yml to enable recursive resolution and DNSSEC.
            </p>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 rounded-xl" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className={T.pageTitle}>Resolver</h2>
          <p className={T.pageDescription}>Unbound recursive resolver with DNSSEC validation</p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={handleFlush}
            disabled={flushing}
            aria-label="Flush resolver cache"
          >
            <Trash2 className="h-3.5 w-3.5 mr-1" />
            {flushing ? "Flushing..." : "Flush Cache"}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={handleReload}
            disabled={reloading}
            aria-label="Reload resolver config"
          >
            <RefreshCw className={cn("h-3.5 w-3.5 mr-1", reloading && "animate-spin")} />
            {reloading ? "Reloading..." : "Reload"}
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}
      {success && (
        <div className="rounded-lg border border-gh-green/30 bg-gh-green/10 px-4 py-3 text-sm text-gh-green">
          {success}
        </div>
      )}

      {/* Status cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
              <Server className="h-3.5 w-3.5" />
              Status
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className={cn("text-2xl font-bold capitalize", stateColor(status.state))}>
              {status.state}
            </div>
            {status.listen_addr && (
              <p className="text-xs text-muted-foreground font-data mt-1">
                {status.listen_addr}
              </p>
            )}
            {status.error && (
              <p className="text-xs text-destructive mt-1">{status.error}</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
              <Shield className="h-3.5 w-3.5" />
              DNSSEC
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-gh-green">Active</div>
            <p className="text-xs text-muted-foreground mt-1">
              validator + iterator
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
              <Gauge className="h-3.5 w-3.5" />
              Cache Hit Rate
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {stats ? `${stats.cache_hit_rate.toFixed(1)}%` : "N/A"}
            </div>
            {stats && (
              <p className="text-xs text-muted-foreground font-data mt-1">
                {stats.cache_hits.toLocaleString()} hits / {stats.total_queries.toLocaleString()} queries
              </p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
              <Clock className="h-3.5 w-3.5" />
              Uptime
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {stats ? formatUptime(stats.uptime_seconds) : "N/A"}
            </div>
            {stats && (
              <p className="text-xs text-muted-foreground font-data mt-1">
                Avg recursion: {stats.avg_recursion_ms.toFixed(1)}ms
              </p>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Detail cards */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>Cache</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <DetailRow label="Message cache" value={stats.msg_cache_count.toLocaleString()} />
              <DetailRow label="RRSet cache" value={stats.rrset_cache_count.toLocaleString()} />
              <DetailRow label="Memory" value={formatBytes(stats.mem_total_bytes)} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>Query Types</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              {Object.entries(stats.query_types)
                .sort(([, a], [, b]) => b - a)
                .slice(0, 8)
                .map(([type, count]) => (
                  <div key={type} className="flex items-center justify-between">
                    <Badge variant="outline" className="text-[10px] font-data">{type}</Badge>
                    <span className="text-sm font-data">{count.toLocaleString()}</span>
                  </div>
                ))}
              {Object.keys(stats.query_types).length === 0 && (
                <p className={T.mutedSm}>No queries yet</p>
              )}
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <span className={T.mutedSm}>{label}</span>
      <span className="text-sm font-data">{value}</span>
    </div>
  );
}
