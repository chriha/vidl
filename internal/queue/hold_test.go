package queue

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHeldQueueStartsNothingAutomatically(t *testing.T) {
	m, r := newManager(t, 2, &MemStore{})
	ids := add(m, "run")
	r.waitStarted(t, "run")
	m.SetHeld(true)
	if !m.Held() {
		t.Fatal("Held() = false after SetHeld(true)")
	}

	add(m, "q1") // Add
	r.assertNoStart(t)

	m.Pause(add(m, "p")[0]) // pausing a queued item
	cfg := m.Config()
	cfg.Concurrency = 5
	m.SetConfig(cfg) // more concurrency
	r.assertNoStart(t)

	paused := add(m, "x")[0]
	if err := m.Pause(paused); err != nil {
		t.Fatal(err)
	}
	if err := m.Requeue(paused); err != nil { // Requeue
		t.Fatal(err)
	}
	r.assertNoStart(t)

	r.finish("run", nil) // a download ending
	waitStatus(t, m, ids[0], Finished)
	r.assertNoStart(t)
}

func TestHeldQueueStaysHeldWhenRunningDownloadIsPausedOrCancelled(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	ids := add(m, "a", "b", "c")
	r.waitStarted(t, "a")
	m.SetHeld(true)

	m.Pause(ids[0])
	waitStatus(t, m, ids[0], Paused)
	r.assertNoStart(t)

	m.Requeue(ids[0])
	r.assertNoStart(t)
	if err := m.Start(ids[0]); err != nil {
		t.Fatal(err)
	}
	r.waitStarted(t, "a")
	m.Cancel(ids[0])
	waitStatus(t, m, ids[0], Cancelled)
	r.assertNoStart(t)
}

func TestStartStillStartsWhileHeld(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	m.SetHeld(true)
	ids := add(m, "a")
	r.assertNoStart(t)
	if err := m.Start(ids[0]); err != nil {
		t.Fatal(err)
	}
	r.waitStarted(t, "a")
}

func TestReleasingHoldStartsInQueueOrderAndNotifies(t *testing.T) {
	m, r := newManager(t, 2, &MemStore{})
	m.SetHeld(true)
	add(m, "a", "b", "c")
	r.assertNoStart(t)
	select { // drain pending notifications
	case <-m.Changes():
	default:
	}

	m.SetHeld(false)
	if m.Held() {
		t.Fatal("Held() = true after SetHeld(false)")
	}
	r.waitStarted(t, "a", "b")
	r.assertNoStart(t) // c waits for a slot
	select {
	case <-m.Changes():
	case <-time.After(2 * time.Second):
		t.Fatal("no change notification")
	}
}

func TestHoldIsNotPersisted(t *testing.T) {
	store := &MemStore{}
	m, _ := newManager(t, 1, store)
	add(m, "a")
	m.SetHeld(true)
	if !m.Held() {
		t.Fatal("Held() = false after SetHeld(true)")
	}
	m.Close()

	if len(store.State.Queue) == 0 {
		t.Fatal("nothing was saved, so the check below proves nothing")
	}
	raw, err := json.Marshal(store.State)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(raw)), "held") {
		t.Fatalf("saved state mentions the hold: %s", raw)
	}

	m2, _ := newManager(t, 1, store)
	if m2.Held() {
		t.Fatal("a new Manager is held")
	}
}
