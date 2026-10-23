package config

import (
	"os"
	"path/filepath"
	"testing"

	"vidl/internal/ytdlp"
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
