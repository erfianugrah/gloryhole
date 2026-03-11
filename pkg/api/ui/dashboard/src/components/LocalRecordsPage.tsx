import { useState, useEffect, useCallback } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
import type { LocalRecord } from "@/lib/api";
import { fetchLocalRecords, createLocalRecord, deleteLocalRecord } from "@/lib/api";

const RECORD_TYPES = ["A", "AAAA", "CNAME", "MX", "TXT", "SRV", "PTR"];

export function LocalRecordsPage() {
  const [records, setRecords] = useState<LocalRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [saving, setSaving] = useState(false);

  const [formDomain, setFormDomain] = useState("");
  const [formType, setFormType] = useState("A");
  const [formValue, setFormValue] = useState("");
  const [formTTL, setFormTTL] = useState(300);

  const loadData = useCallback(async () => {
    try {
      const data = await fetchLocalRecords();
      setRecords(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load records");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  function openDialog() {
    setFormDomain("");
    setFormType("A");
    setFormValue("");
    setFormTTL(300);
    setDialogOpen(true);
  }

  async function handleSave() {
    setSaving(true);
    try {
      await createLocalRecord({ domain: formDomain, type: formType, value: formValue, ttl: formTTL });
      setDialogOpen(false);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create record");
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this record?")) return;
    try {
      await deleteLocalRecord(id);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete record");
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className={T.pageTitle}>Local Records</h2>
          <p className={T.pageDescription}>Custom DNS records served by the local resolver</p>
        </div>
        <Button size="sm" onClick={openDialog}>
          <Plus className="h-3.5 w-3.5 mr-1" /> Add Record
        </Button>
      </div>

      {error && (
        <div className="rounded-lg border border-gh-pink/30 bg-gh-pink/10 px-4 py-3 text-sm text-gh-pink">{error}</div>
      )}

      <Card>
        {loading ? (
          <CardContent className="p-4 space-y-2">
            {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}
          </CardContent>
        ) : records.length === 0 ? (
          <CardContent className="p-8 text-center">
            <p className={T.mutedSm}>No local records configured</p>
            <Button size="sm" className="mt-4" onClick={openDialog}>
              <Plus className="h-3.5 w-3.5 mr-1" /> Add your first record
            </Button>
          </CardContent>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead className="w-[80px]">Type</TableHead>
                <TableHead>Value</TableHead>
                <TableHead className="w-[80px] text-right">TTL</TableHead>
                <TableHead className="w-[50px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {records.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className={T.tableCellMono}>{r.domain}</TableCell>
                  <TableCell>
                    <Badge variant="outline" className="text-[10px]">{r.type}</Badge>
                  </TableCell>
                  <TableCell className={T.tableCellMono}>{r.value}</TableCell>
                  <TableCell className={T.tableCellNumeric}>{r.ttl}s</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="icon-sm" onClick={() => handleDelete(r.id)} className="text-gh-pink hover:text-gh-pink">
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
            <DialogTitle>Add Local Record</DialogTitle>
            <DialogDescription>Create a custom DNS record served by the local resolver.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label className={T.formLabel}>Domain</Label>
              <Input value={formDomain} onChange={(e) => setFormDomain(e.target.value)} placeholder="myapp.local" className="font-data" />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className={T.formLabel}>Type</Label>
                <Select value={formType} onValueChange={setFormType}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {RECORD_TYPES.map((t) => <SelectItem key={t} value={t}>{t}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label className={T.formLabel}>TTL (seconds)</Label>
                <Input type="number" value={formTTL} onChange={(e) => setFormTTL(Number(e.target.value))} />
              </div>
            </div>
            <div className="space-y-2">
              <Label className={T.formLabel}>Value</Label>
              <Input value={formValue} onChange={(e) => setFormValue(e.target.value)} placeholder="192.168.1.100" className="font-data" />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
            <Button onClick={handleSave} disabled={saving || !formDomain || !formValue}>
              {saving ? "Saving..." : "Create"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
