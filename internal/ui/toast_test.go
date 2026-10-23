package ui

import (
	"errors"
	"testing"
)

func TestToastExpires(t *testing.T) {
	m, _, _ := newTestModel(t, 1)
	m.report(errors.New("first"))
	first := m.toast.seq
	m.notice("second")
	if m = update(t, m, toastExpiredMsg{first}); m.toast.text != "second" {
		t.Fatalf("stale expiry hid the newer toast: %q", m.toast.text)
	}
	if m = update(t, m, toastExpiredMsg{m.toast.seq}); m.toast.text != "" {
		t.Errorf("toast %q still shown", m.toast.text)
	}
}

func TestToastSchedulesExpiry(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a")
	if _, cmd := m.Update(keyMsg("R")); cmd == nil {
		t.Error("error toast without expiry timer")
	}
}

func TestSuccessDismissesErrorToast(t *testing.T) {
	m, _, _ := newTestModel(t, 1)
	m.report(errors.New("boom"))
	if m.report(nil); m.toast.text != "" {
		t.Errorf("error toast %q kept after success", m.toast.text)
	}
	m.notice("info")
	if m.report(nil); m.toast.text != "info" {
		t.Errorf("notice dismissed by success")
	}
}
