# Cronus REST API

All application endpoints are versioned under `/api/v1` and speak JSON.
Operational endpoints (`/healthz`, `/metrics`) are unversioned.

- Base URL: `http://<host>:8080`
- Content type: `application/json; charset=utf-8`
- Errors: non-2xx responses carry `{"error": "<message>"}`.
- Request bodies are capped at 1 MiB and reject unknown fields.
- Every response carries hardening headers (`X-Content-Type-Options`,
  `X-Frame-Options`, `Referrer-Policy`, `Content-Security-Policy`).

## Authentication

v1 has no authentication; Cronus is intended to run on a trusted network. The
`POST /api/v1/test` endpoint is rate-limited to **10 requests/minute per client
IP** so it cannot be abused as a UDP-reflector trigger.

Durations in responses are integer **nanoseconds** (Go `time.Duration`), except
chart/analysis payloads (`/measurements`, `/drift`) which use **float seconds**
for direct charting.

---

## `POST /api/v1/test`

Run an on-demand test against one or more servers and return a side-by-side
comparison. Synchronous. Capped at 20 servers per request.

Request:

```json
{ "servers": ["time.cloudflare.com", "pool.ntp.org:123"], "samples": 4 }
```

- `servers` (required): 1–20 targets, each `host` or `host:port`.
- `samples` (optional): 1–10; defaults to the server's configured value.

Response `200`:

```json
{
  "results": [
    {
      "target": "time.cloudflare.com",
      "host": "time.cloudflare.com",
      "port": "123",
      "resolved_ip": "162.159.200.123",
      "reachable": true,
      "samples": [ { "ok": true, "offset": 6163435, "rtt": 3547513, "stratum": 3, "leap": 0, "precision": 14, "reference_id": "10.72.8.4", "root_delay": 70693970, "root_dispersion": 259399, "at": "2026-09-03T14:33:56Z" } ],
      "offset": 6163435,
      "rtt": 3547513,
      "jitter": 51234,
      "stratum": 3,
      "leap": 0,
      "reference_id": "10.72.8.4",
      "precision": 14,
      "root_delay": 70693970,
      "root_dispersion": 259399
    }
  ],
  "comparison": {
    "labels": ["time.cloudflare.com", "pool.ntp.org:123"],
    "median_offset": 6000000,
    "outliers": [],
    "pairwise": [[0, -1837065], [1837065, 0]]
  }
}
```

Errors: `400` (no servers / >20 / invalid address / bad samples), `429` (rate
limited).

---

## `GET /api/v1/servers`

List all saved (monitored) servers. Response `200`: array of server objects.

## `POST /api/v1/servers`

Create a saved server.

Request: `{ "address": "time.cloudflare.com", "label": "cf", "enabled": true }`

- `address` (required): validated as `host` or `host:port`.
- `label` (optional), `enabled` (optional, default `true`).

Response `201`: the created server:

```json
{ "id": "f59c7c1f-...", "address": "time.cloudflare.com", "label": "cf",
  "enabled": true, "created_at": "...", "updated_at": "..." }
```

Errors: `400` (invalid address / body).

## `GET /api/v1/servers/{id}`

Fetch one server. `200` server object, or `404`.

## `PUT /api/v1/servers/{id}`

Update `address`, `label`, and `enabled`. Same body as create. `200` updated
server, `400` invalid, `404` unknown id. Disabling a server drops its metric
series.

## `DELETE /api/v1/servers/{id}`

Delete a server and its measurements (cascade). `204` on success, `404` if
unknown.

---

## `GET /api/v1/servers/{id}/measurements`

Measurement history for charts.

Query params:

- `from`, `to` (optional, RFC3339): time window bounds.
- `step` (optional, Go duration e.g. `5m`): server-side downsampling — results
  are averaged into fixed `step` buckets (bucket reachable if any sample was).

Response `200`:

```json
{
  "server_id": "f59c...",
  "from": "2026-09-01T00:00:00Z",
  "to": "2026-09-02T00:00:00Z",
  "step": "5m",
  "count": 288,
  "points": [ { "ts": "...", "reachable": true, "offset_seconds": 0.0061, "rtt_seconds": 0.0035, "jitter_seconds": 0.00005, "stratum": 3 } ]
}
```

`404` if the server does not exist; `400` for invalid `from`/`to`/`step`.

## `GET /api/v1/servers/{id}/drift`

Clock drift estimate over a sliding window, via least-squares regression of
offset vs. time.

Query params: `window` (optional, Go duration; default `24h`).

Response `200`:

```json
{
  "server_id": "f59c...",
  "drift_ppm": 1.02,
  "r2": 0.98,
  "samples_used": 288,
  "window_start": "2026-09-01T14:00:00Z",
  "window_end": "2026-09-02T14:00:00Z"
}
```

When there are fewer than two reachable measurements, `drift_ppm` is `null` and
a `message` explains why. `404` if the server does not exist.

---

## `GET /api/v1/status`

Application status.

```json
{ "version": "2026.9.0", "uptime_seconds": 1234.5, "scheduler_running": true,
  "db": { "servers": 3, "measurements": 8640 }, "now": "..." }
```

## `GET /api/v1/settings`

Return the current runtime-editable settings (durations as strings):

```json
{ "monitor_interval": "5m0s", "retention": "720h0m0s", "outlier_threshold": "100ms" }
```

## `PUT /api/v1/settings`

Update and persist the settings. Body: same shape as the GET response; all
three fields are durations (Go format, e.g. `"30s"`, `"48h"`, `"50ms"`).
Changes take effect live — the monitoring loop picks them up on the next cycle.

- `monitor_interval` must be ≥ `15s`.
- `retention` must be positive.
- `outlier_threshold` must be ≥ 0.

Response `200`: the updated settings. Errors: `400` (unparseable duration or
out-of-range value).

## `GET /metrics`

Prometheus exposition. Per-monitored-server gauges are labelled `{id, server}`:

| Metric | Type | Description |
|---|---|---|
| `cronus_offset_seconds` | gauge | Clock offset vs. local, seconds |
| `cronus_rtt_seconds` | gauge | Round-trip delay, seconds |
| `cronus_jitter_seconds` | gauge | Offset jitter across samples, seconds |
| `cronus_stratum` | gauge | Reported stratum |
| `cronus_reachable` | gauge | 1 reachable, 0 not, at last poll |
| `cronus_polls_total` | counter | Monitoring poll cycles executed |
| `cronus_measurements_pruned_total` | counter | Measurements deleted by housekeeping |
| `cronus_last_poll_timestamp_seconds` | gauge | Unix time of last poll |
| `cronus_monitored_servers` | gauge | Enabled servers at last poll |

Standard Go/process collectors are also exported.

## `GET /healthz`

Liveness. `200 {"status":"ok"}`.
