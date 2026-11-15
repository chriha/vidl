# Roadmap

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
- [x] Hold the queue (`P`): running downloads finish, nothing new starts
- [x] Stop and start the whole queue (`S`), with a held/stopped badge
- [x] Estimated size for downloads that haven't started
- [x] Add downloads queued, paused or started right away from the dialog
- [x] `vidl add --paused` to add downloads paused
- [x] Wait a set time between downloads (`W`), with a countdown badge
- [x] Warn before re-adding a video that's already downloaded or queued
