import { useState, useEffect, useCallback, useRef } from "react";
import { Plus, Trash2, Pencil, Download, Upload, Play, Wand2 } from "lucide-react";
import { ConditionEditor, emptyTree, treeToExpression } from "./ConditionEditor";
import type { ConditionTree } from "./ConditionEditor";
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
  const [formLogic, setFormLogic] = useState("");
  const [formAction, setFormAction] = useState("BLOCK");
  const [formActionData, setFormActionData] = useState("");
  const [formEnabled, setFormEnabled] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testDomain, setTestDomain] = useState("");
  const [showVisualBuilder, setShowVisualBuilder] = useState(false);
  const [conditionTree, setConditionTree] = useState<ConditionTree>(emptyTree());

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
    setFormLogic("");
    setFormAction("BLOCK");
    setFormActionData("");
    setFormEnabled(true);
    setTestResult(null);
    setTestDomain("");
    setDialogOpen(true);
  }

  function openEditDialog(policy: Policy) {
    setEditingPolicy(policy);
    setFormName(policy.name);
    setFormLogic(policy.logic);
    setFormAction(policy.action);
    setFormActionData(policy.action_data || "");
    setFormEnabled(policy.enabled);
    setTestResult(null);
    setTestDomain("");
    setDialogOpen(true);
  }

  async function handleSave() {
    setSaving(true);
    try {
      const data = {
        name: formName,
        logic: formLogic,
        action: formAction,
        action_data: formActionData || undefined,
        enabled: formEnabled,
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

  async function handleDelete(id: number) {
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
      await updatePolicy(policy.id, {
        name: policy.name,
        logic: policy.logic,
        action: policy.action,
        action_data: policy.action_data,
        enabled: !policy.enabled,
      });
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to toggle policy");
    }
  }

  async function handleTest() {
    if (!formLogic.trim() || !testDomain.trim()) return;
    try {
      const result = await testPolicy(formLogic, testDomain);
      // Go returns {matched: bool}, map to our UI format
      setTestResult({ valid: result.matched ?? false });
    } catch (err) {
      setTestResult({ valid: false, error: err instanceof Error ? err.message : "Test failed" });
    }
  }

  const fileInputRef = useRef<HTMLInputElement>(null);

  async function handleImport(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;

    try {
      const text = await file.text();
      const data = JSON.parse(text);
      const imported: Array<{ name: string; logic: string; action: string; action_data?: string; enabled?: boolean }> =
        Array.isArray(data) ? data : data.policies ?? [];

      if (imported.length === 0) {
        setError("No policies found in import file");
        return;
      }

      let count = 0;
      for (const p of imported) {
        if (!p.name || !p.logic || !p.action) continue;
        await createPolicy({
          name: p.name,
          logic: p.logic,
          action: p.action.toUpperCase(),
          action_data: p.action_data ?? "",
          enabled: p.enabled ?? true,
        });
        count++;
      }

      await loadData();
      setError(null);
      // Show count inline — no toast needed
      alert(`Imported ${count} policies`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Import failed — check JSON format");
    } finally {
      // Reset the file input so the same file can be re-imported
      if (fileInputRef.current) fileInputRef.current.value = "";
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
          <input
            ref={fileInputRef}
            type="file"
            accept=".json"
            className="hidden"
            onChange={handleImport}
            aria-label="Import policies from JSON file"
          />
          <Button variant="outline" size="sm" onClick={() => fileInputRef.current?.click()}>
            <Upload className="h-3.5 w-3.5 mr-1" />
            Import
          </Button>
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
                <TableHead>Logic</TableHead>
                <TableHead className="w-[100px]">Action</TableHead>
                <TableHead className="w-[70px]">Enabled</TableHead>
                <TableHead className="w-[80px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {policies.map((policy) => (
                  <TableRow key={policy.id}>
                    <TableCell className={T.tableCellMono}>{policy.id}</TableCell>
                    <TableCell>
                      <div className={T.tableRowName}>{policy.name}</div>
                    </TableCell>
                    <TableCell className={cn(T.tableCellMono, "max-w-[300px] truncate")}>
                      {policy.logic}
                    </TableCell>
                    <TableCell>
                      <Badge
                        className={
                          policy.action === "BLOCK"
                            ? "bg-gh-red/20 text-gh-red border-gh-red/30"
                            : policy.action === "ALLOW"
                            ? "bg-gh-green/20 text-gh-green border-gh-green/30"
                            : policy.action === "REDIRECT"
                            ? "bg-gh-yellow/20 text-gh-yellow border-gh-yellow/30"
                            : "bg-gh-blue/20 text-gh-blue border-gh-blue/30"
                        }
                      >
                        {policy.action}
                      </Badge>
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
        <DialogContent className="max-w-2xl lg:max-w-3xl max-h-[90vh] overflow-y-auto">
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
                    <SelectItem value="BLOCK">Block</SelectItem>
                    <SelectItem value="ALLOW">Allow</SelectItem>
                    <SelectItem value="REDIRECT">Redirect</SelectItem>
                    <SelectItem value="FORWARD">Forward</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label className={T.formLabel}>Logic Expression</Label>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setShowVisualBuilder(!showVisualBuilder);
                    if (!showVisualBuilder) {
                      setConditionTree(emptyTree());
                    }
                  }}
                  className="h-6 text-xs px-2"
                >
                  <Wand2 className="h-3 w-3 mr-1" />
                  {showVisualBuilder ? "Text editor" : "Visual builder"}
                </Button>
              </div>

              {showVisualBuilder ? (
                <div className="space-y-2">
                  <ConditionEditor
                    tree={conditionTree}
                    onChange={(tree) => {
                      setConditionTree(tree);
                      setFormLogic(treeToExpression(tree));
                      setTestResult(null);
                    }}
                  />
                  <div className="rounded-md border border-border bg-muted/50 p-2">
                    <p className="text-[10px] text-muted-foreground mb-1">Generated expression:</p>
                    <code className="text-xs font-data text-foreground break-all">{formLogic || "—"}</code>
                  </div>
                </div>
              ) : (
                <Textarea
                  value={formLogic}
                  onChange={(e) => { setFormLogic(e.target.value); setTestResult(null); }}
                  placeholder='Domain == "ads.example.com" || DomainMatches(Domain, "tracking")'
                  className="font-data min-h-[80px]"
                />
              )}
            </div>

            {(formAction === "REDIRECT" || formAction === "FORWARD") && (
              <div className="space-y-2">
                <Label className={T.formLabel}>
                  {formAction === "REDIRECT" ? "Redirect IP" : "Upstream DNS servers"}
                </Label>
                <Input
                  value={formActionData}
                  onChange={(e) => setFormActionData(e.target.value)}
                  placeholder={formAction === "REDIRECT" ? "127.0.0.1" : "8.8.8.8:53,8.8.4.4:53"}
                  className="font-data"
                />
              </div>
            )}

            {/* Test expression */}
            <div className="space-y-2">
              <Label className={T.formLabel}>Test Expression (optional)</Label>
              <div className="flex items-center gap-2">
                <Input
                  value={testDomain}
                  onChange={(e) => { setTestDomain(e.target.value); setTestResult(null); }}
                  placeholder="ads.example.com"
                  className="font-data flex-1"
                />
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleTest}
                  disabled={!formLogic.trim() || !testDomain.trim()}
                >
                  <Play className="h-3 w-3 mr-1" />
                  Test
                </Button>
              </div>
              {testResult && (
                <div
                  className={cn(
                    "rounded-md px-3 py-2 text-xs",
                    testResult.valid
                      ? "border border-gh-green/30 bg-gh-green/10 text-gh-green"
                      : "border border-gh-red/30 bg-gh-red/10 text-gh-red"
                  )}
                >
                  {testResult.valid ? "Expression matched" : testResult.error || "Expression did not match"}
                </div>
              )}
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
            <Button onClick={handleSave} disabled={saving || !formName || !formLogic}>
              {saving ? "Saving..." : editingPolicy ? "Update" : "Create"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
