package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"vidl/internal/config"
	"vidl/internal/queue"
	"vidl/internal/ytdlp"
)

// Prober fetches metadata for a URL without downloading it. ytdlp.Runner
// implements it.
type Prober interface {
	Probe(ctx context.Context, url, cookiesFromBrowser string) (*ytdlp.Info, error)
}

type dialogStep int

const (
	stepInput dialogStep = iota
	stepProbing
	stepDetails
	stepBatch // several pasted URLs, queued with defaults
)

const (
	focusFormat = iota
	focusResolution
)

// playlistPreview is the number of playlist entries listed in the dialog.
const playlistPreview = 5

var (
	newDialogStyle    = dialogStyle.BorderForeground(lipgloss.Color("12"))
	optionStyle       = lipgloss.NewStyle().Padding(0, 1)
	chosenStyle       = optionStyle.Bold(true).Underline(true)
	chosenFocusStyle  = activeTabStyle
	errEmptyPlaylist  = errors.New("the playlist has no entries")
	errNothingToQueue = errors.New("enter a URL")
)

// probeResultMsg is the outcome of the probe with the given request id.
type probeResultMsg struct {
	id   int
	info *ytdlp.Info
	err  error
}

// clipboardMsg is the clipboard content read when the dialog opened.
type clipboardMsg string

// newDialog is the state of the new-download dialog.
type newDialog struct {
	step    dialogStep
	input   textinput.Model
	spinner spinner.Model
	probeID int
	cancel  context.CancelFunc
	err     error

	info        *ytdlp.Info
	focus       int
	format      int // index into ytdlp.Formats
	resolutions []string
	res         int // index into resolutions

	batch []string
}

// openDialog opens the new-download dialog and reads the clipboard to
// prefill the URL.
func (m *Model) openDialog() tea.Cmd {
	in := textinput.New()
	in.Prompt = "URL "
	in.Placeholder = "https://…"
	in.SetWidth(m.dialogWidth() - 8)
	d := &newDialog{input: in, spinner: spinner.New(spinner.WithSpinner(spinner.Dot))}
	m.dialog = d
	read := m.clipboard
	return tea.Batch(d.input.Focus(), func() tea.Msg {
		s, err := read()
		if err != nil {
			return nil // no clipboard tool or no text: just don't prefill
		}
		return clipboardMsg(s)
	})
}

func (m *Model) closeDialog() {
	if m.dialog.cancel != nil {
		m.dialog.cancel()
	}
	m.dialog = nil
}

// extractURLs returns the http(s) URLs in s, split on whitespace.
func extractURLs(s string) []string {
	var urls []string
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
			urls = append(urls, f)
		}
	}
	return urls
}

func (m Model) handleDialogMsg(msg tea.Msg) (Model, tea.Cmd) {
	d := m.dialog
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleDialogKey(msg)
	case tea.PasteMsg:
		if d.step != stepInput {
			return m, nil
		}
		if urls := extractURLs(msg.Content); len(urls) > 1 {
			d.step, d.batch, d.err = stepBatch, urls, nil
			return m, nil
		}
		d.input, cmd = d.input.Update(msg)
	case clipboardMsg:
		if urls := extractURLs(string(msg)); d.step == stepInput && d.input.Value() == "" &&
			len(urls) == 1 && urls[0] == strings.TrimSpace(string(msg)) {
			d.input.SetValue(urls[0])
			d.input.CursorEnd()
		}
	case probeResultMsg:
		if d.step != stepProbing || msg.id != d.probeID {
			return m, nil // stale: cancelled or superseded
		}
		d.cancel()
		d.cancel = nil
		if msg.err == nil && msg.info.IsPlaylist && len(msg.info.Entries) == 0 {
			msg.err = errEmptyPlaylist
		}
		if msg.err != nil {
			d.step, d.err = stepInput, msg.err
			return m, d.input.Focus()
		}
		d.showDetails(msg.info, m.mgr.Config())
	case spinner.TickMsg:
		if d.step == stepProbing {
			d.spinner, cmd = d.spinner.Update(msg)
		}
	default:
		d.input, cmd = d.input.Update(msg) // cursor blink
	}
	return m, cmd
}

