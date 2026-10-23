package ui

import (
	"errors"
	"os"
	"slices"
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

func TestOpenCommand(t *testing.T) {
	const win = `C:\Videos\a [b] & c.mp4`
	for _, tt := range []struct {
		name   string
		goos   string
		path   string
		reveal bool
		want   openSpec
	}{
		{"darwin open", "darwin", "/v/a.mp4", false, openSpec{name: "open", args: []string{"/v/a.mp4"}}},
		{"darwin reveal", "darwin", "/v/a.mp4", true, openSpec{name: "open", args: []string{"-R", "/v/a.mp4"}}},
		{"linux open", "linux", "/v/a.mp4", false, openSpec{name: "xdg-open", args: []string{"/v/a.mp4"}}},
		{"linux reveal opens folder", "linux", "/v/a.mp4", true, openSpec{name: "xdg-open", args: []string{"/v"}}},
		{"windows open", "windows", win, false,
			openSpec{name: "rundll32.exe", args: []string{"url.dll,FileProtocolHandler", win}}},
		{"windows reveal", "windows", win, true,
			openSpec{name: "explorer.exe", cmdLine: `explorer.exe /select,"` + win + `"`, ignoreExit: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := openCommand(tt.goos, tt.path, tt.reveal)
			if got.name != tt.want.name || !slices.Equal(got.args, tt.want.args) ||
				got.cmdLine != tt.want.cmdLine || got.ignoreExit != tt.want.ignoreExit {
				t.Errorf("openCommand = %+v, want %+v", got, tt.want)
			}
		})
	}
}
