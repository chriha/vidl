package ytdlp

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseEstimate(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    Estimate
	}{
		{"merged sums requested formats", "estimate_merged.json",
			Estimate{Size: 302044 + 307551, ID: "jNQXAC9IVRw", Title: "Me at the zoo", Uploader: "jawed", Extractor: "Youtube", Duration: 19}},
		{"merged with an unsized part falls back to the total", "estimate_merged_partial.json",
			Estimate{Size: 80000000, ID: "jkl012", Title: "Merged, audio size unknown", Uploader: "Uploader", Extractor: "Youtube", Duration: 300}},
		{"single format prefers filesize", "estimate_single.json",
			Estimate{Size: 10485760, ID: "abc123", Title: "Single stream", Uploader: "Some Channel", Extractor: "Vimeo", Duration: 61.5}},
		{"filesize_approx fallback", "estimate_approx.json",
			Estimate{Size: 52428800, ID: "def456", Title: "Approximate only", Uploader: "Uploader", Extractor: "Generic", Duration: 120}},
		{"no size is unknown", "estimate_none.json",
			Estimate{ID: "ghi789", Title: "Live stream", Uploader: "Uploader", Extractor: "Twitch"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseEstimate(data)
			if err != nil {
				t.Fatal(err)
			}
			if *got != tt.want {
				t.Errorf("ParseEstimate = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestParseEstimateInvalidJSON(t *testing.T) {
	if _, err := ParseEstimate([]byte("not json")); err == nil {
		t.Error("expected an error for invalid JSON")
	}
}

func TestEstimateArgs(t *testing.T) {
	tests := []struct {
		name    string
		f       Format
		res     string
		cookies string
	}{
		{"video with resolution and cookies", MP4, "720", "firefox"},
		{"audio without cookies", MP3, "best", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := EstimateArgs("-weird", tt.f, tt.res, tt.cookies)
			for _, flag := range []string{"-J", "--no-playlist"} {
				if !slices.Contains(args, flag) {
					t.Errorf("missing %s in %v", flag, args)
				}
			}
			sel, _ := argValue(FormatArgs(tt.f, tt.res), "-f")
			if got, ok := argValue(args, "-f"); !ok || got != sel {
				t.Errorf("-f = %q, want %q", got, sel)
			}
			if got, ok := argValue(args, "--cookies-from-browser"); ok != (tt.cookies != "") || got != tt.cookies {
				t.Errorf("--cookies-from-browser = %q (present %v), want %q", got, ok, tt.cookies)
			}
			if n := len(args); n < 2 || args[n-2] != "--" || args[n-1] != "-weird" {
				t.Errorf("URL must come after --: %v", args)
			}
		})
	}
}

func TestRunnerEstimate(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	r := fakeYtdlp(t, `printf '%s\n' "$@" > '`+argsFile+`'
cat '`+filepath.Join(mustAbs(t, "testdata"), "estimate_single.json")+`'
`)
	got, err := r.Estimate(context.Background(), "https://vimeo.com/123", WebM, "480", "")
	if err != nil || got == nil {
		t.Fatalf("Estimate = %v, %v", got, err)
	}
	if got.Size != 10485760 || got.Title != "Single stream" {
		t.Errorf("estimate = %+v", *got)
	}
	recorded, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	want := EstimateArgs("https://vimeo.com/123", WebM, "480", "")
	if lines := strings.Split(strings.TrimSuffix(string(recorded), "\n"), "\n"); !slices.Equal(lines, want) {
		t.Errorf("yt-dlp args = %v, want %v", lines, want)
	}
}

func TestRunnerEstimateError(t *testing.T) {
	r := fakeYtdlp(t, `echo 'ERROR: [generic] Unsupported URL: u' >&2; exit 1`)
	got, err := r.Estimate(context.Background(), "u", MP4, "best", "")
	if err == nil || err.Error() != "[generic] Unsupported URL: u" {
		t.Errorf("Estimate = %v, %v; want the yt-dlp error", got, err)
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
