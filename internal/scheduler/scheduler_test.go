package scheduler

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/t0mer/cronus/internal/notify"
	"github.com/t0mer/cronus/internal/ntp"
	"github.com/t0mer/cronus/internal/store"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeEngine returns a canned result per target based on a lookup.
type fakeEngine struct {
	byTarget map[string]ntp.ServerResult
}

func (f *fakeEngine) Run(_ context.Context, targets []string) []ntp.ServerResult {
	out := make([]ntp.ServerResult, len(targets))
	for i, t := range targets {
		r, ok := f.byTarget[t]
		if !ok {
			r = ntp.ServerResult{Target: t, Reachable: false, Error: "unknown"}
		}
		r.Target = t
		out[i] = r
	}
	return out
}

type fakeObserver struct {
	mu        sync.Mutex
	observed  map[string]ntp.ServerResult
	polls     int
	lastCount int
	pruned    int
}

func newFakeObserver() *fakeObserver {
	return &fakeObserver{observed: map[string]ntp.ServerResult{}}
}
func (o *fakeObserver) ObserveServer(id string, r ntp.ServerResult) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.observed[id] = r
}
func (o *fakeObserver) PollCompleted(n int, _ float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.polls++
	o.lastCount = n
}
func (o *fakeObserver) Pruned(n int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pruned += n
}

type recordingNotifier struct {
	mu     sync.Mutex
	events []notify.Event
}

func (r *recordingNotifier) Notify(_ context.Context, ev notify.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return nil
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPollOnceStoresObservesAndNotifies(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	a, _ := st.CreateServer(ctx, store.Server{Address: "a.example", Enabled: true})
	b, _ := st.CreateServer(ctx, store.Server{Address: "b.example", Enabled: true})
	_, _ = st.CreateServer(ctx, store.Server{Address: "c.example", Enabled: false})

	engine := &fakeEngine{byTarget: map[string]ntp.ServerResult{
		"a.example": {Reachable: true, Offset: 5 * time.Millisecond, RTT: 10 * time.Millisecond, Stratum: 2},
		"b.example": {Reachable: false, Error: "i/o timeout"},
	}}
	obs := newFakeObserver()
	notif := &recordingNotifier{}
	s := New(st, engine, obs, notif, Config{Interval: time.Minute, OutlierThreshold: 100 * time.Millisecond}, nil, quietLogger())

	s.PollOnce(ctx)

	// measurements stored for both enabled servers, none for disabled
	ma, _ := st.Measurements(ctx, a.ID, time.Time{}, time.Time{}, 0)
	mb, _ := st.Measurements(ctx, b.ID, time.Time{}, time.Time{}, 0)
	if len(ma) != 1 || len(mb) != 1 {
		t.Fatalf("measurements: a=%d b=%d, want 1 and 1", len(ma), len(mb))
	}
	if !ma[0].Reachable || ma[0].Offset != 5*time.Millisecond {
		t.Fatalf("a measurement wrong: %+v", ma[0])
	}
	if mb[0].Reachable {
		t.Fatalf("b should be unreachable: %+v", mb[0])
	}
	if obs.polls != 1 || obs.lastCount != 2 {
		t.Fatalf("observer polls=%d lastCount=%d, want 1 and 2", obs.polls, obs.lastCount)
	}
	if len(obs.observed) != 2 {
		t.Fatalf("observed %d servers, want 2", len(obs.observed))
	}
	if len(notif.events) != 1 || notif.events[0].Kind != notify.KindUnreachable {
		t.Fatalf("expected 1 unreachable notification, got %+v", notif.events)
	}
}

func TestPollOnceNoServers(t *testing.T) {
	st := newStore(t)
	obs := newFakeObserver()
	s := New(st, &fakeEngine{}, obs, notify.Nop{}, Config{Interval: time.Minute}, nil, quietLogger())
	s.PollOnce(context.Background())
	if obs.polls != 1 || obs.lastCount != 0 {
		t.Fatalf("expected a completed poll with 0 servers, got polls=%d count=%d", obs.polls, obs.lastCount)
	}
}

func TestHousekeepPrunes(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	srv, _ := st.CreateServer(ctx, store.Server{Address: "a", Enabled: true})
	now := time.Now().UTC()
	_ = st.InsertMeasurement(ctx, store.Measurement{ServerID: srv.ID, TS: now.Add(-48 * time.Hour)})
	_ = st.InsertMeasurement(ctx, store.Measurement{ServerID: srv.ID, TS: now.Add(-1 * time.Hour)})

	obs := newFakeObserver()
	s := New(st, &fakeEngine{}, obs, notify.Nop{}, Config{Interval: time.Minute, Retention: 24 * time.Hour}, nil, quietLogger())
	s.Housekeep(ctx)

	if obs.pruned != 1 {
		t.Fatalf("pruned = %d, want 1", obs.pruned)
	}
	remaining, _ := st.Measurements(ctx, srv.ID, time.Time{}, time.Time{}, 0)
	if len(remaining) != 1 {
		t.Fatalf("remaining = %d, want 1", len(remaining))
	}
}

func TestIntervalFlooredAt15s(t *testing.T) {
	s := New(newStore(t), &fakeEngine{}, newFakeObserver(), notify.Nop{},
		Config{Interval: time.Second}, nil, quietLogger())
	if s.interval() != ntp.MinMonitorInterval {
		t.Fatalf("interval() = %v, want floored to %v", s.interval(), ntp.MinMonitorInterval)
	}
}

type fakeProvider struct {
	interval, retention, threshold time.Duration
}

func (f fakeProvider) Interval() time.Duration         { return f.interval }
func (f fakeProvider) Retention() time.Duration        { return f.retention }
func (f fakeProvider) OutlierThreshold() time.Duration { return f.threshold }

func TestProviderOverridesConfig(t *testing.T) {
	p := fakeProvider{interval: 42 * time.Second, retention: 10 * time.Hour, threshold: 7 * time.Millisecond}
	s := New(newStore(t), &fakeEngine{}, newFakeObserver(), notify.Nop{},
		Config{Interval: time.Minute, Retention: time.Hour, OutlierThreshold: time.Second}, p, quietLogger())
	if s.interval() != 42*time.Second {
		t.Errorf("interval() = %v, want provider's 42s", s.interval())
	}
	if s.retention() != 10*time.Hour {
		t.Errorf("retention() = %v, want provider's 10h", s.retention())
	}
	if s.threshold() != 7*time.Millisecond {
		t.Errorf("threshold() = %v, want provider's 7ms", s.threshold())
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	st := newStore(t)
	_, _ = st.CreateServer(context.Background(), store.Server{Address: "a", Enabled: true})
	s := New(st, &fakeEngine{}, newFakeObserver(), notify.Nop{},
		Config{Interval: time.Minute, Retention: time.Hour}, nil, quietLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
