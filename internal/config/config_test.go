package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestDefaults(t *testing.T) {
	cfg, err := Load(Defaults())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Errorf("Listen = %q, want :8080", cfg.Listen)
	}
	if cfg.NTP.Samples != 4 {
		t.Errorf("Samples = %d, want 4", cfg.NTP.Samples)
	}
	if cfg.NTP.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", cfg.NTP.Timeout)
	}
	if cfg.Monitor.Retention != 720*time.Hour {
		t.Errorf("Retention = %v, want 720h", cfg.Monitor.Retention)
	}
	if cfg.Compare.OutlierThreshold != 100*time.Millisecond {
		t.Errorf("OutlierThreshold = %v, want 100ms", cfg.Compare.OutlierThreshold)
	}
}

func TestEnvOverridesDefault(t *testing.T) {
	t.Setenv("CRONUS_NTP_SAMPLES", "7")
	t.Setenv("CRONUS_LISTEN", ":9999")
	cfg, err := Load(Defaults())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.NTP.Samples != 7 {
		t.Errorf("Samples = %d, want 7 (from env)", cfg.NTP.Samples)
	}
	if cfg.Listen != ":9999" {
		t.Errorf("Listen = %q, want :9999 (from env)", cfg.Listen)
	}
}

func TestYAMLOverridesDefaultAndEnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := "listen: \":7000\"\nntp:\n  samples: 5\n  workers: 3\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	v := Defaults()
	v.SetConfigFile(path)
	// env beats YAML
	t.Setenv("CRONUS_NTP_WORKERS", "2")

	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":7000" {
		t.Errorf("Listen = %q, want :7000 (from yaml)", cfg.Listen)
	}
	if cfg.NTP.Samples != 5 {
		t.Errorf("Samples = %d, want 5 (from yaml)", cfg.NTP.Samples)
	}
	if cfg.NTP.Workers != 2 {
		t.Errorf("Workers = %d, want 2 (env beats yaml)", cfg.NTP.Workers)
	}
}

func TestFlagOverridesEnv(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.String("listen", ":8080", "")
	fs.Int("ntp.samples", 4, "")
	_ = fs.Parse([]string{"--listen", ":6000", "--ntp.samples", "9"})

	t.Setenv("CRONUS_LISTEN", ":9999") // should lose to the flag

	v := Defaults()
	if err := BindFlags(v, fs); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(v)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":6000" {
		t.Errorf("Listen = %q, want :6000 (flag beats env)", cfg.Listen)
	}
	if cfg.NTP.Samples != 9 {
		t.Errorf("Samples = %d, want 9 (from flag)", cfg.NTP.Samples)
	}
}

func TestValidateRejectsBadSamples(t *testing.T) {
	t.Setenv("CRONUS_NTP_SAMPLES", "0")
	if _, err := Load(Defaults()); err == nil {
		t.Fatal("expected validation error for samples=0")
	}
}
