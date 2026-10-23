package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/vidl/internal/queue"
)

const (
	detailMinHeight = 8
	detailLabelW    = 10
	// detailSplitW is the width from which the log sits beside the fields
	// instead of below them.
	detailSplitW  = 100
	detailMinLogH = 3
)

var (
	detailLabelStyle = dimStyle.Width(detailLabelW)
	detailRuleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	detailLogStyle   = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(lipgloss.Color("240")).PaddingLeft(1)
)

// detailItem is the item shown in the detail panel, if it is open.
func (m Model) detailItem() (queue.Item, bool) {
	if !m.detail || (m.tab != tabDownloads && m.tab != tabFinished) {
		return queue.Item{}, false
	}
	return m.selected()
}

// detailHeight is the height of the detail panel including its rule: a third
// of the body, at least detailMinHeight, but always leaving the table its
// header and two rows. 0 when the panel is closed or doesn't fit.
func (m Model) detailHeight() int {
	if _, ok := m.detailItem(); !ok {
		return 0
	}
	body := m.bodyHeight()
	h := min(max(body/3, detailMinHeight), body-3)
	if h < 4 {
		return 0
	}
	return h
}

// showsLog reports whether the panel includes the yt-dlp output for it.
func showsLog(it queue.Item) bool {
	return it.Status == queue.Downloading || it.Status == queue.Failed
}

// logSize is the size of the log viewport for it within the panel, 0×0 when
// there is no log or no room for one.
func (m Model) logSize(it queue.Item) (w, h int) {
	inner := m.detailHeight() - 1
	if !showsLog(it) || inner <= 0 {
		return 0, 0
	}
	if m.width >= detailSplitW {
		return m.width - m.fieldsWidth() - detailLogStyle.GetHorizontalFrameSize() - 1, inner
	}
	if fields := len(m.detailFields(it, m.width-1)); inner-fields-1 >= detailMinLogH {
		return m.width - 2, inner - fields - 1
	}
	if inner >= 2*detailMinLogH+1 {
		return m.width - 2, detailMinLogH
	}
	return 0, 0
}

// fieldsWidth is the width of the field column in the side-by-side layout.
func (m Model) fieldsWidth() int {
	return m.width * 55 / 100
}

// syncLog points the log viewport at the item in the panel and keeps it
// scrolled to the latest output.
func (m *Model) syncLog() {
	it, ok := m.detailItem()
	if !ok {
		return
	}
	w, h := m.logSize(it)
	if w <= 0 || h <= 0 {
		return
	}
	m.log.SetWidth(w)
	m.log.SetHeight(h)
	if len(it.Log) == 0 {
		msg := "no output (from a previous session)"
		if it.Status == queue.Downloading {
			msg = "no output yet"
		}
		m.log.SetContent(dimStyle.Render(msg))
	} else {
		m.log.SetContentLines(it.Log)
	}
	m.log.GotoBottom()
}

func (m Model) detailView() string {
	it, ok := m.detailItem()
	inner := m.detailHeight() - 1
	if !ok || inner <= 0 {
		return ""
	}
	title := " Details "
	rule := detailRuleStyle.Render("─" + title + strings.Repeat("─", max(m.width-1-len(title), 0)))

	logW, logH := m.logSize(it)
	fieldsW := m.width - 1
	if logW > 0 && m.width >= detailSplitW {
		fieldsW = m.fieldsWidth() - 1
	}
	fields := m.detailFields(it, fieldsW)
	fieldsH := inner
	if logW > 0 && m.width < detailSplitW {
		fieldsH = inner - logH - 1
	}
	if len(fields) > fieldsH {
		fields = append(fields[:fieldsH-1], dimStyle.Render("…"))
	}
	for i, l := range fields {
		fields[i] = " " + l
	}
	left := strings.Join(fields, "\n")

	var content string
	switch {
	case logW == 0:
		content = left
	case m.width >= detailSplitW:
		left = lipgloss.NewStyle().Width(m.fieldsWidth()).Render(left)
		content = lipgloss.JoinHorizontal(lipgloss.Top, left, detailLogStyle.Render(m.log.View()))
	default:
		sep := " " + detailRuleStyle.Render("─ Output "+strings.Repeat("─", max(m.width-11, 0)))
		content = lipgloss.NewStyle().Height(fieldsH).Render(left) + "\n" + sep + "\n" +
			lipgloss.NewStyle().PaddingLeft(1).Render(m.log.View())
	}
	content = lipgloss.NewStyle().Height(inner).MaxHeight(inner).Render(content)
	return rule + "\n" + content
}

