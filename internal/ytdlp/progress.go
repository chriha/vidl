package ytdlp

import (
	"encoding/json"
	"strings"
	"time"
)

// Line prefixes injected via --progress-template and --print so our output can
// be told apart from yt-dlp's regular log lines.
const (
	prefixDownload = "vidl-dl:"
	prefixPost     = "vidl-pp:"
	prefixInfo     = "vidl-info:"
	prefixFile     = "vidl-file:"
)

type EventKind int

const (
	EventLog EventKind = iota
	EventProgress
	EventPostprocess
	EventInfo
	EventFile
)

// Event is a single parsed line of yt-dlp output.
type Event struct {
	Kind     EventKind
	Progress Progress  // EventProgress, EventPostprocess
	Info     VideoInfo // EventInfo
	Path     string    // EventFile: final path of the downloaded file
	Files    []string  // EventProgress: files being written (for cleanup)
	Line     string    // EventLog
}

// VideoInfo is printed by yt-dlp right before downloading starts.
type VideoInfo struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Uploader  string  `json:"uploader"`
	Duration  float64 `json:"duration"`
	Extractor string  `json:"extractor_key"`
	FormatID  string  `json:"format_id"`
	URL       string  `json:"webpage_url"`
}

// Progress is the overall progress of a download, combining all streams
// (e.g. video and audio) that yt-dlp fetches separately before merging.
type Progress struct {
	Percent    float64       `json:"percent"`
	Downloaded int64         `json:"downloaded"` // current stream
	Total      int64         `json:"total"`      // current stream, 0 if unknown
	Speed      float64       `json:"speed"`      // bytes per second
	ETA        time.Duration `json:"eta"`        // current stream
	Stream     int           `json:"stream"`     // 1-based index of the current stream
	Streams    int           `json:"streams"`
	Stage      string        `json:"stage"`
}

type rawProgress struct {
	Status        string   `json:"status"`
	Downloaded    float64  `json:"downloaded_bytes"`
	Total         float64  `json:"total_bytes"`
	TotalEstimate float64  `json:"total_bytes_estimate"`
	Speed         float64  `json:"speed"`
	ETA           float64  `json:"eta"`
	Percent       *float64 `json:"_percent"`
	Filename      string   `json:"filename"`
	TmpFilename   string   `json:"tmpfilename"`
	Postprocessor string   `json:"postprocessor"`
}

// Tracker turns yt-dlp output lines into events, keeping the state needed to
// compute progress across multiple streams. It is not safe for concurrent use.
type Tracker struct {
	Clip         Clip // the job's clip; ffmpeg progress lines are only parsed when set
	duration     float64
	streams      int
	done         int
	lastFinished string
	last         Progress
}

// Parse parses one line of output. Lines that aren't ours become EventLog.
func (t *Tracker) Parse(line string) Event {
	switch {
	case strings.HasPrefix(line, prefixDownload):
		var r rawProgress
		if json.Unmarshal([]byte(line[len(prefixDownload):]), &r) == nil {
			ev := Event{Kind: EventProgress, Progress: t.download(r)}
			if r.Status == "downloading" {
				for _, f := range []string{r.TmpFilename, r.Filename} {
					if f != "" {
						ev.Files = append(ev.Files, f)
					}
				}
			}
			return ev
		}
	case strings.HasPrefix(line, prefixPost):
		var r rawProgress
		if json.Unmarshal([]byte(line[len(prefixPost):]), &r) == nil {
			p := t.last
			p.Percent, p.Speed, p.ETA = 100, 0, 0
			p.Stage = stageName(r.Postprocessor)
			t.last = p
			return Event{Kind: EventPostprocess, Progress: p}
		}
	case strings.HasPrefix(line, prefixInfo):
		var info VideoInfo
		if json.Unmarshal([]byte(line[len(prefixInfo):]), &info) == nil {
			t.streams = strings.Count(info.FormatID, "+") + 1
			return Event{Kind: EventInfo, Info: info}
		}
	case strings.HasPrefix(line, prefixFile):
		var path string
		if json.Unmarshal([]byte(line[len(prefixFile):]), &path) == nil {
			return Event{Kind: EventFile, Path: path}
		}
	}
	return Event{Kind: EventLog, Line: line}
}

func (t *Tracker) download(r rawProgress) Progress {
	total := r.Total
	if total == 0 {
		total = r.TotalEstimate
	}
	var frac float64
	switch {
	case r.Status == "finished":
		frac = 1
	case r.Percent != nil:
		frac = *r.Percent / 100
	case total > 0:
		frac = r.Downloaded / total
	}
	p := Progress{
		Downloaded: int64(r.Downloaded),
		Total:      int64(total),
		Speed:      r.Speed,
		ETA:        time.Duration(r.ETA * float64(time.Second)),
		Stream:     t.done + 1,
		Streams:    max(t.streams, t.done+1),
		Stage:      "downloading",
	}
	p.Percent = min(max((float64(t.done)+frac)/float64(p.Streams)*100, 0), 100)
	if r.Status == "finished" && r.Filename != t.lastFinished {
		t.done++
		t.lastFinished = r.Filename
	}
	t.last = p
	return p
}

func stageName(pp string) string {
	switch pp {
	case "Merger":
		return "merging"
	case "ExtractAudio":
		return "extracting audio"
	case "MoveFiles":
		return "moving files"
	case "EmbedThumbnail":
		return "embedding thumbnail"
	case "FFmpegEmbedSubtitle":
		return "embedding subtitles"
	case "FFmpegMetadata":
		return "writing metadata"
	default:
		return "post-processing"
	}
}
