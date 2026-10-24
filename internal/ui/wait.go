package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/chriha/vidl/internal/queue"
)

// waitTickMsg redraws the wait countdown in the tab bar.
type waitTickMsg struct{}

// toggleWait switches the wait timer on or off and saves the config.
func (m *Model) toggleWait() {
	cfg := m.mgr.Config()
	cfg.WaitEnabled = !cfg.WaitEnabled
	if err := m.saveConfig(cfg); err != nil {
		m.report(err)
		return
	}
	if cfg.WaitEnabled {
		m.notice("Wait timer on — " + m.mgr.Config().WaitDuration + " between downloads")
	} else {
		m.notice("Wait timer off")
	}
}

// waitBadge shows the wait timer in the tab bar: the configured duration
// while idle, the time left during a countdown, nothing while it's off.
func (m Model) waitBadge() string {
	cfg := m.mgr.Config()
	if !cfg.WaitEnabled {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	d := cfg.WaitTime()
	if left, ok := m.mgr.WaitRemaining(); ok {
		d = left
		style = style.Bold(true).Foreground(statusColors[queue.Paused])
	}
	return style.Render("⏱ " + formatWait(d))
}

// waitCounting reports whether a countdown is running, not frozen by a hold.
func (m Model) waitCounting() bool {
	_, ok := m.mgr.WaitRemaining()
	return ok && !m.mgr.Held()
}

// waitTick schedules the next redraw of the countdown.
func waitTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return waitTickMsg{} })
}

// formatWait is e.g. "23s" below a minute and "1:45" from a minute up,
// rounded up to whole seconds.
func formatWait(d time.Duration) string {
	s := int((max(d, 0) + time.Second - 1) / time.Second)
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
