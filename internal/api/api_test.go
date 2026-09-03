package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/t0mer/cronus/internal/metrics"
	"github.com/t0mer/cronus/internal/ntp"
	"github.com/t0mer/cronus/internal/settings"
	"github.com/t0mer/cronus/internal/store"
)

type fakeSettings struct{ v settings.Values }

func (f *fakeSettings) Get() settings.Values { return f.v }
func (f *fakeSettings) Update(_ context.Context, v settings.Values) error {
	if err := v.Validate(); err != nil {
		return err
	}
	f.v = v
	return nil
}

type fakeEngine struct {
	results map[string]ntp.ServerResult
}

func (f *fakeEngine) RunWithSamples(_ context.Context, targets []string, _ int) []ntp.ServerResult {
	out := make([]ntp.ServerResult, len(targets))
	for i, t := range targets {
		r := f.results[t]
		r.Target = t
		if _, ok := f.results[t]; !ok {
			r.Reachable = true
		}
		out[i] = r
	}
	return out
}

func newTestAPI(t *testing.T) (*API, *store.Store, *fakeEngine) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	eng := &fakeEngine{results: map[string]ntp.ServerResult{}}
	a := New(Deps{
		Store:            st,
		Engine:           eng,
		Settings:         &fakeSettings{v: settings.Values{MonitorInterval: 5 * time.Minute, Retention: 720 * time.Hour, OutlierThreshold: 100 * time.Millisecond}},
		Metrics:          metrics.New(),
		OutlierThreshold: 100 * time.Millisecond,
		StartTime:        time.Now(),
	})
	return a, st, eng
}

func do(t *testing.T, a *API, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	a, _, _ := newTestAPI(t)
	rec := do(t, a, "GET", "/healthz", nil)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestSecurityHeadersPresent(t *testing.T) {
	a, _, _ := newTestAPI(t)
	rec := do(t, a, "GET", "/healthz", nil)
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options")
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("missing CSP header")
	}
}

func TestCreateAndListServers(t *testing.T) {
	a, _, _ := newTestAPI(t)

	rec := do(t, a, "POST", "/api/v1/servers", map[string]any{"address": "time.cloudflare.com", "label": "cf"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body)
	}
	var created store.Server
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == "" || !created.Enabled {
		t.Fatalf("unexpected created server: %+v", created)
	}

	rec = do(t, a, "GET", "/api/v1/servers", nil)
	var list []store.Server
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("list = %d, want 1", len(list))
	}
}

