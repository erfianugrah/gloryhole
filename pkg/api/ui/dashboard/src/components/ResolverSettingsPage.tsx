import { useState, useEffect, useCallback } from "react";
import { Save, Server } from "lucide-react";
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
import { T } from "@/lib/typography";
import type { UnboundStatus, UnboundConfig, UnboundServerBlock } from "@/lib/api";
import {
  fetchUnboundStatus,
  fetchUnboundConfig,
  updateUnboundServer,
} from "@/lib/api";

export function ResolverSettingsPage() {
  const [status, setStatus] = useState<UnboundStatus | null>(null);
  const [config, setConfig] = useState<UnboundConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  // Form state
  const [msgCache, setMsgCache] = useState("32m");
  const [rrsetCache, setRrsetCache] = useState("64m");
  const [keyCache, setKeyCache] = useState("16m");
  const [cacheMinTTL, setCacheMinTTL] = useState("0");
  const [cacheMaxNegTTL, setCacheMaxNegTTL] = useState("60");
  const [numThreads, setNumThreads] = useState("2");
  const [verbosity, setVerbosity] = useState("1");
  const [hardenGlue, setHardenGlue] = useState(true);
  const [hardenDNSSEC, setHardenDNSSEC] = useState(true);
  const [hardenBelowNX, setHardenBelowNX] = useState(true);
  const [qnameMin, setQnameMin] = useState(true);
  const [aggressiveNSEC, setAggressiveNSEC] = useState(true);
  const [serveExpired, setServeExpired] = useState(true);
  const [serveExpiredTTL, setServeExpiredTTL] = useState("86400");
  const [prefetch, setPrefetch] = useState(true);
  const [logQueries, setLogQueries] = useState(false);
  const [logReplies, setLogReplies] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [s, c] = await Promise.all([fetchUnboundStatus(), fetchUnboundConfig()]);
      setStatus(s);
      setConfig(c);

      if (c) {
        const sv = c.server;
        setMsgCache(sv.msg_cache_size);
        setRrsetCache(sv.rrset_cache_size);
        setKeyCache(sv.key_cache_size);
        setCacheMinTTL(String(sv.cache_min_ttl));
        setCacheMaxNegTTL(String(sv.cache_max_negative_ttl));
        setNumThreads(String(sv.num_threads));
        setVerbosity(String(sv.verbosity));
        setHardenGlue(sv.harden_glue);
        setHardenDNSSEC(sv.harden_dnssec_stripped);
        setHardenBelowNX(sv.harden_below_nxdomain);
        setQnameMin(sv.qname_minimisation);
        setAggressiveNSEC(sv.aggressive_nsec);
        setServeExpired(sv.serve_expired);
        setServeExpiredTTL(String(sv.serve_expired_ttl));
        setPrefetch(sv.prefetch);
        setLogQueries(sv.log_queries);
        setLogReplies(sv.log_replies);
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

  async function handleSave(fields: Partial<UnboundServerBlock>) {
    setSaving(true);
    try {
      await updateUnboundServer(fields);
      showSuccess("Resolver settings updated");
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  }

  if (!status?.enabled) {
    return (
      <div className="space-y-6">
        <h2 className={T.pageTitle}>Resolver Settings</h2>
        <Card>
          <CardContent className="py-12 text-center">
            <Server className="h-10 w-10 mx-auto text-muted-foreground mb-3" />
            <p className="text-sm text-muted-foreground">Unbound resolver is not enabled.</p>
          </CardContent>
        </Card>
      </div>
    );
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
        <h2 className={T.pageTitle}>Resolver Settings</h2>
        <p className={T.pageDescription}>Configure the Unbound recursive resolver</p>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</div>
      )}
      {success && (
        <div className="rounded-lg border border-gh-green/30 bg-gh-green/10 px-4 py-3 text-sm text-gh-green">{success}</div>
      )}

      <Tabs defaultValue="cache">
        <TabsList className="flex-wrap">
          <TabsTrigger value="cache">Cache</TabsTrigger>
          <TabsTrigger value="dnssec">DNSSEC</TabsTrigger>
          <TabsTrigger value="hardening">Hardening</TabsTrigger>
          <TabsTrigger value="stale">Serve Stale</TabsTrigger>
          <TabsTrigger value="logging">Logging</TabsTrigger>
          <TabsTrigger value="performance">Performance</TabsTrigger>
        </TabsList>

        <TabsContent value="cache">
          <Card>
            <CardHeader><CardTitle className={T.cardTitle}>Cache (L2)</CardTitle></CardHeader>
            <CardContent className="space-y-4">
              <p className={T.mutedSm}>
                Unbound's cache acts as L2 behind Glory-Hole's main cache.
                To purge both caches at once, use <a href="/settings" className="underline">Settings &gt; Cache &gt; Purge</a>.
              </p>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <FormField label="Message cache size" value={msgCache} onChange={setMsgCache} placeholder="32m" />
                <FormField label="RRSet cache size" value={rrsetCache} onChange={setRrsetCache} placeholder="64m" />
                <FormField label="Key cache size" value={keyCache} onChange={setKeyCache} placeholder="16m" />
                <FormField label="Min TTL" value={cacheMinTTL} onChange={setCacheMinTTL} type="number" />
                <FormField label="Max negative TTL" value={cacheMaxNegTTL} onChange={setCacheMaxNegTTL} type="number" />
              </div>
              <div className="flex items-center gap-2">
                <Switch checked={prefetch} onCheckedChange={setPrefetch} aria-label="Enable prefetch" />
                <Label className={T.formLabel}>Prefetch expiring entries</Label>
              </div>
              <Button size="sm" onClick={() => handleSave({
                msg_cache_size: msgCache, rrset_cache_size: rrsetCache, key_cache_size: keyCache,
                cache_min_ttl: parseInt(cacheMinTTL), cache_max_negative_ttl: parseInt(cacheMaxNegTTL),
                prefetch, prefetch_key: prefetch,
              })} disabled={saving}>
                <Save className="h-3.5 w-3.5 mr-1" />{saving ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="dnssec">
          <Card>
            <CardHeader><CardTitle className={T.cardTitle}>DNSSEC Validation</CardTitle></CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center gap-2">
                <Switch checked={hardenDNSSEC} onCheckedChange={setHardenDNSSEC} aria-label="Require DNSSEC" />
                <Label className={T.formLabel}>Require DNSSEC for trust-anchored zones</Label>
              </div>
              <p className={T.mutedSm}>When enabled, responses without valid DNSSEC signatures for signed zones return SERVFAIL.</p>
              <Button size="sm" onClick={() => handleSave({ harden_dnssec_stripped: hardenDNSSEC })} disabled={saving}>
                <Save className="h-3.5 w-3.5 mr-1" />{saving ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="hardening">
          <Card>
            <CardHeader><CardTitle className={T.cardTitle}>Hardening</CardTitle></CardHeader>
            <CardContent className="space-y-4">
              <ToggleRow label="Harden glue records" checked={hardenGlue} onChange={setHardenGlue} />
              <ToggleRow label="Harden below NXDOMAIN" checked={hardenBelowNX} onChange={setHardenBelowNX} />
              <ToggleRow label="QNAME minimisation (RFC 7816)" checked={qnameMin} onChange={setQnameMin} />
              <ToggleRow label="Aggressive NSEC (RFC 8198)" checked={aggressiveNSEC} onChange={setAggressiveNSEC} />
              <Button size="sm" onClick={() => handleSave({
                harden_glue: hardenGlue, harden_below_nxdomain: hardenBelowNX,
                qname_minimisation: qnameMin, aggressive_nsec: aggressiveNSEC,
              })} disabled={saving}>
                <Save className="h-3.5 w-3.5 mr-1" />{saving ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="stale">
          <Card>
            <CardHeader><CardTitle className={T.cardTitle}>Serve Stale</CardTitle></CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center gap-2">
                <Switch checked={serveExpired} onCheckedChange={setServeExpired} aria-label="Serve expired entries" />
                <Label className={T.formLabel}>Serve expired cache entries while refreshing</Label>
              </div>
              <p className={T.mutedSm}>
                Prevents DNS failures when upstream is temporarily unreachable by serving stale data.
              </p>
              {serveExpired && (
                <FormField label="Expired TTL (seconds)" value={serveExpiredTTL} onChange={setServeExpiredTTL} type="number" />
              )}
              <Button size="sm" onClick={() => handleSave({
                serve_expired: serveExpired, serve_expired_ttl: parseInt(serveExpiredTTL),
              })} disabled={saving}>
                <Save className="h-3.5 w-3.5 mr-1" />{saving ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="logging">
          <Card>
            <CardHeader><CardTitle className={T.cardTitle}>Logging</CardTitle></CardHeader>
            <CardContent className="space-y-4">
              <p className={T.mutedSm}>
                These control Unbound's internal logging for debugging recursive resolution.
                They do <strong>not</strong> affect Glory-Hole's query log — all queries are
                always logged in the <a href="/queries" className="underline">Query Log</a> regardless of these settings.
              </p>
              <div className="space-y-1">
                <Label className={T.formLabel}>Verbosity (0-5)</Label>
                <Select value={verbosity} onValueChange={setVerbosity}>
                  <SelectTrigger className="w-[200px]"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="0">0 - Errors only</SelectItem>
                    <SelectItem value="1">1 - Operational</SelectItem>
                    <SelectItem value="2">2 - Detailed</SelectItem>
                    <SelectItem value="3">3 - Query level</SelectItem>
                    <SelectItem value="4">4 - Algorithm level</SelectItem>
                    <SelectItem value="5">5 - Client identification</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <ToggleRow label="Log queries" checked={logQueries} onChange={setLogQueries} />
              <ToggleRow label="Log replies" checked={logReplies} onChange={setLogReplies} />
              <Button size="sm" onClick={() => handleSave({
                verbosity: parseInt(verbosity), log_queries: logQueries, log_replies: logReplies,
              })} disabled={saving}>
                <Save className="h-3.5 w-3.5 mr-1" />{saving ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="performance">
          <Card>
            <CardHeader><CardTitle className={T.cardTitle}>Performance</CardTitle></CardHeader>
            <CardContent className="space-y-4">
              <FormField label="Number of threads" value={numThreads} onChange={setNumThreads} type="number" />
              <p className={T.mutedSm}>Changes to thread count require an Unbound restart.</p>
              <Button size="sm" onClick={() => handleSave({ num_threads: parseInt(numThreads) })} disabled={saving}>
                <Save className="h-3.5 w-3.5 mr-1" />{saving ? "Saving..." : "Save"}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

function FormField({ label, value, onChange, placeholder, type = "text" }: {
  label: string; value: string; onChange: (v: string) => void; placeholder?: string; type?: string;
}) {
  return (
    <div className="space-y-1">
      <Label className={T.formLabel}>{label}</Label>
      <Input type={type} value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} />
    </div>
  );
}

function ToggleRow({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="flex items-center gap-2">
      <Switch checked={checked} onCheckedChange={onChange} aria-label={label} />
      <Label className={T.formLabel}>{label}</Label>
    </div>
  );
}
