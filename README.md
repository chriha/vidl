# vidl

A terminal UI for downloading videos, built with Go and
[Bubble Tea](https://github.com/charmbracelet/bubbletea) on top of
[yt-dlp](https://github.com/yt-dlp/yt-dlp).

vidl manages a download queue for you: paste a URL, pick a format and
resolution, and it downloads in the background with progress, speed and ETA
for every item. The queue and history survive restarts, and interrupted
downloads resume where they stopped.

> **Status:** feature complete for a first release; everything ticked on
> the [Roadmap](#roadmap) works. The UI is tested manually on macOS.

## Features

- **New-download dialog**: paste a URL (prefilled from the clipboard), see
  its title, uploader, duration and approximate size, pick a format and
  resolution, and queue it. Per download you can also download only a clip
  (start and end time), use a different filename template, turn embedded
  subtitles on or off, or keep sponsor segments. Playlists queue every entry; pasting several URLs at
  once queues them all with the default settings.
- **Download queue** with a configurable number of concurrent downloads.
  Queued items start automatically, top to bottom.
- **Per-download progress**: percentage, speed and ETA, combined across the
  separate video and audio streams yt-dlp fetches before merging.
- **Pause, resume, cancel and retry.** Pausing keeps partial files so the
  download resumes; `p` on a paused download puts it back in the queue to
  wait for a free slot, while `s` resumes it at once. Cancelling deletes
  partial files. `R` on a finished download downloads it again and
  replaces the file.
- **Reordering**: move waiting downloads up and down the queue.
- **Finished history** of completed downloads: open a file or show it in
  its folder, and clear the history.
- **Search** to filter downloads and history by title or URL, and a status
  filter for the queue.
- **Status line** with the total speed and the number of downloads per
  status, and the overall progress in the terminal's window title.
- **Detail panel** for any download: source, format, clip, template and
  subtitle overrides, progress per stream, timestamps, output and partial
  files, errors, and yt-dlp's latest output
  for running or failed downloads.
- **Error view** (`i`) for a failed download: the full error and yt-dlp's
  output, full-screen and scrollable. Failed downloads keep their last 200
  output lines across restarts.
- **Desktop notifications** when a download finishes or fails. Downloads
  ending within about two seconds of each other share one notification.
- **`vidl add`** hands URLs to the running vidl from scripts or browser
  hooks. Only one vidl runs at a time; with none running, the URLs wait in the
  saved queue for the next start.
- **Persistence**: queue and history are saved on every change; downloads
  running when you quit are resumed on the next start.
- **Output formats**: mp4, mkv, webm (video) and mp3, m4a, opus (audio only),
  with a maximum resolution from 360p up to 2160p or best available.
- **yt-dlp options** exposed as settings: filename template, embedded
  subtitles, thumbnails and metadata, rate limit, cookies from your
  browser for private or age-restricted videos, and
  [SponsorBlock](https://sponsor.ajay.app/) to cut sponsor segments and
  other categories out of videos.
- **Vim-style navigation.**

## Requirements

- [yt-dlp](https://github.com/yt-dlp/yt-dlp) on your `PATH`, ideally a recent
  version (sites change often; update with `yt-dlp -U` or your package
  manager).
- [ffmpeg](https://ffmpeg.org) on your `PATH`, used to merge video and audio
  streams, extract audio and embed thumbnails/subtitles.
- [Go](https://go.dev/dl/) 1.27 or newer to build from source.
- macOS or Linux. Windows is supported but less tested.

vidl checks for yt-dlp and ffmpeg at startup and exits with an install hint
if either is missing. It also reads the yt-dlp version in the background and,
if that release is more than 60 days old, shows a toast with the update
commands. vidl never updates yt-dlp itself.

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

Windows (winget):

```sh
winget install yt-dlp.yt-dlp Gyan.FFmpeg
```

YouTube additionally needs a JavaScript runtime (such as
[Deno](https://deno.com)) for yt-dlp to see every format; see
[Troubleshooting](#troubleshooting).

## Installation

Download a binary for your platform from the
[releases page](https://github.com/chriha/vidl/releases), check it against
`checksums.txt`, make it executable and put it on your `PATH`:

```sh
shasum -a 256 -c checksums.txt --ignore-missing
chmod +x vidl-*-darwin-arm64
mv vidl-*-darwin-arm64 /usr/local/bin/vidl
```

On macOS, a downloaded binary is quarantined by Gatekeeper; allow it with
`xattr -d com.apple.quarantine /usr/local/bin/vidl`.

Or install with Go:

```sh
go install github.com/chriha/vidl/cmd/vidl@latest
```

Or build from source:

```sh
git clone https://github.com/chriha/vidl.git
cd vidl
make install        # installs into $(go env GOPATH)/bin
```

For `go install` and `make install`, make sure `$(go env GOPATH)/bin` is on
your `PATH`. Alternatively, build into the project directory with
`make build` and run `./bin/vidl`.

To uninstall, run `make uninstall` and optionally remove the config directory
(see [Files](#files)).

## Usage

```sh
vidl                        # open the UI
vidl <url> [<url>...]       # open the UI and queue the given URLs
vidl add <url> [<url>...]   # queue URLs in the running vidl
vidl --version
vidl --help
```

Quote URLs in your shell, since characters like `?` and `&` are special:

```sh
vidl 'https://www.youtube.com/watch?v=jNQXAC9IVRw'
```

Only one vidl runs at a time, since it owns the queue file. While it runs,
`vidl add <url>` hands the URLs to it over a local socket (`vidl.sock` in the
config directory), where they're queued with the default format and
resolution and a toast confirms them. `vidl <url>` does the same instead of
opening a second UI, and plain `vidl` exits with `vidl is already running`.
With no vidl running, `vidl add` appends the URLs to the saved queue and they
start the next time vidl opens:

```sh
vidl add 'https://www.youtube.com/watch?v=jNQXAC9IVRw'
# Added 1 download to the running vidl
#   or, with no vidl running:
# Queued 1 download; they start when vidl runs
```

The UI has three tabs:

- **Downloads**: running downloads at the top, followed by the queue in the
  order downloads will start.
- **Finished**: completed downloads, most recent first.
- **Settings**: download preferences, editable in place (see
  [Configuration](#configuration)).

Press `n` to add a download. If the clipboard holds a URL, it's already
filled in; otherwise paste or type one and press `enter` to fetch its details.
Move between the fields with `tab`/`shift+tab` or `↑`/`↓` (`j`/`k` outside
text fields), change the file type, resolution, subtitles and sponsors with `h`/`l`,
and type into the text fields. Then press `enter` to queue it, or `esc` to go
back and edit the URL. Resolutions above the video's best are hidden, and
audio formats ignore the resolution.

- **Clip start / end** download only part of the video. Times are
  `[[h:]m:]s`, e.g. `90`, `1:30` or `1:02:03`; leave start empty to begin at
  the start and end empty to go to the end. The end must be after the start,
  otherwise the dialog shows an error and doesn't queue. Clips are cut by
  ffmpeg, so a paused clip starts over when resumed.
- **Template** is the filename template for this download, prefilled with
  the setting; empty or unchanged follows the setting.
- **Subtitles** embeds subtitles for this download, defaulting to the
  setting.
- **Sponsors** removes the `sponsorblock_remove` categories from this
  download (`remove`, the default) or keeps them (`keep`). It's only shown
  when that setting is set.

These choices stay with the download across pause, retry and restarts, and
the detail panel lists the ones that differ from the settings. For a
playlist, `enter` queues every entry with the chosen settings; the clip fields
are hidden there. Pasting several URLs at once offers to queue them all with the
default format and resolution, without fetching details first. While the
dialog is open, other keys are disabled; `ctrl+c` still quits.

Press `/` to search the current tab: the list narrows to downloads whose
title or URL contains what you type, ignoring case. `enter` keeps the filter
and returns to the list, `esc` clears it. Actions apply to the filtered list,
and `X` on a filtered Finished tab removes only the matching downloads from
the history.

On the Downloads tab, `f` cycles a status filter: all, downloading, queued,
paused, failed, cancelled, and back to all. It combines with the search (a
download must match both), and the line above the table shows it, e.g.
`/ foo  [failed]  3 of 12`. `esc` clears the search and the status filter
together.

On the Downloads and Finished tabs, `space` marks or unmarks the selected
download and moves to the next one; `V` marks every visible download (the
filters apply) or, when they're all marked already, unmarks them. Marked rows
show a `▌` at their left edge and the status line counts them, e.g.
`3 marked`. Marks are kept per tab, while a filter hides them, and drop out
when a download leaves the tab. `esc` clears the marks after closing the
detail panel and before clearing the filters.

While downloads are marked, `s`, `p`, `c`, `R` and `r` act on all marked
downloads (including those a filter hides) instead of the selected one.
Downloads whose state doesn't allow the action are skipped, and a message
sums up the result, e.g. `paused 3 · skipped 1`; if any of them fails, the
message counts the failures and shows the first error. `r` asks once for all
marked downloads. On the Finished tab only `r` uses the marks; `u`, `d`,
`enter`, `o`, `O` and `b` always act on the selected download.

On the Finished tab, `o` opens a file with its default application and `O`
shows it in Finder (macOS), selects it in Explorer (Windows) or opens its
folder (Linux). Removing downloads
from the history never deletes their files.

`b` opens the selected download's source URL in the default browser, on the
Downloads and Finished tabs and for downloads in any state. Only `http` and
`https` URLs are opened, and marks are ignored.

The line above the help shows the total download speed and how many downloads
are running, queued, paused, failed or cancelled. Short messages and errors
appear in the bottom right corner for a few seconds. While downloading, the
terminal's window title (and, in terminals that support it, its progress
indicator) shows the overall progress. vidl needs at least 40×8 cells; on
narrower terminals the table drops the size, ETA, speed and status columns,
in that order.

Quitting stops running downloads and keeps their partial files; they continue
automatically the next time you start vidl.

## Keybindings

| Key                   | Action                                              |
| --------------------- | --------------------------------------------------- |
| `j` / `↓`             | Move down                                           |
| `k` / `↑`             | Move up                                             |
| `g` / `G`             | Jump to top / bottom                                |
| `ctrl+d` / `ctrl+u`   | Half page down / up                                 |
| `tab` / `l`           | Next tab                                            |
| `shift+tab` / `h`     | Previous tab                                        |
| `1` `2` `3`           | Go to Downloads / Finished / Settings               |
| `s`                   | Start or resume; retries failed or cancelled        |
| `p`                   | Pause, or put a paused download back in the queue   |
| `c`                   | Cancel (deletes partial files)                      |
| `R`                   | Retry failed or cancelled, or re-download finished  |
| `u` / `d`             | Move queued download up / down                      |
| `r`                   | Remove (asks for confirmation)                      |
| `enter`               | Toggle the detail panel                             |
| `i`                   | Show why the download failed (`esc`/`i`/`q` close)  |
| `space`               | Mark or unmark the download (actions apply to all)  |
| `V`                   | Mark all visible downloads, or unmark them all      |
| `esc`                 | Close the detail panel, then clear marks, filters   |
| `/`                   | Search the current tab by title or URL              |
| `f`                   | Cycle the status filter (Downloads tab)             |
| `o`                   | Open the file (Finished tab)                        |
| `O`                   | Show the file in its folder (Finished tab)          |
| `b`                   | Open the source URL in the browser                  |
| `X`                   | Clear finished history (asks for confirmation)      |
| `n`                   | New download                                        |
| `?`                   | Toggle full help                                    |
| `q` / `ctrl+c`        | Quit                                                |

On the Settings tab, `j`/`k` move between settings, `h`/`l` (or `←`/`→`)
change the concurrency and cycle through choices, `space` or `enter` toggles
on/off settings, and `enter` edits text settings (`enter` saves, `esc`
cancels). `tab`/`shift+tab` and `1` `2` `3` still switch tabs.

## Configuration

Settings are stored in `~/.config/vidl/config.toml` (or
`$XDG_CONFIG_HOME/vidl/config.toml`). The file is created with the defaults
on first start. Change settings on the Settings tab, which saves every change
immediately, or edit the file while vidl is not running (vidl overwrites it
when you change a setting). Invalid values fall back to their defaults.

The Settings tab checks values as you enter them: the download folder must be
an absolute path or start with `~`, and vidl offers to create it if it doesn't
exist; the rate limit must be a number with an optional `K`, `M` or `G`
suffix; SponsorBlock categories must be a comma-separated list of known
categories, or `all`.

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
notify = true
sponsorblock_remove = ""
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
| `notify`               | `true`                        | Desktop notification when a download finishes or fails. |
| `sponsorblock_remove`  | empty (disabled)              | [SponsorBlock](https://sponsor.ajay.app/) categories to cut out of videos, comma-separated: `sponsor`, `intro`, `outro`, `selfpromo`, `preview`, `filler`, `interaction`, `music_offtopic`, or `all`. Cutting re-encodes around the cuts, so post-processing takes longer. |

Changing settings affects downloads that start afterwards; running downloads
keep the settings they started with. `notify` applies at once.

### Notifications

vidl notifies through `osascript` on macOS, `notify-send` on Linux (in the
`libnotify-bin` or `libnotify` package) and PowerShell's built-in toast API on Windows.
If the notifier isn't installed, vidl says so once and doesn't try again
until the next start. macOS may ask you to allow notifications for your
terminal app the first time. Windows notifications are best-effort: they
appear under PowerShell's name and are only tested by hand on macOS.

### Formats and resolutions

For video formats vidl prefers streams that already match the container, so
merging doesn't need re-encoding, and falls back to the best available
streams otherwise. mp4 and mkv prefer H.264 video and AAC audio, which play
in QuickTime and other Apple players (AV1 often doesn't without hardware
decode). YouTube offers H.264 only up to 1080p, so a 1440p or 2160p request
falls back to VP9 or AV1 streams. For example, mp4 at 1080p uses this yt-dlp
format selector:

```
bv*[height<=1080][vcodec^=avc1]+ba[acodec^=mp4a]/bv*[height<=1080][ext=mp4]+ba[ext=m4a]/bv*[height<=1080]+ba/b[height<=1080]
```

Audio formats download the best audio stream and convert it with ffmpeg.

## Files

| Path                          | Contents |
| ----------------------------- | -------- |
| `~/.config/vidl/config.toml`  | Settings. |
| `~/.config/vidl/queue.json`   | Queue and history, written on every status change. Safe to delete while vidl is not running to start fresh. |
| `~/.config/vidl/queue.lock`   | Held by the running vidl so only one owns the queue; `vidl add` takes it to append while none runs. |
| `~/.config/vidl/vidl.sock`    | Socket the running vidl listens on for `vidl add`; removed on exit. If the config path is too long for a socket, it lives under the system temp directory instead. |

Partial downloads (`*.part`, `*.ytdl`, `*.part-Frag*`) live in the download
directory until the download finishes, is cancelled or removed.

## How it works

vidl runs one `yt-dlp` process per download. It asks yt-dlp to print
progress as JSON using `--progress-template` and to print video info and the
final file path using `--print`, and parses those lines from stdout and
stderr instead of scraping human-readable output. Pausing sends `SIGINT` to
the yt-dlp process group (including ffmpeg), which leaves partial files in
place so the next run continues them; on Windows it sends `CTRL_BREAK`
instead. Metadata for the new-download dialog
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
| `dist`      | Cross-compile for macOS, Linux and Windows (arm64, amd64) into `dist/`, with `checksums.txt`. |
| `clean`     | Remove `bin/`, `dist/` and coverage output. |

Override the embedded version with `make build VERSION=1.0.0`.

### Releasing

CI runs `make check` on Linux and macOS for every push and pull request,
and vets and builds the Windows target. To release, move the changes under
`Unreleased` in [CHANGELOG.md](CHANGELOG.md) to a new version section, then
tag and push:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow runs `make check` and `make dist`, and publishes the
binaries and checksums as a GitHub release with that version's changelog
section as release notes.

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

**`a running vidl holds … but does not answer on …`.** Another vidl owns the
queue but its socket is gone or unusable (`vidl add` needs AF_UNIX sockets,
which Windows has since Windows 10 version 1803). Add the URLs in that vidl,
or quit it and run the command again.

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
- [x] Error view for failed downloads (`i`)
- [x] New-download dialog with format and resolution selection, playlist
      expansion and multi-URL paste
- [x] Settings tab
- [x] Open file, clear history, search, status line, toasts, window title
      progress, small-terminal handling
- [x] Release: MIT license, CI, release binaries, `go install`, pause and
      cancel on Windows
- [x] Status filter on the Downloads tab (`f`)
- [x] Open files from the Finished tab on Windows
- [x] Re-download finished videos (`R`)
- [x] Desktop notifications for finished and failed downloads
- [x] Per-download clip range, filename template and subtitles
- [x] Mark several downloads (`space`, `V`) and start, pause, cancel, retry
      or remove them together
- [x] `vidl add` to queue URLs in a running vidl; single instance

## License

[MIT](LICENSE)
