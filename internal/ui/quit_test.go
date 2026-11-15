package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chriha/vidl/internal/queue"
)

var (
	keyQ     = tea.KeyPressMsg{Code: 'q', Text: "q"}
	keyCtrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

// send applies msg and reports whether the returned command quits.
func send(t *testing.T, m Model, msg tea.Msg) (Model, bool) {
	t.Helper()
	next, cmd := m.Update(msg)
	quits := false
	if cmd != nil {
		_, quits = cmd().(tea.QuitMsg)
	}
	return next.(Model), quits
}

// runningModel returns a model with one download in progress.
func runningModel(t *testing.T) (Model, *queue.Manager) {
	t.Helper()
	m, mgr, _ := newTestModel(t, 1, "a")
	waitFor(t, "a running", func() bool {
		q, _ := mgr.Snapshot()
		return len(q) == 1 && q[0].Status == queue.Downloading
	})
	return update(t, m, changedMsg{}), mgr
}

func TestQuitAsksWhileDownloading(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{keyQ, keyCtrlC} {
		m, _ := runningModel(t)
		m, quits := send(t, m, k)
		if quits {
			t.Fatalf("%v quit with a running download", k)
		}
		c := m.confirm
		if c == nil {
			t.Fatalf("%v opened no confirmation", k)
		}
		if want := "1 download"; !strings.Contains(c.subject, want) {
			t.Errorf("subject %q does not name %q", c.subject, want)
		}
		if !strings.Contains(c.note, "resume") {
			t.Errorf("note %q does not say downloads resume", c.note)
		}
	}
}

func TestQuitConfirmAnswers(t *testing.T) {
	m, _ := runningModel(t)
	m, _ = send(t, m, keyQ)

	m, quits := send(t, m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if quits || m.confirm != nil {
		t.Fatalf("n: quits = %v, confirm open = %v; want stay and closed", quits, m.confirm != nil)
	}
	m, _ = send(t, m, keyQ)
	m, quits = send(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if quits || m.confirm != nil {
		t.Fatalf("esc: quits = %v, confirm open = %v; want stay and closed", quits, m.confirm != nil)
	}
	m, _ = send(t, m, keyQ)
	if _, quits = send(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"}); !quits {
		t.Error("y did not quit")
	}
}

func TestSecondCtrlCQuitsImmediately(t *testing.T) {
	m, _ := runningModel(t)
	m, _ = send(t, m, keyCtrlC)
	if m.confirm == nil {
		t.Fatal("first ctrl+c opened no confirmation")
	}
	if _, quits := send(t, m, keyCtrlC); !quits {
		t.Error("ctrl+c with the confirmation open did not quit")
	}
}

func TestQuitImmediateWithoutRunningDownload(t *testing.T) {
	setups := map[string]func(t *testing.T) Model{
		"empty": func(t *testing.T) Model { m, _, _ := newTestModel(t, 1); return m },
		"queued": func(t *testing.T) Model {
			m, mgr, _ := newTestModel(t, 1, "a", "b")
			waitFor(t, "a running", func() bool { it, _ := mgr.Get(m.queue[0].ID); return it.Status == queue.Downloading })
			mgr.Stop() // a pauses, b stays queued behind the hold
			waitFor(t, "nothing running", func() bool {
				q, _ := mgr.Snapshot()
				for _, it := range q {
					if it.Status == queue.Downloading {
						return false
					}
				}
				return true
			})
			return update(t, m, changedMsg{})
		},
		"stopped": func(t *testing.T) Model {
			m, mgr, _ := newTestModel(t, 1, "a")
			waitFor(t, "a running", func() bool { it, _ := mgr.Get(m.queue[0].ID); return it.Status == queue.Downloading })
			mgr.Stop()
			waitFor(t, "a paused", func() bool { it, _ := mgr.Get(m.queue[0].ID); return it.Status == queue.Paused })
			return update(t, m, changedMsg{})
		},
	}
	for name, setup := range setups {
		for _, k := range []tea.KeyPressMsg{keyQ, keyCtrlC} {
			if _, quits := send(t, setup(t), k); !quits {
				t.Errorf("%s: %v did not quit", name, k)
			}
		}
	}
}

func TestCtrlCStaysImmediateInOverlays(t *testing.T) {
	overlays := map[string]func(t *testing.T) Model{
		"dialog": func(t *testing.T) Model {
			m, _ := runningModel(t)
			return press(t, m, "n")
		},
		"search": func(t *testing.T) Model {
			m, _ := runningModel(t)
			return press(t, m, "/")
		},
		"error view": func(t *testing.T) Model {
			m, _ := failedModel(t)
			m = press(t, m, "i")
			if m.errView == nil {
				t.Fatal("error view not open")
			}
			return m
		},
		"settings edit": func(t *testing.T) Model {
			m := settingsModel(t, "Rate limit")
			return press(t, m, "enter")
		},
	}
	for name, open := range overlays {
		t.Run(name, func(t *testing.T) {
			m := open(t)
			if _, quits := send(t, m, keyCtrlC); !quits {
				t.Error("ctrl+c did not quit")
			}
		})
	}
}
