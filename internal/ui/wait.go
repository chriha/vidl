package ui

import "time"

// waitTickMsg redraws the wait countdown in the tab bar.
type waitTickMsg struct{}

// waitBadge shows the wait timer in the tab bar: the configured duration
// while idle, the time left during a countdown, nothing while it's off.
func (m Model) waitBadge() string {
	return ""
}

// formatWait is e.g. "23s" below a minute and "1:45" from a minute up,
// rounded up to whole seconds.
func formatWait(d time.Duration) string {
	return ""
}
