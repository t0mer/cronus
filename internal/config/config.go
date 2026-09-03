// Package config loads Cronus configuration with the precedence
// flags > env > YAML file > built-in defaults, using Viper. The environment
// prefix is CRONUS_ and nested keys map to underscores
// (e.g. CRONUS_NTP_SAMPLES for ntp.samples).
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config is the fully-resolved Cronus configuration.
type Config struct {
	Listen  string  `mapstructure:"listen"`
	DB      DB      `mapstructure:"db"`
	NTP     NTP     `mapstructure:"ntp"`
	Monitor Monitor `mapstructure:"monitor"`
	Compare Compare `mapstructure:"compare"`
	Log     Log     `mapstructure:"log"`
}

// DB holds storage settings.
type DB struct {
	Path string `mapstructure:"path"`
}

// NTP holds query-engine settings.
type NTP struct {
	Samples int           `mapstructure:"samples"`
	Timeout time.Duration `mapstructure:"timeout"`
	Workers int           `mapstructure:"workers"`
}

// Monitor holds monitoring-mode settings.
type Monitor struct {
	Interval  time.Duration `mapstructure:"interval"`
	Retention time.Duration `mapstructure:"retention"`
}

// Compare holds comparison thresholds.
type Compare struct {
	OutlierThreshold time.Duration `mapstructure:"outlier_threshold"`
}

// Log holds logging settings.
type Log struct {
	Level string `mapstructure:"level"`
}

// Defaults returns a Viper pre-populated with Cronus's default values and
// environment binding configured. Callers may bind command-line flags onto it
// (BindFlags) and set a config file before calling Load.
func Defaults() *viper.Viper {
	v := viper.New()
	v.SetDefault("listen", ":8080")
	v.SetDefault("db.path", "/data/cronus.db")
	v.SetDefault("ntp.samples", 4)
	v.SetDefault("ntp.timeout", "5s")
	v.SetDefault("ntp.workers", 8)
	v.SetDefault("monitor.interval", "5m")
	v.SetDefault("monitor.retention", "720h")
	v.SetDefault("compare.outlier_threshold", "100ms")
	v.SetDefault("log.level", "info")

	v.SetEnvPrefix("CRONUS")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	return v
}

// BindFlags binds a pflag.FlagSet onto the Viper instance so command-line flags
// take precedence over env and file values. Flag names must match the dotted
// config keys (e.g. "ntp.samples").
func BindFlags(v *viper.Viper, fs *pflag.FlagSet) error {
	return v.BindPFlags(fs)
}

// Load reads the config file (if one was set on v) and unmarshals the resolved
// configuration, honouring duration strings.
func Load(v *viper.Viper) (*Config, error) {
	if v.ConfigFileUsed() != "" || v.GetString("config") != "" {
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}
	var cfg Config
	// Viper's default decoder already applies StringToTimeDurationHookFunc, so
	// duration strings ("5s", "720h") decode into time.Duration fields.
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.NTP.Samples < 1 || c.NTP.Samples > 10 {
		return fmt.Errorf("ntp.samples must be 1..10, got %d", c.NTP.Samples)
	}
	if c.NTP.Timeout <= 0 {
		return fmt.Errorf("ntp.timeout must be positive")
	}
	if c.NTP.Workers < 1 {
		return fmt.Errorf("ntp.workers must be >= 1")
	}
	return nil
}
