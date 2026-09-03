// Package scheduler runs Cronus's monitoring loop: it periodically polls every
// enabled server, stores a measurement, updates metrics, and (best-effort)
// emits notifications; a separate housekeeping cadence prunes measurements past
// the retention window. The poll interval is floored at 15s regardless of
// configuration to stay a good NTP citizen.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/t0mer/cronus/internal/notify"
	"github.com/t0mer/cronus/internal/ntp"
	"github.com/t0mer/cronus/internal/store"
)

// Querier runs an NTP test against targets. *ntp.Engine satisfies it.
type Querier interface {
	Run(ctx context.Context, targets []string) []ntp.ServerResult
}

// Store is the persistence surface the scheduler needs.
type Store interface {
	EnabledServers(ctx context.Context) ([]store.Server, error)
	InsertMeasurement(ctx context.Context, m store.Measurement) error
	PruneMeasurements(ctx context.Context, before time.Time) (int64, error)
}

// Observer receives metric updates. *metrics.Metrics satisfies it.
type Observer interface {
	ObserveServer(id string, r ntp.ServerResult)
	PollCompleted(monitored int, unixSeconds float64)
	Pruned(n int)
}

// Config configures the scheduler.
type Config struct {
	Interval         time.Duration
	Retention        time.Duration
	OutlierThreshold time.Duration
	HousekeepEvery   time.Duration
}

// Scheduler ties the store, engine, metrics, and notifier together.
type Scheduler struct {
	store    Store
	engine   Querier
	metrics  Observer
	notifier notify.Notifier
	cfg      Config
	log      *slog.Logger
	now      func() time.Time
}

// New builds a Scheduler, applying defaults and the monitoring-interval floor.
func New(st Store, engine Querier, m Observer, notifier notify.Notifier, cfg Config, log *slog.Logger) *Scheduler {
	if cfg.Interval < ntp.MinMonitorInterval {
		cfg.Interval = ntp.MinMonitorInterval
	}
	if cfg.HousekeepEvery <= 0 {
		cfg.HousekeepEvery = 24 * time.Hour
	}
	if notifier == nil {
		notifier = notify.Nop{}
	}
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		store:    st,
		engine:   engine,
		metrics:  m,
		notifier: notifier,
		cfg:      cfg,
		log:      log,
		now:      time.Now,
	}
}

// Run blocks, polling on the configured interval and running housekeeping on
// its own cadence, until ctx is cancelled. It performs an initial poll and an
// initial prune immediately.
func (s *Scheduler) Run(ctx context.Context) {
	s.PollOnce(ctx)
	s.Housekeep(ctx)

	pollT := time.NewTicker(s.cfg.Interval)
	defer pollT.Stop()
	houseT := time.NewTicker(s.cfg.HousekeepEvery)
	defer houseT.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pollT.C:
			s.PollOnce(ctx)
		case <-houseT.C:
			s.Housekeep(ctx)
		}
	}
}

// PollOnce queries every enabled server once, stores measurements, updates
// metrics, and notifies on unreachable servers. Errors are logged, not fatal.
func (s *Scheduler) PollOnce(ctx context.Context) {
	servers, err := s.store.EnabledServers(ctx)
	if err != nil {
		s.log.Error("scheduler: list enabled servers", "err", err)
		return
	}
	if len(servers) == 0 {
		s.metrics.PollCompleted(0, float64(s.now().Unix()))
		return
	}
	targets := make([]string, len(servers))
	for i, srv := range servers {
		targets[i] = srv.Address
	}
	results := s.engine.Run(ctx, targets)

	var outliers map[string]bool
	if s.cfg.OutlierThreshold > 0 {
		comp := ntp.BuildComparison(results, s.cfg.OutlierThreshold)
		outliers = make(map[string]bool, len(comp.Outliers))
		for _, o := range comp.Outliers {
			outliers[o] = true
		}
	}

	ts := s.now().UTC()
	for i, srv := range servers {
		if i >= len(results) {
			break
		}
		r := results[i]
		s.metrics.ObserveServer(srv.ID, r)
		if err := s.store.InsertMeasurement(ctx, toMeasurement(srv.ID, ts, r)); err != nil {
			s.log.Error("scheduler: insert measurement", "server", srv.Address, "err", err)
		}
		s.emitAlerts(ctx, srv.Address, r, outliers[r.Target])
	}
	s.metrics.PollCompleted(len(servers), float64(s.now().Unix()))
	s.log.Info("scheduler: poll complete", "servers", len(servers))
}

// Housekeep prunes measurements older than the retention window.
func (s *Scheduler) Housekeep(ctx context.Context) {
	if s.cfg.Retention <= 0 {
		return
	}
	before := s.now().Add(-s.cfg.Retention)
	n, err := s.store.PruneMeasurements(ctx, before)
	if err != nil {
		s.log.Error("scheduler: prune", "err", err)
		return
	}
	s.metrics.Pruned(int(n))
	if n > 0 {
		s.log.Info("scheduler: pruned measurements", "count", n)
	}
}

func (s *Scheduler) emitAlerts(ctx context.Context, server string, r ntp.ServerResult, isOutlier bool) {
	if !r.Reachable {
		s.deliver(ctx, notify.Event{Kind: notify.KindUnreachable, Server: server,
			Message: fmt.Sprintf("%s is unreachable: %s", server, r.Error), At: s.now()})
		return
	}
	if isOutlier {
		s.deliver(ctx, notify.Event{Kind: notify.KindOutlier, Server: server,
			Message: fmt.Sprintf("%s is a suspected falseticker (offset %s)", server, r.Offset), At: s.now()})
	}
}

func (s *Scheduler) deliver(ctx context.Context, ev notify.Event) {
	if err := s.notifier.Notify(ctx, ev); err != nil {
		s.log.Warn("scheduler: notify failed", "kind", ev.Kind, "err", err)
	}
}

func toMeasurement(serverID string, ts time.Time, r ntp.ServerResult) store.Measurement {
	return store.Measurement{
		ServerID:    serverID,
		TS:          ts,
		Reachable:   r.Reachable,
		Offset:      r.Offset,
		RTT:         r.RTT,
		Jitter:      r.Jitter,
		Stratum:     r.Stratum,
		Leap:        r.Leap,
		ResolvedIP:  r.ResolvedIP,
		ReferenceID: r.ReferenceID,
		Error:       r.Error,
	}
}
