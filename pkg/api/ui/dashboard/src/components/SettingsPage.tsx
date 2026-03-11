import { useState, useEffect, useCallback } from "react";
import { Save, Trash2, AlertTriangle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { T } from "@/lib/typography";
import type { ConfigResponse, HealthResponse } from "@/lib/api";
import {
  fetchConfig,
  fetchHealth,
  updateUpstreams,
  updateCacheConfig,
  updateLoggingConfig,
  purgeCache,
  resetStorage,
} from "@/lib/api";

export function SettingsPage() {
  const [config, setConfig] = useState<ConfigResponse | null>(null);
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [confirmDialog, setConfirmDialog] = useState<{ action: string; title: string; description: string; onConfirm: () => void } | null>(null);

  // Form state
  const [upstreams, setUpstreams] = useState("");
  const [savingUpstreams, setSavingUpstreams] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [cfg, h] = await Promise.all([fetchConfig(), fetchHealth()]);
      setConfig(cfg);
      setHealth(h);

      // Populate form
      const dns = cfg.dns as Record<string, unknown>;
      if (dns?.upstreams && Array.isArray(dns.upstreams)) {
        setUpstreams((dns.upstreams as string[]).join("\n"));
      }

      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load config");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  function showSuccess(msg: string) {
    setSuccess(msg);
    setTimeout(() => setSuccess(null), 3000);
  }

  async function handleSaveUpstreams() {
    setSavingUpstreams(true);
    try {
      const list = upstreams.split("\n").map((s) => s.trim()).filter(Boolean);
      await updateUpstreams(list);
      showSuccess("Upstream servers updated");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save upstreams");
    } finally {
      setSavingUpstreams(false);
    }
  }

  async function handlePurgeCache() {
    try {
      await purgeCache();
      showSuccess("Cache purged");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Purge failed");
    }
  }

  async function handleResetStorage() {
    try {
      await resetStorage();
      showSuccess("Query log database reset");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Reset failed");
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-[400px] rounded-xl" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className={T.pageTitle}>Settings</h2>
        <p className={T.pageDescription}>Server configuration and management</p>
      </div>

      {error && (
        <div className="rounded-lg border border-gh-red/30 bg-gh-red/10 px-4 py-3 text-sm text-gh-red">{error}</div>
      )}
      {success && (
        <div className="rounded-lg border border-gh-green/30 bg-gh-green/10 px-4 py-3 text-sm text-gh-green">{success}</div>
      )}

      <Tabs defaultValue="dns">
        <TabsList>
          <TabsTrigger value="dns">DNS</TabsTrigger>
          <TabsTrigger value="cache">Cache</TabsTrigger>
          <TabsTrigger value="danger">Danger Zone</TabsTrigger>
          <TabsTrigger value="about">About</TabsTrigger>
        </TabsList>

        {/* DNS */}
        <TabsContent value="dns">
          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>Upstream DNS Servers</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label className={T.formLabel}>Servers (one per line)</Label>
                <textarea
                  value={upstreams}
                  onChange={(e) => setUpstreams(e.target.value)}
                  rows={5}
                  className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm font-data shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                  placeholder={"1.1.1.1\n8.8.8.8\n9.9.9.9"}
                />
              </div>
              <Button size="sm" onClick={handleSaveUpstreams} disabled={savingUpstreams}>
                <Save className="h-3.5 w-3.5 mr-1" />
                {savingUpstreams ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Cache */}
        <TabsContent value="cache">
          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>DNS Cache</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className={T.mutedSm}>Purge the DNS cache to clear all cached responses.</p>
              <Button
                size="sm"
                variant="outline"
                onClick={() => setConfirmDialog({
                  action: "purge",
                  title: "Purge DNS Cache",
                  description: "This will clear all cached DNS responses. Queries will need to be resolved from upstream until the cache is rebuilt.",
                  onConfirm: handlePurgeCache,
                })}
              >
                Purge Cache
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Danger Zone */}
        <TabsContent value="danger">
          <Card className="border-gh-red/30">
            <CardHeader>
              <CardTitle className={cn(T.cardTitle, "text-gh-red")}>
                <AlertTriangle className="h-4 w-4 inline mr-1" />
                Danger Zone
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <p className={T.mutedSm}>Reset the query log database. This permanently deletes all query history and statistics.</p>
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={() => setConfirmDialog({
                    action: "reset",
                    title: "Reset Query Log Database",
                    description: "This will permanently delete all query history, statistics, and client data. This action cannot be undone.",
                    onConfirm: handleResetStorage,
                  })}
                >
                  <Trash2 className="h-3.5 w-3.5 mr-1" />
                  Reset Database
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* About */}
        <TabsContent value="about">
          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>About Glory-Hole</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {health && (
                <>
                  <DetailRow label="Version" value={health.version || "dev"} />
                  <DetailRow label="Status" value={health.status} />
                  <DetailRow label="Uptime" value={health.uptime} />
                </>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Confirmation Dialog */}
      <Dialog open={!!confirmDialog} onOpenChange={(open) => !open && setConfirmDialog(null)}>
        {confirmDialog && (
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{confirmDialog.title}</DialogTitle>
              <DialogDescription>{confirmDialog.description}</DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setConfirmDialog(null)}>Cancel</Button>
              <Button
                variant="destructive"
                onClick={() => {
                  confirmDialog.onConfirm();
                  setConfirmDialog(null);
                }}
              >
                Confirm
              </Button>
            </DialogFooter>
          </DialogContent>
        )}
      </Dialog>
    </div>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center gap-3">
      <span className={T.formLabel}>{label}</span>
      <span className="text-sm font-data">{value}</span>
    </div>
  );
}
