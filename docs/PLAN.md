# vidl implementation plan

Handoff document for continuing development. Milestones 1–6 are done;
this file describes what was decided, what exists, what was learned the hard
way, and what remains (milestones 7–8) in enough detail to proceed without
the original conversation.

Read `README.md` first for the user-facing picture, then this file.

## Product requirements

The user's original request, a TUI on top of yt-dlp:

- **Downloads tab**: a table of running and queued downloads, top to bottom:
  running downloads at the top, the last queued item at the bottom.
- Every download shows a **progress bar, ETA and speed**.
- **Finished tab** listing finished downloads.
- **Vim navigation.**
- **Detail panel**: pressing `enter` on any item (queued, downloading,
  finished) opens a panel at the bottom with details about the download.
- **Shortcuts**: cancel `c`, start `s`, remove `r` (needs confirmation), new
  download `n`, search `/`, move up `u`, move down `d`.
- **New-download dialog**: paste a URL, press `enter` to fetch and show
  details, change file type and resolution, press `enter` again to queue it.
- **Settings**: download folder, number of concurrent downloads, default
  resolution, default file type.

Decisions the user made when asked:

| Question | Answer |
| --- | --- |
| Extras for v1 | All four: pause/resume + retry, playlist support, open file/folder, extra settings (filename template, subtitles, thumbnail/metadata embedding, rate limit, cookies from browser). |
| yt-dlp/ffmpeg | Require on `PATH`, check at startup with an install hint. No auto-download. |
| Persistence | Persist everything; interrupted downloads resume as queued on restart. |
| Auto-start | Yes, up to the concurrency limit. `s` resumes paused/failed items or force-starts out of order. |

Further decisions made in planning that the user accepted:

