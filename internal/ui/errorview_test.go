package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/vidl/internal/queue"
)

// failedModel returns a model whose selected download "fail-a" has failed
// after logging fakeLogLines lines, plus a running "b".
func failedModel(t *testing.T) (Model, *queue.Manager) {
	t.Helper()
	m, mgr, _ := newTestModel(t, 1, "fail-a", "b")
	waitFor(t, "fail-a failed", func() bool {
		q, _ := mgr.Snapshot()
		return slices.ContainsFunc(q, func(it queue.Item) bool { return it.URL == "fail-a" && it.Status == queue.Failed })
	})
	m = update(t, m, changedMsg{})
	return selectURL(t, m, "fail-a"), mgr
}

func TestErrorViewOpensOnFailed(t *testing.T) {
	m, _ := failedModel(t)
	it, _ := m.selected()
	m = press(t, m, "i")
	if m.errView == nil {
		t.Fatal("i didn't open the error view")
	}
	if m.errView.id != it.ID {
		t.Errorf("view shows %q, want %q", m.errView.id, it.ID)
	}
	if !m.errView.vp.AtTop() {
		t.Error("view didn't open at the top")
	}
	text := ansi.Strip(m.errView.vp.GetContent())
	for _, want := range []string{"failed", "fail-a", "boom", fmt.Sprintf("yt-dlp output (%d lines)", fakeLogLines), "fail-a line 1\n", fmt.Sprintf("fail-a line %d", fakeLogLines)} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	assertFits(t, m)
}

func TestErrorViewNothingToShow(t *testing.T) {
	tests := []struct {
		name string
		urls []string
	}{
		{name: "running download", urls: []string{"a"}},
		{name: "nothing selected", urls: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, _ := newTestModel(t, 1, tt.urls...)
			m = press(t, m, "i")
			if m.errView != nil {
				t.Error("error view opened")
			}
			if m.toast.text != "No error to show" {
				t.Errorf("toast = %q", m.toast.text)
			}
		})
	}
}

func TestErrorViewCloses(t *testing.T) {
	for _, k := range []string{"esc", "i", "q"} {
		t.Run(k, func(t *testing.T) {
			m, _ := failedModel(t)
			m = press(t, m, "i")
			msg := tea.KeyPressMsg{Code: rune(k[0]), Text: k}
			if k == "esc" {
				msg = tea.KeyPressMsg{Code: tea.KeyEscape}
			}
			next, cmd := m.Update(msg)
			if next.(Model).errView != nil {
				t.Errorf("%s didn't close the view", k)
			}
			if cmd != nil {
				t.Errorf("%s returned a command, want none (no quit)", k)
			}
		})
	}
}

func TestErrorViewScrolls(t *testing.T) {
	m, _ := failedModel(t)
	m = press(t, m, "i")
	vp := func() int { return m.errView.vp.YOffset() }
	steps := []struct {
		key  string
		want func() bool
	}{
		{"j", func() bool { return vp() == 1 }},
		{"k", func() bool { return vp() == 0 }},
		{"G", func() bool { return m.errView.vp.AtBottom() && vp() > 0 }},
		{"g", func() bool { return vp() == 0 }},
		{"ctrl+d", func() bool { return vp() > 1 }},
		{"ctrl+u", func() bool { return vp() == 0 }},
	}
	for _, s := range steps {
		m = press(t, m, s.key)
		if m.errView == nil {
			t.Fatalf("%s closed the view", s.key)
		}
		if !s.want() {
			t.Errorf("after %s: offset %d", s.key, vp())
		}
	}
}

func TestErrorViewClosesWhenItemGone(t *testing.T) {
	m, mgr := failedModel(t)
	it, _ := m.selected()
	m = press(t, m, "i")
	if err := mgr.Remove(it.ID); err != nil {
		t.Fatal(err)
	}
	if m = update(t, m, changedMsg{}); m.errView != nil {
		t.Error("view still open after the item was removed")
	}
}

func TestErrorViewLines(t *testing.T) {
	longErr := strings.Repeat("ERROR: [youtube] abc: Sign in to confirm you're not a bot. ", 4) + "END"
	tests := []struct {
		name string
		log  []string
		want []string
	}{
		{name: "saved output", log: []string{"first", "second"}, want: []string{"yt-dlp output (2 lines)", "first", "second"}},
		{name: "no saved output", want: []string{"no output saved"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := queue.Item{URL: "https://example.com/v", Title: "Video", Status: queue.Failed, Error: longErr, Log: tt.log}
			const w = 40
			lines := errorViewLines(it, w)
			for _, l := range lines {
				if ansi.StringWidth(l) > w {
					t.Errorf("line wider than %d: %q", w, l)
				}
			}
			text := ansi.Strip(strings.Join(lines, "\n"))
			if strings.Contains(text, "…") {
				t.Errorf("content truncated:\n%s", text)
			}
			if !strings.Contains(strings.Join(strings.Fields(text), " "), longErr) {
				t.Errorf("full error missing in:\n%s", text)
			}
			for _, want := range append([]string{"failed", "Video", "https://example.com/v"}, tt.want...) {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q in:\n%s", want, text)
				}
			}
		})
	}
}

func TestErrorViewHelp(t *testing.T) {
	m, _ := failedModel(t)
	if !hasHelpKey(m.helpKeys(), "i") {
		t.Error("help lacks i on a failed download")
	}
	m = selectURL(t, m, "b")
	if hasHelpKey(m.helpKeys(), "i") {
		t.Error("help shows i on a running download")
	}
}

// hasHelpKey reports whether any help of k lists an enabled binding labelled name.
func hasHelpKey(k tabKeys, name string) bool {
	all := [][]key.Binding{k.ShortHelp()}
	all = append(all, k.FullHelp()...)
	for _, group := range all {
		for _, b := range group {
			if b.Enabled() && b.Help().Key == name {
				return true
			}
		}
	}
	return false
}
