import { useEffect, useState } from "react";
import { clsx } from "clsx";
import { Play, Copy, X, Check, ChevronDown, Plus } from "lucide-react";
import { api, type TestResponse } from "../lib/api";
import { fmtDurationNs, fmtLeap, parseGoDurationNs } from "../lib/format";
import { useToast } from "../components/toast";
import { Button, Card, Badge, Dot, Spinner } from "../components/ui";
import { ConsensusRuler } from "../components/ConsensusRuler";

const DEFAULT_SERVERS = ["time.cloudflare.com", "time.google.com", "pool.ntp.org"];

export function QuickTest() {
  const toast = useToast();
  const [servers, setServers] = useState<string[]>(DEFAULT_SERVERS);
  const [draft, setDraft] = useState("");
  const [samples, setSamples] = useState(4);
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<TestResponse | null>(null);
  const [showDeltas, setShowDeltas] = useState(false);
  const [copied, setCopied] = useState(false);
  const [thresholdNs, setThresholdNs] = useState(100_000_000);

  useEffect(() => {
    api
      .getSettings()
      .then((s) => setThresholdNs(parseGoDurationNs(s.outlier_threshold) ?? 100_000_000))
      .catch(() => {});
  }, []);

  const commitDraft = () => {
    const parts = draft
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    if (parts.length) {
      setServers((prev) => Array.from(new Set([...prev, ...parts])));
    }
    setDraft("");
  };

  const removeServer = (s: string) => setServers((prev) => prev.filter((x) => x !== s));

  const run = async () => {
    if (servers.length === 0) {
      toast.error("Add at least one server to test.");
      return;
    }
    setRunning(true);
    setResult(null);
    try {
      const res = await api.test(servers, samples);
      setResult(res);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Test failed");
    } finally {
      setRunning(false);
    }
  };

  const copyJSON = async () => {
    if (!result) return;
    try {
      await navigator.clipboard.writeText(JSON.stringify(result, null, 2));
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error("Clipboard unavailable");
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight">Quick test</h1>
        <p className="mt-1 text-sm text-muted">
          Query several NTP servers at once and compare their offset, delay, and consensus.
        </p>
      </div>

      <Card className="p-4 sm:p-5">
        <label className="text-xs font-medium uppercase tracking-wide text-muted">Servers</label>
        <div className="mt-2 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-bg p-2">
          {servers.map((s) => (
            <span
              key={s}
              className="tnum inline-flex items-center gap-1.5 rounded-md bg-surface-2 py-1 pl-2.5 pr-1 text-sm"
            >
              {s}
              <button
                onClick={() => removeServer(s)}
                className="rounded p-0.5 text-muted hover:text-danger"
                aria-label={`Remove ${s}`}
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </span>
          ))}
          <div className="flex flex-1 items-center gap-1">
            <input
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === ",") {
                  e.preventDefault();
                  commitDraft();
                } else if (e.key === "Backspace" && !draft && servers.length) {
                  removeServer(servers[servers.length - 1]);
                }
              }}
              onBlur={commitDraft}
              placeholder={servers.length ? "add server…" : "host or host:port"}
              className="tnum min-w-[8rem] flex-1 bg-transparent px-1.5 py-1 text-sm outline-none placeholder:text-muted"
            />
            {draft && (
              <button onClick={commitDraft} className="rounded p-1 text-muted hover:text-text" aria-label="Add">
                <Plus className="h-4 w-4" />
              </button>
            )}
          </div>
        </div>

        <div className="mt-4 flex flex-wrap items-end justify-between gap-4">
          <div>
            <label className="text-xs font-medium uppercase tracking-wide text-muted">Samples per server</label>
            <div className="mt-2 flex overflow-hidden rounded-lg border border-border">
              {[1, 2, 4, 6, 8, 10].map((n) => (
                <button
                  key={n}
                  onClick={() => setSamples(n)}
                  className={clsx(
                    "tnum w-9 py-1.5 text-sm transition",
                    samples === n ? "bg-accent text-black" : "bg-surface text-muted hover:bg-surface-2",
                  )}
                >
                  {n}
                </button>
              ))}
            </div>
          </div>
          <Button onClick={run} disabled={running} className="min-w-[120px]">
            {running ? <Spinner /> : <Play className="h-4 w-4" />}
            {running ? "Testing…" : "Run test"}
          </Button>
        </div>
      </Card>

      {running && <RunningState servers={servers} />}

      {result && !running && (
        <>
          <Card className="p-4 sm:p-5">
            <div className="mb-4 flex items-center justify-between">
              <h2 className="font-display text-sm font-semibold uppercase tracking-wide text-muted">
                Consensus
              </h2>
              <ConsensusSummary result={result} />
            </div>
            <ConsensusRuler
              results={result.results}
              medianNs={result.comparison.median_offset}
              outliers={result.comparison.outliers ?? []}
              thresholdNs={thresholdNs}
            />
          </Card>

          <Card>
            <div className="flex items-center justify-between border-b border-border px-4 py-3">
              <h2 className="font-display text-sm font-semibold uppercase tracking-wide text-muted">
                Per-server results
              </h2>
              <Button variant="ghost" onClick={copyJSON} className="text-xs">
                {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                {copied ? "Copied" : "Copy JSON"}
              </Button>
            </div>
            <ResultsTable result={result} />
          </Card>

          {result.comparison.pairwise && result.comparison.labels.length > 1 && (
            <Card className="p-4 sm:p-5">
              <button
                onClick={() => setShowDeltas((v) => !v)}
                className="flex w-full items-center justify-between text-sm font-medium"
              >
                <span className="font-display uppercase tracking-wide text-muted">
                  Pairwise offset deltas
                </span>
                <ChevronDown className={clsx("h-4 w-4 text-muted transition", showDeltas && "rotate-180")} />
              </button>
              {showDeltas && <DeltaMatrix result={result} />}
            </Card>
          )}
        </>
      )}
    </div>
  );
}

