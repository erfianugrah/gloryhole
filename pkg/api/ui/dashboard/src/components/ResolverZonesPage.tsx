import { useState, useEffect, useCallback } from "react";
import { Plus, Trash2, Server } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { T } from "@/lib/typography";
import type { UnboundStatus, UnboundForwardZone } from "@/lib/api";
import {
  fetchUnboundStatus,
  fetchForwardZones,
  createForwardZone,
  deleteForwardZone,
} from "@/lib/api";

export function ResolverZonesPage() {
  const [status, setStatus] = useState<UnboundStatus | null>(null);
  const [forwardZones, setForwardZones] = useState<UnboundForwardZone[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showDialog, setShowDialog] = useState(false);

  // Form state
  const [fwdName, setFwdName] = useState("");
  const [fwdAddrs, setFwdAddrs] = useState("");
  const [fwdFirst, setFwdFirst] = useState(false);
  const [fwdTLS, setFwdTLS] = useState(false);
  const [saving, setSaving] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const s = await fetchUnboundStatus();
      setStatus(s);
      if (s.state === "running" || s.enabled) {
        const zones = await fetchForwardZones();
        setForwardZones(zones);
      }
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load zones");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  function openCreate() {
    setFwdName("");
    setFwdAddrs("");
    setFwdFirst(false);
    setFwdTLS(false);
    setShowDialog(true);
  }

  async function handleCreate() {
    if (!fwdName.trim() || !fwdAddrs.trim()) return;
    setSaving(true);
    try {
      const addrs = fwdAddrs.split(/[,\n]/).map(s => s.trim()).filter(Boolean);
      await createForwardZone({
        name: fwdName.trim(),
        forward_addrs: addrs,
        forward_first: fwdFirst,
        forward_tls_upstream: fwdTLS,
      });
      setShowDialog(false);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create zone");
    } finally {
      setSaving(false);
    }
  }

  async function handleDeleteFwd(name: string) {
    try {
      await deleteForwardZone(name);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete zone");
    }
  }

  if (!status?.enabled) {
    return (
      <div className="space-y-6">
        <h2 className={T.pageTitle}>Resolver Zones</h2>
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
        <Skeleton className="h-[300px] rounded-xl" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className={T.pageTitle}>Resolver Zones</h2>
          <p className={T.pageDescription}>Forward and stub zone configuration for Unbound</p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</div>
      )}

      <Tabs defaultValue="forward">
        <TabsList>
          <TabsTrigger value="forward">Forward Zones</TabsTrigger>
          <TabsTrigger value="stub">Stub Zones</TabsTrigger>
        </TabsList>

        <TabsContent value="forward">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <CardTitle className={T.cardTitle}>Forward Zones</CardTitle>
              <Button size="sm" onClick={openCreate}>
                <Plus className="h-3.5 w-3.5 mr-1" />
                Add Zone
              </Button>
            </CardHeader>
            <CardContent>
              {forwardZones.length === 0 ? (
                <div className="text-center py-8">
                  <p className={T.mutedSm}>No forward zones configured.</p>
                  <p className="text-xs text-muted-foreground mt-1">
                    Forward zones route queries for specific domains to designated upstream servers.
                  </p>
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Zone Name</TableHead>
                      <TableHead>Upstream Servers</TableHead>
                      <TableHead>Options</TableHead>
                      <TableHead className="w-[50px]"></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {forwardZones.map((z) => (
                      <TableRow key={z.name}>
                        <TableCell className="font-data text-sm">{z.name}</TableCell>
                        <TableCell className="font-data text-sm">
                          {z.forward_addrs.join(", ")}
                        </TableCell>
                        <TableCell>
                          <div className="flex gap-1">
                            {z.forward_first && <Badge variant="outline" className="text-[10px]">first</Badge>}
                            {z.forward_tls_upstream && <Badge variant="outline" className="text-[10px]">TLS</Badge>}
                          </div>
                        </TableCell>
                        <TableCell>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            onClick={() => handleDeleteFwd(z.name)}
                            className="text-destructive hover:text-destructive"
                            aria-label={`Delete zone ${z.name}`}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="stub">
          <Card>
            <CardHeader>
              <CardTitle className={T.cardTitle}>Stub Zones</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-center py-8">
                <p className={T.mutedSm}>Stub zones are configured via config.yml.</p>
                <p className="text-xs text-muted-foreground mt-1">
                  Stub zones delegate resolution for specific domains to authoritative servers.
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Create forward zone dialog */}
      <Dialog open={showDialog} onOpenChange={setShowDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Forward Zone</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-1">
              <Label className={T.formLabel}>Zone name</Label>
              <Input
                value={fwdName}
                onChange={(e) => setFwdName(e.target.value)}
                placeholder="example.com"
                className="font-data"
              />
            </div>
            <div className="space-y-1">
              <Label className={T.formLabel}>Upstream addresses (one per line or comma-separated)</Label>
              <textarea
                value={fwdAddrs}
                onChange={(e) => setFwdAddrs(e.target.value)}
                rows={3}
                className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm font-data shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                placeholder={"10.0.0.1\n10.0.0.2"}
              />
            </div>
            <div className="flex items-center gap-4">
              <div className="flex items-center gap-2">
                <Switch checked={fwdFirst} onCheckedChange={setFwdFirst} aria-label="Forward first" />
                <Label className="text-xs">Forward first (fall back to recursion)</Label>
              </div>
              <div className="flex items-center gap-2">
                <Switch checked={fwdTLS} onCheckedChange={setFwdTLS} aria-label="Use TLS" />
                <Label className="text-xs">TLS upstream</Label>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDialog(false)}>Cancel</Button>
            <Button onClick={handleCreate} disabled={saving || !fwdName.trim() || !fwdAddrs.trim()}>
              {saving ? "Creating..." : "Create"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
