package ui

import (
	"errors"
	"slices"
	"testing"

	"github.com/chriha/vidl/internal/queue"
)

func queuedURLs(mgr *queue.Manager) []string {
	q, _ := mgr.Snapshot()
	var urls []string
	for _, it := range q {
		urls = append(urls, it.URL)
	}
	return urls
}

func TestAddURLsMsgQueuesAndShowsToast(t *testing.T) {
	tests := []struct {
		name   string
		dialog bool // the new-download dialog is open
	}{
		{"main view", false},
		{"dialog open", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, mgr, _ := newTestModel(t, 0, "existing")
			if tt.dialog {
				m = press(t, m, "n")
				if m.dialog == nil {
					t.Fatal("dialog did not open")
				}
			}
			added := make(chan int, 1)
			m = update(t, m, AddURLsMsg{URLs: []string{"https://a", "https://b"}, Added: added})

			if got := queuedURLs(mgr); !slices.Equal(got, []string{"existing", "https://a", "https://b"}) {
				t.Errorf("queue = %q", got)
			}
			if !slices.ContainsFunc(m.queue, func(it queue.Item) bool { return it.URL == "https://b" }) {
				t.Errorf("model queue not refreshed: %+v", m.queue)
			}
			select {
			case n := <-added:
				if n != 2 {
					t.Errorf("added = %d, want 2", n)
				}
			default:
				t.Error("no count sent on Added")
			}
			if want := "Added 2 downloads from vidl add"; m.toast.text != want || m.toast.err != nil {
				t.Errorf("toast = %+v, want notice %q", m.toast, want)
			}
			if tt.dialog && m.dialog == nil {
				t.Error("dialog closed by an external add")
			}
		})
	}
}

func TestAddURLsMsgWithoutReplyChannel(t *testing.T) {
	m, mgr, _ := newTestModel(t, 0)
	m = update(t, m, AddURLsMsg{URLs: []string{"https://a"}})
	if got := queuedURLs(mgr); !slices.Equal(got, []string{"https://a"}) {
		t.Errorf("queue = %q", got)
	}
	if want := "Added 1 download from vidl add"; m.toast.text != want {
		t.Errorf("toast = %q, want %q", m.toast.text, want)
	}
}

func TestErrorMsgShowsErrorToast(t *testing.T) {
	m, _, _ := newTestModel(t, 0)
	err := errors.New("vidl add: accept: too many open files")
	m = update(t, m, ErrorMsg{Err: err})
	if m.toast.err != err || m.toast.text != err.Error() {
		t.Errorf("toast = %+v, want error %v", m.toast, err)
	}
}
