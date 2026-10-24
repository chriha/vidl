package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/chriha/vidl/internal/ytdlp"
)

const (
	MinConcurrency = 1
	MaxConcurrency = 10

	MinWait = time.Second // shortest wait_duration
	MaxWait = time.Hour   // longest wait_duration

	defaultWait = 30 * time.Second
)

// Browsers that yt-dlp can read cookies from.
var Browsers = []string{"", "brave", "chrome", "chromium", "edge", "firefox", "opera", "safari", "vivaldi"}

type Config struct {
	DownloadDir        string       `toml:"download_dir"`
	Concurrency        int          `toml:"concurrency"`
	DefaultResolution  string       `toml:"default_resolution"`
	DefaultFormat      ytdlp.Format `toml:"default_format"`
	FilenameTemplate   string       `toml:"filename_template"`
	EmbedSubtitles     bool         `toml:"embed_subtitles"`
	SubtitleLangs      string       `toml:"subtitle_langs"`
	EmbedThumbnail     bool         `toml:"embed_thumbnail"`
	EmbedMetadata      bool         `toml:"embed_metadata"`
	RateLimit          string       `toml:"rate_limit"`
	CookiesFromBrowser string       `toml:"cookies_from_browser"`
	Notify             bool         `toml:"notify"`              // desktop notification when a download finishes or fails
	SponsorBlockRemove string       `toml:"sponsorblock_remove"` // SponsorBlock categories to cut out, empty to disable
	WaitEnabled        bool         `toml:"wait_enabled"`        // wait between downloads
	WaitDuration       string       `toml:"wait_duration"`       // Go duration, MinWait to MaxWait
}

func Default() Config {
	return Config{
		DownloadDir:       "~/Downloads",
		Concurrency:       3,
		DefaultResolution: "1080",
		DefaultFormat:     ytdlp.MP4,
		FilenameTemplate:  "%(title)s [%(id)s].%(ext)s",
		SubtitleLangs:     "en.*",
		EmbedMetadata:     true,
		Notify:            true,
		WaitDuration:      defaultWait.String(),
	}
}

// Dir returns the directory holding vidl's config and state files:
// $XDG_CONFIG_HOME/vidl, or ~/.config/vidl.
func Dir() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "vidl"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "vidl"), nil
}

// Load reads the config at path. A missing file yields the defaults; missing
// or invalid fields fall back to their defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	_, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	return cfg.Normalize(), err
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Normalize replaces invalid values with defaults.
func (c Config) Normalize() Config {
	def := Default()
	if strings.TrimSpace(c.DownloadDir) == "" {
		c.DownloadDir = def.DownloadDir
	}
	c.Concurrency = min(max(c.Concurrency, MinConcurrency), MaxConcurrency)
	if !slices.Contains(ytdlp.Resolutions, c.DefaultResolution) {
		c.DefaultResolution = def.DefaultResolution
	}
	if !c.DefaultFormat.Valid() {
		c.DefaultFormat = def.DefaultFormat
	}
	if strings.TrimSpace(c.FilenameTemplate) == "" {
		c.FilenameTemplate = def.FilenameTemplate
	}
	if !slices.Contains(Browsers, c.CookiesFromBrowser) {
		c.CookiesFromBrowser = ""
	}
	if _, err := ParseWaitDuration(c.WaitDuration); err != nil {
		c.WaitDuration = def.WaitDuration
	}
	return c
}

// ParseWaitDuration parses a wait_duration value and checks it lies within
// MinWait and MaxWait.
func ParseWaitDuration(v string) (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		return 0, errors.New("use a duration like 30s, 2m or 1m30s")
	}
	if d < MinWait || d > MaxWait {
		return 0, fmt.Errorf("use a duration from %v to %v", MinWait, MaxWait)
	}
	return d, nil
}

// WaitTime returns WaitDuration parsed, or the default for an invalid value.
func (c Config) WaitTime() time.Duration {
	d, err := ParseWaitDuration(c.WaitDuration)
	if err != nil {
		return defaultWait
	}
	return d
}

// DownloadPath returns DownloadDir with a leading ~ expanded.
func (c Config) DownloadPath() string {
	return ExpandHome(c.DownloadDir)
}

func ExpandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}
