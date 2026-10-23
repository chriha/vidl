package ytdlp

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// StaleAfter is the age of a yt-dlp release after which updating it is advised.
const StaleAfter = 60 * 24 * time.Hour

// versionRe matches release (2025.10.22) and nightly (2025.10.22.232853) versions.
var versionRe = regexp.MustCompile(`^(\d{4})\.(\d{2})\.(\d{2})(?:\.\d+)?$`)

// Version returns the output of `yt-dlp --version`, trimmed.
func (r Runner) Version(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, r.bin(), "--version").Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp --version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ParseVersion returns the release date of a yt-dlp version string. Versions
// that aren't dates (git or dev builds) are an error.
func ParseVersion(v string) (time.Time, error) {
	m := versionRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return time.Time{}, fmt.Errorf("unrecognized yt-dlp version %q", v)
	}
	var y, mo, d int
	fmt.Sscanf(m[1]+" "+m[2]+" "+m[3], "%d %d %d", &y, &mo, &d)
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != y || int(t.Month()) != mo || t.Day() != d {
		return time.Time{}, fmt.Errorf("invalid date in yt-dlp version %q", v)
	}
	return t, nil
}

// StaleDays reports how many whole days old version is at now, and whether
// that exceeds StaleAfter. Unparseable versions are an error.
func StaleDays(version string, now time.Time) (days int, stale bool, err error) {
	released, err := ParseVersion(version)
	if err != nil {
		return 0, false, err
	}
	days = int(now.UTC().Sub(released) / (24 * time.Hour))
	return days, time.Duration(days)*24*time.Hour > StaleAfter, nil
}
