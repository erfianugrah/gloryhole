import { useState, useEffect, useCallback } from "react";
import { Plus, Trash2, Pencil, Download, Play } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
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
import type { Policy } from "@/lib/api";
import {
  fetchPolicies,
  createPolicy,
  updatePolicy,
  deletePolicy,
  testPolicy,
  exportPolicies,
} from "@/lib/api";

// ─── Component ──────────────────────────────────────────────────────

export function PoliciesPage() {
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingPolicy, setEditingPolicy] = useState<Policy | null>(null);
  const [testResult, setTestResult] = useState<{ valid: boolean; error?: string } | null>(null);

  // Form state
  const [formName, setFormName] = useState("");
  const [formExpression, setFormExpression] = useState("");
  const [formAction, setFormAction] = useState("block");
  const [formEnabled, setFormEnabled] = useState(true);
  const [formPriority, setFormPriority] = useState(0);
  const [formDescription, setFormDescription] = useState("");
  const [formClientFilter, setFormClientFilter] = useState("");
  const [formGroupFilter, setFormGroupFilter] = useState("");
  const [saving, setSaving] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const data = await fetchPolicies();
      setPolicies(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load policies");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  function openCreateDialog() {
    setEditingPolicy(null);
    setFormName("");
    setFormExpression("");
    setFormAction("block");
    setFormEnabled(true);
    setFormPriority(policies.length);
    setFormDescription("");
    setFormClientFilter("");
    setFormGroupFilter("");
    setTestResult(null);
    setDialogOpen(true);
  }

  function openEditDialog(policy: Policy) {
    setEditingPolicy(policy);
    setFormName(policy.name);
    setFormExpression(policy.expression);
    setFormAction(policy.action);
    setFormEnabled(policy.enabled);
    setFormPriority(policy.priority);
    setFormDescription(policy.description || "");
    setFormClientFilter(policy.client_filter || "");
    setFormGroupFilter(policy.group_filter || "");
    setTestResult(null);
    setDialogOpen(true);
  }

  async function handleSave() {
    setSaving(true);
    try {
      const data = {
        name: formName,
        expression: formExpression,
        action: formAction,
        enabled: formEnabled,
        priority: formPriority,
        description: formDescription || undefined,
        client_filter: formClientFilter || undefined,
        group_filter: formGroupFilter || undefined,
      };

      if (editingPolicy) {
        await updatePolicy(editingPolicy.id, data);
      } else {
        await createPolicy(data as Omit<Policy, "id">);
      }
      setDialogOpen(false);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save policy");
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this policy?")) return;
    try {
      await deletePolicy(id);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete policy");
    }
  }

  async function handleToggle(policy: Policy) {
    try {
      await updatePolicy(policy.id, { enabled: !policy.enabled });
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to toggle policy");
    }
  }

  async function handleTest() {
    try {
      const result = await testPolicy(formExpression);
      setTestResult(result);
    } catch (err) {
      setTestResult({ valid: false, error: err instanceof Error ? err.message : "Test failed" });
    }
  }

  async function handleExport() {
    try {
      const data = await exportPolicies();
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "policies.json";
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Export failed");
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className={T.pageTitle}>Policies</h2>
          <p className={T.pageDescription}>DNS filtering policies with expression-based rules</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={handleExport}>
            <Download className="h-3.5 w-3.5 mr-1" />
            Export
          </Button>
          <Button size="sm" onClick={openCreateDialog}>
            <Plus className="h-3.5 w-3.5 mr-1" />
            Add Policy
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-gh-red/30 bg-gh-red/10 px-4 py-3 text-sm text-gh-red">
          {error}
        </div>
      )}

      <Card>
        {loading ? (
          <CardContent className="p-4 space-y-2">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </CardContent>
        ) : policies.length === 0 ? (
          <CardContent className="p-8 text-center">
            <p className={T.mutedSm}>No policies configured</p>
            <Button size="sm" className="mt-4" onClick={openCreateDialog}>
              <Plus className="h-3.5 w-3.5 mr-1" />
              Create your first policy
            </Button>
          </CardContent>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[50px]">#</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Expression</TableHead>
                <TableHead className="w-[80px]">Action</TableHead>
                <TableHead className="w-[80px]">Scope</TableHead>
                <TableHead className="w-[70px]">Enabled</TableHead>
                <TableHead className="w-[80px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {policies
                .sort((a, b) => a.priority - b.priority)
                .map((policy) => (
                  <TableRow key={policy.id}>
                    <TableCell className={T.tableCellMono}>{policy.priority}</TableCell>
                    <TableCell>
                      <div className={T.tableRowName}>{policy.name}</div>
                      {policy.description && (
                        <div className={T.muted}>{policy.description}</div>
                      )}
                    </TableCell>
                    <TableCell className={cn(T.tableCellMono, "max-w-[300px] truncate")}>
                      {policy.expression}
                    </TableCell>
                    <TableCell>
                      <Badge
                        className={
                          policy.action === "block"
                            ? "bg-gh-red/20 text-gh-red border-gh-red/30"
                            : "bg-gh-green/20 text-gh-green border-gh-green/30"
                        }
                      >
                        {policy.action}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {(policy.client_filter || policy.group_filter) ? (
                        <Badge variant="outline" className="text-[10px]">
                          {policy.client_filter || policy.group_filter}
                        </Badge>
                      ) : (
                        <span className={T.muted}>All</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={policy.enabled}
                        onCheckedChange={() => handleToggle(policy)}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => openEditDialog(policy)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => handleDelete(policy.id)}
                          className="text-gh-red hover:text-gh-red"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
        )}
      </Card>

      {/* Create / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>
              {editingPolicy ? "Edit Policy" : "Create Policy"}
            </DialogTitle>
            <DialogDescription>
              Policies use expr-lang expressions to match DNS queries.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className={T.formLabel}>Name</Label>
                <Input
                  value={formName}
                  onChange={(e) => setFormName(e.target.value)}
                  placeholder="Block ads"
                />
              </div>
              <div className="space-y-2">
                <Label className={T.formLabel}>Action</Label>
                <Select value={formAction} onValueChange={setFormAction}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="block">Block</SelectItem>
                    <SelectItem value="allow">Allow</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label className={T.formLabel}>Expression</Label>
                <Button
                  variant="ghost"
                  size="xs"
                  onClick={handleTest}
                  disabled={!formExpression.trim()}
                >
                  <Play className="h-3 w-3 mr-1" />
                  Test
                </Button>
              </div>
              <Textarea
                value={formExpression}
                onChange={(e) => { setFormExpression(e.target.value); setTestResult(null); }}
                placeholder='domain matches "*.ads.example.com"'
                className="font-data min-h-[80px]"
              />
              {testResult && (
                <div
                  className={cn(
                    "rounded-md px-3 py-2 text-xs",
                    testResult.valid
                      ? "border border-gh-green/30 bg-gh-green/10 text-gh-green"
                      : "border border-gh-red/30 bg-gh-red/10 text-gh-red"
                  )}
                >
                  {testResult.valid ? "Expression is valid" : testResult.error}
                </div>
              )}
            </div>

            <div className="space-y-2">
              <Label className={T.formLabel}>Description (optional)</Label>
              <Input
                value={formDescription}
                onChange={(e) => setFormDescription(e.target.value)}
                placeholder="Block advertising domains"
              />
            </div>

            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label className={T.formLabel}>Priority</Label>
                <Input
                  type="number"
                  value={formPriority}
                  onChange={(e) => setFormPriority(Number(e.target.value))}
                />
              </div>
              <div className="space-y-2">
                <Label className={T.formLabel}>Client filter</Label>
                <Input
                  value={formClientFilter}
                  onChange={(e) => setFormClientFilter(e.target.value)}
                  placeholder="192.168.1.*"
                />
              </div>
              <div className="space-y-2">
                <Label className={T.formLabel}>Group filter</Label>
                <Input
                  value={formGroupFilter}
                  onChange={(e) => setFormGroupFilter(e.target.value)}
                  placeholder="kids"
                />
              </div>
            </div>

            <div className="flex items-center gap-2">
              <Switch checked={formEnabled} onCheckedChange={setFormEnabled} />
              <Label className="text-sm">Enabled</Label>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleSave} disabled={saving || !formName || !formExpression}>
              {saving ? "Saving..." : editingPolicy ? "Update" : "Create"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
