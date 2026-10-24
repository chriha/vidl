package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/vidl/internal/queue"
)

func TestFormatWait(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{-time.Second, "0s"},
		{23 * time.Second, "23s"},
		{22*time.Second + 100*time.Millisecond, "23s"}, // rounds up while counting down
		{59 * time.Second, "59s"},
		{time.Minute, "1:00"},
		{105 * time.Second, "1:45"},
		{time.Hour, "60:00"},
	}
	for _, tt := range tests {
		if got := formatWait(tt.d); got != tt.want {
			t.Errorf("formatWait(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestWaitKeyTogglesTimer(t *testing.T) {
	for _, keys := range [][]string{{"W"}, {"2", "W"}} { // Downloads and Finished tabs
		m, mgr, _ := newTestModel(t, 1, "a")
		m = press(t, m, keys...)
		if !mgr.Config().WaitEnabled {
			t.Fatalf("%v: wait timer not enabled", keys)
		}
		if !saved(t, m).WaitEnabled {
			t.Errorf("%v: wait_enabled not saved", keys)
		}
		if got, want := m.toast.text, "Wait timer on — 30s between downloads"; got != want {
			t.Errorf("toast = %q, want %q", got, want)
		}
		m = press(t, m, "W")
		if mgr.Config().WaitEnabled {
			t.Fatalf("%v: wait timer still enabled", keys)
		}
		if saved(t, m).WaitEnabled {
			t.Errorf("%v: wait_enabled still saved as true", keys)
		}
		if got, want := m.toast.text, "Wait timer off"; got != want {
			t.Errorf("toast = %q, want %q", got, want)
		}
	}
}

func TestWaitKeyIgnoredOnSettingsTab(t *testing.T) {
	m, mgr, _ := newTestModel(t, 1, "a")
	press(t, m, "3", "W")
	if mgr.Config().WaitEnabled {
		t.Fatal("W enabled the wait timer on the Settings tab")
	}
}

func TestWaitKeySaveErrorShown(t *testing.T) {
	m, mgr, _ := newTestModel(t, 1, "a")
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m.cfgPath = filepath.Join(blocker, "config.toml")
	m = press(t, m, "W")
	if m.toast.err == nil {
		t.Fatal("no save error")
	}
	if !mgr.Config().WaitEnabled {
		t.Error("toggle not applied to the manager despite the save error")
	}
}

func TestWaitHelpOnDownloadsTab(t *testing.T) {
	has := func(bs []key.Binding) bool {
		return slices.ContainsFunc(bs, func(b key.Binding) bool { return slices.Contains(b.Keys(), "W") })
	}
	k := tabKeys{keyMap: newKeyMap(), tab: tabDownloads}
	if !has(k.ShortHelp()) || !has(slices.Concat(k.FullHelp()...)) {
		t.Error("W missing from the Downloads help")
	}
}

// endWithWait enables the wait timer with duration d and finishes the running
// download a, which arms the countdown; held freezes it at d first.
func endWithWait(t *testing.T, m Model, mgr *queue.Manager, r *fakeRunner, d string, held bool) Model {
	t.Helper()
	cfg := mgr.Config()
	cfg.WaitEnabled, cfg.WaitDuration = true, d
	mgr.SetConfig(cfg)
	if held {
		m = press(t, m, "P")
	}
	r.job(t, "a")
	r.finish("a")
	waitFor(t, "countdown", func() bool { _, ok := mgr.WaitRemaining(); return ok })
	return update(t, m, changedMsg{})
}

func TestTabBarWaitBadge(t *testing.T) {
	bar := func(m Model) string { return strings.TrimRight(ansi.Strip(m.tabBar()), " ") }

	t.Run("hidden while off", func(t *testing.T) {
		m, _, _ := newTestModel(t, 1, "a")
		if got := bar(m); strings.Contains(got, "⏱") {
			t.Errorf("tab bar %q shows the wait badge while off", got)
		}
	})
	t.Run("configured duration while idle", func(t *testing.T) {
		m, _, _ := newTestModel(t, 1, "a")
		m = press(t, m, "W")
		if got := bar(m); !strings.HasSuffix(got, "⏱ 30s") {
			t.Errorf("tab bar %q lacks ⏱ 30s at its end", got)
		}
	})
	t.Run("time left in seconds next to the hold badge", func(t *testing.T) {
		m, mgr, r := newTestModel(t, 1, "a", "b")
		m = endWithWait(t, m, mgr, r, "23s", true)
		if got := bar(m); !strings.HasSuffix(got, "⏸ HELD  ⏱ 23s") {
			t.Errorf("tab bar %q lacks ⏸ HELD then ⏱ 23s at its end", got)
		}
		if w := ansi.StringWidth(m.tabBar()); w > m.width {
			t.Errorf("tab bar wider than the terminal (%d > %d)", w, m.width)
		}
	})
	t.Run("time left in minutes", func(t *testing.T) {
		m, mgr, r := newTestModel(t, 1, "a", "b")
		m = endWithWait(t, m, mgr, r, "1m45s", true)
		if got := bar(m); !strings.HasSuffix(got, "⏱ 1:45") {
			t.Errorf("tab bar %q lacks ⏱ 1:45 at its end", got)
		}
	})
	t.Run("live countdown", func(t *testing.T) {
		m, mgr, r := newTestModel(t, 1, "a", "b")
		m = endWithWait(t, m, mgr, r, "2m", false)
		if got := bar(m); !regexp.MustCompile(`⏱ [12]:\d\d$`).MatchString(got) {
			t.Errorf("tab bar %q lacks the countdown at its end", got)
		}
	})
	t.Run("fits a narrow terminal", func(t *testing.T) {
		m, mgr, r := newTestModel(t, 1, "a", "b")
		m = endWithWait(t, m, mgr, r, "1m45s", false)
		m = press(t, m, "S")
		m = update(t, m, tea.WindowSizeMsg{Width: minWidth, Height: 12})
		if w := ansi.StringWidth(m.tabBar()); w > m.width {
			t.Errorf("tab bar %q wider than the terminal (%d > %d)", ansi.Strip(m.tabBar()), w, m.width)
		}
	})
}

func TestWaitTickOnlyWhileCountingDown(t *testing.T) {
	idle, _, _ := newTestModel(t, 1, "a")
	idle = press(t, idle, "W")
	if _, cmd := idle.Update(waitTickMsg{}); cmd != nil {
		t.Error("tick scheduled without a countdown")
	}

	m, mgr, r := newTestModel(t, 1, "a", "b")
	m = endWithWait(t, m, mgr, r, "2m", false)
	next, cmd := m.Update(waitTickMsg{})
	if cmd == nil {
		t.Fatal("no tick scheduled during the countdown")
	}
	m = next.(Model)
	if _, cmd := m.Update(waitTickMsg{}); cmd == nil {
		t.Fatal("countdown stopped ticking")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}); cmd != nil {
		t.Error("a key press scheduled a second tick while one is pending")
	}

	m = press(t, m, "P") // the hold freezes the countdown
	if _, cmd := m.Update(waitTickMsg{}); cmd != nil {
		t.Error("tick scheduled while the countdown is frozen")
	}
}
