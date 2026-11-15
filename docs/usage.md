# Usage

```sh
vidl                        # open the UI
vidl <url> [<url>...]       # open the UI and queue the given URLs
vidl add <url> [<url>...]   # queue URLs in the running vidl
vidl add --paused <url>...  # add them paused, to start later
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
resolution and a toast confirms them. URLs already downloaded or still in
the queue are skipped, and the toast and the `Added …` line count only the
downloads actually added (e.g. `Added 2 downloads from vidl add · skipped 1
already downloaded or queued`). `vidl <url>` does the same instead of
opening a second UI, and plain `vidl` exits with `vidl is already running`.
With no vidl running, `vidl add` appends the URLs to the saved queue and they
start the next time vidl opens; they aren't checked for duplicates then.
URLs passed to `vidl <url>` when it starts the UI are checked against the
saved queue and history, and a toast names how many were skipped:

```sh
vidl add 'https://www.youtube.com/watch?v=jNQXAC9IVRw'
# Added 1 download to the running vidl
#   or, with no vidl running:
# Queued 1 download; they start when vidl runs
```

`vidl add --paused <url>...` adds the downloads paused instead, in the
running vidl or in the saved queue: they wait until you start them with `s`
or queue them with `p`. The flag can stand anywhere among the URLs; any other
argument starting with `-` is a usage error.

The UI has three tabs:

- **Downloads**: running downloads at the top, followed by the queue in the
  order downloads will start. The tab label counts the listed downloads per
  status, e.g. `Downloads (2/5/1)`: downloading, queued, paused, failed and
  cancelled in that order, each in its status colour, zero counts left out.
- **Finished**: completed downloads, most recent first. The label shows how
  many are listed, e.g. `Finished (12)`, in green.
- **Settings**: download preferences, editable in place (see
  [Configuration](configuration.md)).

Press `n` to add a download. If the clipboard holds a URL, it's already
filled in; otherwise paste or type one and press `enter` to fetch its details.
Move between the fields with `tab`/`shift+tab` or `↑`/`↓` (`j`/`k` outside
text fields), change the file type, resolution, subtitles, sponsors and status with `h`/`l`,
and type into the text fields. Then press `enter` to queue it, or `esc` to go
back and edit the URL. Resolutions above the video's best are hidden, and
audio formats ignore the resolution.

If the video is already downloaded, or waiting, running or paused in the
queue, the dialog says so first (`Already downloaded` or `Already in the
queue`) with its title, status, date and file: `enter` adds it anyway and
continues to the fields, `esc` goes back to the URL. Videos match on
yt-dlp's video id, or on the exact URL when the id isn't known yet. Failed
and cancelled downloads don't count.

- **Clip start / end** download only part of the video. Times are
  `[[h:]m:]s`, e.g. `90`, `1:30` or `1:02:03`; leave start empty to begin at
  the start and end empty to go to the end. The end must be after the start,
  otherwise the dialog shows an error and doesn't queue. Clips are cut by
  ffmpeg, so a paused clip starts over when resumed. Progress is read from
  ffmpeg's output time, so it needs the video's length when the end is empty.
- **Template** is the filename template for this download, prefilled with
  the setting; empty or unchanged follows the setting.
- **Subtitles** embeds subtitles for this download, defaulting to the
  setting.
- **Sponsors** removes the `sponsorblock_remove` categories from this
  download (`remove`, the default) or keeps them (`keep`). It's only shown
  when that setting is set.
- **Status** is how the download is added: `Queued` (the default) starts once
  a slot is free, `Paused` waits until you start it with `s` or queue it with
  `p`, and `Start now` starts it right away like `s`, even past the
  concurrency limit or while the queue is held. Playlists offer only `Queued`
  and `Paused`.

These choices stay with the download across pause, retry and restarts, and
the detail panel lists the ones that differ from the settings. For a
playlist, the dialog first lists the entries, all checked: `j`/`k` (or `↓`/`↑`)
move, `g`/`G` jump to the first/last entry, `ctrl+d`/`ctrl+u` move half a page,
`space` toggles the entry and moves down, and `a` checks all (or unchecks all
when everything is checked). `enter` continues to the settings above (at least
one entry must be checked) and queues only the checked entries in playlist
order; `esc` goes back a step with the selection kept. Entries already
downloaded or in the queue start unchecked and are marked `already
downloaded` or `in queue`; check them to add them anyway. The clip fields are
hidden for playlists. Pasting several URLs at once offers to queue them all with the
default format and resolution, without fetching details first; `h`/`l`/`space`
switch them between `Queued` and `Paused`. URLs already downloaded or in the
queue are marked and skipped, and the header counts them; if every URL is a
duplicate, the dialog shows an error. To add one anyway, add it on its own.
While the
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

The Downloads tab's Size column shows the size yt-dlp reports once a download
runs. Before that, vidl asks yt-dlp in the background (two at a time) how large
each queued download will be with its format and resolution, and shows the
estimate dimmed, e.g. `~412 MB`; for a clip it's scaled to the clip's length.
The same lookup fills in a missing title, uploader and duration. If it fails,
the size stays `–` and the reason is in the detail panel's log; it isn't
retried until vidl starts again.

The line above the help shows the total download speed and how many downloads
are running, queued, paused, failed or cancelled. Short messages and errors
appear in the bottom right corner for a few seconds. While downloading, the
terminal's window title (and, in terminals that support it, its progress
indicator) shows the overall progress. vidl needs at least 40×8 cells; on
narrower terminals the table drops the size, ETA, speed and status columns,
in that order.

Quitting stops running downloads and keeps their partial files; they continue
automatically the next time you start vidl. While a download is running, `q`
and `ctrl+c` first ask for confirmation (`y` quits, `n`/`esc` stays); a second
`ctrl+c` quits immediately.
