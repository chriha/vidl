# Configuration

Settings are stored in `~/.config/vidl/config.toml` (or
`$XDG_CONFIG_HOME/vidl/config.toml`). The file is created with the defaults
on first start. Change settings on the Settings tab, which saves every change
immediately, or edit the file while vidl is not running (vidl overwrites it
when you change a setting). Invalid values fall back to their defaults.

The Settings tab checks values as you enter them: the download folder must be
an absolute path or start with `~`, and vidl offers to create it if it doesn't
exist; the rate limit must be a number with an optional `K`, `M` or `G`
suffix; SponsorBlock categories must be a comma-separated list of known
categories, or `all`; the wait duration must be a duration such as `30s`, `2m`
or `1m30s`, from 1 second to 1 hour.

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
wait_enabled = false
wait_duration = "30s"
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
| `wait_enabled`         | `false`                       | Wait `wait_duration` after a download ends before the next queued one starts. |
| `wait_duration`        | `30s`                         | How long to wait between downloads, as a duration such as `45s`, `2m` or `1m30s` (1s–1h). |

Changing settings affects downloads that start afterwards; running downloads
keep the settings they started with. `notify` applies at once.

## Notifications

vidl notifies through `osascript` on macOS, `notify-send` on Linux (in the
`libnotify-bin` or `libnotify` package) and PowerShell's built-in toast API on Windows.
If the notifier isn't installed, vidl says so once and doesn't try again
until the next start. macOS may ask you to allow notifications for your
terminal app the first time. Windows notifications are best-effort: they
appear under PowerShell's name and are only tested by hand on macOS.

## Formats and resolutions

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
| `~/.config/vidl/vidl.sock`    | Socket the running vidl listens on for `vidl add`; removed on exit. If the config path is too long for a socket, it lives under the system temp directory instead (vidl refuses that directory unless you own it and its mode is 0700). |

Partial downloads (`*.part`, `*.ytdl`, `*.part-Frag*`) live in the download
directory until the download finishes, is cancelled or removed.
