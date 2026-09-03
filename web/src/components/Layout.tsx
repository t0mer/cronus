import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useEffect, useState } from "react";
import { clsx } from "clsx";
import { Activity, Gauge, Radar, Settings as SettingsIcon, Moon, Sun } from "lucide-react";
import { api, type Status } from "../lib/api";
import { useTheme } from "../lib/theme";
import { fmtUptime } from "../lib/format";
import { Dot } from "./ui";
import { ErrorBoundary } from "./ErrorBoundary";

const tabs = [
  { to: "/", label: "Quick Test", icon: Gauge, end: true },
  { to: "/monitoring", label: "Monitoring", icon: Radar, end: false },
  { to: "/settings", label: "Settings", icon: SettingsIcon, end: false },
];

export function Layout() {
  const { isDark, toggle } = useTheme();
  const [status, setStatus] = useState<Status | null>(null);
  const location = useLocation();

  useEffect(() => {
    let alive = true;
    const load = () => api.status().then((s) => alive && setStatus(s)).catch(() => {});
    load();
    const t = setInterval(load, 60_000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, []);

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-30 border-b border-border bg-bg/85 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center gap-4 px-4">
          <div className="flex items-center gap-2.5">
            <Wordmark />
          </div>

          <nav className="ml-2 hidden items-center gap-1 sm:flex">
            {tabs.map((t) => (
              <NavLink
                key={t.to}
                to={t.to}
                end={t.end}
                className={({ isActive }) =>
                  clsx(
                    "flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition",
                    isActive ? "bg-surface-2 text-text" : "text-muted hover:text-text",
                  )
                }
              >
                <t.icon className="h-4 w-4" />
                {t.label}
              </NavLink>
            ))}
          </nav>

          <div className="ml-auto flex items-center gap-3">
            {status && (
              <div className="hidden items-center gap-2 text-xs text-muted md:flex" title="Scheduler state and uptime">
                <Dot tone={status.scheduler_running ? "ok" : "muted"} />
                <span className="tnum">{fmtUptime(status.uptime_seconds)}</span>
              </div>
            )}
            <button
              onClick={toggle}
              className="rounded-lg border border-border p-2 text-muted transition hover:text-text focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
              aria-label="Toggle theme"
            >
              {isDark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
          </div>
        </div>

        {/* mobile tabs */}
        <nav className="flex items-center gap-1 border-t border-border px-2 py-1 sm:hidden">
          {tabs.map((t) => (
            <NavLink
              key={t.to}
              to={t.to}
              end={t.end}
              className={({ isActive }) =>
                clsx(
                  "flex flex-1 flex-col items-center gap-0.5 rounded-lg px-2 py-1.5 text-xs font-medium",
                  isActive ? "bg-surface-2 text-text" : "text-muted",
                )
              }
            >
              <t.icon className="h-4 w-4" />
              {t.label}
            </NavLink>
          ))}
        </nav>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-6">
        {/* Keyed by path so navigating to another tab resets a caught error. */}
        <ErrorBoundary key={location.pathname}>
          <Outlet />
        </ErrorBoundary>
      </main>

      <footer className="mx-auto max-w-6xl px-4 py-8 text-xs text-muted">
        <span className="tnum">cronus {status?.version ?? ""}</span> · NTP server tester &amp; comparator
      </footer>
    </div>
  );
}

function Wordmark() {
  return (
    <div className="flex items-center gap-2">
      <span className="relative flex h-7 w-7 items-center justify-center rounded-md bg-accent/15 text-accent">
        <Activity className="h-4 w-4" />
      </span>
      <span className="font-display text-lg font-semibold tracking-tight text-text">
        cronus
      </span>
    </div>
  );
}
