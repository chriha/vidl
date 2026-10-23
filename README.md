# vidl

A terminal UI for downloading videos, built with Go and
[Bubble Tea](https://github.com/charmbracelet/bubbletea) on top of
[yt-dlp](https://github.com/yt-dlp/yt-dlp).

vidl manages a download queue for you: paste a URL, pick a format and
resolution, and it downloads in the background with progress, speed and ETA
for every item. The queue and history survive restarts, and interrupted
downloads resume where they stopped.

> **Status:** early development. The download engine, queue, persistence and
> configuration are complete and tested, and so are the Downloads and Finished
> tables and the detail panel; the new-download dialog and settings editor are
> in progress.
> See [Roadmap](#roadmap) for what works today.

## Features

- **Download queue** with a configurable number of concurrent downloads.
  Queued items start automatically, top to bottom.
- **Per-download progress**: percentage, speed and ETA, combined across the
  separate video and audio streams yt-dlp fetches before merging.
- **Pause, resume, cancel and retry.** Pausing keeps partial files so the
  download resumes; cancelling deletes them.
- **Reordering**: move waiting downloads up and down the queue.
- **Finished history** of completed downloads with their file paths.
- **Detail panel** for any download: source, format, progress per stream,
  timestamps, output and partial files, errors, and yt-dlp's latest output
  for running or failed downloads.
- **Persistence**: queue and history are saved on every change; downloads
  running when you quit are resumed on the next start.
- **Output formats**: mp4, mkv, webm (video) and mp3, m4a, opus (audio only),
  with a maximum resolution from 360p up to 2160p or best available.
- **yt-dlp options** exposed as settings: filename template, embedded
  subtitles, thumbnails and metadata, rate limit, and cookies from your
  browser for private or age-restricted videos.
- **Vim-style navigation.**

## Requirements

- [yt-dlp](https://github.com/yt-dlp/yt-dlp) on your `PATH`, ideally a recent
  version (sites change often; update with `yt-dlp -U` or your package
  manager).
- [ffmpeg](https://ffmpeg.org) on your `PATH`, used to merge video and audio
  streams, extract audio and embed thumbnails/subtitles.
- [Go](https://go.dev/dl/) 1.27 or newer to build from source.
- macOS or Linux. Windows builds compile but are untested.

vidl checks for yt-dlp and ffmpeg at startup and exits with an install hint
if either is missing.

### Installing the dependencies

macOS (Homebrew):

```sh
brew install yt-dlp ffmpeg
```

Debian/Ubuntu (the distro's yt-dlp package is often outdated, so prefer
pipx):

```sh
sudo apt install ffmpeg pipx
pipx install yt-dlp
```

Arch Linux:

```sh
sudo pacman -S yt-dlp ffmpeg
```

YouTube additionally needs a JavaScript runtime (such as
[Deno](https://deno.com)) for yt-dlp to see every format; see
[Troubleshooting](#troubleshooting).

## Installation

Build from source:

```sh
git clone <repository-url> vidl
cd vidl
make install        # installs into $(go env GOPATH)/bin
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`. Alternatively, build into
the project directory with `make build` and run `./bin/vidl`.

To uninstall, run `make uninstall` and optionally remove the config directory
(see [Files](#files)).

## Usage

```sh
vidl                        # open the UI
vidl <url> [<url>...]       # open the UI and queue the given URLs
vidl --version
vidl --help
```

Quote URLs in your shell, since characters like `?` and `&` are special:

```sh
vidl 'https://www.youtube.com/watch?v=jNQXAC9IVRw'
```

The UI has three tabs:

- **Downloads**: running downloads at the top, followed by the queue in the
  order downloads will start.
- **Finished**: completed downloads, most recent first.
- **Settings**: download preferences.

Quitting stops running downloads and keeps their partial files; they continue
automatically the next time you start vidl.

## Keybindings

| Key                   | Action                                         | Status  |
| --------------------- | ---------------------------------------------- | ------- |
| `j` / `↓`             | Move down                                      | ✅      |
| `k` / `↑`             | Move up                                        | ✅      |
| `g` / `G`             | Jump to top / bottom                           | ✅      |
| `ctrl+d` / `ctrl+u`   | Half page down / up                            | ✅      |
| `tab` / `l`           | Next tab                                       | ✅      |
| `shift+tab` / `h`     | Previous tab                                   | ✅      |
| `1` `2` `3`           | Go to Downloads / Finished / Settings          | ✅      |
| `s`                   | Start or resume; retries failed or cancelled   | ✅      |
| `p`                   | Pause                                          | ✅      |
| `c`                   | Cancel (deletes partial files)                 | ✅      |
| `enter`               | Toggle the detail panel                        | ✅      |
| `esc`                 | Close the detail panel                         | ✅      |
| `n`                   | New download                                   | planned |
| `r`                   | Remove (asks for confirmation)                 | ✅      |
| `u` / `d`             | Move queued download up / down                 | ✅      |
| `/`                   | Search / filter                                | planned |
| `R`                   | Retry failed or cancelled download             | ✅      |
| `o`                   | Open file (Finished tab)                       | planned |
| `X`                   | Clear finished history                         | planned |
| `?`                   | Toggle full help                               | ✅      |
| `q` / `ctrl+c`        | Quit                                           | ✅      |

## Configuration

Settings are stored in `~/.config/vidl/config.toml` (or
`$XDG_CONFIG_HOME/vidl/config.toml`). The file is created with the defaults
on first start; edit it while vidl is not running. Invalid values fall back
to their defaults.

```toml
download_dir = "~/Downloads"
concurrency = 3
default_resolution = "1080"
default_format = "mp4"
filename_template = "%(title)s [%(id)s].%(ext)s"
embed_subtitles = false
subtitle_langs = "en.*"
embed_thumbnail = false
embed_metadata = true
rate_limit = ""
cookies_from_browser = ""
```

| Key                    | Default                       | Description |
| ---------------------- | ----------------------------- | ----------- |
| `download_dir`         | `~/Downloads`                 | Where files are saved. `~` is expanded. |
| `concurrency`          | `3`                           | Downloads running at the same time (1–10). |
| `default_resolution`   | `1080`                        | Maximum video height for new downloads: `best`, `2160`, `1440`, `1080`, `720`, `480` or `360`. |
| `default_format`       | `mp4`                         | `mp4`, `mkv`, `webm`, or audio only: `mp3`, `m4a`, `opus`. |
| `filename_template`    | `%(title)s [%(id)s].%(ext)s`  | yt-dlp [output template](https://github.com/yt-dlp/yt-dlp#output-template), relative to `download_dir`. |
| `embed_subtitles`      | `false`                       | Embed subtitles into video files. |
| `subtitle_langs`       | `en.*`                        | Subtitle languages, in yt-dlp `--sub-langs` syntax (e.g. `en.*,de`). |
| `embed_thumbnail`      | `false`                       | Embed the thumbnail as cover art. |
| `embed_metadata`       | `true`                        | Write title, uploader, date, etc. into the file. |
| `rate_limit`           | empty (unlimited)             | Maximum download rate per download, e.g. `500K` or `5M`. |
| `cookies_from_browser` | empty (disabled)              | Use cookies from `brave`, `chrome`, `chromium`, `edge`, `firefox`, `opera`, `safari` or `vivaldi`, for private, members-only or age-restricted videos. |

Changing settings affects downloads that start afterwards; running downloads
keep the settings they started with.

### Formats and resolutions

For video formats vidl prefers streams that already match the container, so
merging doesn't need re-encoding, and falls back to the best available
streams otherwise. For example, mp4 at 1080p uses this yt-dlp format
selector:

```
bv*[height<=1080][ext=mp4]+ba[ext=m4a]/bv*[height<=1080]+ba/b[height<=1080]
```

Audio formats download the best audio stream and convert it with ffmpeg.

## Files

| Path                          | Contents |
| ----------------------------- | -------- |
| `~/.config/vidl/config.toml`  | Settings. |
| `~/.config/vidl/queue.json`   | Queue and history, written on every status change. Safe to delete while vidl is not running to start fresh. |

Partial downloads (`*.part`, `*.ytdl`, `*.part-Frag*`) live in the download
directory until the download finishes, is cancelled or removed.

## How it works

vidl runs one `yt-dlp` process per download. It asks yt-dlp to print
progress as JSON using `--progress-template` and to print video info and the
final file path using `--print`, and parses those lines from stdout and
stderr instead of scraping human-readable output. Pausing sends `SIGINT` to
the yt-dlp process group (including ffmpeg), which leaves partial files in
place so the next run continues them. Metadata for the new-download dialog
comes from `yt-dlp -J --flat-playlist`.

## Development

```sh
make            # build into bin/vidl
make run ARGS="'https://www.youtube.com/watch?v=jNQXAC9IVRw'"
make test       # unit tests
make race       # tests with the race detector
make check      # gofmt check, go vet and race tests; run before committing
make help       # list all targets
```

| Target      | Description |
| ----------- | ----------- |
| `build`     | Build `bin/vidl` with the version from `git describe`. |
| `run`       | Build and run, passing `ARGS`. |
| `install`   | `go install` into `$(go env GOPATH)/bin`. |
| `uninstall` | Remove the installed binary. |
| `test`      | Run tests. |
| `race`      | Run tests with the race detector. |
| `cover`     | Tests with a per-function coverage report. |
| `vet`       | `go vet`. |
| `fmt`       | Format code with gofmt. |
| `fmt-check` | Fail if any file isn't gofmt-formatted. |
| `lint`      | Run `golangci-lint` if it's installed. |
| `tidy`      | `go mod tidy`. |
| `check`     | `fmt-check`, `vet` and `race`. |
| `dist`      | Cross-compile for macOS and Linux (arm64, amd64) into `dist/`. |
| `clean`     | Remove `bin/`, `dist/` and coverage output. |

Override the embedded version with `make build VERSION=1.0.0`.

To try vidl without touching your real config and downloads, point it at a
scratch config directory:

```sh
mkdir -p /tmp/vidl-dev/vidl
printf 'download_dir = "/tmp/vidl-dev/downloads"\n' > /tmp/vidl-dev/vidl/config.toml
XDG_CONFIG_HOME=/tmp/vidl-dev ./bin/vidl
```

### Project layout

```
cmd/vidl/          entry point: dependency checks, config loading, startup
internal/config/   settings: defaults, TOML load/save, validation
internal/queue/    download manager: statuses, scheduling, persistence
internal/ytdlp/    yt-dlp integration: arguments, progress parsing, probing
internal/ui/       Bubble Tea models and views
```

The `ytdlp` and `queue` packages don't depend on the UI. Tests use recorded
yt-dlp output (`internal/ytdlp/testdata`) and fake runners, so they need
neither network access nor yt-dlp.

## Troubleshooting

**Missing formats or resolutions on YouTube.** yt-dlp needs a JavaScript
runtime such as Deno to solve YouTube's challenges. Run yt-dlp directly to see
its warnings:

```sh
yt-dlp -F 'https://www.youtube.com/watch?v=jNQXAC9IVRw'
```

If it reports `n challenge solving failed`, install or fix the runtime it
names (see the [yt-dlp EJS wiki](https://github.com/yt-dlp/yt-dlp/wiki/EJS)).
For example, Deno fails if `~/.npmrc` is a directory instead of a file.

**Downloads fail with HTTP 403 or "Sign in to confirm".** Update yt-dlp
(`yt-dlp -U` or `brew upgrade yt-dlp`). For private, members-only or
age-restricted videos, set `cookies_from_browser` to a browser where you're
logged in.

**`[yt-dlp ffmpeg] not found on PATH`.** Install the missing tool (see
[Requirements](#requirements)) and check `which yt-dlp ffmpeg`.

**A download is stuck or behaves oddly.** Run the same URL with yt-dlp
directly to see whether the problem is the site or vidl.

## Roadmap

- [x] yt-dlp integration: progress parsing, probing, interruptible downloads
- [x] Download queue: concurrency, auto-start, pause/resume, cancel, retry,
      reorder, remove, persistence
- [x] Configuration file
- [x] UI scaffold: tabs, navigation, help
- [x] Downloads and Finished tables with progress bars, speed and ETA
- [x] Detail panel (`enter`)
- [ ] New-download dialog with format and resolution selection, playlist
      expansion and multi-URL paste
- [ ] Settings tab
- [ ] Open file, clear history, global speed footer, search

## License

No license has been chosen yet.
