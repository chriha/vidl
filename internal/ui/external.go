package ui

import (
	"fmt"
	"sync/atomic"

	"github.com/chriha/vidl/internal/queue"
)

// AddURLsMsg asks the model to add URLs handed over by `vidl add`, with the
// default format and resolution, queued or (with Paused) paused. When Added is
// set, it receives the number of downloads added, so it needs room for one value.
type AddURLsMsg struct {
	URLs   []string
	Paused bool
	Added  chan<- int
	// Claim, when set, lets the sender withdraw the request: the model adds
	// the URLs only if it wins the claim.
	Claim *Claim
}

// Claim decides who owns an AddURLsMsg: the model that adds it or the sender
// that gave up waiting. Exactly one of Take and Abandon succeeds.
type Claim struct{ state atomic.Int32 }

const (
	claimTaken     = 1
	claimAbandoned = 2
)

// Take reports whether the model may add the URLs. A nil Claim always allows it.
func (c *Claim) Take() bool {
	return c == nil || c.state.CompareAndSwap(0, claimTaken) || c.state.Load() == claimTaken
}

// Abandon reports whether the sender withdrew the request before the model
// took it; the model then adds nothing.
func (c *Claim) Abandon() bool { return c.state.CompareAndSwap(0, claimAbandoned) }

// ErrorMsg shows an error from outside the UI loop as a toast.
type ErrorMsg struct{ Err error }

// NoticeMsg shows an informational toast from outside the UI loop.
type NoticeMsg struct{ Text string }

// AddUnique adds the requests that aren't already downloaded or queued and
// returns the ids of the added downloads and the number skipped.
func AddUnique(mgr *queue.Manager, reqs []queue.NewItem) (ids []string, skipped int) {
	var fresh []queue.NewItem
	for _, r := range reqs {
		if _, dup := mgr.FindDuplicate(r); !dup {
			fresh = append(fresh, r)
		}
	}
	if len(fresh) == 0 {
		return nil, len(reqs)
	}
	return mgr.Add(fresh...), len(reqs) - len(fresh)
}

// addExternal queues the URLs of msg without touching the open view or dialog.
func (m *Model) addExternal(msg AddURLsMsg) {
	if !msg.Claim.Take() {
		return // the sender gave up and reported a failure
	}
	reqs := make([]queue.NewItem, len(msg.URLs))
	for i, u := range msg.URLs {
		reqs[i] = queue.NewItem{URL: u, Paused: msg.Paused}
	}
	ids, skipped := AddUnique(m.mgr, reqs)
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
	text := "Added " + plural(len(ids), "download") + how + " from vidl add"
	if len(ids) == 0 {
		text = "Nothing added from vidl add"
	}
	if skipped > 0 {
		text += fmt.Sprintf(" · skipped %d already downloaded or queued", skipped)
	}
	m.notice(text)
	m.refresh()
}
