package ytdlp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Clip is the part of a video to download. The zero value downloads the whole
// video; a zero End means "until the end".
type Clip struct {
	Start time.Duration
	End   time.Duration
}

// IsZero reports whether the clip covers the whole video.
func (c Clip) IsZero() bool {
	return c == Clip{}
}

// sectionArg returns the --download-sections value, e.g. "*90-inf".
func (c Clip) sectionArg() string {
	end := "inf"
	if c.End > 0 {
		end = seconds(c.End)
	}
	return "*" + seconds(c.Start) + "-" + end
}

func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64)
}

// ParseClip parses optional start and end times (see ParseClipTime). Empty
// fields are open ends; both empty yield the zero Clip. End must be after start.
func ParseClip(start, end string) (Clip, error) {
	var c Clip
	var err error
	if strings.TrimSpace(start) != "" {
		if c.Start, err = ParseClipTime(start); err != nil {
			return Clip{}, fmt.Errorf("start: %w", err)
		}
	}
	if strings.TrimSpace(end) != "" {
		if c.End, err = ParseClipTime(end); err != nil {
			return Clip{}, fmt.Errorf("end: %w", err)
		}
		if c.End <= c.Start {
			return Clip{}, errors.New("end must be after start")
		}
	}
	return c, nil
}

// ParseClipTime parses a time of the form [[h:]m:]s, e.g. "90", "1:30" or
// "1:02:03". Seconds may have a fraction; minutes and seconds after a colon
// must be below 60.
func ParseClipTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if s == "" || len(parts) > 3 {
		return 0, fmt.Errorf("invalid time %q, want [[h:]m:]s", s)
	}
	var total time.Duration
	for i, p := range parts {
		last := i == len(parts)-1
		v, ok := parseClipField(p, last)
		if !ok || (i > 0 && v >= 60) {
			return 0, fmt.Errorf("invalid time %q, want [[h:]m:]s", s)
		}
		total = total*60 + time.Duration(v*float64(time.Second))
	}
	return total, nil
}

// parseClipField parses one non-negative number of a clip time; only the last
// field may have a fraction. Signs, exponents and words like "inf" are rejected.
func parseClipField(p string, fraction bool) (float64, bool) {
	whole, frac, hasFrac := strings.Cut(p, ".")
	if whole == "" || !digits(whole) || (hasFrac && (!fraction || frac == "" || !digits(frac))) {
		return 0, false
	}
	v, err := strconv.ParseFloat(p, 64)
	return v, err == nil
}

func digits(s string) bool {
	return strings.Trim(s, "0123456789") == ""
}
