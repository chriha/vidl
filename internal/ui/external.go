package ui

import "github.com/chriha/vidl/internal/queue"

// AddURLsMsg asks the model to add URLs handed over by `vidl add`, with the
// default format and resolution, queued or (with Paused) paused. When Added is
// set, it receives the number of downloads added, so it needs room for one value.
type AddURLsMsg struct {
	URLs   []string
	Paused bool
	Added  chan<- int
}

// ErrorMsg shows an error from outside the UI loop as a toast.
type ErrorMsg struct{ Err error }

// NoticeMsg shows an informational toast from outside the UI loop.
type NoticeMsg struct{ Text string }

// AddUnique adds the requests that aren't already downloaded or queued and
// returns the ids of the added downloads and the number skipped.
func AddUnique(mgr *queue.Manager, reqs []queue.NewItem) (ids []string, skipped int) {
	return mgr.Add(reqs...), 0
}

// addExternal queues the URLs of msg without touching the open view or dialog.
func (m *Model) addExternal(msg AddURLsMsg) {
	reqs := make([]queue.NewItem, len(msg.URLs))
	for i, u := range msg.URLs {
		reqs[i] = queue.NewItem{URL: u, Paused: msg.Paused}
	}
	ids, _ := AddUnique(m.mgr, reqs)
	if msg.Added != nil {
		select {
		case msg.Added <- len(ids):
		default: // never block the UI loop on a full channel
		}
	}
	how := ""
	if msg.Paused {
		how = " paused"
	}
	m.notice("Added " + plural(len(ids), "download") + how + " from vidl add")
	m.refresh()
}
