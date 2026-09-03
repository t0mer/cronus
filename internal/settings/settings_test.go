package settings

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/t0mer/cronus/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func defaults() Values {
	return Values{MonitorInterval: 5 * time.Minute, Retention: 720 * time.Hour, OutlierThreshold: 100 * time.Millisecond}
}

func TestNewUsesDefaultsWhenUnset(t *testing.T) {
	svc, err := New(context.Background(), newStore(t), defaults())
	if err != nil {
		t.Fatal(err)
	}
	if svc.Get() != defaults() {
		t.Fatalf("got %+v, want defaults %+v", svc.Get(), defaults())
	}
}

func TestUpdatePersistsAndReloads(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	svc, _ := New(ctx, st, defaults())

	next := Values{MonitorInterval: 30 * time.Second, Retention: 48 * time.Hour, OutlierThreshold: 50 * time.Millisecond}
	if err := svc.Update(ctx, next); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if svc.Get() != next {
		t.Fatalf("in-memory Get = %+v, want %+v", svc.Get(), next)
	}

	// A fresh service over the same store must load the persisted overrides.
	svc2, err := New(ctx, st, defaults())
	if err != nil {
		t.Fatal(err)
	}
	if svc2.Get() != next {
		t.Fatalf("reloaded = %+v, want persisted %+v", svc2.Get(), next)
	}
}

func TestUpdateRejectsInvalid(t *testing.T) {
	svc, _ := New(context.Background(), newStore(t), defaults())
	bad := Values{MonitorInterval: time.Second, Retention: time.Hour, OutlierThreshold: 0} // interval < 15s
	if err := svc.Update(context.Background(), bad); err == nil {
		t.Fatal("expected validation error for sub-15s interval")
	}
	// unchanged after a rejected update
	if svc.Get() != defaults() {
		t.Fatalf("settings changed after rejected update: %+v", svc.Get())
	}
}

func TestProviderMethods(t *testing.T) {
	svc, _ := New(context.Background(), newStore(t), defaults())
	if svc.Interval() != 5*time.Minute || svc.Retention() != 720*time.Hour || svc.OutlierThreshold() != 100*time.Millisecond {
		t.Fatalf("provider methods returned unexpected values: %v %v %v",
			svc.Interval(), svc.Retention(), svc.OutlierThreshold())
	}
}
