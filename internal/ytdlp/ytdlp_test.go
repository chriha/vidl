package ytdlp

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTrackerFixture(t *testing.T) {
	f, err := os.Open("testdata/download.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var (
		tr       Tracker
		info     VideoInfo
		path     string
		last     Progress
		stages   []string
		percents []float64
		files    []string
	)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ev := tr.Parse(sc.Text())
		switch ev.Kind {
		case EventInfo:
			info = ev.Info
		case EventFile:
			path = ev.Path
		case EventProgress:
			percents = append(percents, ev.Progress.Percent)
			files = append(files, ev.Files...)
			last = ev.Progress
		case EventPostprocess:
			stages = append(stages, ev.Progress.Stage)
			last = ev.Progress
		case EventLog:
			t.Errorf("unexpected log line: %q", ev.Line)
		}
	}

	if info.Title != "Me at the zoo" || info.FormatID != "395+140" || info.Duration != 19 {
		t.Errorf("info = %+v", info)
	}
	if path != "/dl/Me at the zoo [jNQXAC9IVRw].mp4" {
		t.Errorf("path = %q", path)
	}
	if !slices.IsSorted(percents) {
		t.Errorf("progress not monotonic: %v", percents)
	}
	if percents[len(percents)-1] != 100 {
		t.Errorf("final percent = %v", percents[len(percents)-1])
	}
	if mid := percents[len(percents)/2]; mid <= 0 || mid >= 100 {
		t.Errorf("mid percent = %v", mid)
	}
	if last.Streams != 2 || last.Percent != 100 {
		t.Errorf("last = %+v", last)
	}
	if !slices.Equal(stages, []string{"merging", "merging", "moving files", "moving files"}) {
		t.Errorf("stages = %v", stages)
	}
	if !slices.Contains(files, "/dl/Me at the zoo [jNQXAC9IVRw].f395.mp4.part") {
		t.Errorf("partial files not reported: %v", files)
	}
}

func TestTrackerSingleStreamWithoutTotal(t *testing.T) {
	var tr Tracker
	tr.Parse(`vidl-info:{"title": "x", "format_id": "18"}`)
	ev := tr.Parse(`vidl-dl:{"status": "downloading", "downloaded_bytes": 50, "total_bytes": null, "total_bytes_estimate": 200.0, "speed": null, "eta": null}`)
	if ev.Progress.Percent != 25 || ev.Progress.Streams != 1 || ev.Progress.Total != 200 {
		t.Errorf("progress = %+v", ev.Progress)
	}
}

func TestTrackerMalformedLineIsLog(t *testing.T) {
	var tr Tracker
	ev := tr.Parse(`vidl-dl:{"status": NaN}`)
	if ev.Kind != EventLog {
		t.Errorf("kind = %v", ev.Kind)
	}
}

func TestParseInfoVideo(t *testing.T) {
	data, err := os.ReadFile("testdata/probe_video.json")
	if err != nil {
		t.Fatal(err)
	}
	info, err := ParseInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.IsPlaylist || info.Title != "Me at the zoo" || info.Uploader != "jawed" {
		t.Errorf("info = %+v", info)
	}
	if !slices.Equal(info.Heights, []int{240, 144}) {
		t.Errorf("heights = %v", info.Heights)
	}
}

