package queue

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/chriha/vidl/internal/config"
)

// fakeClock drives the wait timer: timers fire only when the test advances it.
type fakeClock struct {
	mu     sync.Mutex
	t      time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	c    *fakeClock
	at   time.Time
	f    func()
	done bool // fired or stopped
}

func (ft *fakeTimer) Stop() bool {
	ft.c.mu.Lock()
	defer ft.c.mu.Unlock()
	was := !ft.done
	ft.done = true
	return was
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) waitTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	ft := &fakeTimer{c: c, at: c.t.Add(d), f: f}
	c.timers = append(c.timers, ft)
	return ft
}

// Advance moves the clock forward and runs the timers that came due.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	var due []func()
	for _, ft := range c.timers {
		if !ft.done && !ft.at.After(c.t) {
			ft.done = true
			due = append(due, ft.f)
		}
	}
	c.mu.Unlock()
	for _, f := range due {
		f()
	}
}

// pending counts the timers neither fired nor stopped.
func (c *fakeClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, ft := range c.timers {
		if !ft.done {
			n++
		}
	}
	return n
}

// newWaitManager returns a manager with the wait timer enabled (30s) on a
// fake clock.
func newWaitManager(t *testing.T, concurrency int) (*Manager, *fakeRunner, *fakeClock) {
	t.Helper()
	cfg := config.Default()
	cfg.Concurrency = concurrency
	cfg.DownloadDir = t.TempDir()
	cfg.WaitEnabled = true
	cfg.WaitDuration = "30s"
	r := newFakeRunner()
	m, err := New(cfg, r, &MemStore{})
	if err != nil {
		t.Fatal(err)
	}
	c := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	m.mu.Lock()
	m.now, m.afterFunc = c.Now, c.AfterFunc
	m.mu.Unlock()
	t.Cleanup(func() { m.Close() })
	return m, r, c
}

func waitRemaining(t *testing.T, m *Manager, want time.Duration) {
	t.Helper()
	waitFor(t, "a countdown of "+want.String(), func() bool {
		d, ok := m.WaitRemaining()
		return ok && d == want
	})
}

func assertNoCountdown(t *testing.T, m *Manager) {
	t.Helper()
	if d, ok := m.WaitRemaining(); ok {
		t.Fatalf("WaitRemaining() = %v, true, want no countdown", d)
	}
}

func TestWaitArmsOnEveryEnd(t *testing.T) {
	tests := []struct {
		name string
		end  func(m *Manager, r *fakeRunner, id string)
	}{
		{"finished", func(m *Manager, r *fakeRunner, id string) { r.finish("a", nil) }},
		{"failed", func(m *Manager, r *fakeRunner, id string) { r.finish("a", errors.New("boom")) }},
		{"paused", func(m *Manager, r *fakeRunner, id string) { m.Pause(id) }},
		{"cancelled", func(m *Manager, r *fakeRunner, id string) { m.Cancel(id) }},
		{"removed while running", func(m *Manager, r *fakeRunner, id string) { m.Remove(id) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, r, c := newWaitManager(t, 1)
			ids := add(m, "a", "b")
			r.waitStarted(t, "a")
			assertNoCountdown(t, m)

			tt.end(m, r, ids[0])
			waitRemaining(t, m, 30*time.Second)
			r.assertNoStart(t)

			c.Advance(29 * time.Second)
			r.assertNoStart(t)
			c.Advance(time.Second)
			r.waitStarted(t, "b")
			assertNoCountdown(t, m)
		})
	}
}

func TestWaitDisabledStartsNextAtOnce(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	add(m, "a", "b")
	r.waitStarted(t, "a")
	r.finish("a", nil)
	r.waitStarted(t, "b")
	assertNoCountdown(t, m)
}

func TestWaitFurtherEndRestartsSharedCountdown(t *testing.T) {
	m, r, c := newWaitManager(t, 2)
	add(m, "a", "b", "c", "d")
	r.waitStarted(t, "a", "b")

	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)
	c.Advance(20 * time.Second)
	waitRemaining(t, m, 10*time.Second)

	r.finish("b", nil)
	waitRemaining(t, m, 30*time.Second)
	if n := c.pending(); n != 1 {
		t.Fatalf("%d pending timers after the restart, want 1", n)
	}
	c.Advance(20 * time.Second)
	r.assertNoStart(t)

	c.Advance(10 * time.Second)
	r.waitStarted(t, "c", "d") // both free slots fill
	assertNoCountdown(t, m)
}

