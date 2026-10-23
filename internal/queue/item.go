package queue

import (
	"fmt"
	"slices"
	"time"

	"vidl/internal/ytdlp"
)

type Status int

const (
	Queued Status = iota
	Downloading
	Paused
	Failed
	Cancelled
	Finished
)

var statusNames = []string{"queued", "downloading", "paused", "failed", "cancelled", "finished"}

func (s Status) String() string {
	if int(s) < len(statusNames) {
		return statusNames[s]
	}
	return fmt.Sprintf("status(%d)", int(s))
}

func (s Status) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

func (s *Status) UnmarshalText(b []byte) error {
	i := slices.Index(statusNames, string(b))
	if i < 0 {
		return fmt.Errorf("unknown status %q", b)
	}
	*s = Status(i)
	return nil
}

const maxLogLines = 200

// Item is a single download, queued or finished.
type Item struct {
	ID         string         `json:"id"`
	URL        string         `json:"url"`
	Title      string         `json:"title,omitempty"`
	Uploader   string         `json:"uploader,omitempty"`
	Extractor  string         `json:"extractor,omitempty"`
	Duration   float64        `json:"duration,omitempty"`
	Format     ytdlp.Format   `json:"format"`
	Resolution string         `json:"resolution"`
	Status     Status         `json:"status"`
	Progress   ytdlp.Progress `json:"progress"`
	FilePath   string         `json:"file_path,omitempty"`
	Error      string         `json:"error,omitempty"`
	PartFiles  []string       `json:"part_files,omitempty"`
	Log        []string       `json:"-"`
	AddedAt    time.Time      `json:"added_at"`
	StartedAt  time.Time      `json:"started_at,omitzero"`
	FinishedAt time.Time      `json:"finished_at,omitzero"`
}

// DisplayTitle is the title, or the URL while the title is still unknown.
func (it Item) DisplayTitle() string {
	if it.Title != "" {
		return it.Title
	}
	return it.URL
}

func (it *Item) clone() Item {
	c := *it
	c.PartFiles = slices.Clone(it.PartFiles)
	c.Log = slices.Clone(it.Log)
	return c
}

func (it *Item) appendLog(line string) {
	it.Log = append(it.Log, line)
	if over := len(it.Log) - maxLogLines; over > 0 {
		it.Log = slices.Delete(it.Log, 0, over)
	}
}

func (it *Item) addPartFiles(files []string) {
	for _, f := range files {
		if !slices.Contains(it.PartFiles, f) {
			it.PartFiles = append(it.PartFiles, f)
		}
	}
}
