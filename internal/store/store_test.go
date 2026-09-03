package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestServerCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.CreateServer(ctx, Server{Address: "time.cloudflare.com", Label: "cf", Enabled: true})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected assigned ID")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps set")
	}

	got, err := s.GetServer(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}
	if got.Address != "time.cloudflare.com" || got.Label != "cf" || !got.Enabled {
		t.Fatalf("GetServer returned %+v", got)
	}

	got.Label = "cloudflare"
	got.Enabled = false
	updated, err := s.UpdateServer(ctx, got)
	if err != nil {
		t.Fatalf("UpdateServer: %v", err)
	}
	if updated.Label != "cloudflare" || updated.Enabled {
		t.Fatalf("update not applied: %+v", updated)
	}

	list, err := s.ListServers(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListServers = %v, %v", list, err)
	}

	if err := s.DeleteServer(ctx, created.ID); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if _, err := s.GetServer(ctx, created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestGetUpdateDeleteMissing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.GetServer(ctx, "nope"); err != ErrNotFound {
		t.Errorf("GetServer missing = %v, want ErrNotFound", err)
	}
	if _, err := s.UpdateServer(ctx, Server{ID: "nope"}); err != ErrNotFound {
		t.Errorf("UpdateServer missing = %v, want ErrNotFound", err)
	}
	if err := s.DeleteServer(ctx, "nope"); err != ErrNotFound {
		t.Errorf("DeleteServer missing = %v, want ErrNotFound", err)
	}
}

func TestEnabledServers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateServer(ctx, Server{Address: "a", Enabled: true})
	_, _ = s.CreateServer(ctx, Server{Address: "b", Enabled: false})
	_, _ = s.CreateServer(ctx, Server{Address: "c", Enabled: true})
	enabled, err := s.EnabledServers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 2 {
		t.Fatalf("EnabledServers = %d, want 2", len(enabled))
	}
}

func TestMeasurementsRoundTripAndWindow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	srv, _ := s.CreateServer(ctx, Server{Address: "a", Enabled: true})

	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		m := Measurement{
			ServerID:    srv.ID,
			TS:          base.Add(time.Duration(i) * time.Minute),
			Reachable:   true,
			Offset:      time.Duration(i+1) * time.Millisecond,
			RTT:         10 * time.Millisecond,
			Jitter:      500 * time.Microsecond,
			Stratum:     2,
			ResolvedIP:  "203.0.113.1",
			ReferenceID: "GPS",
		}
		if err := s.InsertMeasurement(ctx, m); err != nil {
			t.Fatalf("InsertMeasurement: %v", err)
		}
	}

	all, err := s.Measurements(ctx, srv.ID, time.Time{}, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("got %d measurements, want 5", len(all))
	}
	// ordering oldest-first and duration round-trip
	if all[0].Offset != 1*time.Millisecond || all[4].Offset != 5*time.Millisecond {
		t.Fatalf("offset round-trip wrong: %v .. %v", all[0].Offset, all[4].Offset)
	}
	if all[0].TS.After(all[4].TS) {
		t.Fatal("expected oldest-first ordering")
	}

	// window: [base+1m, base+3m] inclusive -> 3 rows
	win, err := s.Measurements(ctx, srv.ID, base.Add(time.Minute), base.Add(3*time.Minute), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(win) != 3 {
		t.Fatalf("windowed = %d, want 3", len(win))
	}
}

func TestPruneMeasurements(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	srv, _ := s.CreateServer(ctx, Server{Address: "a"})
	now := time.Now().UTC()
	old := Measurement{ServerID: srv.ID, TS: now.Add(-48 * time.Hour), Reachable: true}
	recent := Measurement{ServerID: srv.ID, TS: now.Add(-1 * time.Hour), Reachable: true}
	_ = s.InsertMeasurement(ctx, old)
	_ = s.InsertMeasurement(ctx, recent)

	n, err := s.PruneMeasurements(ctx, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	remaining, _ := s.Measurements(ctx, srv.ID, time.Time{}, time.Time{}, 0)
	if len(remaining) != 1 {
		t.Fatalf("remaining %d, want 1", len(remaining))
	}
}

func TestDeleteServerCascadesMeasurements(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	srv, _ := s.CreateServer(ctx, Server{Address: "a"})
	_ = s.InsertMeasurement(ctx, Measurement{ServerID: srv.ID, TS: time.Now().UTC(), Reachable: true})

	if err := s.DeleteServer(ctx, srv.ID); err != nil {
		t.Fatal(err)
	}
	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Measurements != 0 {
		t.Fatalf("measurements after cascade delete = %d, want 0", st.Measurements)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idem.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	srv, _ := s1.CreateServer(ctx, Server{Address: "keep"})
	s1.Close()

	// Re-open: migrations must not re-run destructively; data survives.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if _, err := s2.GetServer(ctx, srv.ID); err != nil {
		t.Fatalf("data lost after reopen: %v", err)
	}
}
