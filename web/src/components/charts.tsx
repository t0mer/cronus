import {
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  CartesianGrid,
  ReferenceLine,
} from "recharts";
import { fmtClock, fmtDateTime, fmtSeconds } from "../lib/format";

// A rotating palette for multi-series overlays, drawn from the theme accents.
export const SERIES_COLORS = [
  "var(--accent)",
  "var(--accent-2)",
  "var(--warn)",
  "#a855f7",
  "#ec4899",
  "#22c55e",
  "#eab308",
  "#f97316",
];

export function Sparkline({ data, color = "var(--accent)" }: { data: number[]; color?: string }) {
  const rows = data.map((v, i) => ({ i, v }));
  return (
    <ResponsiveContainer width="100%" height={36}>
      <LineChart data={rows} margin={{ top: 4, bottom: 4, left: 0, right: 0 }}>
        <Line type="monotone" dataKey="v" stroke={color} strokeWidth={1.5} dot={false} isAnimationActive={false} />
      </LineChart>
    </ResponsiveContainer>
  );
}

interface Series {
  key: string;
  label: string;
  color: string;
}

interface Row {
  ts: string;
  [k: string]: string | number | null;
}

export function OffsetChart({
  rows,
  series,
  height = 260,
}: {
  rows: Row[];
  series: Series[];
  height?: number;
}) {
  return (
    <ResponsiveContainer width="100%" height={height}>
      <LineChart data={rows} margin={{ top: 8, right: 12, bottom: 4, left: 4 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
        <XAxis
          dataKey="ts"
          tickFormatter={fmtClock}
          stroke="var(--muted)"
          fontSize={11}
          minTickGap={40}
        />
        <YAxis
          tickFormatter={(v) => fmtSeconds(v as number, true)}
          stroke="var(--muted)"
          fontSize={11}
          width={64}
        />
        <ReferenceLine y={0} stroke="var(--muted)" strokeDasharray="4 4" />
        <Tooltip
          contentStyle={{
            background: "var(--surface)",
            border: "1px solid var(--border)",
            borderRadius: 8,
            fontSize: 12,
          }}
          labelStyle={{ color: "var(--muted)" }}
          labelFormatter={(l) => fmtDateTime(l as string)}
          formatter={(v: number | string, name: string) => [fmtSeconds(v as number, true), name]}
        />
        {series.map((s) => (
          <Line
            key={s.key}
            type="monotone"
            dataKey={s.key}
            name={s.label}
            stroke={s.color}
            strokeWidth={1.75}
            dot={false}
            connectNulls
            isAnimationActive={false}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  );
}

export type { Series as ChartSeries, Row as ChartRow };
