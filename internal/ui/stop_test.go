package ui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
)

func TestStopKeyTogglesQueueStop(t *testing.T) {
	for _, keys := range [][]string{{"S"}, {"2", "S"}} { // Downloads and Finished tabs
		m, mgr, _ := newTestModel(t, 1, "a") // a is downloading
		m = press(t, m, keys...)
		if !mgr.Stopped() {
			t.Fatalf("%v: queue not stopped", keys)
		}
		if got := m.toast.text; got != "Queue stopped — 1 download paused" {
			t.Errorf("toast = %q", got)
		}
		m = press(t, m, "S")
		if mgr.Stopped() {
			t.Fatalf("%v: queue still stopped", keys)
		}
		if got := m.toast.text; !strings.HasPrefix(got, "Queue started — ") {
			t.Errorf("toast = %q", got)
		}
	}
}

func TestStopKeyIgnoredOnSettingsTab(t *testing.T) {
	m, mgr, _ := newTestModel(t, 1, "a")
	press(t, m, "3", "S")
	if mgr.Stopped() {
		t.Fatal("S stopped the queue on the Settings tab")
	}
}

func TestTabBarShowsQueueBadge(t *testing.T) {
	m, mgr, _ := newTestModel(t, 1, "a")
	bar := func() string { return ansi.Strip(m.tabBar()) }
	if strings.Contains(bar(), "HELD") || strings.Contains(bar(), "STOPPED") {
		t.Fatalf("tab bar %q shows a badge without a hold", bar())
	}
	m = press(t, m, "P")
	for _, tb := range []string{"1", "2", "3"} {
		m = press(t, m, tb)
		if got := bar(); !strings.HasSuffix(strings.TrimRight(got, " "), "⏸ HELD") {
			t.Errorf("tab %s: tab bar %q lacks the HELD badge at its end", tb, got)
		}
	}
	m = press(t, m, "1", "S")
	if !mgr.Stopped() {
		t.Fatal("queue not stopped")
	}
	if got := bar(); !strings.HasSuffix(strings.TrimRight(got, " "), "⏸ STOPPED") {
		t.Errorf("tab bar %q lacks the STOPPED badge at its end", got)
	}
	if w := ansi.StringWidth(m.tabBar()); w > m.width {
		t.Errorf("tab bar wider than the terminal (%d > %d)", w, m.width)
	}
	if strings.Contains(ansi.Strip(m.statusLine()), "queue held") {
		t.Error("status line still shows the hold")
	}
}

func TestStopHelpOnDownloadsTab(t *testing.T) {
	has := func(bs []key.Binding) bool {
		return slices.ContainsFunc(bs, func(b key.Binding) bool { return slices.Contains(b.Keys(), "S") })
	}
	k := tabKeys{keyMap: newKeyMap(), tab: tabDownloads}
	if !has(k.ShortHelp()) || !has(slices.Concat(k.FullHelp()...)) {
		t.Error("S missing from the Downloads help")
	}
}
