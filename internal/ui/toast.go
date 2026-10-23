package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const toastDuration = 4 * time.Second

var (
	toastStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("10")).Padding(0, 1)
	errorToastStyle = toastStyle.BorderForeground(lipgloss.Color("9")).Foreground(lipgloss.Color("9"))
)

// toast is a transient message drawn in the bottom right corner of the body.
type toast struct {
	text string
	err  error // the reported error; nil for notices
	seq  int   // id of the timer that hides it
}

// toastExpiredMsg hides the toast with the same seq.
type toastExpiredMsg struct{ seq int }

// report shows err as a toast. A nil error (a successful action) dismisses an
// error toast, so a stale error never outlives the action that fixed it.
func (m *Model) report(err error) {
	if err == nil {
		if m.toast.err != nil {
			m.toast = toast{seq: m.toast.seq}
		}
		return
	}
	m.toast = toast{text: err.Error(), err: err, seq: m.toast.seq + 1}
}

// notice shows an informational toast.
func (m *Model) notice(text string) {
	m.toast = toast{text: text, seq: m.toast.seq + 1}
}

// expireToast hides the toast with the given seq after toastDuration, unless
// a newer one replaced it by then.
func expireToast(seq int) tea.Cmd {
	return tea.Tick(toastDuration, func(time.Time) tea.Msg { return toastExpiredMsg{seq} })
}

func (m Model) toastView() string {
	if m.toast.text == "" {
		return ""
	}
	style := toastStyle
	if m.toast.err != nil {
		style = errorToastStyle
	}
	frame := style.GetHorizontalFrameSize()
	w := min(lipgloss.Width(m.toast.text)+frame, 60, max(m.width-2, 10))
	text := lipgloss.NewStyle().Width(w - frame).MaxHeight(3).Render(m.toast.text)
	return style.Width(w).Render(text)
}
