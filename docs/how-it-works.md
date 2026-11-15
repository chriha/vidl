# How it works

vidl runs one `yt-dlp` process per download. It asks yt-dlp to print
progress as JSON using `--progress-template` and to print video info and the
final file path using `--print`, and parses those lines from stdout and
stderr instead of scraping human-readable output. Pausing sends `SIGINT` to
the yt-dlp process group (including ffmpeg), which leaves partial files in
place so the next run continues them; on Windows it sends `CTRL_BREAK`
instead. Metadata for the new-download dialog
comes from `yt-dlp -J --flat-playlist`.
