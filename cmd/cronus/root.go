package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/t0mer/cronus/internal/config"
	"github.com/t0mer/cronus/internal/ntp"
)

// sampleSpacing is the delay Cronus leaves between successive samples of a
// single server, to stay a good NTP citizen (§2: ">= 2s apart").
const sampleSpacing = 2 * time.Second

func newRootCmd() *cobra.Command {
	var cfgFile string

	root := &cobra.Command{
		Use:           "cronus",
		Short:         "NTP server tester and comparator",
		SilenceUsage:  true,
		SilenceErrors: true,
		// With no subcommand, run the server (serve is the default, per spec).
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd, cfgFile)
		},
	}

	pf := root.PersistentFlags()
	pf.StringVar(&cfgFile, "config", "", "path to config.yaml")
	pf.String("log.level", "info", "log level: debug|info|warning|error")
	pf.String("listen", ":8080", "HTTP listen address")
	pf.String("db.path", "/data/cronus.db", "SQLite database path")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newTestCmd(&cfgFile))
	root.AddCommand(newServeCmd(&cfgFile))
	return root
}

// loadConfig builds the resolved configuration for a command, binding the
// command's flags so flags > env > file > defaults.
func loadConfig(cmd *cobra.Command, cfgFile string) (*config.Config, error) {
	v := config.Defaults()
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	}
	if err := config.BindFlags(v, cmd.Flags()); err != nil {
		return nil, err
	}
	// Persistent flags live on the root; bind them too.
	if err := config.BindFlags(v, cmd.Root().PersistentFlags()); err != nil {
		return nil, err
	}
	return config.Load(v)
}

// setupLogger installs a slog default logger at the configured level (JSON).
func setupLogger(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warning", "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
}

// buildEngine constructs the query engine from configuration.
func buildEngine(cfg *config.Config) *ntp.Engine {
	return ntp.NewEngine(ntp.Config{
		Samples:       cfg.NTP.Samples,
		Timeout:       cfg.NTP.Timeout,
		Workers:       cfg.NTP.Workers,
		SampleSpacing: sampleSpacing,
	})
}
