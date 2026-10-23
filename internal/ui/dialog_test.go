package ui

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chriha/vidl/internal/queue"
	"github.com/chriha/vidl/internal/ytdlp"
)

// fakeProber returns infos[url], or errors for unknown URLs. URLs in block
// wait until the probe is cancelled.
type fakeProber struct {
	infos map[string]*ytdlp.Info
	block map[string]bool
}

func (f *fakeProber) Probe(ctx context.Context, url, _ string) (*ytdlp.Info, error) {
	if f.block[url] {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if info, ok := f.infos[url]; ok {
		return info, nil
	}
	return nil, errors.New("unsupported URL: " + url)
}

func newDialogModel(t *testing.T, p *fakeProber, urls ...string) (Model, *queue.Manager) {
	t.Helper()
	m, mgr, _ := newTestModel(t, 1, urls...)
	m.prober = p
	return press(t, m, "n"), mgr
}

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = update(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// submit presses enter and returns the probe's result message.
func submit(t *testing.T, m Model) (Model, probeResultMsg) {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(Model), findMsg[probeResultMsg](t, cmd)
}

// findMsg runs cmd, expanding batches, and returns the first message of type T.
func findMsg[T tea.Msg](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	found := make(chan T, 1)
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			for _, sub := range msg {
				go run(sub)
			}
		case T:
			select {
			case found <- msg:
			default:
			}
		}
	}
	go run(cmd)
	select {
	case msg := <-found:
		return msg
	case <-time.After(2 * time.Second):
		var zero T
		t.Fatalf("no %T from command", zero)
		return zero
	}
}

func TestDialogProbeAndQueue(t *testing.T) {
	p := &fakeProber{infos: map[string]*ytdlp.Info{
		"https://x.test/v": {Title: "Video", URL: "https://x.test/v", Uploader: "me", Heights: []int{720, 360}},
	}}
	m, mgr := newDialogModel(t, p, "a")
	m = typeText(t, m, "https://x.test/v")
	m, res := submit(t, m)
	if m.dialog.step != stepProbing {
		t.Fatalf("step = %d, want probing", m.dialog.step)
	}
	m = update(t, m, res)
	d := m.dialog
	if d.step != stepDetails {
		t.Fatalf("step = %d, err = %v", d.step, d.err)
	}
	// Defaults: mp4, and 720 as the closest value below the configured 1080.
	if got := d.resolutions; !slices.Equal(got, []string{"best", "720", "480", "360"}) {
		t.Errorf("resolutions = %v", got)
	}
	if f, r := ytdlp.Formats[d.format], d.resolutions[d.res]; f != ytdlp.MP4 || r != "720" {
		t.Errorf("defaults %s/%s, want mp4/720", f, r)
	}
	m = press(t, m, "l", "j", "h", "enter")
	if m.dialog != nil {
		t.Fatal("dialog still open")
	}
	it, _ := m.selected()
	if it.URL != "https://x.test/v" || it.Title != "Video" || it.Uploader != "me" || it.Format != ytdlp.MKV || it.Resolution != "best" {
		t.Errorf("queued %+v", it)
	}
	if q, _ := mgr.Snapshot(); len(q) != 2 {
		t.Errorf("queue has %d items, want 2", len(q))
	}
}

func TestDialogGlobalKeysDisabled(t *testing.T) {
	m, _ := newDialogModel(t, &fakeProber{})
	// q and n reach the input instead of quitting or reopening the dialog.
	m = typeText(t, m, "qn")
	if m.dialog == nil || m.dialog.input.Value() != "qn" {
		t.Fatal("keys not typed into the input")
	}
	m = press(t, m, "esc")
	if m.dialog != nil {
		t.Error("esc didn't close the dialog")
	}
}

func TestDialogProbeErrorReturnsToInput(t *testing.T) {
	m, _ := newDialogModel(t, &fakeProber{})
	m = typeText(t, m, "https://bad.test")
	m, res := submit(t, m)
	m = update(t, m, res)
	if m.dialog.step != stepInput || m.dialog.err == nil {
		t.Errorf("step = %d, err = %v; want input step with error", m.dialog.step, m.dialog.err)
	}
}