func (m Model) handleDialogKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	d := m.dialog
	if msg.String() == "ctrl+c" {
		m.closeDialog()
		return m, tea.Quit
	}
	switch d.step {
	case stepInput:
		switch msg.String() {
		case "esc":
			m.closeDialog()
		case "enter":
			return m.submitURL()
		default:
			var cmd tea.Cmd
			d.input, cmd = d.input.Update(msg)
			d.err = nil
			return m, cmd
		}
	case stepProbing:
		if msg.String() == "esc" {
			d.cancel()
			d.cancel, d.step = nil, stepInput
			return m, d.input.Focus()
		}
	case stepDetails:
		switch msg.String() {
		case "esc":
			d.step = stepInput
			return m, d.input.Focus()
		case "enter":
			m.queueDetails()
		case "tab", "shift+tab", "j", "k", "up", "down":
			d.toggleFocus()
		case "h", "left":
			d.change(-1)
		case "l", "right":
			d.change(1)
		}
	case stepBatch:
		switch msg.String() {
		case "esc":
			d.step, d.batch = stepInput, nil
		case "enter":
			reqs := make([]queue.NewItem, len(d.batch))
			for i, u := range d.batch {
				reqs[i] = queue.NewItem{URL: u}
			}
			m.queueAndClose(reqs)
		}
	}
	return m, nil
}

// submitURL probes the entered URL, or asks to queue several at once.
func (m Model) submitURL() (Model, tea.Cmd) {
	d := m.dialog
	value := strings.TrimSpace(d.input.Value())
	if urls := extractURLs(value); len(urls) > 1 {
		d.step, d.batch, d.err = stepBatch, urls, nil
		return m, nil
	}
	if value == "" {
		d.err = errNothingToQueue
		return m, nil
	}
	m.probeSeq++
	ctx, cancel := context.WithCancel(context.Background())
	d.step, d.err, d.cancel, d.probeID = stepProbing, nil, cancel, m.probeSeq
	d.input.Blur()
	id, prober, cookies := m.probeSeq, m.prober, m.mgr.Config().CookiesFromBrowser
	probe := func() tea.Msg {
		info, err := prober.Probe(ctx, value, cookies)
		return probeResultMsg{id: id, info: info, err: err}
	}
	return m, tea.Batch(d.spinner.Tick, probe)
}

// showDetails switches to the details step with the config's defaults.
func (d *newDialog) showDetails(info *ytdlp.Info, cfg config.Config) {
	d.step, d.info, d.focus = stepDetails, info, focusFormat
	d.format = max(slices.Index(ytdlp.Formats, cfg.DefaultFormat), 0)
	d.resolutions = availableResolutions(info.Heights)
	d.res = defaultResolution(d.resolutions, cfg.DefaultResolution)
}

// availableResolutions is "best" plus the standard resolutions up to the
// highest available height, or all of them when heights are unknown.
func availableResolutions(heights []int) []string {
	if len(heights) == 0 {
		return slices.Clone(ytdlp.Resolutions)
	}
	res := []string{"best"}
	for _, r := range ytdlp.Resolutions[1:] {
		if h, _ := strconv.Atoi(r); h <= heights[0] {
			res = append(res, r)
		}
	}
	return res
}

// defaultResolution is the index of want in res, or of the closest lower
// resolution, or of "best" when there is none.
func defaultResolution(res []string, want string) int {
	if i := slices.Index(res, want); i >= 0 {
		return i
	}
	w, _ := strconv.Atoi(want)
	for i, r := range res {
		if h, err := strconv.Atoi(r); err == nil && h <= w {
			return i
		}
	}
	return 0
}

func (d *newDialog) audio() bool {
	return ytdlp.Formats[d.format].IsAudio()
}

func (d *newDialog) toggleFocus() {
	if d.focus == focusFormat && !d.audio() {
		d.focus = focusResolution
	} else {
		d.focus = focusFormat
	}
}

func (d *newDialog) change(delta int) {
	if d.focus == focusFormat {
		d.format = (d.format + delta + len(ytdlp.Formats)) % len(ytdlp.Formats)
		return
	}
	d.res = (d.res + delta + len(d.resolutions)) % len(d.resolutions)
}

// queueDetails queues the probed video, or every playlist entry, with the
// chosen format and resolution.
func (m *Model) queueDetails() {
	d := m.dialog
	info, format, res := d.info, ytdlp.Formats[d.format], d.resolutions[d.res]
	if !info.IsPlaylist {
		m.queueAndClose([]queue.NewItem{{
			URL:        cmpStr(info.URL, strings.TrimSpace(d.input.Value())),
			Title:      info.Title,
			Uploader:   info.Uploader,
			Duration:   info.Duration,
			Extractor:  info.Extractor,
			Format:     format,
			Resolution: res,
		}})
		return
	}
	reqs := make([]queue.NewItem, len(info.Entries))
	for i, e := range info.Entries {
		reqs[i] = queue.NewItem{URL: e.URL, Title: e.Title, Duration: e.Duration, Format: format, Resolution: res}
	}
	m.queueAndClose(reqs)
}

// queueAndClose adds reqs, closes the dialog and puts the Downloads cursor on
// the first new item.
func (m *Model) queueAndClose(reqs []queue.NewItem) {
	ids := m.mgr.Add(reqs...)
	m.closeDialog()
	m.tab = tabDownloads
	if len(ids) > 0 {
		m.sel[tabDownloads] = ids[0]
	}
	m.refresh()
}

func cmpStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (m Model) dialogWidth() int {
	return min(72, max(m.width-4, 24))
}

func (m Model) dialogView() string {
	d := m.dialog
	w := m.dialogWidth()
	inner := w - newDialogStyle.GetHorizontalFrameSize()
	bold := lipgloss.NewStyle().Bold(true)
	var lines []string
	switch d.step {
	case stepInput:
		lines = append(lines, bold.Render("New download"), "", d.input.View())
		if d.err != nil {
			lines = append(lines, "", errorStyle.Width(inner).MaxHeight(4).Render(d.err.Error()))
		}
		lines = append(lines, "", dimStyle.Render("enter fetch info · esc close"))
	case stepProbing:
		lines = append(lines, bold.Render("New download"), "",
			d.spinner.View()+" Fetching info…",
			dimStyle.Render(ansi.Truncate(strings.TrimSpace(d.input.Value()), inner, "…")),
			"", dimStyle.Render("esc cancel"))
	case stepDetails:
		lines = append(lines, d.detailsLines(inner)...)
	case stepBatch:
		lines = append(lines, bold.Render(fmt.Sprintf("Add %d URLs with defaults?", len(d.batch))), "")
		for i, u := range d.batch {
			if i == playlistPreview {
				lines = append(lines, dimStyle.Render(fmt.Sprintf("… and %d more", len(d.batch)-i)))
				break
			}
			lines = append(lines, dimStyle.Render(ansi.Truncate(u, inner, "…")))
		}
		lines = append(lines, "", dimStyle.Render("enter add · esc back"))
	}
	return newDialogStyle.Width(w).Render(strings.Join(lines, "\n"))
}

func (d *newDialog) detailsLines(w int) []string {
	info := d.info
	bold := lipgloss.NewStyle().Bold(true)
	var lines []string
	if info.IsPlaylist {
		lines = append(lines,
			ansi.Truncate(bold.Render("Playlist: ")+cmpStr(info.Title, info.URL), w, "…"),
			dimStyle.Render(joinNonEmpty(fmt.Sprintf("%d entries", len(info.Entries)), info.Uploader, info.Extractor)),
			"")
		for i, e := range info.Entries {
			if i == playlistPreview {
				lines = append(lines, dimStyle.Render(fmt.Sprintf("   … and %d more", len(info.Entries)-i)))
				break
			}
			line := fmt.Sprintf("%2d. %s", i+1, cmpStr(e.Title, e.URL))
			if dur := durationLabel(e.Duration); dur != "" {
				line += dimStyle.Render(" · " + dur)
			}
			lines = append(lines, ansi.Truncate(line, w, "…"))
		}
	} else {
		title := bold.Width(w).MaxHeight(2).Render(cmpStr(info.Title, info.URL))
		size := ""
		if info.FilesizeApprox > 0 {
			size = "~" + humanBytes(info.FilesizeApprox)
		}
		lines = append(lines, title,
			dimStyle.Render(ansi.Truncate(joinNonEmpty(info.Uploader, info.Extractor, durationLabel(info.Duration), size), w, "…")))
	}

	formats := make([]string, len(ytdlp.Formats))
	for i, f := range ytdlp.Formats {
		formats[i] = string(f)
	}
	resLabels := make([]string, len(d.resolutions))
	for i, r := range d.resolutions {
		resLabels[i] = resolutionName(r)
	}
	lines = append(lines, "",
		ansi.Truncate(selector("Type", formats, d.format, d.focus == focusFormat, false), w, "…"),
		ansi.Truncate(selector("Resolution", resLabels, d.res, d.focus == focusResolution, d.audio()), w, "…"),
		"")
	action := "enter queue"
	if info.IsPlaylist {
		action = fmt.Sprintf("enter queue %d downloads", len(info.Entries))
	}
	lines = append(lines, dimStyle.Render(ansi.Truncate(action+" · tab/j/k field · h/l change · esc back", w, "…")))
	return lines
}

// selector renders a labelled row of options with the chosen one marked.
func selector(label string, options []string, chosen int, focused, disabled bool) string {
	marker := "  "
	if focused {
		marker = "› "
	}
	parts := []string{marker + lipgloss.NewStyle().Width(11).Render(label)}
	for i, o := range options {
		style := optionStyle
		switch {
		case disabled:
			style = optionStyle.Foreground(lipgloss.Color("240"))
		case i == chosen && focused:
			style = chosenFocusStyle
		case i == chosen:
			style = chosenStyle
		}
		parts = append(parts, style.Render(o))
	}
	return strings.Join(parts, "")
}
