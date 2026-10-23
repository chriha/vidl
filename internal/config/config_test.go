package config

import (
	"os"
	"path/filepath"
	"testing"

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