func TestParseInfoPlaylist(t *testing.T) {
	info, err := ParseInfo([]byte(`{"_type": "playlist", "title": "List", "channel": "Ch",
		"webpage_url": "https://example.com/list",
		"entries": [
			{"_type": "url", "id": "a", "title": "A", "url": "https://example.com/a", "duration": 10},
			{"_type": "url", "id": "b", "title": "B", "webpage_url": "https://example.com/b"}
		]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsPlaylist || info.Uploader != "Ch" || len(info.Entries) != 2 {
		t.Fatalf("info = %+v", info)
	}
	if info.Entries[1].URL != "https://example.com/b" {
		t.Errorf("entry url = %q", info.Entries[1].URL)
	}
}

func TestFormatArgs(t *testing.T) {
	tests := []struct {
		f    Format
		res  string
		want []string
	}{
		{MP4, "1080", []string{"-f", "bv*[height<=1080][ext=mp4]+ba[ext=m4a]/bv*[height<=1080]+ba/b[height<=1080]", "--merge-output-format", "mp4"}},
		{MKV, "best", []string{"-f", "bv*+ba/b", "--merge-output-format", "mkv"}},
		{WebM, "", []string{"-f", "bv*[ext=webm]+ba[ext=webm]/bv*+ba/b", "--merge-output-format", "webm"}},
		{MP3, "720", []string{"-f", "ba/b", "-x", "--audio-format", "mp3"}},
	}
	for _, tt := range tests {
		if got := FormatArgs(tt.f, tt.res); !slices.Equal(got, tt.want) {
			t.Errorf("FormatArgs(%s, %q) = %v, want %v", tt.f, tt.res, got, tt.want)
		}
	}
}

func TestJobArgs(t *testing.T) {
	args := Job{URL: "-weird", Dir: "/dl", Format: MP3, EmbedSubtitles: true, RateLimit: "2M"}.Args()
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--embed-subs") {
		t.Error("subtitles should not be embedded into audio")
	}
	if !strings.Contains(joined, "--limit-rate 2M") {
		t.Error("missing rate limit")
	}
	if args[len(args)-2] != "--" || args[len(args)-1] != "-weird" {
		t.Error("URL must come after --")
	}
}

// fakeYtdlp writes a shell script standing in for yt-dlp.
func fakeYtdlp(t *testing.T, body string) Runner {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "yt-dlp")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return Runner{Bin: bin}
}

func TestRunSuccess(t *testing.T) {
	r := fakeYtdlp(t, `
echo 'vidl-info:{"title": "T", "format_id": "1"}'
echo 'vidl-dl:{"status": "downloading", "downloaded_bytes": 5, "total_bytes": 10, "filename": "t.mp4", "tmpfilename": "t.mp4.part"}'
echo 'some warning' >&2
echo 'vidl-dl:{"status": "finished", "downloaded_bytes": 10, "total_bytes": 10, "filename": "t.mp4"}'
echo 'vidl-file:"t.mp4"'
`)
	dir := t.TempDir()
	var events []Event
	err := r.Run(context.Background(), Job{URL: "u", Dir: dir, Format: MP4}, func(ev Event) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatal(err)
	}
	// stdout and stderr are read concurrently, so only stdout order is fixed.
	var kinds []EventKind
	var path string
	var files []string
	for _, ev := range events {
		if ev.Kind == EventLog {
			continue
		}
		kinds = append(kinds, ev.Kind)
		path = cmp(path, ev.Path)
		files = append(files, ev.Files...)
	}
	if !slices.Equal(kinds, []EventKind{EventInfo, EventProgress, EventProgress, EventFile}) {
		t.Errorf("kinds = %v (%d events total)", kinds, len(events))
	}
	if path != filepath.Join(dir, "t.mp4") {
		t.Errorf("path = %q", path)
	}
	if !slices.Equal(files, []string{filepath.Join(dir, "t.mp4.part"), filepath.Join(dir, "t.mp4")}) {
		t.Errorf("files = %v", files)
	}
}

func TestRunError(t *testing.T) {
	r := fakeYtdlp(t, `echo 'ERROR: [generic] Unsupported URL: u' >&2; exit 1`)
	err := r.Run(context.Background(), Job{URL: "u", Dir: t.TempDir()}, func(Event) {})
	if err == nil || err.Error() != "[generic] Unsupported URL: u" {
		t.Errorf("err = %v", err)
	}
}

func TestRunCancel(t *testing.T) {
	r := fakeYtdlp(t, `echo started; trap 'exit 130' INT; while :; do sleep 0.05; done`)
	ctx, cancel := context.WithCancelCause(context.Background())
	stop := errors.New("paused")
	started := make(chan struct{})
	done := make(chan error)
	go func() {
		done <- r.Run(ctx, Job{URL: "u", Dir: t.TempDir()}, func(ev Event) {
			if ev.Line == "started" {
				close(started)
			}
		})
	}()
	<-started
	cancel(stop)
	select {
	case err := <-done:
		if !errors.Is(err, stop) {
			t.Errorf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
