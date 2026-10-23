package ui

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/chriha/vidl/internal/queue"
)

// errorView is the full-screen view of why a download failed (i).
type errorView struct {
	id string         // the failed item
	vp viewport.Model // its error and saved yt-dlp output
}

// errorViewLines is the content of the error view for it, w cells wide.
func errorViewLines(it queue.Item, w int) []string {
	return nil
}
