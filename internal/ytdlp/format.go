package ytdlp

import "fmt"

// Format is the container or audio codec of the final file.
type Format string

const (
	MP4  Format = "mp4"
	MKV  Format = "mkv"
	WebM Format = "webm"
	MP3  Format = "mp3"
	M4A  Format = "m4a"
	Opus Format = "opus"
)

// Formats lists every supported output format, video first.
var Formats = []Format{MP4, MKV, WebM, MP3, M4A, Opus}

// Resolutions are the selectable maximum video heights. "best" means no limit.
var Resolutions = []string{"best", "2160", "1440", "1080", "720", "480", "360"}

func (f Format) IsAudio() bool {
	return f == MP3 || f == M4A || f == Opus
}

func (f Format) Valid() bool {
	for _, v := range Formats {
		if f == v {
			return true
		}
	}
	return false
}

// FormatArgs returns the yt-dlp arguments that select streams for the given
// output format and maximum resolution. Preferred stream extensions match the
// container so that merging doesn't need to re-encode. MP4 and MKV prefer
// H.264 and AAC because AV1 doesn't play in Apple players without hardware
// decode; anything else falls back to the best available streams.
func FormatArgs(f Format, resolution string) []string {
	if f.IsAudio() {
		return []string{"-f", "ba/b", "-x", "--audio-format", string(f)}
	}
	h := ""
	if resolution != "" && resolution != "best" {
		h = "[height<=" + resolution + "]"
	}
	var sel string
	switch f {
	case MP4:
		sel = fmt.Sprintf("bv*%[1]s[vcodec^=avc1]+ba[acodec^=mp4a]/bv*%[1]s[ext=mp4]+ba[ext=m4a]/bv*%[1]s+ba/b%[1]s", h)
	case MKV:
		sel = fmt.Sprintf("bv*%[1]s[vcodec^=avc1]+ba[acodec^=mp4a]/bv*%[1]s+ba/b%[1]s", h)
	case WebM:
		sel = fmt.Sprintf("bv*%[1]s[ext=webm]+ba[ext=webm]/bv*%[1]s+ba/b%[1]s", h)
	default:
		sel = fmt.Sprintf("bv*%[1]s+ba/b%[1]s", h)
	}
	return []string{"-f", sel, "--merge-output-format", string(f)}
}
