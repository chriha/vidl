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

func TestAddURLsMsgPausedAddsPaused(t *testing.T) {
	m, mgr, _ := newTestModel(t, 2)
	added := make(chan int, 1)
	m = update(t, m, AddURLsMsg{URLs: []string{"https://a", "https://b"}, Paused: true, Added: added})

	q, _ := mgr.Snapshot()
	if len(q) != 2 {
		t.Fatalf("queue = %+v, want 2 items", q)
	}
	for _, it := range q {
		if it.Status != queue.Paused {
			t.Errorf("%s status = %v, want Paused although slots are free", it.URL, it.Status)
		}
	}
	if n := <-added; n != 2 {
		t.Errorf("added = %d, want 2", n)
	}
	if want := "Added 2 downloads paused from vidl add"; m.toast.text != want {
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

func TestAddURLsMsgSkipsDuplicates(t *testing.T) {
	tests := []struct {
		name      string
		urls      []string
		paused    bool
		wantQueue []string
		wantAdded int
		wantToast string
	}{
		{
			name:      "some duplicates",
			urls:      []string{"https://a", "https://b"},
			wantQueue: []string{"https://a", "https://b"},
			wantAdded: 1,
			wantToast: "Added 1 download from vidl add · skipped 1 already downloaded or queued",
		},
		{
			name:      "paused",
			urls:      []string{"https://b", "https://a", "https://c"},
			paused:    true,
			wantQueue: []string{"https://a", "https://b", "https://c"},
			wantAdded: 2,
			wantToast: "Added 2 downloads paused from vidl add · skipped 1 already downloaded or queued",
		},
		{
			name:      "all duplicates",
			urls:      []string{"https://a", "https://a"},
			wantQueue: []string{"https://a"},
			wantAdded: 0,
			wantToast: "Nothing added from vidl add · skipped 2 already downloaded or queued",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, mgr, _ := newTestModel(t, 0, "https://a")
			added := make(chan int, 1)
			m = update(t, m, AddURLsMsg{URLs: tt.urls, Paused: tt.paused, Added: added})

			if got := queuedURLs(mgr); !slices.Equal(got, tt.wantQueue) {
				t.Errorf("queue = %q, want %q", got, tt.wantQueue)
			}
			if n := <-added; n != tt.wantAdded {
				t.Errorf("added = %d, want %d", n, tt.wantAdded)
			}
			if m.toast.text != tt.wantToast || m.toast.err != nil {
				t.Errorf("toast = %+v, want notice %q", m.toast, tt.wantToast)
			}
		})
	}
}

func TestNoticeMsgShowsNotice(t *testing.T) {
	m, _, _ := newTestModel(t, 0)
	m = update(t, m, NoticeMsg{Text: "Skipped 1 URL already downloaded or queued"})
	if want := "Skipped 1 URL already downloaded or queued"; m.toast.text != want || m.toast.err != nil {
		t.Errorf("toast = %+v, want notice %q", m.toast, want)
	}
}

func TestAddURLsMsgAbandonedAddsNothing(t *testing.T) {
	m, mgr, _ := newTestModel(t, 0, "existing")
	claim := &Claim{}
	if !claim.Abandon() {
		t.Fatal("abandon failed on a fresh claim")
	}
	added := make(chan int, 1)
	m = update(t, m, AddURLsMsg{URLs: []string{"https://a"}, Added: added, Claim: claim})

	if got := queuedURLs(mgr); !slices.Equal(got, []string{"existing"}) {
		t.Errorf("queue = %q, want only the existing download", got)
	}
	if len(added) != 0 {
		t.Error("count sent for an abandoned request")
	}
	if m.toast.text != "" {
		t.Errorf("toast = %q, want none", m.toast.text)
	}
}

func TestAddURLsMsgTakenClaimAdds(t *testing.T) {
	m, mgr, _ := newTestModel(t, 0)
	m = update(t, m, AddURLsMsg{URLs: []string{"https://a"}, Claim: &Claim{}})
	if got := queuedURLs(mgr); !slices.Equal(got, []string{"https://a"}) {
		t.Errorf("queue = %q", got)
	}
}

func TestClaim(t *testing.T) {
	var nilClaim *Claim
	if !nilClaim.Take() {
		t.Error("nil claim: Take = false, want true")
	}
	c := &Claim{}
	if !c.Take() || c.Abandon() {
		t.Error("after Take, Abandon must fail")
	}
	c = &Claim{}
	if !c.Abandon() || c.Take() {
		t.Error("after Abandon, Take must fail")
	}
}
