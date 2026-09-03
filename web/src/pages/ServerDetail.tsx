import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { clsx } from "clsx";
import { ArrowLeft, Activity, Gauge, TrendingUp } from "lucide-react";
import { api, type Server, type Point, type Drift } from "../lib/api";
import { fmtSeconds, fmtPPM } from "../lib/format";
import { useToast } from "../components/toast";
import { Card, Badge, Dot, Spinner, EmptyState } from "../components/ui";
import { OffsetChart, type ChartRow } from "../components/charts";

const RANGES = [
  { label: "1h", window: "1h", step: "1m" },
  { label: "6h", window: "6h", step: "5m" },
  { label: "24h", window: "24h", step: "15m" },
  { label: "7d", window: "168h", step: "1h" },
];

export function ServerDetail() {
  const { id = "" } = useParams();
  const toast = useToast();
  const [server, setServer] = useState<Server | null>(null);
  const [points, setPoints] = useState<Point[] | null>(null);
  const [drift, setDrift] = useState<Drift | null>(null);
  const [range, setRange] = useState(RANGES[1]);

  useEffect(() => {
    api.getServer(id).then(setServer).catch((e) => toast.error(e.message));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  useEffect(() => {
    let alive = true;
    const from = new Date(Date.now() - windowMs(range.window)).toISOString();
    const load = () =>
      Promise.all([
        api.measurements(id, { from, step: range.step }),
        api.drift(id, range.window),
      ])
        .then(([m, d]) => {
          if (!alive) return;
          setPoints(m.points);
          setDrift(d);
        })
        .catch(() => {});
    load();
    const t = setInterval(load, 60_000);
    return () => {
      alive = false;
      clearInterval(t);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, range]);

  const offsetRows: ChartRow[] =
    points?.filter((p) => p.reachable).map((p) => ({ ts: p.ts, offset: p.offset_seconds })) ?? [];
  const rttRows: ChartRow[] =
    points?.filter((p) => p.reachable).map((p) => ({ ts: p.ts, rtt: p.rtt_seconds })) ?? [];

  const last = points?.[points.length - 1];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Link to="/monitoring" className="rounded-lg p-2 text-muted hover:bg-surface-2 hover:text-text">
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1 className="font-display text-xl font-semibold tracking-tight">
              {server?.label || server?.address || "Server"}
            </h1>
            {server && <span className="tnum text-xs text-muted">{server.address}</span>}
          </div>
          {server && (
            <Badge tone={server.enabled ? "ok" : "muted"}>{server.enabled ? "monitored" : "paused"}</Badge>
          )}
        </div>
        <div className="flex overflow-hidden rounded-lg border border-border">
          {RANGES.map((r) => (
            <button
              key={r.label}
              onClick={() => setRange(r)}
              className={clsx(
                "tnum px-3 py-1.5 text-sm transition",
                range.label === r.label ? "bg-accent text-black" : "bg-surface text-muted hover:bg-surface-2",
              )}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <Stat
          icon={Activity}
          label="Current offset"
          value={last && last.reachable ? fmtSeconds(last.offset_seconds, true) : "—"}
          tone={last?.reachable ? "ok" : "danger"}
        />
        <Stat
          icon={Gauge}
          label="Current delay"
          value={last && last.reachable ? fmtSeconds(last.rtt_seconds) : "—"}
        />
        <Stat
          icon={TrendingUp}
          label={`Drift (${range.label})`}
          value={drift?.drift_ppm != null ? fmtPPM(drift.drift_ppm) : "—"}
          hint={
            drift?.drift_ppm != null
              ? `R² ${drift.r2?.toFixed(3)} · ${drift.samples_used} samples`
              : drift?.message
          }
        />
      </div>

      {points === null ? (
        <Card className="flex items-center gap-2 p-6 text-sm text-muted">
          <Spinner /> Loading history…
        </Card>
      ) : offsetRows.length === 0 ? (
        <EmptyState title="No measurements in this range">
          Once the scheduler has polled this server, its offset and delay history will appear here.
        </EmptyState>
      ) : (
        <>
          <Card className="p-4 sm:p-5">
            <h2 className="mb-3 font-display text-sm font-semibold uppercase tracking-wide text-muted">
              Offset over time
            </h2>
            <OffsetChart rows={offsetRows} series={[{ key: "offset", label: "offset", color: "var(--accent)" }]} />
          </Card>
          <Card className="p-4 sm:p-5">
            <h2 className="mb-3 font-display text-sm font-semibold uppercase tracking-wide text-muted">
              Round-trip delay
            </h2>
            <OffsetChart rows={rttRows} series={[{ key: "rtt", label: "rtt", color: "var(--accent-2)" }]} />
          </Card>
        </>
      )}
    </div>
  );
}

function Stat({
  icon: Icon,
  label,
  value,
  hint,
  tone = "muted",
}: {
  icon: typeof Activity;
  label: string;
  value: string;
  hint?: string;
  tone?: "ok" | "danger" | "muted";
}) {
  return (
    <Card className="p-4">
      <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-muted">
        <Icon className="h-3.5 w-3.5" /> {label}
      </div>
      <div className="mt-2 flex items-center gap-2">
        {tone !== "muted" && <Dot tone={tone} />}
        <span className="tnum text-xl font-semibold">{value}</span>
      </div>
      {hint && <div className="tnum mt-1 text-xs text-muted">{hint}</div>}
    </Card>
  );
}

function windowMs(w: string): number {
  const h = parseInt(w, 10);
  return (isNaN(h) ? 6 : h) * 3600_000;
}
