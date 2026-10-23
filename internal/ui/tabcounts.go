package ui

import "github.com/chriha/vidl/internal/queue"

// statusCount is the number of listed items with one status.
type statusCount struct {
	status queue.Status
	n      int
}

// tabCounts returns the counts shown after a tab's name: per status on
// Downloads, one total on Finished, none elsewhere. Zero counts are left out.
func (m Model) tabCounts(t tab) []statusCount {
	return nil // stub
}