func TestStartBypassesWait(t *testing.T) {
	m, r, _ := newWaitManager(t, 1)
	ids := add(m, "a", "b")
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)

	if err := m.Start(ids[1]); err != nil {
		t.Fatal(err)
	}
	r.waitStarted(t, "b")
}

func TestAddStartsAtOnceUnlessCountingDown(t *testing.T) {
	m, r, c := newWaitManager(t, 1)
	add(m, "a")
	r.waitStarted(t, "a") // idle queue: no wait
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)

	add(m, "b")
	r.assertNoStart(t)
	c.Advance(30 * time.Second)
	r.waitStarted(t, "b")
}

func TestRequeueWaitsDuringCountdown(t *testing.T) {
	m, r, c := newWaitManager(t, 1)
	add(m, "a")
	paused := m.Add(NewItem{URL: "p", Paused: true})[0]
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)

	if err := m.Requeue(paused); err != nil {
		t.Fatal(err)
	}
	r.assertNoStart(t)
	c.Advance(30 * time.Second)
	r.waitStarted(t, "p")
}

func TestHoldFreezesCountdown(t *testing.T) {
	m, r, c := newWaitManager(t, 1)
	add(m, "a", "b")
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)
	c.Advance(10 * time.Second)

	m.SetHeld(true)
	waitRemaining(t, m, 20*time.Second)
	c.Advance(time.Hour)
	waitRemaining(t, m, 20*time.Second)
	r.assertNoStart(t)

	m.SetHeld(false)
	waitRemaining(t, m, 20*time.Second)
	r.assertNoStart(t)
	c.Advance(19 * time.Second)
	r.assertNoStart(t)
	c.Advance(time.Second)
	r.waitStarted(t, "b")
}

func TestStopFreezesCountdown(t *testing.T) {
	m, r, c := newWaitManager(t, 2)
	ids := add(m, "a", "b", "c")
	r.waitStarted(t, "a", "b")
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)
	c.Advance(10 * time.Second)

	m.Stop() // its pause of b doesn't restart the countdown
	waitStatus(t, m, ids[1], Paused)
	waitRemaining(t, m, 20*time.Second)
	c.Advance(time.Hour)
	r.assertNoStart(t)

	m.Resume()
	waitRemaining(t, m, 20*time.Second)
	r.assertNoStart(t)
	c.Advance(20 * time.Second)
	r.waitStarted(t, "b", "c")
}

func TestDisablingDropsCountdown(t *testing.T) {
	m, r, _ := newWaitManager(t, 1)
	add(m, "a", "b")
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)

	cfg := m.Config()
	cfg.WaitEnabled = false
	m.SetConfig(cfg)
	assertNoCountdown(t, m)
	r.waitStarted(t, "b")
}

func TestDisablingWhileHeldDropsCountdownButStartsNothing(t *testing.T) {
	m, r, _ := newWaitManager(t, 1)
	add(m, "a", "b")
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)
	m.SetHeld(true)

	cfg := m.Config()
	cfg.WaitEnabled = false
	m.SetConfig(cfg)
	assertNoCountdown(t, m)
	r.assertNoStart(t)

	m.SetHeld(false)
	r.waitStarted(t, "b")
}

func TestDurationChangeAppliesToNextCountdown(t *testing.T) {
	m, r, c := newWaitManager(t, 1)
	add(m, "a", "b")
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)

	cfg := m.Config()
	cfg.WaitDuration = "1m"
	m.SetConfig(cfg)
	waitRemaining(t, m, 30*time.Second)
	c.Advance(30 * time.Second)
	r.waitStarted(t, "b")

	r.finish("b", nil)
	waitRemaining(t, m, time.Minute)
}

func TestCountdownExpiryNotifies(t *testing.T) {
	m, r, c := newWaitManager(t, 1)
	add(m, "a")
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)
	drainChanges(m)

	c.Advance(30 * time.Second)
	waitChange(t, m)
	assertNoCountdown(t, m)
}

func TestCountdownFreezeAndResumeNotify(t *testing.T) {
	m, r, _ := newWaitManager(t, 1)
	add(m, "a")
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)

	drainChanges(m)
	m.SetHeld(true)
	waitChange(t, m)
	drainChanges(m)
	m.SetHeld(false)
	waitChange(t, m)
}

func TestCloseStopsWaitTimer(t *testing.T) {
	m, r, c := newWaitManager(t, 1)
	add(m, "a", "b")
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitRemaining(t, m, 30*time.Second)

	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if n := c.pending(); n != 0 {
		t.Fatalf("%d pending timers after Close, want 0", n)
	}
	c.Advance(time.Hour)
	r.assertNoStart(t)
}
