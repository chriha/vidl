package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/chriha/vidl/internal/ytdlp"
)

// versionMsg carries the result of the startup yt-dlp version check.
type versionMsg struct {
	version string
	err     error
}

// checkVersion runs the version lookup off the UI loop; nil when disabled.
func (m Model) checkVersion() tea.Cmd {
	fn := m.versionFn
	if fn == nil {
		return nil
	}
	return func() tea.Msg {
		v, err := fn(context.Background())
		return versionMsg{version: v, err: err}
	}
}

// handleVersion reports a failed lookup, or warns when yt-dlp is stale.
// Versions that aren't dates (git or dev builds) are ignored.
func (m *Model) handleVersion(msg versionMsg) {
	if msg.err != nil {
		m.report(msg.err)
		return
	}
	days, stale, err := ytdlp.StaleDays(msg.version, m.now())
	if err != nil || !stale {
		return
	}
	m.longNotice(fmt.Sprintf("yt-dlp is %d days old — if downloads fail, update it (yt-dlp -U, brew upgrade yt-dlp or pip install -U yt-dlp)", days))
}
