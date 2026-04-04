import { useState, useEffect, useCallback } from "react";
import { RefreshCw, Search, ShieldCheck, ShieldBan, Plus, Trash2 } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
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
import type { BlocklistInfo, FeatureState } from "@/lib/api";
import {
  fetchBlocklists,
  reloadBlocklists,
  checkDomain,
  fetchFeatures,
  enableBlocklist,
  disableBlocklist,
  updateBlocklistSources,
  isBlocklistActive,
} from "@/lib/api";

function formatNumber(n: number): string {
  if (n == null) return "0";
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
  const [disableDuration, setDisableDuration] = useState("indefinite");
  const [checkQuery, setCheckQuery] = useState("");
  const [checkResult, setCheckResult] = useState<{ blocked: boolean; source?: string } | null>(null);
  const [checking, setChecking] = useState(false);
  const [newSource, setNewSource] = useState("");
  const [savingSources, setSavingSources] = useState(false);

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
      await reloadBlocklists(); // Returns 202 immediately; reload runs in background
      // Poll until the domain count changes or timeout after 120s
      const startDomains = info?.total_domains ?? 0;
      const startTime = Date.now();
      const poll = async () => {
        while (Date.now() - startTime < 120_000) {
          await new Promise((r) => setTimeout(r, 2000));
          try {
            await loadData();
            // Reload is done when domain count changes or enough time has passed
            if ((info?.total_domains ?? 0) !== startDomains || Date.now() - startTime > 10_000) {
              return;
            }
          } catch { /* keep polling */ }
        }
      };
      await poll();
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

  const blocklistActive = isBlocklistActive(features);

  async function handleToggleBlocklist() {
    try {
      if (blocklistActive) {
        const dur = disableDuration === "indefinite" ? undefined : disableDuration;
        await disableBlocklist(dur);
      } else {
        await enableBlocklist();
      }
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Toggle failed");
    }
  }

  async function handleDisableDurationChange(dur: string) {
    setDisableDuration(dur);
    if (!blocklistActive) {
      try {
        await disableBlocklist(dur === "indefinite" ? undefined : dur);
        await loadData();
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to update duration");
      }
    }
  }

  async function handleAddSource() {
    const url = newSource.trim();
    if (!url || !info) return;
    if (info.sources.includes(url)) {
      setError("This source URL is already in the list");
      return;
    }
    setSavingSources(true);
    try {
      await updateBlocklistSources([...info.sources, url]);
      setNewSource("");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add source");
    } finally {
      setSavingSources(false);
    }
  }

  async function handleRemoveSource(url: string) {
    if (!info) return;
    if (!confirm(`Remove blocklist source?\n${url}`)) return;
    setSavingSources(true);
    try {
      await updateBlocklistSources(info.sources.filter((s) => s !== url));
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to remove source");
    } finally {
      setSavingSources(false);
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
              checked={blocklistActive}
              onCheckedChange={handleToggleBlocklist}
              aria-label="Toggle blocklist"
            />
            <Label className="text-xs">
              {blocklistActive ? "Enabled" : "Disabled"}
            </Label>
            {!blocklistActive && (
              <Select value={disableDuration} onValueChange={handleDisableDurationChange}>
                <SelectTrigger className="h-7 w-[130px] text-xs">
                  <SelectValue placeholder="Duration" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="indefinite">Indefinite</SelectItem>
                  <SelectItem value="5m">5 minutes</SelectItem>
                  <SelectItem value="15m">15 minutes</SelectItem>
                  <SelectItem value="30m">30 minutes</SelectItem>
                  <SelectItem value="1h">1 hour</SelectItem>
                  <SelectItem value="6h">6 hours</SelectItem>
                  <SelectItem value="24h">24 hours</SelectItem>
                </SelectContent>
              </Select>
            )}
          </div>
          <Button size="sm" variant="outline" onClick={handleReload} disabled={reloading}>
            <RefreshCw className={cn("h-3.5 w-3.5 mr-1", reloading && "animate-spin")} />
            {reloading ? "Reloading..." : "Reload"}
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-gh-red/30 bg-gh-red/10 px-4 py-3 text-sm text-gh-red">{error}</div>
      )}

      {/* Stats */}
      {info && (
        <div className="grid gap-4 md:grid-cols-3">
          <Card className="animate-fade-in-up">
            <CardContent className="p-4">
              <div className={T.statLabelUpper}>Total Domains Blocked</div>
              <div className={cn(T.statValue, "text-gh-red mt-2")}>{formatNumber(info.total_domains)}</div>
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
                {info.last_updated ? new Date(info.last_updated).toLocaleString() : "Never"}
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
                ? "border border-gh-red/30 bg-gh-red/10 text-gh-red"
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
      {info && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className={T.cardTitle}>Sources</CardTitle>
          </CardHeader>
          <CardContent className="pb-2">
            <div className="flex items-center gap-2">
              <Input
                placeholder="https://example.com/blocklist.txt"
                value={newSource}
                onChange={(e) => setNewSource(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && handleAddSource()}
                className="font-data flex-1"
              />
              <Button
                size="sm"
                onClick={handleAddSource}
                disabled={savingSources || !newSource.trim()}
              >
                <Plus className="h-3.5 w-3.5 mr-1" />
                Add
              </Button>
            </div>
          </CardContent>
          {info.sources.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>URL</TableHead>
                  <TableHead className="w-[50px]"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {info.sources.map((url, i) => (
                  <TableRow key={i}>
                    <TableCell className={cn(T.tableCellMono, "truncate max-w-[600px]")}>
                      {url}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleRemoveSource(url)}
                        disabled={savingSources}
                        className="text-gh-red hover:text-gh-red"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <CardContent className="pt-0 pb-4">
              <p className={T.mutedSm}>No blocklist sources configured</p>
            </CardContent>
          )}
        </Card>
      )}
    </div>
  );
}
