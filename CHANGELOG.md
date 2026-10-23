# Changelog

All notable changes to vidl are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0]

First release.

### Added

- Download queue with configurable concurrency, auto-start, pause/resume,
  cancel, retry, reordering and removal; queue and history persist across
  restarts and interrupted downloads resume.
- Downloads and Finished tables with progress bars, speed, ETA and size,
  combined across separate video and audio streams.
- Detail panel with metadata, per-stream progress, files, errors and
  yt-dlp's latest output.
- New-download dialog with clipboard prefill, format and resolution
  selection, playlist expansion and multi-URL paste.
- Settings tab for every config option, saved immediately.
- Search, open file / show in folder, clear history, status line, toasts,
  window title progress and small-terminal handling.
- Pause and cancel on Windows (CTRL_BREAK to the yt-dlp process group).
- Release binaries for macOS, Linux and Windows (amd64, arm64) with
  checksums; `go install github.com/chriha/vidl/cmd/vidl@latest`.
- MIT license.

[Unreleased]: https://github.com/chriha/vidl/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/chriha/vidl/releases/tag/v0.1.0