func TestDialogEscCancelsProbe(t *testing.T) {
	p := &fakeProber{block: map[string]bool{"https://slow.test": true}, infos: map[string]*ytdlp.Info{
		"https://fast.test": {Title: "Fast"},
	}}
	m, _ := newDialogModel(t, p)
	m = typeText(t, m, "https://slow.test")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = press(t, next.(Model), "esc")
	if m.dialog.step != stepInput {
		t.Fatalf("esc: step = %d", m.dialog.step)
	}
	stale := findMsg[probeResultMsg](t, cmd) // returns once cancelled
	if !errors.Is(stale.err, context.Canceled) {
		t.Errorf("probe err = %v, want cancelled", stale.err)
	}

	m.dialog.input.SetValue("https://fast.test")
	m, res := submit(t, m)
	m = update(t, m, stale)
	if m.dialog.step != stepProbing {
		t.Fatalf("stale result handled: step = %d", m.dialog.step)
	}
	m = update(t, m, res)
	if m.dialog.step != stepDetails || m.dialog.info.Title != "Fast" {
		t.Errorf("step = %d, info = %+v", m.dialog.step, m.dialog.info)
	}
}

func TestDialogPasteSeveralURLs(t *testing.T) {
	m, mgr := newDialogModel(t, &fakeProber{})
	m = update(t, m, tea.PasteMsg{Content: "https://a.test/1\nnot a url\n https://a.test/2\thttp://a.test/3\n"})
	if m.dialog.step != stepBatch || len(m.dialog.batch) != 3 {
		t.Fatalf("step = %d, batch = %v", m.dialog.step, m.dialog.batch)
	}
	m = press(t, m, "enter")
	q, _ := mgr.Snapshot()
	if m.dialog != nil || len(q) != 3 {
		t.Fatalf("dialog = %v, queue = %d items", m.dialog, len(q))
	}
	if got := selectedURL(m); got != "https://a.test/1" {
		t.Errorf("selected %q, want the first pasted URL", got)
	}
}

func TestDialogPasteSingleURLGoesToInput(t *testing.T) {
	m, _ := newDialogModel(t, &fakeProber{})
	m = update(t, m, tea.PasteMsg{Content: "https://a.test/1"})
	if m.dialog.step != stepInput || m.dialog.input.Value() != "https://a.test/1" {
		t.Errorf("step = %d, value = %q", m.dialog.step, m.dialog.input.Value())
	}
}

func TestDialogClipboardPrefill(t *testing.T) {
	m, _ := newDialogModel(t, &fakeProber{})
	m = update(t, m, clipboardMsg("some text"))
	if v := m.dialog.input.Value(); v != "" {
		t.Errorf("prefilled %q from non-URL text", v)
	}
	m = update(t, m, clipboardMsg(" https://a.test/1\n"))
	if v := m.dialog.input.Value(); v != "https://a.test/1" {
		t.Errorf("value = %q", v)
	}
}

func TestDialogPlaylistQueuesEntries(t *testing.T) {
	p := &fakeProber{infos: map[string]*ytdlp.Info{
		"https://x.test/pl": {Title: "List", IsPlaylist: true, Entries: []ytdlp.Entry{
			{Title: "One", URL: "https://x.test/1", Duration: 60},
			{Title: "Two", URL: "https://x.test/2"},
		}},
		"https://x.test/empty": {Title: "Empty", IsPlaylist: true},
	}}
	m, mgr := newDialogModel(t, p)
	m.dialog.input.SetValue("https://x.test/empty")
	m, res := submit(t, m)
	if m = update(t, m, res); !errors.Is(m.dialog.err, errEmptyPlaylist) {
		t.Fatalf("empty playlist: err = %v", m.dialog.err)
	}
	m.dialog.input.SetValue("https://x.test/pl")
	m, res = submit(t, m)
	m = update(t, m, res)
	// Heights are unknown for playlists, so every resolution is offered.
	if !slices.Equal(m.dialog.resolutions, ytdlp.Resolutions) {
		t.Errorf("resolutions = %v", m.dialog.resolutions)
	}
	m = press(t, m, "h", "enter") // mp4 → opus
	q, _ := mgr.Snapshot()
	if len(q) != 2 {
		t.Fatalf("queue = %d items, want 2", len(q))
	}
	for _, it := range q {
		if it.Format != ytdlp.Opus || it.Title == "" {
			t.Errorf("queued %+v", it)
		}
	}
}

