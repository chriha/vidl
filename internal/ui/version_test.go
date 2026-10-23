package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func versionModel(t *testing.T, version string, err error) Model {
	t.Helper()
	m, _, _ := newTestModel(t, 1)
	m.now = func() time.Time { return time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC) }
	return m.WithVersionCheck(func(context.Context) (string, error) { return version, err })
}

// runCheck executes the startup command and feeds its message to the model.
func runCheck(t *testing.T, m Model) Model {
	t.Helper()
	cmd := m.checkVersion()
	if cmd == nil {
		t.Fatal("no version check command")
	}
	return update(t, m, cmd())
}

func TestVersionCheckRunsAsCommand(t *testing.T) {
	m := versionModel(t, "2026.01.01", nil)
	if m.toast.text != "" {
		t.Fatalf("toast %q shown before the command ran", m.toast.text)
	}
	if _, ok := m.checkVersion()().(versionMsg); !ok {
		t.Error("command did not yield a versionMsg")
	}
	if cmd := m.Init(); cmd == nil {
		t.Error("Init returned no command")
	}
	m.versionFn = nil
	if m.checkVersion() != nil {
		t.Error("check scheduled without a version function")
	}
}

func TestVersionCheckWarnsWhenStale(t *testing.T) {
	m := runCheck(t, versionModel(t, "2026.08.01", nil))
	if !strings.Contains(m.toast.text, "83 days") || !strings.Contains(m.toast.text, "yt-dlp -U") {
		t.Errorf("toast = %q", m.toast.text)
	}
	if m.toast.err != nil || !m.toast.long {
		t.Errorf("want a long notice, got err=%v long=%v", m.toast.err, m.toast.long)
	}
	if _, cmd := versionModel(t, "2026.08.01", nil).Update(versionMsg{version: "2026.08.01"}); cmd == nil {
		t.Error("no expiry timer for the warning")
	}
}

func TestVersionCheckQuietWhenFreshOrUnparseable(t *testing.T) {
	for _, v := range []string{"2026.08.24", "2026.10.20.123456", "master-abc", ""} {
		if m := runCheck(t, versionModel(t, v, nil)); m.toast.text != "" {
			t.Errorf("version %q: unexpected toast %q", v, m.toast.text)
		}
	}
}

func TestVersionCheckErrorIsShown(t *testing.T) {
	m := runCheck(t, versionModel(t, "", errors.New("yt-dlp --version: boom")))
	if m.toast.err == nil || !strings.Contains(m.toast.text, "boom") {
		t.Errorf("toast = %q err=%v", m.toast.text, m.toast.err)
	}
}
