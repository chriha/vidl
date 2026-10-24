package queue

import "time"

// waitTimer is a pending call of the wait timer; *time.Timer implements it.
type waitTimer interface {
	Stop() bool
}

// WaitRemaining returns the time left until queued downloads start again
// after a download ended, and whether such a countdown is running. A held or
// stopped queue freezes the countdown.
func (m *Manager) WaitRemaining() (time.Duration, bool) {
	return 0, false
}