func TestCreateServerRejectsBadAddress(t *testing.T) {
	a, _, _ := newTestAPI(t)
	rec := do(t, a, "POST", "/api/v1/servers", map[string]any{"address": "bad address with spaces"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUpdateAndDeleteServer(t *testing.T) {
	a, st, _ := newTestAPI(t)
	srv, _ := st.CreateServer(context.Background(), store.Server{Address: "a.example", Enabled: true})

	enabled := false
	rec := do(t, a, "PUT", "/api/v1/servers/"+srv.ID, map[string]any{
		"address": "a.example", "label": "renamed", "enabled": enabled,
	})
	if rec.Code != 200 {
		t.Fatalf("update status = %d body=%s", rec.Code, rec.Body)
	}
	var updated store.Server
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Label != "renamed" || updated.Enabled {
		t.Fatalf("update not applied: %+v", updated)
	}

	rec = do(t, a, "DELETE", "/api/v1/servers/"+srv.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	rec = do(t, a, "GET", "/api/v1/servers/"+srv.ID, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", rec.Code)
	}
}

func TestTestEndpoint(t *testing.T) {
	a, _, eng := newTestAPI(t)
	eng.results["time.cloudflare.com"] = ntp.ServerResult{Reachable: true, Offset: 5 * time.Millisecond}
	eng.results["pool.ntp.org"] = ntp.ServerResult{Reachable: true, Offset: 8 * time.Millisecond}

	rec := do(t, a, "POST", "/api/v1/test", map[string]any{
		"servers": []string{"time.cloudflare.com", "pool.ntp.org"},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var resp testResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(resp.Results))
	}
	if len(resp.Comparison.Labels) != 2 {
		t.Fatalf("comparison labels = %d, want 2", len(resp.Comparison.Labels))
	}
}

func TestTestEndpointValidation(t *testing.T) {
	a, _, _ := newTestAPI(t)
	// empty
	if rec := do(t, a, "POST", "/api/v1/test", map[string]any{"servers": []string{}}); rec.Code != 400 {
		t.Errorf("empty servers = %d, want 400", rec.Code)
	}
	// too many
	many := make([]string, 21)
	for i := range many {
		many[i] = fmt.Sprintf("h%d", i)
	}
	if rec := do(t, a, "POST", "/api/v1/test", map[string]any{"servers": many}); rec.Code != 400 {
		t.Errorf("too many servers = %d, want 400", rec.Code)
	}
	// bad address
	if rec := do(t, a, "POST", "/api/v1/test", map[string]any{"servers": []string{"a b c"}}); rec.Code != 400 {
		t.Errorf("bad address = %d, want 400", rec.Code)
	}
	// bad samples
	if rec := do(t, a, "POST", "/api/v1/test", map[string]any{"servers": []string{"a"}, "samples": 99}); rec.Code != 400 {
		t.Errorf("bad samples = %d, want 400", rec.Code)
	}
}

func TestTestEndpointRateLimited(t *testing.T) {
	a, _, _ := newTestAPI(t)
	body := map[string]any{"servers": []string{"a"}}
	for i := 0; i < 10; i++ {
		if rec := do(t, a, "POST", "/api/v1/test", body); rec.Code != 200 {
			t.Fatalf("request %d = %d, want 200", i, rec.Code)
		}
	}
	if rec := do(t, a, "POST", "/api/v1/test", body); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("11th request = %d, want 429", rec.Code)
	}
}

func TestMeasurementsAndDownsample(t *testing.T) {
	a, st, _ := newTestAPI(t)
	ctx := context.Background()
	srv, _ := st.CreateServer(ctx, store.Server{Address: "a", Enabled: true})
	// Align base to a 5-minute grid so the 10 one-minute points fall into
	// exactly two 5m buckets (downsampling truncates to the absolute grid).
	base := time.Now().UTC().Add(-time.Hour).Truncate(10 * time.Minute)
	for i := 0; i < 10; i++ {
		_ = st.InsertMeasurement(ctx, store.Measurement{
			ServerID: srv.ID, TS: base.Add(time.Duration(i) * time.Minute),
			Reachable: true, Offset: time.Duration(i) * time.Millisecond, Stratum: 2,
		})
	}
	// raw
	rec := do(t, a, "GET", "/api/v1/servers/"+srv.ID+"/measurements", nil)
	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var raw measurementsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	if raw.Count != 10 {
		t.Fatalf("raw count = %d, want 10", raw.Count)
	}
	// downsample by 5m -> 2 buckets
	rec = do(t, a, "GET", "/api/v1/servers/"+srv.ID+"/measurements?step=5m", nil)
	var ds measurementsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &ds)
	if ds.Count != 2 {
		t.Fatalf("downsampled count = %d, want 2", ds.Count)
	}
}

func TestMeasurementsEmptyReturnsArrayNotNull(t *testing.T) {
	a, st, _ := newTestAPI(t)
	// A server with no measurements yet — exactly the state right after adding
	// one in the Monitoring UI.
	srv, _ := st.CreateServer(context.Background(), store.Server{Address: "a", Enabled: true})

	// The Monitoring page requests with a step, which goes through downsampling.
	rec := do(t, a, "GET", "/api/v1/servers/"+srv.ID+"/measurements?step=5m", nil)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"points":null`) {
		t.Fatalf("points must be an empty array, not null: %s", rec.Body.String())
	}
	var m measurementsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Points == nil {
		t.Fatal("Points decoded to nil; the API should always return an array")
	}
}

func TestMeasurementsServerNotFound(t *testing.T) {
	a, _, _ := newTestAPI(t)
	rec := do(t, a, "GET", "/api/v1/servers/nope/measurements", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDrift(t *testing.T) {
	a, st, _ := newTestAPI(t)
	ctx := context.Background()
	srv, _ := st.CreateServer(ctx, store.Server{Address: "a", Enabled: true})

	// no data -> message, drift null
	rec := do(t, a, "GET", "/api/v1/servers/"+srv.ID+"/drift", nil)
	var d driftResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &d)
	if d.DriftPPM != nil || d.Message == "" {
		t.Fatalf("expected no drift with message, got %+v", d)
	}

	// linear drift of 1ppm
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		_ = st.InsertMeasurement(ctx, store.Measurement{
			ServerID: srv.ID, TS: now.Add(time.Duration(-4+i) * time.Hour),
			Reachable: true, Offset: time.Duration(i) * 3600 * time.Microsecond, // 3.6ms/hr = 1ppm
		})
	}
	rec = do(t, a, "GET", "/api/v1/servers/"+srv.ID+"/drift?window=48h", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &d)
	if d.DriftPPM == nil {
		t.Fatalf("expected drift, got %+v", d)
	}
	if *d.DriftPPM < 0.9 || *d.DriftPPM > 1.1 {
		t.Fatalf("drift = %v ppm, want ~1.0", *d.DriftPPM)
	}
}

func TestSettingsGetAndPut(t *testing.T) {
	a, _, _ := newTestAPI(t)

	rec := do(t, a, "GET", "/api/v1/settings", nil)
	if rec.Code != 200 {
		t.Fatalf("get status = %d", rec.Code)
	}
	var got settingsBody
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.MonitorInterval != "5m0s" {
		t.Fatalf("interval = %q, want 5m0s", got.MonitorInterval)
	}

	rec = do(t, a, "PUT", "/api/v1/settings", map[string]any{
		"monitor_interval": "30s", "retention": "48h", "outlier_threshold": "50ms",
	})
	if rec.Code != 200 {
		t.Fatalf("put status = %d body=%s", rec.Code, rec.Body)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.MonitorInterval != "30s" || got.Retention != "48h0m0s" {
		t.Fatalf("updated = %+v", got)
	}
}

func TestSettingsPutRejectsInvalid(t *testing.T) {
	a, _, _ := newTestAPI(t)
	// interval below the 15s floor
	rec := do(t, a, "PUT", "/api/v1/settings", map[string]any{
		"monitor_interval": "1s", "retention": "48h", "outlier_threshold": "50ms",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	// unparseable duration
	rec = do(t, a, "PUT", "/api/v1/settings", map[string]any{
		"monitor_interval": "soon", "retention": "48h", "outlier_threshold": "50ms",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUpdateServerRenameForgetsOldMetricSeries(t *testing.T) {
	a, st, _ := newTestAPI(t)
	ctx := context.Background()
	srv, _ := st.CreateServer(ctx, store.Server{Address: "old.example", Enabled: true})
	// Simulate a poll having recorded a series under the old address.
	a.deps.Metrics.ObserveServer(srv.ID, ntp.ServerResult{Target: "old.example", Reachable: true})

	rec := do(t, a, "PUT", "/api/v1/servers/"+srv.ID, map[string]any{
		"address": "new.example", "enabled": true,
	})
	if rec.Code != 200 {
		t.Fatalf("update status = %d body=%s", rec.Code, rec.Body)
	}

	// Scrape metrics; the old-address series must be gone.
	mrec := httptest.NewRecorder()
	a.Handler().ServeHTTP(mrec, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(mrec.Body.String(), `server="old.example"`) {
		t.Fatalf("stale metric series for old.example still present:\n%s", mrec.Body.String())
	}
}

func TestStatus(t *testing.T) {
	a, _, _ := newTestAPI(t)
	rec := do(t, a, "GET", "/api/v1/status", nil)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var s statusResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s.Version == "" {
		t.Error("missing version")
	}
}
