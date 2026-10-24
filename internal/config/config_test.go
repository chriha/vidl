package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chriha/vidl/internal/ytdlp"
)

func TestLoadMissingReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != Default() {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	want := Default()
	want.Concurrency = 5
	want.DefaultFormat = ytdlp.MKV
	want.CookiesFromBrowser = "firefox"
	want.Notify = false
	want.SponsorBlockRemove = "sponsor,selfpromo"
	want.WaitEnabled = true
	want.WaitDuration = "2m"
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoadNormalizesInvalidValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := "concurrency = 99\ndefault_format = \"avi\"\ndefault_resolution = \"999\"\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Concurrency != MaxConcurrency || cfg.DefaultFormat != ytdlp.MP4 || cfg.DefaultResolution != "1080" {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestNotifyDefaultsToOn(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{name: "key missing", data: "concurrency = 2\n", want: true},
		{name: "on", data: "notify = true\n", want: true},
		{name: "off", data: "notify = false\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tt.data), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Notify != tt.want {
				t.Errorf("notify = %v, want %v", cfg.Notify, tt.want)
			}
		})
	}
}

func TestSponsorBlockRemoveKey(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "key missing", data: "concurrency = 2\n", want: ""},
		{name: "set", data: "sponsorblock_remove = \"sponsor,intro\"\n", want: "sponsor,intro"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tt.data), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.SponsorBlockRemove != tt.want {
				t.Errorf("sponsorblock_remove = %q, want %q", cfg.SponsorBlockRemove, tt.want)
			}
		})
	}
}

func TestWaitDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WaitEnabled {
		t.Error("wait_enabled defaults to on")
	}
	if cfg.WaitDuration != "30s" || cfg.WaitTime() != 30*time.Second {
		t.Errorf("wait_duration = %q (%v), want 30s", cfg.WaitDuration, cfg.WaitTime())
	}
}

func TestWaitKeysLoad(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		enabled  bool
		duration string
		want     time.Duration
	}{
		{name: "keys missing", data: "concurrency = 2\n", duration: "30s", want: 30 * time.Second},
		{name: "set", data: "wait_enabled = true\nwait_duration = \"1m30s\"\n", enabled: true, duration: "1m30s", want: 90 * time.Second},
		{name: "minimum", data: "wait_duration = \"1s\"\n", duration: "1s", want: time.Second},
		{name: "maximum", data: "wait_duration = \"1h\"\n", duration: "1h", want: time.Hour},
		{name: "zero", data: "wait_duration = \"0s\"\n", duration: "30s", want: 30 * time.Second},
		{name: "below minimum", data: "wait_duration = \"500ms\"\n", duration: "30s", want: 30 * time.Second},
		{name: "above maximum", data: "wait_duration = \"2h\"\n", duration: "30s", want: 30 * time.Second},
		{name: "unparsable", data: "wait_duration = \"abc\"\n", duration: "30s", want: 30 * time.Second},
		{name: "empty", data: "wait_duration = \"\"\n", duration: "30s", want: 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tt.data), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.WaitEnabled != tt.enabled || cfg.WaitDuration != tt.duration || cfg.WaitTime() != tt.want {
				t.Errorf("wait_enabled = %v, wait_duration = %q (%v); want %v, %q (%v)",
					cfg.WaitEnabled, cfg.WaitDuration, cfg.WaitTime(), tt.enabled, tt.duration, tt.want)
			}
		})
	}
}

func TestParseWaitDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		invalid bool
	}{
		{input: "45s", want: 45 * time.Second},
		{input: "2m", want: 2 * time.Minute},
		{input: " 1m30s ", want: 90 * time.Second},
		{input: "1s", want: time.Second},
		{input: "1h", want: time.Hour},
		{input: "0s", invalid: true},
		{input: "999ms", invalid: true},
		{input: "1h1s", invalid: true},
		{input: "2h", invalid: true},
		{input: "-5s", invalid: true},
		{input: "30", invalid: true},
		{input: "abc", invalid: true},
		{input: "", invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseWaitDuration(tt.input)
			if tt.invalid {
				if err == nil {
					t.Errorf("got %v, want an error", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("got %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}