func TestDialogAudioDisablesResolution(t *testing.T) {
	p := &fakeProber{infos: map[string]*ytdlp.Info{"https://x.test/v": {Title: "V"}}}
	m, _ := newDialogModel(t, p)
	m.dialog.input.SetValue("https://x.test/v")
	m, res := submit(t, m)
	m = update(t, m, res)
	m = press(t, m, "h") // mp4 → opus
	m = press(t, m, "tab")
	if m.dialog.focus != focusClipStart {
		t.Errorf("tab from an audio format: focus = %d, want the clip start (resolution skipped)", m.dialog.focus)
	}
}

func TestDefaultResolution(t *testing.T) {
	res := []string{"best", "720", "480", "360"}
	for want, idx := range map[string]int{"best": 0, "720": 1, "1080": 1, "2160": 1, "480": 2, "240": 0} {
		if got := defaultResolution(res, want); got != idx {
			t.Errorf("defaultResolution(%q) = %d, want %d", want, got, idx)
		}
	}
	if got := availableResolutions([]int{1920, 1080}); !slices.Equal(got, []string{"best", "1440", "1080", "720", "480", "360"}) {
		t.Errorf("availableResolutions = %v", got)
	}
}

// probeDetails opens the dialog, probes url and returns the model at the
// details step.
func probeDetails(t *testing.T, p *fakeProber, url string) (Model, *queue.Manager) {
	t.Helper()
	m, mgr := newDialogModel(t, p)
	m.dialog.input.SetValue(url)
	m, res := submit(t, m)
	m = update(t, m, res)
	if m.dialog.step != stepDetails {
		t.Fatalf("step = %d, err = %v", m.dialog.step, m.dialog.err)
	}
	return m, mgr
}

// focusOrder presses tab n times and returns the focused field after each.
func focusOrder(t *testing.T, m Model, n int) []int {
	t.Helper()
	var got []int
	for range n {
		m = press(t, m, "tab")
		got = append(got, m.dialog.focus)
	}
	return got
}