- `u`/`d` reorder the queue; paging uses `ctrl+u`/`ctrl+d` (vim's convention)
  so bare `u`/`d` stay free.
- `p` pauses, `s` starts/resumes, `c` cancels (deletes partial files),
  `R` retries, `o` opens a finished file, `X` clears finished history,
  `?` toggles full help, `1`/`2`/`3` and `tab`/`shift+tab` (also `l`/`h`)
  switch tabs.
- Clipboard auto-detect: opening `n` prefills the URL field when the
  clipboard holds a URL.
- Batch add: pasting several URLs (one per line) queues them all with default
  settings.
- Global footer with total speed and active/queued counts.
- Settings are TOML, state is JSON, both in `~/.config/vidl/`.
- Unit tests for parsing, state machine and format selection; the UI is
  verified manually (see [Verifying changes](#verifying-changes)).

## Environment

- macOS (darwin/arm64), zsh, Go **1.27.3**, yt-dlp 2026.10.21, ffmpeg and
  tmux installed via Homebrew.
- Module path is `vidl` (no remote yet), branch `main`.
- The user's `~/.npmrc` is a directory, which breaks yt-dlp's Deno-based
  YouTube challenge solver (warning: `n challenge solving failed`). Downloads
  still work but some formats may be missing. Not a vidl bug; the user was
  told.

## Dependencies

Bubble Tea **v2** (the code was migrated from v1 when the user upgraded Go):

| Module | Version | Notes |
| --- | --- | --- |
| `charm.land/bubbletea/v2` | v2.1.0 | `View()` returns `tea.View`; keys are `tea.KeyPressMsg`; alt screen via `v.AltScreen = true`, not a program option. |
| `charm.land/bubbles/v2` | v2.2.1 | `help.Model` uses `SetWidth`; check other components with `go doc` before use, as v2 changed many fields to setters. |
| `charm.land/lipgloss/v2` | v2.0.6 | `lipgloss.Color(s)` returns `color.Color`. |
| `github.com/charmbracelet/x/ansi` | | `ansi.Truncate` for ANSI-aware truncation. |
| `github.com/BurntSushi/toml` | v1.6.0 | Config. |

Use `go doc charm.land/bubbles/v2/<pkg>` to confirm APIs instead of relying
on v1 knowledge. Useful v2 features to consider: `tea.PasteMsg` (bracketed
paste, for multi-URL paste), `View.WindowTitle`, `View.ProgressBar`
(terminal-native progress, OSC 9;4), clipboard read/write commands.

## Architecture

```
cmd/vidl/main.go       flags, PATH check, config load (writes defaults on
                       first run), queue.New, CLI URLs → Add, run program,
                       mgr.Close() on exit
internal/ytdlp/        no UI or queue knowledge
  format.go            Format type, Formats, Resolutions, FormatArgs
  run.go               Job, Job.Args(), Runner.Run
  progress.go          Event types, Tracker (line → Event, multi-stream %)
  probe.go             Runner.Probe, ParseInfo, Info/Entry
  proc_unix.go         process group + SIGINT on cancel
  proc_windows.go      no-op fallback
internal/queue/        no UI knowledge
  item.go              Status, Item
  manager.go           Manager: scheduling, actions, events → items
  store.go             State, Store, FileStore (atomic JSON), MemStore
internal/config/       Config, Default, Load, Save, Normalize, Dir,
                       DownloadPath, ExpandHome, Browsers
internal/ui/           Bubble Tea root model
  model.go, keys.go, format.go (humanBytes/humanSpeed/humanDuration)
  table.go, views.go   Downloads/Finished tables
  detail.go            detail panel
  dialog.go            new-download dialog, Prober interface
```

### `ytdlp` package

- `Job.Args()` always passes `--newline --progress --no-simulate
  --no-playlist --color never`, two `--progress-template`s (download,
  postprocess) and two `--print`s (`before_dl` info JSON, `after_move` final
  path), `-P <dir>`, then format args and optional flags, and the URL after
  `--`.
- All injected lines carry a prefix (`vidl-dl:`, `vidl-pp:`, `vidl-info:`,
  `vidl-file:`); everything else is an `EventLog` line.
- `Tracker` computes overall percent as `(finishedStreams + currentFraction)
  / streams`, where `streams` is the number of `+`-separated parts in the
  actual `format_id` from the `before_dl` print. Post-processing events keep
  100% and set `Progress.Stage` ("merging", "extracting audio", ...).
- `Event.Files` lists `filename`/`tmpfilename` only for `status ==
  "downloading"` events, so a pre-existing file ("already downloaded") is
  never recorded and never deleted on cancel.
- `Runner.Run` returns `context.Cause(ctx)` when cancelled, otherwise the last
  `ERROR: ` line from yt-dlp as the error.
- `Runner.Probe(ctx, url, cookiesFromBrowser)` runs `yt-dlp -J
  --flat-playlist`. `Info.Heights` lists available video heights, highest
  first. `Info.IsPlaylist` with `Info.Entries` for playlists (entries have
  `URL`, `Title`, `Duration`, often no formats).

### `queue` package

Manager API (all safe for concurrent use; reads return copies):

| Method | Behaviour |
| --- | --- |
| `New(cfg, runner, store)` | Loads state; `Downloading` items become `Queued`; schedules. |
| `Changes() <-chan struct{}` | Coalesced change notifications (buffer 1). |
| `Snapshot() (queue, history []Item)` | Queue in display order (running first, then queue order); history newest first. |
| `Get(id)` | Copy of one item from queue or history. |
| `Config()` / `SetConfig(cfg)` | Normalizes, reschedules (higher concurrency starts more immediately). |
| `Add(...NewItem) []string` | Appends; empty format/resolution fall back to config defaults. |
| `Start(id)` | Force-starts (ignores the limit), also retries Failed/Cancelled/Paused. |
| `Pause(id)` | Running: interrupt, keep part files → `Paused`. Queued: → `Paused`. |
| `Cancel(id)` | Interrupt and delete part files → `Cancelled`. |
| `Remove(id)` | From queue or history; running ones are stopped and part files deleted; finished files are kept. |
| `Move(id, delta)` | Reorders waiting items only; never across running items. |
| `ClearHistory()` | Empties history. |
| `SaveError()` | Last persistence error, shown in the UI footer. |
| `Close()` | Stops running downloads as `Queued`, waits, saves. |

Status flow: `Queued → Downloading → Finished | Paused | Failed | Cancelled`;
`Start` moves any non-finished status back to `Downloading`. Finished items
move from `queue` to the front of `history`.

Invariants worth keeping:

- File I/O (deleting part files) happens **outside** the manager lock, right
  after the status change. Tests must wait for it, not assume it.
- The final status is set by the worker goroutine after yt-dlp exits, not by
  `Pause`/`Cancel` themselves (those only cancel the context with a cause).
- A run that completed successfully wins over a racing stop request.
- State is saved on structural changes, not on every progress event.
- `Item.Log` (last 200 output lines) is in memory only (`json:"-"`).

### UI

`ui.Model` holds `*queue.Manager`, a `keyMap`, `help.Model`, the current
tab, window size, the latest snapshot, and a cursor, selected id and scroll
offset per tab. It waits on `mgr.Changes()` via a `tea.Cmd` that returns
`changedMsg`, refreshes the snapshot and re-arms. `table.go`/`views.go` render
the Downloads and Finished tables; `detail.go` renders the detail panel
(`enter`/`esc`) below the table, with the log in a `bubbles/v2/viewport`.
`Update` wraps `update` so the viewport is synced after every message. Errors
from actions show above the help footer.

`dialog.go` is the new-download dialog (`n`), a `*newDialog` overlaid on the
body with the same compositor as the remove confirmation. While it's open
`update` routes every message except `WindowSizeMsg`/`changedMsg` to
`handleDialogMsg`, so global keys are off. Steps: input → probing → details,
plus a batch step for multi-URL pastes. `ui.New(mgr, prober)` takes a
`Prober` (`ytdlp.Runner` in `main`, a fake in tests); `Model.clipboard` reads
the clipboard via `atotto/clipboard` (already a dependency of
`bubbles/textinput`) and is replaced in tests.

## Lessons learned (don't rediscover these)

- **yt-dlp writes postprocess progress to stderr** and download progress to
  stdout. Both are parsed; order between the two streams is not
  deterministic, so tests must not assume interleaving.
- **With `-P`, yt-dlp reports absolute paths** in progress and `--print`.
  `Run` still joins relative paths with `job.Dir` as a fallback.
- `--print` implies quiet/simulate; `--progress` and `--no-simulate` are
  required to keep downloading and progress output.
- `%(filepath)j` prints a JSON string (safe for odd characters).
  `%(.{a,b,c})j` prints a JSON object of selected fields.
- Video and audio are separate streams with their own 0→100% progress;
  that's why `Tracker` exists.
- Part files: `*.part`, `*.ytdl`, and `*.part-Frag*` for fragmented (HLS/DASH)
  downloads. Titles contain glob metacharacters (`[id]`), so globs are
  escaped (`globEscape`).
- Concurrent downloads start in parallel goroutines: test helpers accept
  starts in any order.
- **zsh**: always quote URLs in shell commands (`?` is a glob and the command
  fails with `no matches found`).
- `make fmt-check` catches gofmt drift; run `make check` before committing.
- **Tables**: `bubbles/v2/progress` pulls in `harmonica` (a `go.mod` change),
  so `ui/table.go` draws a static bar itself. `help.Model` can overshoot its
  width by one item, so the footer is truncated with `ansi.Truncate`.
- **Detail panel** (`ui/detail.go`): `detailHeight()` is 0 whenever the
  panel is closed, has no item or doesn't fit, and both `tableRows()` and
  `detailView()` rely on that, so check it rather than `m.detail`. The log
  viewport is synced in `Update` (`syncLog`), never in `View`, and always
  jumps to the bottom. Terminals narrower than the tab bar (~31 columns)
  still overflow; that's milestone 8.
- **New-download dialog**: probe results carry a request id (`probeSeq`) and
  are ignored unless they match the dialog's current probe, so a cancelled
  probe (`esc`) that returns late can't overwrite a newer one. Bubble Tea v2's
  OSC52 clipboard read needs terminal support, so the dialog shells out via
  `atotto/clipboard` (`pbpaste`, `wl-paste`, `xclip`) and ignores errors.
  "Me at the zoo" tops out at 240p, below every standard resolution, so its
  resolution selector only offers `best`; that's intended. In UI tests, don't
  execute the textinput's `Focus` cmd (cursor blink sleeps ~0.5 s); use
  `findMsg[T]` to pull a specific message out of a batch.
- In zsh, `T="tmux -L vidl-test"; $T ...` fails (no word splitting); use a
  function `T() { tmux -L vidl-test "$@"; }`. Multi-URL paste can be tested
  with `tmux load-buffer -` + `paste-buffer -p` (bracketed paste).
- **Manual checks in tmux**: use a private server (`tmux -L vidl-test`) and
  never `kill-session` on the default one; the developer may be running the
  agent inside a tmux session with the same name.

## Conventions

- Match the existing style: short doc comments on exported identifiers,
  comments only where the code isn't self-explanatory, small focused files.
- Keep `ytdlp` and `queue` free of UI imports; UI talks to the manager only
  through its public methods and snapshots.
- Never swallow errors silently; surface them in the UI footer or as a
  toast.
- Commit once per milestone (or meaningful sub-step), after `make check`
  passes, with a descriptive message and the attribution line your session
  specifies. Ask before pushing (there's no remote yet).
- Update `README.md` when behaviour changes: flip the keybinding table's
  "planned" entries to ✅ and tick the roadmap.

## Remaining milestones

Suggested order: 4 → 5 → 6 → 7 → 8 (4, 5 and 6 are done). Each should be usable on its own.

### Milestone 4: Downloads and Finished tables

Replace `listView` with real tables.

- Custom row renderer rather than `bubbles/table`: cells need styled content
  (progress bars, colored statuses) and the table component truncates by
  rune width, which breaks ANSI sequences. Render rows with lipgloss widths
  and `ansi.Truncate`.
- **Downloads columns**: `#` (queue position), Title (flexible width, takes
  the remaining space), Status (colored: downloading blue, queued gray, paused
  yellow, failed red, cancelled dim), Progress (a `bubbles/v2/progress` bar
  rendered statically with `ViewAs(percent)` after `SetWidth`, without
  animation, plus `42%`), Speed, ETA, Size (`Progress.Total` when known). While
  post-processing, show `Progress.Stage` (e.g. "merging") instead of
  speed/ETA. For multi-stream downloads show the stream, e.g. `2/2`, in the
  detail panel, not the table.
- **Finished columns**: Title, Format, Resolution, Size, Finished (relative
  or local time), File name. Size needs a new `Item.Size int64` set in
  `Manager.run` on success via `os.Stat(it.FilePath)`, so it's persisted.
- Header row, cursor row highlight, scrolling that keeps the cursor visible,
  `ctrl+d`/`ctrl+u` half-page, `g`/`G`.
- Responsive: drop Size, then ETA, then Speed columns on narrow terminals;
  never wrap rows.
- New keys: `u`/`d` → `mgr.Move(id, -1/+1)` and keep the cursor on the
  moved item (find it by id after refresh); `r` → confirmation modal (`y`
  confirms, `n`/`esc` cancels) → `mgr.Remove`; `R` → `mgr.Start` on failed or
  cancelled items. Keep the cursor on the same item id across refreshes
  (items move when they start or finish), falling back to the same index.
- Empty states: "No downloads. Press n to add one."
- Update the help bindings: short help shows the most used keys per tab;
  full help shows all.

### Milestone 5: Detail panel

- `enter` toggles a panel below the table (about a third of the height,
  minimum 8 lines, the table shrinks). It follows the cursor.
- Fields: title, URL, uploader, extractor, duration, format + resolution,
  status, progress (percent, downloaded/total of the current stream, stream
  `n/m`, stage), speed, ETA, added/started/finished times, output path, part
  files, error (red, wrapped).
- For running or failed items, show the tail of `Item.Log` in a
  `bubbles/v2/viewport` that sticks to the bottom; `Log` isn't persisted, so
  say "no output (from a previous session)" when empty.
- `esc` closes the panel.

### Milestone 6: New-download dialog

A modal overlay; while it's open it receives all keys (global keys
disabled except `ctrl+c`).

1. **Input**: a `bubbles/v2/textinput` for the URL. Prefill from the
   clipboard if it holds an `http(s)://` URL (`pbpaste` on macOS,
   `wl-paste`/`xclip -o` on Linux via `exec`, or Bubble Tea v2's clipboard
   read; ignore errors). If a paste (`tea.PasteMsg`) contains several URLs
   (split on whitespace/newlines, keep `http(s)://` ones), show "Add N
   URLs with defaults? (enter/esc)" and queue them via `mgr.Add` without
   probing (titles fill in from the `before_dl` info print when they start).
2. **Probing**: `enter` runs `Runner.Probe` in a `tea.Cmd` with a cancelable
   context (cookies from `mgr.Config().CookiesFromBrowser`), with a
   `bubbles/v2/spinner`. Tag each probe with a request id so stale results
   after `esc` or a new URL are ignored. `esc` cancels the probe. Errors are
   shown inline and return to the input step.
3. **Details**: title, uploader, duration, extractor, approximate size.
   Two selectors, focus moved with `tab`/`j`/`k`, values changed with
   `h`/`l` or `←`/`→`:
   - Type: `ytdlp.Formats` (mp4, mkv, webm, mp3, m4a, opus), defaulting to
     `cfg.DefaultFormat`.
   - Resolution: `best` plus the standard values from `ytdlp.Resolutions`
     that are ≤ the highest of `Info.Heights` (or all values if heights are
     unknown, e.g. for playlists), defaulting to `cfg.DefaultResolution` or
     the closest lower value. Disabled (shown dim) for audio formats.
4. **Playlists**: show the playlist title, entry count and the first entries;
   `enter` queues all entries (`NewItem` with entry URL, title, duration and
   the chosen type/resolution).
5. `enter` on the details step queues via `mgr.Add(queue.NewItem{...})` with
   the probed metadata, closes the dialog and moves the cursor to the new
   item.

Inject the prober into the UI (`ui.New(mgr, prober)`; milestone 7 adds `cfgPath`) with a small
`interface{ Probe(ctx, url, cookies string) (*ytdlp.Info, error) }`) so the
dialog can be tested with a fake.

### Milestone 7: Settings tab

- A form listing every `config.Config` field with its current value; `j`/`k`
  moves between fields.
- Editing by field type: strings (download dir, filename template, subtitle
  langs, rate limit) via inline `textinput` on `enter`, confirmed with
  `enter` or cancelled with `esc`; concurrency with `h`/`l` (−/+, 1–10); enums
  (default format, default resolution, cookies browser from
  `config.Browsers`) cycle with `h`/`l`; booleans toggle with `space`/`enter`.
- Every change applies immediately: `cfg.Normalize()`, `config.Save(cfgPath,
  cfg)`, `mgr.SetConfig(cfg)`. Show save errors. Validate the download dir
  (expand `~`, offer to create it if missing) and the rate limit format
  (`^\d+(\.\d+)?[KMG]?$`).
- A note under the form: "Applies to downloads started from now on."
- Show the config file path.

### Milestone 8: Polish

- `o` on a finished item opens the file (`open` on macOS, `xdg-open` on
  Linux); `O` reveals it in the file manager (`open -R` on macOS; the parent
  directory with `xdg-open` on Linux). Report a missing file.
- `X` on the Finished tab clears history after confirmation.
- `/` search: a filter input above the table; filters the current tab by a
  case-insensitive substring of the title or URL; `enter` keeps the filter,
  `esc` clears it. Actions apply to the filtered selection.
- Global footer/status line: total speed of running downloads, counts
  (`2 downloading · 5 queued · 1 failed`), and save errors.
- Toasts for transient messages and errors (auto-hide after a few seconds
  with `tea.Tick`) instead of the sticky error line.
- Window title with aggregate progress, and optionally `View.ProgressBar`.
- Narrow and short terminal handling: hide the detail panel or columns
  gracefully, minimum size message.
- README: final keybinding table, screenshots/GIF (e.g. with
  [vhs](https://github.com/charmbracelet/vhs)), and roadmap ticked.

## Verifying changes

- `make check` (gofmt, vet, race tests) must pass.
- Add unit tests for non-UI logic, and for UI state transitions where
  practical (call `Update` with key messages on a model backed by a
  `queue.Manager` with a fake runner and `MemStore`, and assert on the
  model's state, not rendered strings).
- Manual end-to-end check in tmux with an isolated config, so the user's real
  config and downloads are untouched:

  ```sh
  S=$(mktemp -d)
  mkdir -p "$S/xdg/vidl" "$S/dl"
  printf 'download_dir = "%s"\nconcurrency = 1\ndefault_resolution = "360"\n' "$S/dl" > "$S/xdg/vidl/config.toml"
  make build
  tmux new-session -d -s vidl -x 120 -y 30 \
    "XDG_CONFIG_HOME='$S/xdg' ./bin/vidl 'https://www.youtube.com/watch?v=jNQXAC9IVRw' 'https://example.invalid/nope'"
  sleep 5; tmux capture-pane -pt vidl     # inspect the screen
  tmux send-keys -t vidl 2                # drive it with keys
  tmux send-keys -t vidl q; tmux kill-session -t vidl
  cat "$S/xdg/vidl/queue.json"            # inspect persisted state
  ```

  "Me at the zoo" (`jNQXAC9IVRw`, 19 s) is the short test video used so far;
  `https://example.invalid/nope` exercises the failure path.
