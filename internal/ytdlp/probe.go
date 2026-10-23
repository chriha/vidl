package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// Info describes a URL before downloading: either a single video or a
// playlist whose entries can be queued individually.
type Info struct {
	ID             string
	Title          string
	Uploader       string
	URL            string
	Extractor      string
	Thumbnail      string
	Duration       float64
	FilesizeApprox int64
	Heights        []int // available video heights, highest first
	IsPlaylist     bool
	Entries        []Entry
}

// Entry is a playlist item.
type Entry struct {
	ID       string
	Title    string
	URL      string
	Duration float64
}

type rawInfo struct {
	Type           string  `json:"_type"`
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Uploader       string  `json:"uploader"`
	Channel        string  `json:"channel"`
	WebpageURL     string  `json:"webpage_url"`
	URL            string  `json:"url"`
	Extractor      string  `json:"extractor_key"`
	Thumbnail      string  `json:"thumbnail"`
	Duration       float64 `json:"duration"`
	FilesizeApprox float64 `json:"filesize_approx"`
	Formats        []struct {
		Height float64 `json:"height"`
		VCodec string  `json:"vcodec"`
	} `json:"formats"`
	Entries []rawInfo `json:"entries"`
}

// Probe fetches metadata for url without downloading it. Playlists are not
// resolved beyond their entry list.
func (r Runner) Probe(ctx context.Context, url, cookiesFromBrowser string) (*Info, error) {
	args := []string{"-J", "--flat-playlist", "--no-warnings", "--color", "never"}
	if cookiesFromBrowser != "" {
		args = append(args, "--cookies-from-browser", cookiesFromBrowser)
	}
	args = append(args, "--", url)
	out, err := r.output(ctx, args)
	if err != nil {
		return nil, err
	}
	return ParseInfo(out)
}

// output runs yt-dlp with args and returns its stdout. On failure the error
// is the context's error or yt-dlp's last ERROR line, when there is one.
func (r Runner) output(ctx context.Context, args []string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, r.bin(), args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if msg := lastError(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return out, nil
}

// ParseInfo parses the output of `yt-dlp -J --flat-playlist`.
func ParseInfo(data []byte) (*Info, error) {
	var raw rawInfo
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse yt-dlp info: %w", err)
	}
	info := &Info{
		ID:             raw.ID,
		Title:          raw.Title,
		Uploader:       cmp(raw.Uploader, raw.Channel),
		URL:            cmp(raw.WebpageURL, raw.URL),
		Extractor:      raw.Extractor,
		Thumbnail:      raw.Thumbnail,
		Duration:       raw.Duration,
		FilesizeApprox: int64(raw.FilesizeApprox),
		IsPlaylist:     raw.Type == "playlist" || raw.Type == "multi_video",
	}
	for _, e := range raw.Entries {
		info.Entries = append(info.Entries, Entry{
			ID:       e.ID,
			Title:    e.Title,
			URL:      cmp(e.URL, e.WebpageURL),
			Duration: e.Duration,
		})
	}
	for _, f := range raw.Formats {
		h := int(f.Height)
		if h > 0 && f.VCodec != "none" && !slices.Contains(info.Heights, h) {
			info.Heights = append(info.Heights, h)
		}
	}
	slices.Sort(info.Heights)
	slices.Reverse(info.Heights)
	return info, nil
}

func cmp(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func lastError(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if msg, ok := strings.CutPrefix(lines[i], "ERROR: "); ok {
			return msg
		}
	}
	return ""
}