function ConsensusSummary({ result }: { result: TestResponse }) {
  const outliers = result.comparison.outliers ?? [];
  const reachable = result.results.filter((r) => r.reachable).length;
  const total = result.results.length;
  return (
    <div className="flex items-center gap-2">
      <Badge tone="muted">
        {reachable}/{total} reachable
      </Badge>
      {outliers.length > 0 ? (
        <Badge tone="warn">
          {outliers.length} falseticker{outliers.length > 1 ? "s" : ""}
        </Badge>
      ) : reachable > 0 ? (
        <Badge tone="ok">in agreement</Badge>
      ) : null}
    </div>
  );
}

function ResultsTable({ result }: { result: TestResponse }) {
  const outliers = new Set(result.comparison.outliers ?? []);
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[720px] text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted">
            <th className="px-4 py-2 font-medium">Server</th>
            <th className="px-4 py-2 font-medium">Offset</th>
            <th className="px-4 py-2 font-medium">RTT</th>
            <th className="px-4 py-2 font-medium">Jitter</th>
            <th className="px-4 py-2 font-medium">Stratum</th>
            <th className="px-4 py-2 font-medium">Ref ID</th>
            <th className="px-4 py-2 font-medium">Leap</th>
            <th className="px-4 py-2 font-medium">Resolved</th>
          </tr>
        </thead>
        <tbody>
          {result.results.map((r) => {
            const out = outliers.has(r.target);
            return (
              <tr key={r.target} className="border-b border-border/60 last:border-0">
                <td className="px-4 py-2.5">
                  <div className="flex items-center gap-2">
                    <Dot tone={r.reachable ? "ok" : "danger"} />
                    <span className="tnum font-medium">{r.target}</span>
                    {out && <Badge tone="warn">outlier</Badge>}
                  </div>
                </td>
                {r.reachable ? (
                  <>
                    <td className={clsx("tnum px-4 py-2.5", out && "text-warn")}>
                      {fmtDurationNs(r.offset, true)}
                    </td>
                    <td className="tnum px-4 py-2.5 text-muted">{fmtDurationNs(r.rtt)}</td>
                    <td className="tnum px-4 py-2.5 text-muted">{fmtDurationNs(r.jitter)}</td>
                    <td className="tnum px-4 py-2.5">{r.stratum}</td>
                    <td className="tnum px-4 py-2.5 text-muted">{r.reference_id || "—"}</td>
                    <td className="px-4 py-2.5 text-muted">{fmtLeap(r.leap)}</td>
                    <td className="tnum px-4 py-2.5 text-muted">{r.resolved_ip || "—"}</td>
                  </>
                ) : (
                  <td colSpan={7} className="px-4 py-2.5 text-danger">
                    {r.error || "unreachable"}
                  </td>
                )}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function DeltaMatrix({ result }: { result: TestResponse }) {
  const { labels, pairwise } = result.comparison;
  if (!pairwise) return null;
  return (
    <div className="mt-3 overflow-x-auto">
      <table className="min-w-full text-xs">
        <thead>
          <tr>
            <th className="p-2" />
            {labels.map((l) => (
              <th key={l} className="tnum p-2 text-right font-medium text-muted">
                {l.replace(/:123$/, "")}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {labels.map((row, i) => (
            <tr key={row}>
              <td className="tnum p-2 font-medium text-muted">{row.replace(/:123$/, "")}</td>
              {labels.map((_, j) => (
                <td
                  key={j}
                  className={clsx(
                    "tnum p-2 text-right",
                    i === j ? "text-muted/40" : Math.abs(pairwise[i][j]) > 0 ? "text-text" : "text-muted",
                  )}
                >
                  {i === j ? "·" : fmtDurationNs(pairwise[i][j], true)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function RunningState({ servers }: { servers: string[] }) {
  return (
    <Card className="p-4 sm:p-5">
      <div className="mb-3 flex items-center gap-2 text-sm text-muted">
        <Spinner /> Querying {servers.length} server{servers.length > 1 ? "s" : ""}…
      </div>
      <div className="space-y-2">
        {servers.map((s) => (
          <div key={s} className="flex items-center gap-3">
            <span className="tnum w-48 truncate text-sm text-muted">{s}</span>
            <div className="h-2 flex-1 overflow-hidden rounded-full bg-surface-2">
              <div className="h-full w-1/3 animate-pulse rounded-full bg-accent/40" />
            </div>
          </div>
        ))}
      </div>
    </Card>
  );
}
