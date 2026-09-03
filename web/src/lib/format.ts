// Formatting helpers. Offsets and delays are small; we render them in the
// largest unit that keeps 1–3 significant figures, always with a sign for
// offsets so direction is unmistakable.

const NS = 1;
const US = 1_000;
const MS = 1_000_000;
const S = 1_000_000_000;

export function fmtDurationNs(ns: number, signed = false): string {
  if (ns === 0) return signed ? "±0" : "0";
  const sign = ns < 0 ? "-" : signed ? "+" : "";
  const a = Math.abs(ns);
  if (a >= S) return `${sign}${(a / S).toFixed(3)} s`;
  if (a >= MS) return `${sign}${(a / MS).toFixed(a >= 10 * MS ? 1 : 3)} ms`;
  if (a >= US) return `${sign}${(a / US).toFixed(1)} µs`;
  return `${sign}${(a / NS).toFixed(0)} ns`;
}

export function fmtSeconds(s: number, signed = false): string {
  return fmtDurationNs(s * S, signed);
}

// parseGoDurationNs parses a Go-style duration string (e.g. "100ms", "5m0s",
// "1.5s", "720h0m0s") into nanoseconds. Returns null on a malformed string.
export function parseGoDurationNs(s: string): number | null {
  const units: Record<string, number> = {
    ns: NS,
    us: US,
    "µs": US,
    "μs": US,
    ms: MS,
    s: S,
    m: 60 * S,
    h: 3600 * S,
  };
  const re = /([0-9]*\.?[0-9]+)(ns|us|µs|μs|ms|s|m|h)/g;
  let total = 0;
  let matched = false;
  let m: RegExpExecArray | null;
  while ((m = re.exec(s)) !== null) {
    matched = true;
    total += parseFloat(m[1]) * units[m[2]];
  }
  return matched ? total : null;
}

export function fmtPPM(ppm: number): string {
  const sign = ppm > 0 ? "+" : "";
  return `${sign}${ppm.toFixed(3)} ppm`;
}

export function fmtLeap(leap: number): string {
  switch (leap) {
    case 0:
      return "none";
    case 1:
      return "+1s";
    case 2:
      return "-1s";
    default:
      return "unsync";
  }
}

export function fmtUptime(seconds: number): string {
  const s = Math.floor(seconds);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m ${s % 60}s`;
}

export function fmtClock(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function fmtDateTime(iso: string): string {
  return new Date(iso).toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
