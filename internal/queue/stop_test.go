package queue

import (
	"testing"
	"time"
)

func drainChanges(m *Manager) {
	select {
	case <-m.Changes():
	default:
	}
}

func waitChange(t *testing.T, m *Manager) {
	t.Helper()
	select {
	case <-m.Changes():
	case <-time.After(2 * time.Second):
		t.Fatal("no change notification")
	}
}

func TestStopPausesRunningDownloadsAndHoldsTheQueue(t *testing.T) {
	m, r := newManager(t, 2, &MemStore{})
	ids := add(m, "a", "b", "c")
	r.waitStarted(t, "a", "b")
	drainChanges(m)

	if n := m.Stop(); n != 2 {
		t.Errorf("Stop() = %d, want 2", n)
	}
	waitChange(t, m)
	if !m.Stopped() || !m.Held() {
		t.Fatalf("Stopped() = %v, Held() = %v, want both true", m.Stopped(), m.Held())
	}
	waitStatus(t, m, ids[0], Paused)
	waitStatus(t, m, ids[1], Paused)
	r.assertNoStart(t) // c stays queued
	if it, _ := m.Get(ids[2]); it.Status != Queued {
		t.Errorf("c status = %v, want queued", it.Status)
	}
}

func TestStoppedQueueStartsNothingAutomatically(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	ids := add(m, "run")
	r.waitStarted(t, "run")
	m.Stop()
	waitStatus(t, m, ids[0], Paused)

	add(m, "q1") // Add
	paused := add(m, "x")[0]
	m.Pause(paused)
	m.Requeue(paused) // Requeue
	cfg := m.Config()
	cfg.Concurrency = 5
	m.SetConfig(cfg) // more concurrency
	r.assertNoStart(t)

	if err := m.Start(ids[0]); err != nil { // Start still starts
		t.Fatal(err)
	}
	r.waitStarted(t, "run")
	r.finish("run", nil) // a download ending
	waitStatus(t, m, ids[0], Finished)
	r.assertNoStart(t)
}

func TestResumeRequeuesOnlyStopPausedDownloadsInQueueOrder(t *testing.T) {
	m, r := newManager(t, 2, &MemStore{})
	ids := add(m, "pre", "a", "b")
	r.waitStarted(t, "pre", "a")
	m.Pause(ids[0]) // paused by hand before the stop
	waitStatus(t, m, ids[0], Paused)
	r.waitStarted(t, "b")

	m.Stop()
	waitStatus(t, m, ids[1], Paused)
	waitStatus(t, m, ids[2], Paused)
	cfg := m.Config()
	cfg.Concurrency = 1
	m.SetConfig(cfg)
	drainChanges(m)

	if n := m.Resume(); n != 2 {
		t.Errorf("Resume() = %d, want 2", n)
	}
	waitChange(t, m)
	if m.Stopped() || m.Held() {
		t.Fatalf("Stopped() = %v, Held() = %v after Resume", m.Stopped(), m.Held())
	}
	r.waitStarted(t, "a") // queue order, up to the concurrency limit
	r.assertNoStart(t)
	if it, _ := m.Get(ids[2]); it.Status != Queued {
		t.Errorf("b status = %v, want queued", it.Status)
	}
	if it, _ := m.Get(ids[0]); it.Status != Paused {
		t.Errorf("pre status = %v, want paused", it.Status)
	}
}

func TestResumeLeavesAddedAsPausedDownloadsAlone(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	m.Stop()
	id := add(m, "a")[0]
	m.Pause(id)
	if n := m.Resume(); n != 0 {
		t.Errorf("Resume() = %d, want 0", n)
	}
	r.assertNoStart(t)
	if it, _ := m.Get(id); it.Status != Paused {
		t.Errorf("status = %v, want paused", it.Status)
	}
}

func TestResumeSkipsDownloadsThatLeftTheStopSet(t *testing.T) {
	m, r := newManager(t, 4, &MemStore{})
	ids := add(m, "started", "cancelled", "removed", "kept")
	r.waitStarted(t, "started", "cancelled", "removed", "kept")
	if n := m.Stop(); n != 4 {
		t.Fatalf("Stop() = %d, want 4", n)
	}
	for _, id := range ids {
		waitStatus(t, m, id, Paused)
	}

	m.Start(ids[0])
	r.waitStarted(t, "started")
	m.Pause(ids[0]) // paused by hand after leaving the set
	waitStatus(t, m, ids[0], Paused)
	m.Cancel(ids[1])
	m.Remove(ids[2])

	if n := m.Resume(); n != 1 {
		t.Errorf("Resume() = %d, want 1", n)
	}
	r.waitStarted(t, "kept")
	r.assertNoStart(t)
	if it, _ := m.Get(ids[0]); it.Status != Paused {
		t.Errorf("started-then-paused status = %v, want paused", it.Status)
	}
	if it, _ := m.Get(ids[1]); it.Status != Cancelled {
		t.Errorf("cancelled status = %v, want cancelled", it.Status)
	}
}

func TestStopWhileHeldStopsTheQueue(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	ids := add(m, "a")
	r.waitStarted(t, "a")
	m.SetHeld(true)
	if m.Stopped() {
		t.Fatal("holding stopped the queue")
	}
	if n := m.Stop(); n != 1 {
		t.Errorf("Stop() = %d, want 1", n)
	}
	waitStatus(t, m, ids[0], Paused)
	if !m.Stopped() || !m.Held() {
		t.Fatalf("Stopped() = %v, Held() = %v, want both true", m.Stopped(), m.Held())
	}
}

func TestReleasingHoldWhileStoppedResumes(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	ids := add(m, "a")
	r.waitStarted(t, "a")
	m.Stop()
	waitStatus(t, m, ids[0], Paused)

	m.SetHeld(false)
	if m.Stopped() || m.Held() {
		t.Fatalf("Stopped() = %v, Held() = %v after SetHeld(false)", m.Stopped(), m.Held())
	}
	r.waitStarted(t, "a")
}

func TestStopIsNotPersisted(t *testing.T) {
	store := &MemStore{}
	m, r := newManager(t, 1, store)
	ids := add(m, "a")
	r.waitStarted(t, "a")
	m.Stop()
	waitStatus(t, m, ids[0], Paused)
	m.Close()

	m2, r2 := newManager(t, 1, store)
	if m2.Stopped() || m2.Held() {
		t.Fatal("a new Manager is stopped or held")
	}
	if it, _ := m2.Get(ids[0]); it.Status != Paused {
		t.Errorf("status after restart = %v, want paused", it.Status)
	}
	if n := m2.Resume(); n != 0 {
		t.Errorf("Resume() after restart = %d, want 0", n)
	}
	r2.assertNoStart(t)
}
