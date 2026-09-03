import { useMemo } from "react";
import { clsx } from "clsx";
import type { ServerResult } from "../lib/api";
import { fmtDurationNs } from "../lib/format";

// The consensus ruler is Cronus's signature view: every reachable server is a
// mark on a shared offset axis, so agreement and falsetickers read at a glance.
// The solid line is the median consensus; the shaded band is ±threshold (it may
// extend past the axis when servers agree far more tightly than the threshold).
export function ConsensusRuler({
  results,
  medianNs,
  outliers,
  thresholdNs,
}: {
  results: ServerResult[];
  medianNs: number;
  outliers: string[];
  thresholdNs: number;
}) {
  const reachable = results.filter((r) => r.reachable);
  const outlierSet = new Set(outliers);

  const { lo, hi, pos } = useMemo(() => {
    const offs = reachable.map((r) => r.offset);
    // Scale to the data (offsets and the local clock at 0), NOT the threshold
    // band — otherwise a wide threshold crushes closely-agreeing servers into a
    // single point.
    let lo = Math.min(0, ...offs);
    let hi = Math.max(0, ...offs);
    if (lo === hi) {
      lo -= 1_000_000;
      hi += 1_000_000;
    }
    const pad = (hi - lo) * 0.18;
    lo -= pad;
    hi += pad;
    const pos = (ns: number) => ((ns - lo) / (hi - lo)) * 100;
    return { lo, hi, pos };
  }, [reachable]);

  if (reachable.length === 0) {
    return <p className="text-sm text-muted">No reachable servers to compare.</p>;
  }

  const clamp = (n: number) => Math.max(0, Math.min(100, n));
  const bandLeft = clamp(pos(medianNs - thresholdNs));
  const bandRight = clamp(pos(medianNs + thresholdNs));
  const zeroIn = 0 >= lo && 0 <= hi;

  return (
    <div className="select-none">
      <div className="relative mx-2 h-32">
        {/* consensus band */}
        <div
          className="absolute inset-y-10 rounded bg-accent/10"
          style={{ left: `${bandLeft}%`, width: `${Math.max(0, bandRight - bandLeft)}%` }}
        />
        {/* baseline */}
        <div className="absolute inset-x-0 top-1/2 h-px -translate-y-1/2 bg-border" />

        {/* median consensus — label above */}
        <AxisMarker left={pos(medianNs)} tone="accent" side="above" value={fmtDurationNs(medianNs, true)} tag="median" />
        {/* zero (local clock) — label below, so it never collides with median */}
        {zeroIn && <AxisMarker left={pos(0)} tone="muted" side="below" value="0" tag="your clock" />}

        {/* server dots */}
        {reachable.map((r, i) => {
          const isOut = outlierSet.has(r.target);
          const above = i % 2 === 0;
          return (
            <div
              key={r.target}
              className="group absolute top-1/2 z-10 -translate-x-1/2 -translate-y-1/2"
              style={{ left: `${pos(r.offset)}%` }}
              title={`${r.target}: ${fmtDurationNs(r.offset, true)}`}
            >
              <span
                className={clsx(
                  "block h-3 w-3 rounded-full ring-2 ring-surface transition-transform group-hover:scale-125",
                  isOut ? "bg-warn" : "bg-accent-2",
                )}
              />
              <span
                className={clsx(
                  "pointer-events-none absolute left-1/2 w-max max-w-[130px] -translate-x-1/2 truncate text-[11px] font-medium",
                  above ? "bottom-5" : "top-5",
                  isOut ? "text-warn" : "text-text",
                )}
              >
                {r.target.replace(/:123$/, "")}
              </span>
            </div>
          );
        })}
      </div>

      <div className="mt-1 flex justify-between text-[11px] text-muted tnum">
        <span>{fmtDurationNs(lo, true)}</span>
        <span className="font-sans uppercase tracking-wide">offset from local clock</span>
        <span>{fmtDurationNs(hi, true)}</span>
      </div>
    </div>
  );
}

function AxisMarker({
  left,
  tone,
  side,
  value,
  tag,
}: {
  left: number;
  tone: "accent" | "muted";
  side: "above" | "below";
  value: string;
  tag: string;
}) {
  return (
    <div className="pointer-events-none absolute inset-y-8" style={{ left: `${left}%` }}>
      <div
        className={clsx("h-full w-px -translate-x-1/2", tone === "accent" ? "bg-accent" : "bg-muted/50")}
      />
      <div
        className={clsx(
          "absolute w-max -translate-x-1/2 whitespace-nowrap text-center",
          side === "above" ? "-top-6" : "-bottom-6",
          tone === "accent" ? "text-accent" : "text-muted",
        )}
      >
        <div className="tnum text-[11px] font-semibold">{value}</div>
        <div className="font-sans text-[9px] font-normal uppercase tracking-wide opacity-70">{tag}</div>
      </div>
    </div>
  );
}
