package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/t0mer/cronus/internal/api"
	"github.com/t0mer/cronus/internal/metrics"
	"github.com/t0mer/cronus/internal/notify"
	"github.com/t0mer/cronus/internal/scheduler"
	"github.com/t0mer/cronus/internal/settings"
	"github.com/t0mer/cronus/internal/store"
	"github.com/t0mer/cronus/internal/version"
)

func newServeCmd(cfgFile *string) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the Cronus HTTP server and monitoring scheduler",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd, *cfgFile)
		},
	}
}

func runServe(cmd *cobra.Command, cfgFile string) error {
	cfg, err := loadConfig(cmd, cfgFile)
	if err != nil {
		return err
	}
	setupLogger(cfg.Log.Level)
	log := slog.Default()

	if dir := filepath.Dir(cfg.DB.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	st, err := store.Open(cfg.DB.Path)
	if err != nil {
		return err
	}
	defer st.Close()

	engine := buildEngine(cfg)
	m := metrics.New()

	settingsSvc, err := settings.New(context.Background(), st, settings.Values{
		MonitorInterval:  cfg.Monitor.Interval,
		Retention:        cfg.Monitor.Retention,
		OutlierThreshold: cfg.Compare.OutlierThreshold,
	})
	if err != nil {
		return err
	}

	sched := scheduler.New(st, engine, m, notify.Nop{}, scheduler.Config{
		Interval:         cfg.Monitor.Interval,
		Retention:        cfg.Monitor.Retention,
		OutlierThreshold: cfg.Compare.OutlierThreshold,
	}, settingsSvc, log)

	var schedRunning atomic.Bool
	apiSrv := api.New(api.Deps{
		Store:            st,
		Engine:           engine,
		Settings:         settingsSvc,
		Metrics:          m,
		OutlierThreshold: cfg.Compare.OutlierThreshold,
		DefaultSamples:   cfg.NTP.Samples,
		Log:              log,
		StartTime:        time.Now(),
		SchedulerRunning: schedRunning.Load,
		UI:               uiHandler(),
	})

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           apiSrv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		schedRunning.Store(true)
		sched.Run(ctx)
		schedRunning.Store(false)
	}()

	serveErr := make(chan error, 1)
	go func() {
		log.Info("cronus listening", "addr", cfg.Listen, "version", version.String())
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-serveErr:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}
