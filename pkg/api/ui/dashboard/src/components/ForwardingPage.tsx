import { useState, useEffect, useCallback } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
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
import type { ConditionalForwardingRule } from "@/lib/api";
import { fetchForwardingRules, createForwardingRule, deleteForwardingRule } from "@/lib/api";

export function ForwardingPage() {
  const [rules, setRules] = useState<ConditionalForwardingRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [saving, setSaving] = useState(false);

  const [formDomain, setFormDomain] = useState("");
  const [formUpstreams, setFormUpstreams] = useState("");

  const loadData = useCallback(async () => {
    try {
      const data = await fetchForwardingRules();
      setRules(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load rules");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  function openDialog() {
    setFormDomain("");
    setFormUpstreams("");
    setDialogOpen(true);
  }

  async function handleSave() {
    setSaving(true);
    try {
      const upstreams = formUpstreams.split(",").map((s) => s.trim()).filter(Boolean);
      await createForwardingRule({ domain: formDomain, upstreams });
      setDialogOpen(false);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create rule");
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this forwarding rule?")) return;
    try {
      await deleteForwardingRule(id);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete rule");
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className={T.pageTitle}>Conditional Forwarding</h2>
          <p className={T.pageDescription}>Route DNS queries for specific domains to designated upstream servers</p>
        </div>
        <Button size="sm" onClick={openDialog}>
          <Plus className="h-3.5 w-3.5 mr-1" /> Add Rule
        </Button>
      </div>

      {error && (
        <div className="rounded-lg border border-gh-red/30 bg-gh-red/10 px-4 py-3 text-sm text-gh-red">{error}</div>
      )}

      <Card>
        {loading ? (
          <CardContent className="p-4 space-y-2">
            {Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}
          </CardContent>
        ) : rules.length === 0 ? (
          <CardContent className="p-8 text-center">
            <p className={T.mutedSm}>No forwarding rules configured</p>
            <Button size="sm" className="mt-4" onClick={openDialog}>
              <Plus className="h-3.5 w-3.5 mr-1" /> Add your first rule
            </Button>
          </CardContent>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Domain(s)</TableHead>
                <TableHead>Upstream Server(s)</TableHead>
                <TableHead className="w-[70px]">Priority</TableHead>
                <TableHead className="w-[50px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className={T.tableRowName}>{r.name}</TableCell>
                  <TableCell className={T.tableCellMono}>
                    {(r.domains ?? []).join(", ") || "—"}
                  </TableCell>
                  <TableCell className={T.tableCellMono}>
                    {r.upstreams.join(", ")}
                  </TableCell>
                  <TableCell className={T.tableCellMono}>{r.priority}</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="icon-sm" onClick={() => handleDelete(r.id)} className="text-gh-red hover:text-gh-red">
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Forwarding Rule</DialogTitle>
            <DialogDescription>Forward DNS queries matching a domain to specific upstream servers.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label className={T.formLabel}>Domain / Pattern</Label>
              <Input value={formDomain} onChange={(e) => setFormDomain(e.target.value)} placeholder="corp.example.com" className="font-data" />
            </div>
            <div className="space-y-2">
              <Label className={T.formLabel}>Upstream Server(s)</Label>
              <Input value={formUpstreams} onChange={(e) => setFormUpstreams(e.target.value)} placeholder="10.0.0.1, 10.0.0.2" className="font-data" />
              <p className={T.muted}>Comma-separated list of upstream DNS servers</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleSave} disabled={saving || !formDomain || !formUpstreams}>
              {saving ? "Saving..." : "Create"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
