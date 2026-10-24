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
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.waiting {
		return 0, false
	}
	if m.waitTimer == nil {
		return m.waitLeft, true
	}
	return max(m.waitUntil.Sub(m.now()), 0), true
}

// armWaitLocked (re)starts the countdown after a download ended, frozen with
// the full duration while the queue is held.
func (m *Manager) armWaitLocked() {
	if !m.cfg.WaitEnabled || m.closed {
		return
	}
	m.stopWaitTimerLocked()
	m.waiting = true
	m.waitLeft = m.cfg.WaitTime()
	if !m.held {
		m.runWaitLocked()
	}
}

// runWaitLocked lets a frozen countdown tick down from waitLeft.
func (m *Manager) runWaitLocked() {
	if !m.waiting || m.waitTimer != nil {
		return
	}
	gen := m.waitGen
	m.waitUntil = m.now().Add(m.waitLeft)
	m.waitTimer = m.afterFunc(m.waitLeft, func() { m.expireWait(gen) })
}

// freezeWaitLocked stops a running countdown and keeps its remaining time.
func (m *Manager) freezeWaitLocked() {
	if !m.waiting || m.waitTimer == nil {
		return
	}
	left := m.waitUntil.Sub(m.now())
	m.stopWaitTimerLocked()
	if left <= 0 {
		m.waiting = false // due already; its callback lost the race
		return
	}
	m.waitLeft = left
}

// dropWaitLocked ends any countdown without waiting for it.
func (m *Manager) dropWaitLocked() {
	m.stopWaitTimerLocked()
	m.waiting = false
}

// stopWaitTimerLocked stops the timer; the generation bump makes a callback
// that already fired a no-op.
func (m *Manager) stopWaitTimerLocked() {
	if m.waitTimer != nil {
		m.waitTimer.Stop()
		m.waitTimer = nil
	}
	m.waitGen++
}

func (m *Manager) expireWait(gen int) {
	m.mu.Lock()
	if m.closed || gen != m.waitGen || !m.waiting {
		m.mu.Unlock()
		return
	}
	m.waiting = false
	m.waitTimer = nil
	m.scheduleLocked()
	m.saveLocked()
	m.mu.Unlock()
	m.notify()
}
