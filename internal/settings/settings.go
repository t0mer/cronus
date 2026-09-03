// Package settings holds Cronus's runtime-editable settings: the monitoring
// interval, measurement retention, and outlier threshold. Values are seeded
// from static configuration, overridden by anything persisted in the store, and
// updated live through the API. The service is safe for concurrent use and
// implements the scheduler's Provider so changes take effect without a restart.
package settings

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Store is the persistence surface settings needs.
type Store interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// Keys under which values are persisted.
const (
	KeyInterval  = "monitor.interval"
	KeyRetention = "monitor.retention"
	KeyThreshold = "compare.outlier_threshold"
)

// MinInterval is the smallest accepted monitoring interval.
const MinInterval = 15 * time.Second

// Values is the set of runtime-editable settings.
type Values struct {
	MonitorInterval  time.Duration
	Retention        time.Duration
	OutlierThreshold time.Duration
}

// Validate checks the values are within acceptable bounds.
func (v Values) Validate() error {
	if v.MonitorInterval < MinInterval {
		return fmt.Errorf("monitor interval must be >= %s", MinInterval)
	}
	if v.Retention <= 0 {
		return errors.New("retention must be positive")
	}
	if v.OutlierThreshold < 0 {
		return errors.New("outlier threshold must be >= 0")
	}
	return nil
}

// Service provides thread-safe access to the current settings.
type Service struct {
	mu    sync.RWMutex
	store Store
	v     Values
}

// New builds a Service seeded from defaults, then applies any persisted
// overrides found in the store.
func New(ctx context.Context, store Store, defaults Values) (*Service, error) {
	s := &Service{store: store, v: defaults}
	if err := s.load(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) load(ctx context.Context) error {
	get := func(key string, dst *time.Duration) error {
		raw, err := s.store.GetSetting(ctx, key)
		if err != nil {
			return nil // unset: keep default; ErrNotFound and others are non-fatal at load
		}
		d, perr := time.ParseDuration(raw)
		if perr != nil {
			return nil
		}
		*dst = d
		return nil
	}
	_ = get(KeyInterval, &s.v.MonitorInterval)
	_ = get(KeyRetention, &s.v.Retention)
	_ = get(KeyThreshold, &s.v.OutlierThreshold)
	return nil
}

// Get returns the current settings snapshot.
func (s *Service) Get() Values {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.v
}

// Update validates, persists, and caches new settings.
func (s *Service) Update(ctx context.Context, v Values) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if err := s.store.SetSetting(ctx, KeyInterval, v.MonitorInterval.String()); err != nil {
		return err
	}
	if err := s.store.SetSetting(ctx, KeyRetention, v.Retention.String()); err != nil {
		return err
	}
	if err := s.store.SetSetting(ctx, KeyThreshold, v.OutlierThreshold.String()); err != nil {
		return err
	}
	s.mu.Lock()
	s.v = v
	s.mu.Unlock()
	return nil
}

// Interval implements scheduler.Provider.
func (s *Service) Interval() time.Duration { return s.Get().MonitorInterval }

// Retention implements scheduler.Provider.
func (s *Service) Retention() time.Duration { return s.Get().Retention }

// OutlierThreshold implements scheduler.Provider.
func (s *Service) OutlierThreshold() time.Duration { return s.Get().OutlierThreshold }
