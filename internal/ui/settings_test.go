package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"vidl/internal/config"
)

// settingsModel returns a model on the Settings tab with the cursor on the
// setting with the given label.
func settingsModel(t *testing.T, label string) Model {
	t.Helper()
	m, _, _ := newTestModel(t, 1)
	m = press(t, m, "3")
	for m.settings.cursor < len(settings) && settings[m.settings.cursor].label != label {
		m = press(t, m, "j")
	}
	if settings[m.settings.cursor].label != label {
		t.Fatalf("no setting %q", label)
	}
	return m
}

// saved reads the config file the model saves to.
func saved(t *testing.T, m Model) config.Config {
	t.Helper()
	if m.toast.err != nil {
		t.Fatal(m.toast.err)
	}
	cfg, err := config.Load(m.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestSettingsConcurrencyClamped(t *testing.T) {
	m := settingsModel(t, "Concurrent downloads")
	m = press(t, m, "l", "l")
	if got := m.mgr.Config().Concurrency; got != 3 {
		t.Errorf("after l l: concurrency = %d, want 3", got)
	}
	if got := saved(t, m).Concurrency; got != 3 {
		t.Errorf("saved concurrency = %d, want 3", got)
	}
	for range 5 {
		m = press(t, m, "h")
	}
	if got := m.mgr.Config().Concurrency; got != config.MinConcurrency {
		t.Errorf("after h×5: concurrency = %d, want %d", got, config.MinConcurrency)
	}
	for range 15 {
		m = press(t, m, "l")
	}
	if got := m.mgr.Config().Concurrency; got != config.MaxConcurrency {
		t.Errorf("after l×15: concurrency = %d, want %d", got, config.MaxConcurrency)
	}
}

func TestSettingsHLStayOnTab(t *testing.T) {
	m := settingsModel(t, "Default file type")
	m = press(t, m, "h", "l", "l")
	if m.tab != tabSettings {
		t.Fatalf("tab = %v, want settings", m.tab)
	}
	m = press(t, m, "tab")
	if m.tab != tabDownloads {
		t.Errorf("tab key: tab = %v, want downloads", m.tab)
	}
}

func TestSettingsEnumWraps(t *testing.T) {
	m := settingsModel(t, "Cookies from browser")
	m = press(t, m, "h")
	if got := m.mgr.Config().CookiesFromBrowser; got != "vivaldi" {
		t.Errorf("h from none: %q, want vivaldi", got)
	}
	m = press(t, m, "l", "l")
	if got := saved(t, m).CookiesFromBrowser; got != "brave" {
		t.Errorf("saved %q, want brave", got)
	}
}

func TestSettingsBoolToggle(t *testing.T) {
	m := settingsModel(t, "Embed thumbnail")
	m = press(t, m, "space")
	if !saved(t, m).EmbedThumbnail {
		t.Error("space didn't enable")
	}
	m = press(t, m, "enter")
	if saved(t, m).EmbedThumbnail {
		t.Error("enter didn't disable")
	}
}

func TestSettingsRateLimit(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		invalid bool
	}{
		{input: "500k", want: "500K"},
		{input: "1.5M", want: "1.5M"},
		{input: "42", want: "42"},
		{input: "", want: ""},
		{input: "fast", invalid: true},
		{input: "5MB", invalid: true},
		{input: "-1M", invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m := settingsModel(t, "Rate limit")
			m = press(t, m, "enter")
			if m.settings.edit == nil {
				t.Fatal("enter didn't open the editor")
			}
			m.settings.edit.SetValue(tt.input)
			m = press(t, m, "enter")
			if tt.invalid {
				if m.settings.err == nil || m.settings.edit == nil {
					t.Errorf("err = %v, editor open = %v; want error and open editor", m.settings.err, m.settings.edit != nil)
				}
				if m.mgr.Config().RateLimit != "" {
					t.Errorf("applied %q", m.mgr.Config().RateLimit)
				}
				return
			}
			if m.settings.edit != nil {
				t.Fatalf("editor still open: %v", m.settings.err)
			}
			if got := saved(t, m).RateLimit; got != tt.want {
				t.Errorf("saved %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSettingsEditKeysGoToInput(t *testing.T) {
	m := settingsModel(t, "Subtitle languages")
	m = press(t, m, "enter")
	m.settings.edit.SetValue("")
	m = typeText(t, m, "qjl2")
	if m.tab != tabSettings || m.settings.edit == nil {
		t.Fatalf("tab = %v, editor open = %v", m.tab, m.settings.edit != nil)
	}
	if got := m.settings.edit.Value(); got != "qjl2" {
		t.Errorf("input = %q", got)
	}
	m = press(t, m, "esc")
	if m.settings.edit != nil || m.mgr.Config().SubtitleLangs != "en.*" {
		t.Errorf("esc: editor open = %v, langs = %q", m.settings.edit != nil, m.mgr.Config().SubtitleLangs)
	}
}

func TestSettingsDownloadDir(t *testing.T) {
	existing := t.TempDir()
	file := filepath.Join(existing, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{name: "existing", input: existing, valid: true},
		{name: "relative", input: "downloads"},
		{name: "empty", input: "  "},
		{name: "file", input: file},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := settingsModel(t, "Download folder")
			before := m.mgr.Config().DownloadDir
			m = press(t, m, "enter")
			m.settings.edit.SetValue(tt.input)
			m = press(t, m, "enter")
			if !tt.valid {
				if m.settings.err == nil || m.mgr.Config().DownloadDir != before {
					t.Errorf("err = %v, dir = %q; want error and unchanged", m.settings.err, m.mgr.Config().DownloadDir)
				}
				return
			}
			if got := saved(t, m).DownloadDir; got != tt.input {
				t.Errorf("saved %q, want %q", got, tt.input)
			}
		})
	}
}

func TestSettingsDownloadDirCreate(t *testing.T) {
	m := settingsModel(t, "Download folder")
	dir := filepath.Join(t.TempDir(), "new", "dl")
	m = press(t, m, "enter")
	m.settings.edit.SetValue(dir)
	m = press(t, m, "enter")
	if m.settings.mkdir != dir {
		t.Fatalf("mkdir prompt = %q, want %q", m.settings.mkdir, dir)
	}
	m = press(t, m, "n")
	if m.settings.mkdir != "" || m.settings.edit == nil {
		t.Fatalf("n: prompt %q, editor open = %v; want editor back", m.settings.mkdir, m.settings.edit != nil)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("n created the folder")
	}
	m = press(t, m, "enter", "y")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	if got := saved(t, m).DownloadDir; got != dir || m.settings.edit != nil {
		t.Errorf("saved %q, editor open = %v", got, m.settings.edit != nil)
	}
}

func TestSettingsSaveErrorShown(t *testing.T) {
	m := settingsModel(t, "Embed metadata")
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m.cfgPath = filepath.Join(blocker, "config.toml")
	m = press(t, m, "space")
	if m.toast.err == nil {
		t.Fatal("no save error")
	}
	if m.mgr.Config().EmbedMetadata {
		t.Error("change not applied to the manager despite the save error")
	}
}

func TestSettingsRenderFits(t *testing.T) {
	for _, size := range [][2]int{{100, 12}, {100, 30}, {40, 10}} {
		m := settingsModel(t, "Rate limit")
		m = update(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, editing := range []bool{false, true} {
			if editing {
				m = press(t, m, "enter")
			}
			out := m.render()
			for _, line := range strings.Split(out, "\n") {
				if w := ansi.StringWidth(line); w > m.width {
					t.Errorf("%v editing=%v: line wider than %d (%d): %q", size, editing, m.width, w, line)
				}
			}
			if h := strings.Count(out, "\n") + 1; h > m.height {
				t.Errorf("%v editing=%v: height %d > %d", size, editing, h, m.height)
			}
			if !strings.Contains(out, "Rate limit") {
				t.Errorf("%v editing=%v: focused setting not visible", size, editing)
			}
		}
	}
}