func shiftTab(t *testing.T, m Model) Model {
	t.Helper()
	return update(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
}

func TestDialogFieldNavigation(t *testing.T) {
	p := &fakeProber{infos: map[string]*ytdlp.Info{"https://x.test/v": {Title: "V"}}}
	m, _ := probeDetails(t, p, "https://x.test/v")
	want := []int{focusResolution, focusClipStart, focusClipEnd, focusTemplate, focusSubtitles, focusFormat}
	if got := focusOrder(t, m, len(want)); !slices.Equal(got, want) {
		t.Errorf("tab order = %v, want %v", got, want)
	}
	if m = shiftTab(t, m); m.dialog.focus != focusSubtitles {
		t.Errorf("shift+tab from the type: focus = %d, want subtitles", m.dialog.focus)
	}
	if m = press(t, m, "k"); m.dialog.focus != focusTemplate {
		t.Errorf("k from subtitles: focus = %d, want template", m.dialog.focus)
	}
	// In a text field, letters are typed instead of moving the focus.
	m.dialog.template.SetValue("")
	m = typeText(t, m, "jkhl")
	if m.dialog.focus != focusTemplate || m.dialog.template.Value() != "jkhl" {
		t.Errorf("focus = %d, template = %q; want letters typed into the template", m.dialog.focus, m.dialog.template.Value())
	}
}

func TestDialogOverridesQueued(t *testing.T) {
	p := &fakeProber{infos: map[string]*ytdlp.Info{"https://x.test/v": {Title: "V", URL: "https://x.test/v"}}}
	m, _ := probeDetails(t, p, "https://x.test/v")
	cfg := m.mgr.Config()
	if d := m.dialog; d.template.Value() != cfg.FilenameTemplate || d.subtitles != cfg.EmbedSubtitles {
		t.Fatalf("defaults: template %q, subtitles %v; want the settings", d.template.Value(), d.subtitles)
	}
	m = press(t, m, "tab", "tab")
	m = typeText(t, m, "1:30")
	m = press(t, m, "tab")
	m = typeText(t, m, "2:00")
	m = press(t, m, "tab")
	m.dialog.template.SetValue("%(id)s.%(ext)s")
	m = press(t, m, "tab", "space", "enter")
	if m.dialog != nil {
		t.Fatalf("dialog still open: err = %v", m.dialog.err)
	}
	it, _ := m.selected()
	if it.ClipStart != 90*time.Second || it.ClipEnd != 120*time.Second {
		t.Errorf("clip = %v-%v, want 1m30s-2m0s", it.ClipStart, it.ClipEnd)
	}
	if it.Template != "%(id)s.%(ext)s" {
		t.Errorf("template = %q", it.Template)
	}
	if it.Subtitles == nil || *it.Subtitles != !cfg.EmbedSubtitles {
		t.Errorf("subtitles = %v, want %v", it.Subtitles, !cfg.EmbedSubtitles)
	}
}

func TestDialogUnchangedFieldsFollowSettings(t *testing.T) {
	p := &fakeProber{infos: map[string]*ytdlp.Info{"https://x.test/v": {Title: "V", URL: "https://x.test/v"}}}
	for name, edit := range map[string]func(*newDialog){
		"untouched":      func(*newDialog) {},
		"empty template": func(d *newDialog) { d.template.SetValue("  ") },
		"toggled back":   func(d *newDialog) {},
	} {
		t.Run(name, func(t *testing.T) {
			m, _ := probeDetails(t, p, "https://x.test/v")
			edit(m.dialog)
			if name == "toggled back" {
				m = shiftTab(t, m)
				m = press(t, m, "space", "space")
			}
			m = press(t, m, "enter")
			it, _ := m.selected()
			if it.Template != "" || it.Subtitles != nil || !it.Clip().IsZero() {
				t.Errorf("overrides stored: template %q, subtitles %v, clip %+v", it.Template, it.Subtitles, it.Clip())
			}
		})
	}
}

func TestDialogClipValidation(t *testing.T) {
	p := &fakeProber{infos: map[string]*ytdlp.Info{"https://x.test/v": {Title: "V", URL: "https://x.test/v"}}}
	for _, tc := range []struct {
		start, end string
		ok         bool
	}{
		{"", "", true},
		{"1:30", "", true},
		{"", "45", true},
		{"0:10", "1:00:00", true},
		{"abc", "", false},
		{"", "1:60", false},
		{"-5", "", false},
		{"1:30", "1:30", false},
		{"2:00", "1:00", false},
	} {
		t.Run(tc.start+"_"+tc.end, func(t *testing.T) {
			m, mgr := probeDetails(t, p, "https://x.test/v")
			m.dialog.clipStart.SetValue(tc.start)
			m.dialog.clipEnd.SetValue(tc.end)
			m = press(t, m, "enter")
			q, _ := mgr.Snapshot()
			if tc.ok {
				if m.dialog != nil || len(q) != 1 {
					t.Errorf("not queued: queue = %d items", len(q))
				}
				return
			}
			if m.dialog == nil || m.dialog.step != stepDetails || m.dialog.err == nil || len(q) != 0 {
				t.Fatalf("invalid clip queued or no error: queue = %d items", len(q))
			}
			m = press(t, m, "tab", "tab")
			m = typeText(t, m, "1")
			if m.dialog.err != nil {
				t.Errorf("error kept after editing a clip field: %v", m.dialog.err)
			}
		})
	}
}

func TestDialogPlaylistOverrides(t *testing.T) {
	p := &fakeProber{infos: map[string]*ytdlp.Info{
		"https://x.test/pl": {Title: "List", IsPlaylist: true, Entries: []ytdlp.Entry{
			{Title: "One", URL: "https://x.test/1"},
			{Title: "Two", URL: "https://x.test/2"},
		}},
	}}
	m, mgr := probeDetails(t, p, "https://x.test/pl")
	want := []int{focusResolution, focusTemplate, focusSubtitles, focusFormat}
	if got := focusOrder(t, m, len(want)); !slices.Equal(got, want) {
		t.Errorf("tab order = %v, want %v (no clip fields)", got, want)
	}
	m.dialog.clipStart.SetValue("1:00") // hidden, so ignored
	m = press(t, m, "tab", "tab")
	m.dialog.template.SetValue("%(playlist_index)s.%(ext)s")
	m = press(t, m, "tab", "l", "enter")
	q, _ := mgr.Snapshot()
	if m.dialog != nil || len(q) != 2 {
		t.Fatalf("dialog = %v, queue = %d items", m.dialog, len(q))
	}
	for _, it := range q {
		if it.Template != "%(playlist_index)s.%(ext)s" || it.Subtitles == nil || !*it.Subtitles || !it.Clip().IsZero() {
			t.Errorf("%s: template %q, subtitles %v, clip %+v", it.URL, it.Template, it.Subtitles, it.Clip())
		}
	}
}
