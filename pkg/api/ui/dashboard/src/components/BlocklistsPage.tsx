import { useState, useEffect, useCallback } from "react";
import { RefreshCw, Search, ShieldCheck, ShieldBan } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
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
import type { BlocklistInfo, FeatureState } from "@/lib/api";
import {
  fetchBlocklists,
  reloadBlocklists,
  checkDomain,
  fetchFeatures,
  enableBlocklist,
  disableBlocklist,
} from "@/lib/api";

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

export function BlocklistsPage() {
  const [info, setInfo] = useState<BlocklistInfo | null>(null);
  const [features, setFeatures] = useState<FeatureState | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reloading, setReloading] = useState(false);
  const [checkQuery, setCheckQuery] = useState("");
  const [checkResult, setCheckResult] = useState<{ blocked: boolean; source?: string } | null>(null);
  const [checking, setChecking] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [bl, ft] = await Promise.all([fetchBlocklists(), fetchFeatures()]);
      setInfo(bl);
      setFeatures(ft);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load blocklists");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  async function handleReload() {
    setReloading(true);
    try {
      await reloadBlocklists();
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Reload failed");
    } finally {
      setReloading(false);
    }
  }

  async function handleCheck() {
    if (!checkQuery.trim()) return;
    setChecking(true);
    try {
      const result = await checkDomain(checkQuery.trim());
      setCheckResult(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Check failed");
    } finally {
      setChecking(false);
    }
  }

  async function handleToggleBlocklist() {
    try {
      if (features?.blocklist_enabled) {
        await disableBlocklist();
      } else {
        await enableBlocklist();
      }
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Toggle failed");
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <div className="grid gap-4 md:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-24 rounded-xl" />)}
        </div>
        <Skeleton className="h-[300px] rounded-xl" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className={T.pageTitle}>Blocklists</h2>
          <p className={T.pageDescription}>Domain blocklist sources and status</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <Switch
              checked={features?.blocklist_enabled ?? true}
              onCheckedChange={handleToggleBlocklist}
            />
            <Label className="text-xs">
              {features?.blocklist_enabled ? "Enabled" : "Disabled"}
            </Label>
          </div>
          <Button size="sm" variant="outline" onClick={handleReload} disabled={reloading}>
            <RefreshCw className={cn("h-3.5 w-3.5 mr-1", reloading && "animate-spin")} />
            {reloading ? "Reloading..." : "Reload"}
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-gh-pink/30 bg-gh-pink/10 px-4 py-3 text-sm text-gh-pink">{error}</div>
      )}

      {/* Stats */}
      {info && (
        <div className="grid gap-4 md:grid-cols-3">
          <Card className="animate-fade-in-up">
            <CardContent className="p-4">
              <div className={T.statLabelUpper}>Total Domains Blocked</div>
              <div className={cn(T.statValue, "text-gh-pink mt-2")}>{formatNumber(info.total_domains)}</div>
            </CardContent>
          </Card>
          <Card className="animate-fade-in-up">
            <CardContent className="p-4">
              <div className={T.statLabelUpper}>Sources</div>
              <div className={cn(T.statValue, "mt-2")}>{info.sources.length}</div>
            </CardContent>
          </Card>
          <Card className="animate-fade-in-up">
            <CardContent className="p-4">
              <div className={T.statLabelUpper}>Last Refresh</div>
              <div className={cn(T.statValueSm, "mt-2 font-data")}>
                {info.last_refresh ? new Date(info.last_refresh).toLocaleString() : "Never"}
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Domain Check */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className={T.cardTitle}>Check Domain</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="example.com"
                value={checkQuery}
                onChange={(e) => { setCheckQuery(e.target.value); setCheckResult(null); }}
                onKeyDown={(e) => e.key === "Enter" && handleCheck()}
                className="pl-9 font-data"
              />
            </div>
            <Button onClick={handleCheck} disabled={checking || !checkQuery.trim()}>
              {checking ? "Checking..." : "Check"}
            </Button>
          </div>
          {checkResult && (
            <div className={cn(
              "mt-3 flex items-center gap-2 rounded-md px-3 py-2 text-sm",
              checkResult.blocked
                ? "border border-gh-pink/30 bg-gh-pink/10 text-gh-pink"
                : "border border-gh-green/30 bg-gh-green/10 text-gh-green"
            )}>
              {checkResult.blocked ? <ShieldBan className="h-4 w-4" /> : <ShieldCheck className="h-4 w-4" />}
              <span className="font-data">{checkQuery}</span>
              <span>is {checkResult.blocked ? "blocked" : "not blocked"}</span>
              {checkResult.source && <span className="text-muted-foreground">({checkResult.source})</span>}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Sources Table */}
      {info && info.sources.length > 0 && (
        <Card>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Source</TableHead>
                <TableHead className="w-[100px] text-right">Domains</TableHead>
                <TableHead className="w-[160px]">Last Updated</TableHead>
                <TableHead className="w-[80px]">Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {info.sources.map((src, i) => (
                <TableRow key={i}>
                  <TableCell>
                    <div className={T.tableRowName}>{src.name || src.url}</div>
                    {src.name && <div className={cn(T.muted, "truncate max-w-[400px]")}>{src.url}</div>}
                  </TableCell>
                  <TableCell className={T.tableCellNumeric}>{formatNumber(src.domain_count)}</TableCell>
                  <TableCell className={T.tableCellMono}>
                    {src.last_updated ? new Date(src.last_updated).toLocaleString() : "Never"}
                  </TableCell>
                  <TableCell>
                    <Badge className={
                      src.last_status === "ok" || src.last_status === "success"
                        ? "bg-gh-green/20 text-gh-green border-gh-green/30"
                        : src.last_status === "error"
                          ? "bg-gh-pink/20 text-gh-pink border-gh-pink/30"
                          : "bg-secondary text-secondary-foreground"
                    }>
                      {src.last_status || "unknown"}
                    </Badge>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </div>
  );
}
