import { useState, useEffect, useCallback } from "react";
import { Save, Trash2, AlertTriangle, Shield, Key, Lock, Server, Plus, X } from "lucide-react";
import type { UnboundStatus } from "@/lib/api";
import { fetchUnboundStatus } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
  updateTLSConfig,
  updateBlockPageConfig,
  updateAllowedClients,
  purgeCache,
  resetStorage,
} from "@/lib/api";

/** Parse a Go duration string like "5m0s", "1h30m0s", or "300s" into total seconds. */
function parseDurationToSeconds(dur?: string): number | undefined {
  if (!dur) return undefined;
  let total = 0;
  const h = dur.match(/(\d+)h/);
  const m = dur.match(/(\d+)m/);
  const s = dur.match(/(\d+)s/);
  if (h) total += parseInt(h[1], 10) * 3600;
  if (m) total += parseInt(m[1], 10) * 60;
  if (s) total += parseInt(s[1], 10);
  // Bare number fallback (already seconds)
  if (!h && !m && !s && /^\d+$/.test(dur)) total = parseInt(dur, 10);
  return total || undefined;
}

export function SettingsPage() {
  const [config, setConfig] = useState<ConfigResponse | null>(null);
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [confirmDialog, setConfirmDialog] = useState<{
    action: string;
    title: string;
    description: string;
    onConfirm: () => void;
  } | null>(null);

  // Resolver status (to show notices when Unbound is active)
  const [resolverActive, setResolverActive] = useState(false);

  // DNS form state
  const [upstreams, setUpstreams] = useState("");
  const [savingUpstreams, setSavingUpstreams] = useState(false);

  // Cache form state
  const [cacheEnabled, setCacheEnabled] = useState(true);
  const [cacheMaxEntries, setCacheMaxEntries] = useState("10000");
  const [cacheMinTTL, setCacheMinTTL] = useState("60");
  const [cacheMaxTTL, setCacheMaxTTL] = useState("86400");
  const [cacheNegTTL, setCacheNegTTL] = useState("300");
  const [cacheShards, setCacheShards] = useState("0");
  const [savingCache, setSavingCache] = useState(false);

  // Logging form state
  const [logLevel, setLogLevel] = useState("info");
  const [logFormat, setLogFormat] = useState("text");
  const [logOutput, setLogOutput] = useState("stdout");
  const [retentionDays] = useState("7"); // Read-only display; not editable from UI
  const [savingLogging, setSavingLogging] = useState(false);

  // TLS form state
  const [dotEnabled, setDotEnabled] = useState(false);
  const [dotAddress, setDotAddress] = useState(":853");
  const [certFile, setCertFile] = useState("");
  const [keyFile, setKeyFile] = useState("");
  const [acmeEnabled, setAcmeEnabled] = useState(false);
  const [acmeHosts, setAcmeHosts] = useState("");
  const [savingTLS, setSavingTLS] = useState(false);

  // Auth display state (read-only display, no editing passwords through UI)
  const [authEnabled, setAuthEnabled] = useState(false);
  const [authUsername, setAuthUsername] = useState("");
  const [authHasApiKey, setAuthHasApiKey] = useState(false);

  // Block page form state
  const [blockPageEnabled, setBlockPageEnabled] = useState(false);
  const [blockPageIP, setBlockPageIP] = useState("");
  const [savingBlockPage, setSavingBlockPage] = useState(false);

  // Client ACL form state
  const [allowedClients, setAllowedClients] = useState<string[]>([]);
  const [newClient, setNewClient] = useState("");
  const [savingACL, setSavingACL] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [cfg, h] = await Promise.all([fetchConfig(), fetchHealth()]);
      setConfig(cfg);
      setHealth(h);

      // Populate DNS form — Go returns upstream_dns_servers at top level
      if (cfg.upstream_dns_servers?.length) {
        setUpstreams(cfg.upstream_dns_servers.join("\n"));
      }

      // Populate cache form — Go returns durations as strings like "5m0s"
      const cache = cfg.cache;
      if (cache) {
        setCacheEnabled(cache.enabled ?? true);
        setCacheMaxEntries(String(cache.max_entries ?? 10000));
        setCacheMinTTL(String(parseDurationToSeconds(cache.min_ttl) ?? 60));
        setCacheMaxTTL(String(parseDurationToSeconds(cache.max_ttl) ?? 86400));
        setCacheNegTTL(String(parseDurationToSeconds(cache.negative_ttl) ?? 300));
        setCacheShards(String(cache.shard_count ?? 0));
      }

      // Populate logging form
      const logging = cfg.logging;
      if (logging) {
        setLogLevel(logging.level ?? "info");
        setLogFormat(logging.format ?? "text");
        setLogOutput(logging.output ?? "stdout");
      }

      // Populate TLS form — nested under cfg.server.tls
      const srv = cfg.server;
      if (srv) {
        setDotEnabled(srv.dot_enabled ?? false);
        setDotAddress(srv.dot_address ?? ":853");
        const tls = srv.tls;
        if (tls) {
          setCertFile(tls.cert_file ?? "");
          setKeyFile(tls.key_file ?? "");
          const acme = tls.acme;
          if (acme) {
            setAcmeEnabled(acme.enabled ?? false);
            if (Array.isArray(acme.hosts)) {
              setAcmeHosts(acme.hosts.join(", "));
            }
          }
        }
      }

      // Populate block page form
      const blockPage = cfg.block_page as Record<string, unknown> | undefined;
      if (blockPage) {
        setBlockPageEnabled(blockPage.enabled as boolean ?? false);
        setBlockPageIP(String(blockPage.block_ip ?? ""));
      }

      // Populate client ACL
      if (cfg.server?.allowed_clients) {
        setAllowedClients(cfg.server.allowed_clients);
      } else {
        setAllowedClients([]);
      }

      // Check resolver status (non-blocking)
      try {
        const rs = await fetchUnboundStatus();
        setResolverActive(rs.enabled && rs.state === "running");
      } catch {
        setResolverActive(false);
      }

      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load config");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  function showSuccess(msg: string) {
    setSuccess(msg);
    setTimeout(() => setSuccess(null), 3000);
  }

  async function handleSaveUpstreams() {
    setSavingUpstreams(true);
    try {
      const list = upstreams
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean);
      await updateUpstreams(list);
      showSuccess("Upstream servers updated");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save upstreams");
    } finally {
      setSavingUpstreams(false);
    }
  }

  async function handleSaveCache() {
    setSavingCache(true);
    try {
      await updateCacheConfig({
        enabled: cacheEnabled,
        max_entries: parseInt(cacheMaxEntries, 10),
        min_ttl: cacheMinTTL,       // Go parseDurationField accepts bare ints as seconds
        max_ttl: cacheMaxTTL,
        negative_ttl: cacheNegTTL,
        blocked_ttl: "300",         // sensible default
        shard_count: parseInt(cacheShards, 10),
      });
      showSuccess("Cache settings updated");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save cache settings");
    } finally {
      setSavingCache(false);
    }
  }

  async function handleSaveLogging() {
    setSavingLogging(true);
    try {
      await updateLoggingConfig({
        level: logLevel,
        format: logFormat,
        output: logOutput,
        max_size: config?.logging?.max_size ?? 100,
        max_backups: config?.logging?.max_backups ?? 3,
        max_age: config?.logging?.max_age ?? 28,
      });
      showSuccess("Logging settings updated");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save logging settings");
    } finally {
      setSavingLogging(false);
    }
  }

  async function handleSaveTLS() {
    setSavingTLS(true);
    try {
      await updateTLSConfig({
        dot_enabled: dotEnabled,
        dot_address: dotAddress,
        cert_file: certFile,
        key_file: keyFile,
        acme: {
          enabled: acmeEnabled,
          hosts: acmeHosts
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean),
        },
      });
      showSuccess("TLS settings updated (restart required for bind address changes)");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save TLS settings");
    } finally {
      setSavingTLS(false);
    }
  }

  async function handleSaveBlockPage() {
    setSavingBlockPage(true);
    try {
      await updateBlockPageConfig({
        enabled: blockPageEnabled,
        block_ip: blockPageIP,
      });
      showSuccess("Block page settings updated");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save block page settings");
    } finally {
      setSavingBlockPage(false);
    }
  }

  function handleAddClient() {
    const entry = newClient.trim();
    if (!entry || allowedClients.includes(entry)) return;
    setAllowedClients([...allowedClients, entry]);
    setNewClient("");
  }

  function handleRemoveClient(index: number) {
    setAllowedClients(allowedClients.filter((_, i) => i !== index));
  }

  async function handleSaveACL() {
    setSavingACL(true);
    try {
      await updateAllowedClients(allowedClients);
      showSuccess("Client ACL updated");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save client ACL");
    } finally {
      setSavingACL(false);
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
        <p className={T.pageDescription}>
          Server configuration and management
        </p>
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

      <Tabs defaultValue="dns">
        <TabsList className="flex-wrap">
          <TabsTrigger value="dns">DNS</TabsTrigger>
          <TabsTrigger value="cache">Cache</TabsTrigger>
          <TabsTrigger value="logging">Logging</TabsTrigger>
          <TabsTrigger value="tls">TLS</TabsTrigger>
          <TabsTrigger value="acl">Client ACL</TabsTrigger>
          <TabsTrigger value="blockpage">Block Page</TabsTrigger>
          <TabsTrigger value="auth">Authentication</TabsTrigger>
          <TabsTrigger value="danger">Danger Zone</TabsTrigger>
          <TabsTrigger value="about">About</TabsTrigger>
        </TabsList>

        {/* DNS */}
        <TabsContent value="dns">
          {resolverActive && (
            <div className="rounded-lg border border-gh-blue/30 bg-gh-blue/10 px-4 py-3 text-sm text-gh-blue mb-4 flex items-center gap-2">
              <Server className="h-4 w-4 flex-shrink-0" />
              <span>
                Upstream resolution is handled by the <a href="/resolver" className="underline font-medium">Unbound resolver</a>.
                These servers are used as fallback if Unbound is unavailable.
              </span>
            </div>
          )}
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
                  placeholder={"1.1.1.1:53\n8.8.8.8:53\n9.9.9.9:53"}
                />
              </div>
              <Button
                size="sm"
                onClick={handleSaveUpstreams}
                disabled={savingUpstreams}
              >
                <Save className="h-3.5 w-3.5 mr-1" />
                {savingUpstreams ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Cache */}
        <TabsContent value="cache">
          <div className="space-y-4">
            {resolverActive && (
              <div className="rounded-lg border border-gh-blue/30 bg-gh-blue/10 px-4 py-3 text-sm text-gh-blue flex items-center gap-2">
                <Server className="h-4 w-4 flex-shrink-0" />
                <span>
                  Two-layer caching is active. These settings control Glory-Hole's L1 cache.
                  Unbound's L2 cache is configured in{" "}
                  <a href="/resolver/settings" className="underline font-medium">Resolver Settings</a>.
                  Purging below clears both caches.
                </span>
              </div>
            )}
            <Card>
              <CardHeader>
                <CardTitle className={T.cardTitle}>Cache Settings</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex items-center gap-3">
                  <Switch
                    checked={cacheEnabled}
                    onCheckedChange={setCacheEnabled}
                    aria-label="Enable DNS cache"
                  />
                  <Label className={T.formLabel}>Cache enabled</Label>
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-1">
                    <Label className={T.formLabel}>Max entries</Label>
                    <Input
                      type="number"
                      value={cacheMaxEntries}
                      onChange={(e) => setCacheMaxEntries(e.target.value)}
                    />
                  </div>
                  <div className="space-y-1">
                    <Label className={T.formLabel}>Shard count</Label>
                    <Input
                      type="number"
                      value={cacheShards}
                      onChange={(e) => setCacheShards(e.target.value)}
                      placeholder="0 = non-sharded"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label className={T.formLabel}>Min TTL (seconds)</Label>
                    <Input
                      type="number"
                      value={cacheMinTTL}
                      onChange={(e) => setCacheMinTTL(e.target.value)}
                    />
                  </div>
                  <div className="space-y-1">
                    <Label className={T.formLabel}>Max TTL (seconds)</Label>
                    <Input
                      type="number"
                      value={cacheMaxTTL}
                      onChange={(e) => setCacheMaxTTL(e.target.value)}
                    />
                  </div>
                  <div className="space-y-1">
                    <Label className={T.formLabel}>Negative TTL (seconds)</Label>
                    <Input
                      type="number"
                      value={cacheNegTTL}
                      onChange={(e) => setCacheNegTTL(e.target.value)}
                    />
                  </div>
                </div>
                <Button
                  size="sm"
                  onClick={handleSaveCache}
                  disabled={savingCache}
                >
                  <Save className="h-3.5 w-3.5 mr-1" />
                  {savingCache ? "Saving..." : "Save"}
                </Button>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className={T.cardTitle}>Cache Actions</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <p className={T.mutedSm}>
                  {resolverActive
                    ? "Purge both Glory-Hole and Unbound caches to clear all cached responses."
                    : "Purge the DNS cache to clear all cached responses."}
                </p>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() =>
                    setConfirmDialog({
                      action: "purge",
                      title: "Purge DNS Cache",
                      description: resolverActive
                        ? "This will clear both Glory-Hole's L1 cache and Unbound's L2 cache. All queries will need fresh recursive resolution until the caches are rebuilt."
                        : "This will clear all cached DNS responses. Queries will need to be resolved from upstream until the cache is rebuilt.",
                      onConfirm: handlePurgeCache,
                    })
                  }
                >
                  Purge Cache
                </Button>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        {/* Logging */}
        <TabsContent value="logging">
          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>Logging</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-1">
                  <Label className={T.formLabel}>Log Level</Label>
                  <Select value={logLevel} onValueChange={setLogLevel}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="debug">Debug</SelectItem>
                      <SelectItem value="info">Info</SelectItem>
                      <SelectItem value="warn">Warning</SelectItem>
                      <SelectItem value="error">Error</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1">
                  <Label className={T.formLabel}>Format</Label>
                  <Select value={logFormat} onValueChange={setLogFormat}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="text">Text</SelectItem>
                      <SelectItem value="json">JSON</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1">
                  <Label className={T.formLabel}>Output</Label>
                  <Select value={logOutput} onValueChange={setLogOutput}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="stdout">stdout</SelectItem>
                      <SelectItem value="stderr">stderr</SelectItem>
                      <SelectItem value="file">File</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <Button
                size="sm"
                onClick={handleSaveLogging}
                disabled={savingLogging}
              >
                <Save className="h-3.5 w-3.5 mr-1" />
                {savingLogging ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {/* TLS */}
        <TabsContent value="tls">
          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>
                <Lock className="h-4 w-4 inline mr-1.5" />
                DNS-over-TLS (DoT)
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center gap-3">
                <Switch
                  checked={dotEnabled}
                  onCheckedChange={setDotEnabled}
                  aria-label="Enable DNS-over-TLS"
                />
                <Label className={T.formLabel}>DoT enabled</Label>
              </div>

              {dotEnabled && (
                <div className="space-y-4 border-l-2 border-border pl-4 ml-1">
                  <div className="space-y-1">
                    <Label className={T.formLabel}>Listen Address</Label>
                    <Input
                      value={dotAddress}
                      onChange={(e) => setDotAddress(e.target.value)}
                      placeholder=":853"
                    />
                    <p className={T.mutedSm}>
                      Requires server restart to take effect.
                    </p>
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="space-y-1">
                      <Label className={T.formLabel}>Certificate File</Label>
                      <Input
                        value={certFile}
                        onChange={(e) => setCertFile(e.target.value)}
                        placeholder="/path/to/cert.pem"
                        className="font-data"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label className={T.formLabel}>Key File</Label>
                      <Input
                        value={keyFile}
                        onChange={(e) => setKeyFile(e.target.value)}
                        placeholder="/path/to/key.pem"
                        className="font-data"
                      />
                    </div>
                  </div>

                  <div className="border-t border-border pt-4 space-y-3">
                    <div className="flex items-center gap-3">
                      <Switch
                        checked={acmeEnabled}
                        onCheckedChange={setAcmeEnabled}
                        aria-label="Enable ACME DNS-01"
                      />
                      <Label className={T.formLabel}>
                        ACME DNS-01 (automatic certificates)
                      </Label>
                    </div>

                    {acmeEnabled && (
                      <div className="space-y-1">
                        <Label className={T.formLabel}>
                          Hostnames (comma-separated)
                        </Label>
                        <Input
                          value={acmeHosts}
                          onChange={(e) => setAcmeHosts(e.target.value)}
                          placeholder="dot.example.com"
                        />
                      </div>
                    )}
                  </div>
                </div>
              )}

              <Button
                size="sm"
                onClick={handleSaveTLS}
                disabled={savingTLS}
              >
                <Save className="h-3.5 w-3.5 mr-1" />
                {savingTLS ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Client ACL */}
        <TabsContent value="acl">
          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>
                <Shield className="h-4 w-4 inline mr-1.5" />
                Client ACL
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className={T.mutedSm}>
                Restrict plain DNS (port 53 UDP/TCP) to specific IPs and CIDRs.
                When the list is empty, all clients are allowed (open resolver).
                DoT and DoH bypass this ACL — they have their own auth layers.
              </p>

              {allowedClients.length === 0 ? (
                <div className="rounded-md border border-gh-yellow/30 bg-gh-yellow/10 px-3 py-2 text-xs text-gh-yellow">
                  No client restrictions — port 53 is open to all IPs
                </div>
              ) : (
                <div className="space-y-1.5">
                  {allowedClients.map((client, i) => (
                    <div
                      key={`${client}-${i}`}
                      className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-1.5"
                    >
                      <code className="text-xs font-data flex-1">{client}</code>
                      <button
                        type="button"
                        onClick={() => handleRemoveClient(i)}
                        className="text-muted-foreground hover:text-gh-red transition-colors"
                      >
                        <X className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  ))}
                </div>
              )}

              <div className="flex items-center gap-2">
                <Input
                  value={newClient}
                  onChange={(e) => setNewClient(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      handleAddClient();
                    }
                  }}
                  placeholder="192.168.1.0/24 or 10.0.0.1 or fd00::/8"
                  className="font-data max-w-sm"
                />
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleAddClient}
                  disabled={!newClient.trim()}
                >
                  <Plus className="h-3.5 w-3.5 mr-1" />
                  Add
                </Button>
              </div>

              <Button
                size="sm"
                onClick={handleSaveACL}
                disabled={savingACL}
              >
                <Save className="h-3.5 w-3.5 mr-1" />
                {savingACL ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Block Page */}
        <TabsContent value="blockpage">
          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>
                <Shield className="h-4 w-4 inline mr-1.5" />
                Block Page
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className={T.mutedSm}>
                When enabled, blocked domains resolve to the configured IP instead of
                returning NXDOMAIN. The server then shows a styled page explaining
                why the domain was blocked. Only works for plain HTTP connections.
              </p>
              <div className="flex items-center gap-3">
                <Switch
                  checked={blockPageEnabled}
                  onCheckedChange={setBlockPageEnabled}
                  aria-label="Enable block page"
                />
                <Label className={T.formLabel}>Block page enabled</Label>
              </div>

              {blockPageEnabled && (
                <div className="space-y-4 border-l-2 border-border pl-4 ml-1">
                  <div className="space-y-1">
                    <Label className={T.formLabel}>Block IP Address</Label>
                    <Input
                      value={blockPageIP}
                      onChange={(e) => setBlockPageIP(e.target.value)}
                      placeholder="10.0.10.10"
                      className="font-data max-w-xs"
                    />
                    <p className={T.muted}>
                      Must be this server's IP address that clients use to reach it.
                      Blocked domains will resolve to this IP.
                    </p>
                  </div>
                </div>
              )}

              <Button
                size="sm"
                onClick={handleSaveBlockPage}
                disabled={savingBlockPage}
              >
                <Save className="h-3.5 w-3.5 mr-1" />
                {savingBlockPage ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Authentication */}
        <TabsContent value="auth">
          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>
                <Shield className="h-4 w-4 inline mr-1.5" />
                Authentication
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-3">
                <DetailRow
                  label="Status"
                  value={authEnabled ? "Enabled" : "Disabled"}
                />
                {authEnabled && (
                  <>
                    <DetailRow label="Username" value={authUsername || "N/A"} />
                    <DetailRow
                      label="API Key"
                      value={authHasApiKey ? "Configured" : "Not set"}
                    />
                    <DetailRow label="Password" value="Configured (bcrypt)" />
                  </>
                )}
              </div>

              <div className="rounded-lg border border-border bg-muted/50 p-4 space-y-2">
                <div className="flex items-center gap-2 text-sm font-medium">
                  <Key className="h-4 w-4" />
                  Managing credentials
                </div>
                <p className={T.mutedSm}>
                  For security, authentication settings are managed through{" "}
                  <code className="text-xs font-data bg-muted px-1 py-0.5 rounded">
                    config.yml
                  </code>{" "}
                  or environment variables. To change your password, run:
                </p>
                <pre className="text-xs font-data bg-background rounded px-3 py-2 border border-border overflow-x-auto">
                  glory-hole hash-password &quot;your-new-password&quot;
                </pre>
                <p className={T.mutedSm}>
                  Environment variables:{" "}
                  <code className="text-xs font-data bg-muted px-1 py-0.5 rounded">
                    GLORYHOLE_API_KEY
                  </code>
                  ,{" "}
                  <code className="text-xs font-data bg-muted px-1 py-0.5 rounded">
                    GLORYHOLE_BASIC_USER
                  </code>
                  ,{" "}
                  <code className="text-xs font-data bg-muted px-1 py-0.5 rounded">
                    GLORYHOLE_BASIC_PASS
                  </code>
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Danger Zone */}
        <TabsContent value="danger">
          <Card className="border-destructive/30">
            <CardHeader>
              <CardTitle className={cn(T.cardTitle, "text-destructive")}>
                <AlertTriangle className="h-4 w-4 inline mr-1" />
                Danger Zone
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <p className={T.mutedSm}>
                  Reset the query log database. This permanently deletes all
                  query history and statistics.
                </p>
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={() =>
                    setConfirmDialog({
                      action: "reset",
                      title: "Reset Query Log Database",
                      description:
                        "This will permanently delete all query history, statistics, and client data. This action cannot be undone.",
                      onConfirm: handleResetStorage,
                    })
                  }
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
      <Dialog
        open={!!confirmDialog}
        onOpenChange={(open) => !open && setConfirmDialog(null)}
      >
        {confirmDialog && (
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{confirmDialog.title}</DialogTitle>
              <DialogDescription>{confirmDialog.description}</DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => setConfirmDialog(null)}
              >
                Cancel
              </Button>
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
