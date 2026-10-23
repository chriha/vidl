package ui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
)

func TestHoldKeyTogglesQueueHold(t *testing.T) {
	for _, keys := range [][]string{{"P"}, {"2", "P"}} { // Downloads and Finished tabs
		m, mgr, _ := newTestModel(t, 1, "a")
		m = press(t, m, keys...)
		if !mgr.Held() {
			t.Fatalf("%v: queue not held", keys)
		}
		if got := m.toast.text; !strings.Contains(got, "held") {
			t.Errorf("toast = %q", got)
		}
		m = press(t, m, "P")
		if mgr.Held() {
			t.Fatalf("%v: queue still held", keys)
		}
		if got := m.toast.text; !strings.Contains(got, "resumed") {
			t.Errorf("toast = %q", got)
		}
	}
}

func TestStatusLineShowsQueueHold(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a")
	if strings.Contains(ansi.Strip(m.statusLine()), "queue held") {
		t.Fatal("status line shows a hold that isn't set")
	}
	m = press(t, m, "P")
	if !strings.Contains(ansi.Strip(m.statusLine()), "queue held") {
		t.Errorf("status line %q lacks the hold segment", ansi.Strip(m.statusLine()))
	}
}

func TestHoldHelpOnDownloadsTab(t *testing.T) {
	has := func(bs []key.Binding) bool {
		return slices.ContainsFunc(bs, func(b key.Binding) bool { return slices.Contains(b.Keys(), "P") })
	}
	k := tabKeys{keyMap: newKeyMap(), tab: tabDownloads}
	if !has(k.ShortHelp()) || !has(slices.Concat(k.FullHelp()...)) {
		t.Error("P missing from the Downloads help")
	}
}
