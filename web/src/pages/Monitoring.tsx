import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Plus, Trash2, Pencil, ChevronRight, X } from "lucide-react";
import { api, type Server, type Point } from "../lib/api";
import { fmtSeconds, fmtClock } from "../lib/format";
import { useToast } from "../components/toast";
import { Button, Card, Input, Field, Toggle, Dot, Badge, EmptyState, Spinner } from "../components/ui";
import { Sparkline, OffsetChart, SERIES_COLORS, type ChartRow, type ChartSeries } from "../components/charts";

interface History {
  points: Point[];
}

export function Monitoring() {
  const toast = useToast();
  const [servers, setServers] = useState<Server[] | null>(null);
  const [history, setHistory] = useState<Record<string, History>>({});
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Server | null>(null);

  const loadServers = async () => {
    try {
      setServers(await api.listServers());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to load servers");
    }
  };

  useEffect(() => {
    loadServers();
    const t = setInterval(loadServers, 60_000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Fetch recent history for enabled servers (5m-downsampled, last 6h).
  useEffect(() => {
    if (!servers) return;
    let alive = true;
    const from = new Date(Date.now() - 6 * 3600_000).toISOString();
    const load = async () => {
      const entries = await Promise.all(
        servers
          .filter((s) => s.enabled)
          .map(async (s) => {
            try {
              const m = await api.measurements(s.id, { from, step: "5m" });
              return [s.id, { points: m.points }] as const;
            } catch {
              return [s.id, { points: [] }] as const;
            }
          }),
      );
      if (alive) setHistory(Object.fromEntries(entries));
    };
    load();
    const t = setInterval(load, 60_000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, [servers]);

  const overlay = useMemo(() => buildOverlay(servers ?? [], history), [servers, history]);

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="font-display text-2xl font-semibold tracking-tight">Monitoring</h1>
          <p className="mt-1 text-sm text-muted">
            Saved servers polled on a schedule. Offsets and reachability update as new samples arrive.
          </p>
        </div>
        <Button onClick={() => setAdding(true)}>
          <Plus className="h-4 w-4" /> Add server
        </Button>
      </div>

      {(adding || editing) && (
        <ServerForm
          server={editing}
          onClose={() => {
            setAdding(false);
            setEditing(null);
          }}
          onSaved={() => {
            setAdding(false);
            setEditing(null);
            loadServers();
          }}
        />
      )}

      {servers === null ? (
        <Card className="flex items-center gap-2 p-6 text-sm text-muted">
          <Spinner /> Loading servers…
        </Card>
      ) : servers.length === 0 ? (
        <EmptyState title="No servers yet">
          Add a server such as <span className="tnum">time.cloudflare.com</span> to start tracking its
          offset and drift over time.
        </EmptyState>
      ) : (
        <>
          {overlay.series.length > 0 && overlay.rows.length > 1 && (
            <Card className="p-4 sm:p-5">
              <h2 className="mb-3 font-display text-sm font-semibold uppercase tracking-wide text-muted">
                Offset over time
              </h2>
              <OffsetChart rows={overlay.rows} series={overlay.series} />
              <div className="mt-3 flex flex-wrap gap-3">
                {overlay.series.map((s) => (
                  <span key={s.key} className="flex items-center gap-1.5 text-xs text-muted">
                    <span className="h-2.5 w-2.5 rounded-full" style={{ background: s.color }} />
                    {s.label}
                  </span>
                ))}
              </div>
            </Card>
          )}

          <div className="grid gap-3">
            {servers.map((s) => (
              <ServerRow
                key={s.id}
                server={s}
                points={history[s.id]?.points ?? []}
                onToggle={async (enabled) => {
                  try {
                    await api.updateServer(s.id, s.address, s.label, enabled);
                    loadServers();
                  } catch (e) {
                    toast.error(e instanceof Error ? e.message : "Update failed");
                  }
                }}
                onEdit={() => setEditing(s)}
                onDelete={async () => {
                  try {
                    await api.deleteServer(s.id);
                    toast.success(`Removed ${s.label || s.address}`);
                    loadServers();
                  } catch (e) {
                    toast.error(e instanceof Error ? e.message : "Delete failed");
                  }
                }}
              />
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function ServerRow({
  server,
  points,
  onToggle,
  onEdit,
  onDelete,
}: {
  server: Server;
  points: Point[];
  onToggle: (v: boolean) => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const last = points[points.length - 1];
  const reachable = last?.reachable ?? false;
  const spark = points.filter((p) => p.reachable).map((p) => p.offset_seconds);

  return (
    <Card className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <Dot tone={!server.enabled ? "muted" : reachable ? "ok" : "danger"} />
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="truncate font-medium">{server.label || server.address}</span>
            {!server.enabled && <Badge tone="muted">paused</Badge>}
          </div>
          {server.label && <span className="tnum text-xs text-muted">{server.address}</span>}
        </div>
      </div>

      <div className="hidden w-40 sm:block">
        {spark.length > 1 ? (
          <Sparkline data={spark} color={reachable ? "var(--accent)" : "var(--danger)"} />
        ) : (
          <div className="h-9" />
        )}
      </div>

      <div className="w-28 text-right">
        {last ? (
          reachable ? (
            <div>
              <div className="tnum text-sm font-medium">{fmtSeconds(last.offset_seconds, true)}</div>
              <div className="tnum text-[11px] text-muted">{fmtClock(last.ts)}</div>
            </div>
          ) : (
            <Badge tone="danger">unreachable</Badge>
          )
        ) : (
          <span className="text-xs text-muted">no data</span>
        )}
      </div>

      <div className="flex items-center gap-1">
        <Toggle checked={server.enabled} onChange={onToggle} label="Enabled" />
        <Button variant="ghost" onClick={onEdit} aria-label="Edit" className="px-2">
          <Pencil className="h-4 w-4" />
        </Button>
        <Button variant="ghost" onClick={onDelete} aria-label="Delete" className="px-2 text-muted hover:text-danger">
          <Trash2 className="h-4 w-4" />
        </Button>
        <Link
          to={`/monitoring/${server.id}`}
          className="rounded-lg p-2 text-muted hover:bg-surface-2 hover:text-text"
          aria-label="Details"
        >
          <ChevronRight className="h-4 w-4" />
        </Link>
      </div>
    </Card>
  );
}

function ServerForm({
  server,
  onClose,
  onSaved,
}: {
  server: Server | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const toast = useToast();
  const [address, setAddress] = useState(server?.address ?? "");
  const [label, setLabel] = useState(server?.label ?? "");
  const [enabled, setEnabled] = useState(server?.enabled ?? true);
  const [saving, setSaving] = useState(false);

  const save = async () => {
    if (!address.trim()) {
      toast.error("Enter a server address.");
      return;
    }
    setSaving(true);
    try {
      if (server) {
        await api.updateServer(server.id, address.trim(), label.trim(), enabled);
        toast.success("Server updated");
      } else {
        await api.createServer(address.trim(), label.trim(), enabled);
        toast.success("Server added");
      }
      onSaved();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card className="p-4 sm:p-5">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="font-display text-sm font-semibold uppercase tracking-wide text-muted">
          {server ? "Edit server" : "Add server"}
        </h2>
        <button onClick={onClose} className="text-muted hover:text-text" aria-label="Close">
          <X className="h-4 w-4" />
        </button>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Address" hint="host or host:port">
          <Input
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            placeholder="time.cloudflare.com"
            className="tnum"
          />
        </Field>
        <Field label="Label" hint="optional friendly name">
          <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Cloudflare" />
        </Field>
      </div>
      <div className="mt-4 flex items-center justify-between">
        <label className="flex items-center gap-2 text-sm">
          <Toggle checked={enabled} onChange={setEnabled} label="Enabled" />
          <span className="text-muted">Poll on schedule</span>
        </label>
        <div className="flex gap-2">
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={save} disabled={saving}>
            {saving ? <Spinner /> : null}
            {server ? "Save changes" : "Add server"}
          </Button>
        </div>
      </div>
    </Card>
  );
}

// buildOverlay merges each enabled server's points into a shared time series.
// The scheduler stamps every server in a poll cycle with the same ts, so rows
// align by timestamp across servers.
function buildOverlay(
  servers: Server[],
  history: Record<string, History>,
): { rows: ChartRow[]; series: ChartSeries[] } {
  const enabled = servers.filter((s) => s.enabled && (history[s.id]?.points.length ?? 0) > 0);
  const series: ChartSeries[] = enabled.map((s, i) => ({
    key: s.id,
    label: s.label || s.address,
    color: SERIES_COLORS[i % SERIES_COLORS.length],
  }));
  const byTs = new Map<string, ChartRow>();
  for (const s of enabled) {
    for (const p of history[s.id].points) {
      if (!p.reachable) continue;
      let row = byTs.get(p.ts);
      if (!row) {
        row = { ts: p.ts };
        byTs.set(p.ts, row);
      }
      row[s.id] = p.offset_seconds;
    }
  }
  const rows = Array.from(byTs.values()).sort((a, b) => (a.ts < b.ts ? -1 : 1));
  return { rows, series };
}
