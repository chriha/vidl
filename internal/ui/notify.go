package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chriha/vidl/internal/notify"
	"github.com/chriha/vidl/internal/queue"
)

// notifyWindow collects downloads ending close together into one
// notification.
const notifyWindow = 2 * time.Second

// notifyFlushMsg ends the coalescing window and sends the collected
// notifications.
type notifyFlushMsg struct{}

// notifiedMsg carries the notifier's result.
type notifiedMsg struct{ err error }

// collectEnded remembers the downloads that finished or failed between the
// previous snapshot and the current one. Items that weren't queued or running
// before, such as ones already finished at startup, are skipped.
func (m *Model) collectEnded(prev map[string]queue.Status) {
	if m.notifier == nil || m.notifyUnavailable || !m.mgr.Config().Notify {
		return
	}
	for _, list := range [][]queue.Item{m.queue, m.history} {
		for _, it := range list {
			if it.Status != queue.Finished && it.Status != queue.Failed {
				continue
			}
			if s, ok := prev[it.ID]; ok && (s == queue.Queued || s == queue.Downloading) {
				m.ended = append(m.ended, it)
			}
		}
	}
}

// statuses maps the ids of the current snapshot to their status.
func (m Model) statuses() map[string]queue.Status {
	s := make(map[string]queue.Status, len(m.queue)+len(m.history))
	for _, list := range [][]queue.Item{m.queue, m.history} {
		for _, it := range list {
			s[it.ID] = it.Status
		}
	}
	return s
}

// flushNotifications sends one notification for the collected downloads off
// the UI loop.
func (m *Model) flushNotifications() tea.Cmd {
	ended := m.ended
	m.ended = nil
	if len(ended) == 0 || m.notifier == nil || m.notifyUnavailable || !m.mgr.Config().Notify {
		return nil
	}
	title, body := notificationText(ended)
	n := m.notifier
	return func() tea.Msg { return notifiedMsg{n.Notify(title, body)} }
}

// handleNotified reports a notifier error; a missing notifier is reported
// once and disables notifications for the session.
func (m *Model) handleNotified(err error) {
	if errors.Is(err, notify.ErrUnavailable) {
		if m.notifyUnavailable {
			return
		}
		m.notifyUnavailable = true
		err = fmt.Errorf("desktop notifications are off for this session: %w", err)
	}
	if err != nil {
		m.report(err)
	}
}

// notificationText returns the title and body for the ended downloads.
func notificationText(ended []queue.Item) (title, body string) {
	if len(ended) == 1 {
		it := ended[0]
		if it.Status == queue.Failed {
			return "Failed: " + it.DisplayTitle(), it.Error
		}
		var file string
		if it.FilePath != "" {
			file = filepath.Base(it.FilePath)
		}
		return "Downloaded: " + it.DisplayTitle(), file
	}
	var finished, failed int
	titles := make([]string, len(ended))
	for i, it := range ended {
		if it.Status == queue.Failed {
			failed++
		} else {
			finished++
		}
		titles[i] = it.DisplayTitle()
	}
	switch {
	case failed == 0:
		title = plural(finished, "download") + " finished"
	case finished == 0:
		title = plural(failed, "download") + " failed"
	default:
		title = fmt.Sprintf("%s finished, %d failed", plural(finished, "download"), failed)
	}
	return title, strings.Join(titles, ", ")
}
