package ytdlp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Job describes a single download.
type Job struct {
	URL                string
	Dir                string
	Template           string // output filename template, e.g. "%(title)s.%(ext)s"
	Format             Format
	Resolution         string
	EmbedSubtitles     bool
	SubtitleLangs      string
	EmbedThumbnail     bool
	EmbedMetadata      bool
	RateLimit          string // e.g. "5M", empty for unlimited
	CookiesFromBrowser string // e.g. "firefox", empty to disable
	Clip               Clip   // part of the video to download; zero for all of it
	ForceOverwrites    bool   // replace an existing file instead of skipping the download
}

// Args returns the full yt-dlp argument list for the job.
func (j Job) Args() []string {
	args := []string{
		"--newline", "--progress", "--no-simulate", "--no-playlist",
		"--color", "never",
		"--progress-template", "download:" + prefixDownload + "%(progress)j",
		"--progress-template", "postprocess:" + prefixPost + "%(progress)j",
		"--print", "before_dl:" + prefixInfo + "%(.{id,title,uploader,duration,extractor_key,format_id,webpage_url})j",
		"--print", "after_move:" + prefixFile + "%(filepath)j",
		"-P", j.Dir,
	}
	if j.Template != "" {
		args = append(args, "-o", j.Template)
	}
	args = append(args, FormatArgs(j.Format, j.Resolution)...)
	if !j.Clip.IsZero() {
		args = append(args, "--download-sections", j.Clip.sectionArg(), "--force-keyframes-at-cuts")
	}
	if j.EmbedSubtitles && !j.Format.IsAudio() {
		args = append(args, "--embed-subs")
		if j.SubtitleLangs != "" {
			args = append(args, "--sub-langs", j.SubtitleLangs)
		}
	}
	if j.EmbedThumbnail {
		args = append(args, "--embed-thumbnail")
	}
	if j.EmbedMetadata {
		args = append(args, "--embed-metadata")
	}
	if j.RateLimit != "" {
		args = append(args, "--limit-rate", j.RateLimit)
	}
	if j.CookiesFromBrowser != "" {
		args = append(args, "--cookies-from-browser", j.CookiesFromBrowser)
	}
	if j.ForceOverwrites {
		args = append(args, "--force-overwrites")
	}
	return append(args, "--", j.URL)
}

// Runner executes yt-dlp.
type Runner struct {
	Bin string // defaults to "yt-dlp" on PATH
}

func (r Runner) bin() string {
	if r.Bin != "" {
		return r.Bin
	}
	return "yt-dlp"
}

// Run downloads the job, calling onEvent for every line of output. Calls to
// onEvent are serialized. Cancelling ctx interrupts yt-dlp, which leaves
// partial files in place so the download can be resumed; Run then returns
// context.Cause(ctx).
func (r Runner) Run(ctx context.Context, job Job, onEvent func(Event)) error {
	if err := os.MkdirAll(job.Dir, 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, r.bin(), job.Args()...)
	cmd.Dir = job.Dir
	setInterruptible(cmd)
	cmd.WaitDelay = 10 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var (
		mu      sync.Mutex
		tracker Tracker
		lastErr string
		wg      sync.WaitGroup
	)
	abs := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(job.Dir, p)
	}
	handle := func(line string) {
		mu.Lock()
		defer mu.Unlock()
		ev := tracker.Parse(line)
		if msg, ok := strings.CutPrefix(ev.Line, "ERROR: "); ok {
			lastErr = msg
		}
		ev.Path = abs(ev.Path)
		for i, f := range ev.Files {
			ev.Files[i] = abs(f)
		}
		onEvent(ev)
	}
	scan := func(rd io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(rd)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			handle(sc.Text())
		}
		// Keep draining if the scanner gave up, so yt-dlp never blocks on a full pipe.
		_, _ = io.Copy(io.Discard, rd)
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	wg.Wait()

	err = cmd.Wait()
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if err != nil && lastErr != "" {
		return errors.New(lastErr)
	}
	return err
}
