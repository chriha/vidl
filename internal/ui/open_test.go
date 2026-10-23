package ui

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chriha/vidl/internal/queue"
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

const site = "https://example.com/"

// browseModel returns a model whose browser records the URLs it is given.
func browseModel(t *testing.T, urls ...string) (Model, *queue.Manager, *fakeRunner, *[]string) {
	t.Helper()
	m, mgr, r := newTestModel(t, 1, urls...)
	var got []string
	m.browse = func(url string) error { got = append(got, url); return nil }
	return m, mgr, r, &got
}

// browse presses b and runs the resulting command, feeding its message back.
func browse(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(keyMsg("b"))
	m = next.(Model)
	if cmd == nil {
		return m
	}
	// A toast also returns a (slow) expiry command; only run the opener's.
	if m.toast.err != nil {
		return m
	}
	return update(t, m, cmd())
}

func TestBrowseOpensURLOnBothTabs(t *testing.T) {
	m, mgr, r, got := browseModel(t, site+"a")
	m = browse(t, m)
	m = finishAll(t, m, mgr, r, site+"a")
	m = press(t, m, "2")
	m = browse(t, m)
	if want := []string{site + "a", site + "a"}; !slices.Equal(*got, want) {
		t.Errorf("opened %v, want %v", *got, want)
	}
	if m.toast.text != "" {
		t.Errorf("toast %q", m.toast.text)
	}
}

func TestBrowseAnyStatus(t *testing.T) {
	m, mgr, _, got := browseModel(t, site+"run", site+"queued", site+"paused", site+"fail")
	ids := map[string]string{}
	for _, it := range m.queue {
		ids[it.URL] = it.ID
	}
	if err := mgr.Pause(ids[site+"paused"]); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Start(ids[site+"fail"]); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "statuses", func() bool {
		a, _ := mgr.Get(ids[site+"run"])
		f, _ := mgr.Get(ids[site+"fail"])
		return a.Status == queue.Downloading && f.Status == queue.Failed
	})
	m = update(t, m, changedMsg{})
	for _, tt := range []struct {
		url    string
		status queue.Status
	}{
		{site + "run", queue.Downloading},
		{site + "queued", queue.Queued},
		{site + "paused", queue.Paused},
		{site + "fail", queue.Failed},
	} {
		t.Run(tt.status.String(), func(t *testing.T) {
			m := selectURL(t, m, tt.url)
			if it, _ := m.selected(); it.Status != tt.status {
				t.Fatalf("status = %v, want %v", it.Status, tt.status)
			}
			*got = nil
			browse(t, m)
			if want := []string{tt.url}; !slices.Equal(*got, want) {
				t.Errorf("opened %v, want %v", *got, want)
			}
		})
	}
}

func TestBrowseOpensOnlyCursorRow(t *testing.T) {
	m, _, _, got := browseModel(t, site+"a", site+"b", site+"c")
	m = press(t, m, "space", "space") // marks a and b, cursor on c
	m = browse(t, m)
	if want := []string{site + "c"}; !slices.Equal(*got, want) {
		t.Errorf("opened %v, want %v", *got, want)
	}
}

func TestBrowseRejectsNonWebURLs(t *testing.T) {
	for _, url := range []string{"file:///etc/passwd", "ftp://example.com/a", "not a url", "https://", "http:///a", "//example.com/a", "-flag"} {
		t.Run(url, func(t *testing.T) {
			m, _, _, got := browseModel(t, url)
			m = browse(t, m)
			if len(*got) != 0 {
				t.Errorf("opened %v", *got)
			}
			if m.toast.err == nil || !strings.Contains(m.toast.text, "not a web URL") {
				t.Errorf("toast = %q, want not a web URL", m.toast.text)
			}
		})
	}
}

func TestBrowseReportsOpenerError(t *testing.T) {
	m, _, _, _ := browseModel(t, site+"a")
	m.browse = func(string) error { return errors.New("no browser") }
	if m = browse(t, m); m.toast.err == nil {
		t.Error("opener error not reported")
	}
}

func TestBrowseIgnoresSettingsTabAndEmptyList(t *testing.T) {
	m, _, _, got := browseModel(t, site+"a")
	browse(t, press(t, m, "3"))
	empty, _, _, got2 := browseModel(t)
	browse(t, empty)
	if len(*got)+len(*got2) != 0 {
		t.Errorf("opened %v %v", *got, *got2)
	}
}

func TestBrowseCommand(t *testing.T) {
	const url = "https://example.com/watch?v=a&t=1"
	for _, tt := range []struct {
		goos string
		want openSpec
	}{
		{"darwin", openSpec{name: "open", args: []string{url}}},
		{"windows", openSpec{name: "rundll32.exe", args: []string{"url.dll,FileProtocolHandler", url}}},
		{"linux", openSpec{name: "xdg-open", args: []string{url}}},
	} {
		t.Run(tt.goos, func(t *testing.T) {
			got := openCommand(tt.goos, url, false)
			if got.name != tt.want.name || !slices.Equal(got.args, tt.want.args) || got.cmdLine != "" {
				t.Errorf("openCommand = %+v, want %+v", got, tt.want)
			}
		})
	}
}
