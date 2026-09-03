// Typed client for the Cronus REST API. Durations in /test results and server
// records are integer nanoseconds (Go time.Duration); measurement and drift
// payloads use float seconds.

export interface Sample {
  ok: boolean;
  offset: number;
  rtt: number;
  stratum: number;
  leap: number;
  precision: number;
  reference_id: string;
  root_delay: number;
  root_dispersion: number;
  kiss_code?: string;
  error?: string;
  at: string;
}

export interface ServerResult {
  target: string;
  host: string;
  port: string;
  resolved_ip: string;
  reachable: boolean;
  samples: Sample[];
  offset: number;
  rtt: number;
  jitter: number;
  stratum: number;
  leap: number;
  reference_id: string;
  precision: number;
  root_delay: number;
  root_dispersion: number;
  error?: string;
}

export interface Comparison {
  labels: string[];
  median_offset: number;
  outliers: string[] | null;
  pairwise: number[][] | null;
}

export interface TestResponse {
  results: ServerResult[];
  comparison: Comparison;
}

export interface Server {
  id: string;
  address: string;
  label: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface Point {
  ts: string;
  reachable: boolean;
  offset_seconds: number;
  rtt_seconds: number;
  jitter_seconds: number;
  stratum: number;
}

export interface Measurements {
  server_id: string;
  from?: string;
  to?: string;
  step?: string;
  count: number;
  points: Point[];
}

export interface Drift {
  server_id: string;
  drift_ppm: number | null;
  r2?: number;
  samples_used: number;
  window_start?: string;
  window_end?: string;
  message?: string;
}

export interface Status {
  version: string;
  uptime_seconds: number;
  scheduler_running: boolean;
  db: { servers: number; measurements: number };
  now: string;
}

export interface Settings {
  monitor_interval: string;
  retention: string;
  outlier_threshold: string;
}

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  test: (servers: string[], samples?: number) =>
    req<TestResponse>("POST", "/api/v1/test", { servers, samples: samples ?? 0 }),

  listServers: () => req<Server[]>("GET", "/api/v1/servers"),
  getServer: (id: string) => req<Server>("GET", `/api/v1/servers/${id}`),
  createServer: (address: string, label: string, enabled: boolean) =>
    req<Server>("POST", "/api/v1/servers", { address, label, enabled }),
  updateServer: (id: string, address: string, label: string, enabled: boolean) =>
    req<Server>("PUT", `/api/v1/servers/${id}`, { address, label, enabled }),
  deleteServer: (id: string) => req<void>("DELETE", `/api/v1/servers/${id}`),

  measurements: (id: string, params: { from?: string; to?: string; step?: string } = {}) => {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    if (params.step) q.set("step", params.step);
    const qs = q.toString();
    return req<Measurements>("GET", `/api/v1/servers/${id}/measurements${qs ? "?" + qs : ""}`);
  },
  drift: (id: string, window?: string) =>
    req<Drift>("GET", `/api/v1/servers/${id}/drift${window ? "?window=" + window : ""}`),

  status: () => req<Status>("GET", "/api/v1/status"),
  getSettings: () => req<Settings>("GET", "/api/v1/settings"),
  updateSettings: (s: Settings) => req<Settings>("PUT", "/api/v1/settings", s),
};

export { ApiError };