// detailFields renders the item's details as lines at most w cells wide.
func (m Model) detailFields(it queue.Item, w int) []string {
	var lines []string
	add := func(label, value string) {
		if value == "" {
			return
		}
		lines = append(lines, ansi.Truncate(detailLabelStyle.Render(label)+value, w, "…"))
	}
	status := lipgloss.NewStyle().Foreground(statusColors[it.Status]).Render(it.Status.String())
	lines = append(lines, ansi.Truncate(status+"  "+lipgloss.NewStyle().Bold(true).Render(it.DisplayTitle()), w, "…"))
	add("URL", it.URL)
	add("Source", joinNonEmpty(it.Uploader, it.Extractor, durationLabel(it.Duration)))
	add("Format", joinNonEmpty(string(it.Format), resolutionLabel(it)))
	add("Clip", clipLabel(it))
	add("Template", it.Template)
	switch {
	case it.Subtitles == nil:
	case *it.Subtitles:
		add("Subtitles", "embedded")
	default:
		add("Subtitles", "off")
	}
	if it.Status == queue.Finished {
		if it.Size > 0 {
			add("Size", humanBytes(it.Size))
		}
	} else {
		add("Progress", progressLabel(it))
	}
	add("Times", joinNonEmpty(timeLabel("added", it.AddedAt), timeLabel("started", it.StartedAt), timeLabel("finished", it.FinishedAt)))
	add("Output", it.FilePath)
	switch n := len(it.PartFiles); {
	case n == 1:
		add("Part file", it.PartFiles[0])
	case n > 1:
		add("Part files", fmt.Sprintf("%d: %s", n, strings.Join(it.PartFiles, ", ")))
	}
	if it.Error != "" {
		label := detailLabelStyle.Render("Error")
		wrapped := errorStyle.Width(max(w-detailLabelW, 1)).Render(it.Error)
		for i, l := range strings.Split(wrapped, "\n") {
			if i > 0 {
				label = strings.Repeat(" ", detailLabelW)
			}
			lines = append(lines, label+l)
		}
	}
	return lines
}

func progressLabel(it queue.Item) string {
	p := it.Progress
	parts := []string{fmt.Sprintf("%.1f%%", p.Percent)}
	switch {
	case p.Total > 0:
		parts = append(parts, humanBytes(p.Downloaded)+" / "+humanBytes(p.Total))
	case p.Downloaded > 0:
		parts = append(parts, humanBytes(p.Downloaded))
	}
	if p.Streams > 1 {
		parts = append(parts, fmt.Sprintf("stream %d/%d", p.Stream, p.Streams))
	}
	if it.Status == queue.Downloading {
		if p.Stage != "" {
			parts = append(parts, p.Stage)
		} else {
			parts = append(parts, humanSpeed(p.Speed), "ETA "+humanDuration(p.ETA))
		}
	}
	return strings.Join(parts, " · ")
}

// clipLabel is the item's clip range ("1:30 – end"), or "" for the whole video.
func clipLabel(it queue.Item) string {
	clip := it.Clip()
	if clip.IsZero() {
		return ""
	}
	end := "end"
	if clip.End > 0 {
		end = clipTimeLabel(clip.End)
	}
	return clipTimeLabel(clip.Start) + " – " + end
}

// clipTimeLabel formats a clip time like "1:30" or "0:02.5", keeping the
// fraction of a second that clip times may have.
func clipTimeLabel(d time.Duration) string {
	whole := d.Truncate(time.Second)
	label := "0:00"
	if whole > 0 {
		label = humanDuration(whole)
	}
	if frac := d - whole; frac > 0 {
		label += strings.TrimPrefix(strconv.FormatFloat(frac.Seconds(), 'f', -1, 64), "0")
	}
	return label
}

// resolutionLabel is the item's resolution ("1080p", "best"), or "" for audio.
func resolutionLabel(it queue.Item) string {
	if it.Format.IsAudio() || it.Resolution == "" {
		return ""
	}
	return resolutionName(it.Resolution)
}

// resolutionName labels a resolution value: "1080p", or "best".
func resolutionName(r string) string {
	if r == "best" {
		return r
	}
	return r + "p"
}

func durationLabel(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	return humanDuration(time.Duration(seconds * float64(time.Second)))
}

func timeLabel(what string, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return what + " " + t.Local().Format("Jan 2 15:04:05")
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}
