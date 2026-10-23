package ytdlp

import "context"

// Estimate is yt-dlp's size estimate for one download, plus the metadata
// fetched with it.
type Estimate struct {
	Size      int64 // bytes; 0 when yt-dlp reports no size
	Title     string
	Uploader  string
	Extractor string
	Duration  float64
}

// EstimateArgs returns the yt-dlp arguments that resolve url's stream
// selection for the given format and resolution without downloading.
func EstimateArgs(url string, f Format, resolution, cookiesFromBrowser string) []string {
	return nil
}

// Estimate asks yt-dlp which streams it would download for url and how
// large they are.
func (r Runner) Estimate(ctx context.Context, url string, f Format, resolution, cookiesFromBrowser string) (*Estimate, error) {
	return nil, nil
}

// ParseEstimate parses the output of `yt-dlp -J` run with a format selector.
func ParseEstimate(data []byte) (*Estimate, error) {
	return &Estimate{}, nil
}
