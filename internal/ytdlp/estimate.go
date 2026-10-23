package ytdlp

import (
	"context"
	"encoding/json"
	"fmt"
)

// Estimate is yt-dlp's size estimate for one download, plus the metadata
// fetched with it.
type Estimate struct {
	Size      int64 // bytes; 0 when yt-dlp reports no size
	Title     string
	Uploader  string
	Extractor string
	Duration  float64
}

type rawEstimate struct {
	rawSize
	Title            string    `json:"title"`
	Uploader         string    `json:"uploader"`
	Channel          string    `json:"channel"`
	Extractor        string    `json:"extractor_key"`
	Duration         float64   `json:"duration"`
	RequestedFormats []rawSize `json:"requested_formats"`
}

type rawSize struct {
	Filesize       float64 `json:"filesize"`
	FilesizeApprox float64 `json:"filesize_approx"`
}

func (s rawSize) bytes() int64 {
	if s.Filesize > 0 {
		return int64(s.Filesize)
	}
	return int64(s.FilesizeApprox)
}

// EstimateArgs returns the yt-dlp arguments that resolve url's stream
// selection for the given format and resolution without downloading.
func EstimateArgs(url string, f Format, resolution, cookiesFromBrowser string) []string {
	args := []string{"-J", "--no-playlist", "--no-warnings", "--color", "never"}
	args = append(args, FormatArgs(f, resolution)...)
	if cookiesFromBrowser != "" {
		args = append(args, "--cookies-from-browser", cookiesFromBrowser)
	}
	return append(args, "--", url)
}

// Estimate asks yt-dlp which streams it would download for url and how
// large they are.
func (r Runner) Estimate(ctx context.Context, url string, f Format, resolution, cookiesFromBrowser string) (*Estimate, error) {
	out, err := r.output(ctx, EstimateArgs(url, f, resolution, cookiesFromBrowser))
	if err != nil {
		return nil, err
	}
	return ParseEstimate(out)
}

// ParseEstimate parses the output of `yt-dlp -J` run with a format selector.
// A merged download's size is the sum of its requested formats; if any of
// them has no size, the top-level size is used instead.
func ParseEstimate(data []byte) (*Estimate, error) {
	var raw rawEstimate
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse yt-dlp estimate: %w", err)
	}
	return &Estimate{
		Size:      raw.size(),
		Title:     raw.Title,
		Uploader:  cmp(raw.Uploader, raw.Channel),
		Extractor: raw.Extractor,
		Duration:  raw.Duration,
	}, nil
}

func (raw rawEstimate) size() int64 {
	var sum int64
	for _, f := range raw.RequestedFormats {
		b := f.bytes()
		if b <= 0 {
			return raw.bytes()
		}
		sum += b
	}
	if sum > 0 {
		return sum
	}
	return raw.bytes()
}
