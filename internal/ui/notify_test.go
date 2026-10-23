package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/chriha/vidl/internal/notify"
	"github.com/chriha/vidl/internal/queue"
)

type notification struct{ title, body string }

// fakeNotifier records notifications and returns err for each of them.
type fakeNotifier struct {
	mu   sync.Mutex
	sent []notification
	err  error
}

func (f *fakeNotifier) Notify(title, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, notification{title, body})
	return f.err
}

func (f *fakeNotifier) calls() []notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]notification(nil), f.sent...)
}

// notifyModel is a test model that notifies through a fake.
func notifyModel(t *testing.T, concurrency int, urls ...string) (Model, *queue.Manager, *fakeRunner, *fakeNotifier) {
	t.Helper()
	m, mgr, r := newTestModel(t, concurrency, urls...)
	n := &fakeNotifier{}
	return m.WithNotifier(n), mgr, r, n
}

// flush ends the coalescing window and delivers the notifier's result.
func flush(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(notifyFlushMsg{})
	m = next.(Model)
	if cmd != nil {
		m = update(t, m, cmd())
	}
	return m
}

// waitStatus waits until the item with url has status s, then refreshes m.
func waitStatus(t *testing.T, m Model, mgr *queue.Manager, url string, s queue.Status) Model {
	t.Helper()
	waitFor(t, url+" "+s.String(), func() bool {
		q, h := mgr.Snapshot()
		for _, it := range append(q, h...) {
			if it.URL == url {
				return it.Status == s
			}
		}
		return false
	})
	return update(t, m, changedMsg{})
}

func TestNotifyOnLiveTransition(t *testing.T) {
	tests := []struct {
		name string
		urls []string
		run  func(t *testing.T, m Model, mgr *queue.Manager, r *fakeRunner) Model
		want []notification
	}{
		{
			name: "finished",
			urls: []string{"a"},
			run: func(t *testing.T, m Model, mgr *queue.Manager, r *fakeRunner) Model {
				return finishAll(t, m, mgr, r, "a")
			},
			want: []notification{{"Downloaded: a", "a.mp4"}},
		},
		{
			name: "failed",
			urls: []string{"broken-a"},
			run: func(t *testing.T, m Model, mgr *queue.Manager, r *fakeRunner) Model {
				r.finish("broken-a")
				return waitStatus(t, m, mgr, "broken-a", queue.Failed)
			},
			want: []notification{{"Failed: broken-a", "boom"}},
		},
		{
			name: "two within the window",
			urls: []string{"a", "b"},
			run: func(t *testing.T, m Model, mgr *queue.Manager, r *fakeRunner) Model {
				m = finishAll(t, m, mgr, r, "a")
				return finishAll(t, m, mgr, r, "b")
			},
			want: []notification{{"2 downloads finished", "a, b"}},
		},
		{
			name: "finished and failed",
			urls: []string{"a", "broken-b"},
			run: func(t *testing.T, m Model, mgr *queue.Manager, r *fakeRunner) Model {
				r.finish("broken-b")
				m = waitStatus(t, m, mgr, "broken-b", queue.Failed)
				return finishAll(t, m, mgr, r, "a")
			},
			want: []notification{{"1 download finished, 1 failed", "broken-b, a"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, mgr, r, n := notifyModel(t, 2, tt.urls...)
			for _, u := range tt.urls {
				m = waitStatus(t, m, mgr, u, queue.Downloading)
			}
			m = tt.run(t, m, mgr, r)
			m = flush(t, m)
			if got := n.calls(); fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("notifications = %q, want %q", got, tt.want)
			}
			// The window is empty after a flush.
			flush(t, m)
			if got := len(n.calls()); got != len(tt.want) {
				t.Errorf("%d notifications after a second flush, want %d", got, len(tt.want))
			}
		})
	}
}

func TestNoNotification(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, m Model, mgr *queue.Manager, r *fakeRunner, n *fakeNotifier) Model
	}{
		{
			name: "finished before startup",
			run: func(t *testing.T, m Model, mgr *queue.Manager, r *fakeRunner, n *fakeNotifier) Model {
				r.finish("a")
				waitFor(t, "a finished", func() bool { _, h := mgr.Snapshot(); return len(h) == 1 })
				m = New(mgr, &fakeProber{}, filepath.Join(t.TempDir(), "config.toml")).WithNotifier(n)
				return update(t, m, changedMsg{})
			},
		},
		{
			name: "cancelled",
			run: func(t *testing.T, m Model, mgr *queue.Manager, _ *fakeRunner, _ *fakeNotifier) Model {
				m = press(t, m, "c")
				return waitStatus(t, m, mgr, "a", queue.Cancelled)
			},
		},
		{
			name: "paused",
			run: func(t *testing.T, m Model, mgr *queue.Manager, _ *fakeRunner, _ *fakeNotifier) Model {
				m = press(t, m, "p")
				return waitStatus(t, m, mgr, "a", queue.Paused)
			},
		},
		{
			name: "setting off",
			run: func(t *testing.T, m Model, mgr *queue.Manager, r *fakeRunner, _ *fakeNotifier) Model {
				cfg := mgr.Config()
				cfg.Notify = false
				mgr.SetConfig(cfg)
				return finishAll(t, m, mgr, r, "a")
			},
		},
		{
			name: "setting turned off within the window",
			run: func(t *testing.T, m Model, mgr *queue.Manager, r *fakeRunner, _ *fakeNotifier) Model {
				m = finishAll(t, m, mgr, r, "a")
				cfg := mgr.Config()
				cfg.Notify = false
				mgr.SetConfig(cfg)
				return m
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, mgr, r, n := notifyModel(t, 1, "a")
			m = waitStatus(t, m, mgr, "a", queue.Downloading)
			m = tt.run(t, m, mgr, r, n)
			flush(t, m)
			if got := n.calls(); len(got) != 0 {
				t.Errorf("notifications = %q, want none", got)
			}
		})
	}
}

func TestNotifierUnavailableToastsOnce(t *testing.T) {
	m, mgr, r, n := notifyModel(t, 1, "a", "b")
	n.err = fmt.Errorf("%w: osascript", notify.ErrUnavailable)
	m = waitStatus(t, m, mgr, "a", queue.Downloading)
	m = finishAll(t, m, mgr, r, "a")
	m = flush(t, m)
	if m.toast.err == nil || !errors.Is(m.toast.err, notify.ErrUnavailable) {
		t.Fatalf("toast = %+v, want the missing notifier", m.toast)
	}
	m.toast = toast{seq: m.toast.seq}
	m = waitStatus(t, m, mgr, "b", queue.Downloading)
	m = finishAll(t, m, mgr, r, "b")
	m = flush(t, m)
	if m.toast.text != "" {
		t.Errorf("second toast %q", m.toast.text)
	}
	if got := len(n.calls()); got != 1 {
		t.Errorf("notifier called %d times, want 1", got)
	}
}

func TestNotifierErrorToasts(t *testing.T) {
	m, mgr, r, n := notifyModel(t, 1, "a", "b")
	n.err = errors.New("osascript: exit status 1")
	for _, u := range []string{"a", "b"} {
		m = waitStatus(t, m, mgr, u, queue.Downloading)
		m = finishAll(t, m, mgr, r, u)
		m = flush(t, m)
		if m.toast.err == nil || m.toast.err.Error() != n.err.Error() {
			t.Fatalf("%s: toast = %+v, want the notifier error", u, m.toast)
		}
	}
	if got := len(n.calls()); got != 2 {
		t.Errorf("notifier called %d times, want 2", got)
	}
}
