import { useState, useEffect, useCallback } from "react";
import { Search, X, Pencil } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
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
import { cn } from "@/lib/utils";
import { T } from "@/lib/typography";
import { TablePagination } from "./TablePagination";
import type { ClientSummary } from "@/lib/api";
import { fetchClients, updateClient } from "@/lib/api";

export function ClientsPage() {
  const [clients, setClients] = useState<ClientSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [editDialog, setEditDialog] = useState(false);
  const [editingClient, setEditingClient] = useState<ClientSummary | null>(null);
  const [editName, setEditName] = useState("");
  const [editGroup, setEditGroup] = useState("");
  const [saving, setSaving] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const data = await fetchClients(pageSize, (page - 1) * pageSize, search || undefined);
      setClients(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load clients");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, search]);

  useEffect(() => { loadData(); }, [loadData]);
  useEffect(() => { setPage(1); }, [search]);

  function openEdit(client: ClientSummary) {
    setEditingClient(client);
    setEditName(client.display_name);
    setEditGroup(client.group_name || "");
    setEditDialog(true);
  }

  async function handleSave() {
    if (!editingClient) return;
    setSaving(true);
    try {
      await updateClient(editingClient.client_ip, { display_name: editName, group_name: editGroup });
      setEditDialog(false);
      await loadData();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update client");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className={T.pageTitle}>Clients</h2>
        <p className={T.pageDescription}>Connected DNS clients and group management</p>
      </div>

      {error && (
        <div className="rounded-lg border border-gh-red/30 bg-gh-red/10 px-4 py-3 text-sm text-gh-red flex items-center justify-between">
          <span>{error}</span>
          <Button variant="outline" size="xs" onClick={() => { setError(null); setLoading(true); loadData(); }}>
            Retry
          </Button>
        </div>
      )}

      <Card>
        <CardContent className="p-4">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Search clients..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9 font-data"
            />
            {search && (
              <button onClick={() => setSearch("")} className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                <X className="h-3 w-3" />
              </button>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        {loading ? (
          <CardContent className="p-4 space-y-2">
            {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
          </CardContent>
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Client IP</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Group</TableHead>
                  <TableHead className="text-right">Queries</TableHead>
                  <TableHead className="text-right">Blocked</TableHead>
                  <TableHead>Last Seen</TableHead>
                  <TableHead className="w-[50px]"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {clients.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} className="text-center py-8">
                      <span className={T.mutedSm}>No clients found</span>
                    </TableCell>
                  </TableRow>
                ) : (
                  clients.map((c) => (
                    <TableRow key={c.client_ip}>
                      <TableCell className={T.tableCellMono}>{c.client_ip}</TableCell>
                      <TableCell className={T.tableRowName}>{c.display_name || <span className={T.muted}>—</span>}</TableCell>
                      <TableCell>
                        {c.group_name ? (
                          <Badge variant="outline" className="text-[10px]">{c.group_name}</Badge>
                        ) : (
                          <span className={T.muted}>—</span>
                        )}
                      </TableCell>
                      <TableCell className={T.tableCellNumeric}>{c.total_queries.toLocaleString()}</TableCell>
                      <TableCell className={cn(T.tableCellNumeric, "text-gh-red")}>{c.blocked_queries.toLocaleString()}</TableCell>
                      <TableCell className={T.tableCellMono}>
                        {c.last_seen ? new Date(c.last_seen).toLocaleString() : "—"}
                      </TableCell>
                      <TableCell>
                        <Button variant="ghost" size="icon-sm" onClick={() => openEdit(c)}>
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>

            <TablePagination
              page={page}
              totalPages={Math.max(1, clients.length === pageSize ? page + 1 : page)}
              pageSize={pageSize}
              pageSizeOptions={[25, 50, 100]}
              hasPrev={page > 1}
              hasNext={clients.length === pageSize}
              onPageChange={setPage}
              onPageSizeChange={(s) => { setPageSize(s); setPage(1); }}
            />
          </>
        )}
      </Card>

      <Dialog open={editDialog} onOpenChange={setEditDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit Client — {editingClient?.client_ip}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label className={T.formLabel}>Name</Label>
              <Input value={editName} onChange={(e) => setEditName(e.target.value)} placeholder="Living room PC" />
            </div>
            <div className="space-y-2">
              <Label className={T.formLabel}>Group</Label>
              <Input value={editGroup} onChange={(e) => setEditGroup(e.target.value)} placeholder="default" />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditDialog(false)}>Cancel</Button>
            <Button onClick={handleSave} disabled={saving}>{saving ? "Saving..." : "Save"}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
