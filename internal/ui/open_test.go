package ui

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type openCall struct {
	path   string
	reveal bool
}

func TestOpenFinishedFile(t *testing.T) {
	for _, tt := range []struct {
		key    string
		reveal bool
	}{{"o", false}, {"O", true}} {
		t.Run(tt.key, func(t *testing.T) {
			m, mgr, r := newTestModel(t, 1, "a")
			m = finishAll(t, m, mgr, r, "a")
			var got []openCall
			m.open = func(path string, reveal bool) error {
				got = append(got, openCall{path, reveal})
				return nil
			}
			m = press(t, m, "2")
			next, cmd := m.Update(keyMsg(tt.key))
			m = next.(Model)
			if cmd == nil {
				t.Fatal("no open command")
			}
			m = update(t, m, cmd())
			_, h := mgr.Snapshot()
			if want := []openCall{{h[0].FilePath, tt.reveal}}; len(got) != 1 || got[0] != want[0] {
				t.Errorf("opened %v, want %v", got, want)
			}
			if m.toast.text != "" {
				t.Errorf("toast %q", m.toast.text)
			}
		})
	}
}

func TestOpenReportsErrors(t *testing.T) {
	m, mgr, r := newTestModel(t, 1, "a")
	m = finishAll(t, m, mgr, r, "a")
	m.open = func(string, bool) error { return errors.New("no handler") }
	m = press(t, m, "2")
	_, cmd := m.Update(keyMsg("o"))
	if m = update(t, m, cmd()); m.toast.err == nil {
		t.Error("open error not reported")
	}

	_, h := mgr.Snapshot()
	if err := os.Remove(h[0].FilePath); err != nil {
		t.Fatal(err)
	}
	m.open = func(string, bool) error { t.Error("opened a missing file"); return nil }
	next, cmd := m.Update(keyMsg("o"))
	m = next.(Model)
	if cmd != nil {
		cmd()
	}
	if m.toast.err == nil || !strings.Contains(m.toast.text, "not found") {
		t.Errorf("toast = %q, want file not found", m.toast.text)
	}
}

func TestOpenIgnoresDownloadsTab(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a")
	m.open = func(string, bool) error { t.Error("opened a running download"); return nil }
	if _, cmd := m.Update(keyMsg("o")); cmd != nil {
		cmd()
	}
}

func keyMsg(k string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}
