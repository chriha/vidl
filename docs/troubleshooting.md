# Troubleshooting

**Missing formats or resolutions on YouTube.** yt-dlp needs a JavaScript
runtime such as Deno to solve YouTube's challenges. Run yt-dlp directly to see
its warnings:

```sh
yt-dlp -F 'https://www.youtube.com/watch?v=jNQXAC9IVRw'
```

If it reports `n challenge solving failed`, install or fix the runtime it
names (see the [yt-dlp EJS wiki](https://github.com/yt-dlp/yt-dlp/wiki/EJS)).
For example, Deno fails if `~/.npmrc` is a directory instead of a file.

**Downloads fail with HTTP 403.** Update yt-dlp (`yt-dlp -U` or
`brew upgrade yt-dlp`). For private, members-only or age-restricted videos,
set `cookies_from_browser` to a browser where you're logged in.

**"Sign in to confirm you're not a bot".** YouTube flags the IP address or a
request without a PO token. Without using cookies:

- Turn off any VPN or switch networks; data-center and shared IPs get flagged
  most.
- Install a PO token provider such as
  [bgutil-ytdlp-pot-provider](https://github.com/Brainicism/bgutil-ytdlp-pot-provider)
  (a local server plus a yt-dlp plugin in `~/.config/yt-dlp/plugins`).
- Try other YouTube clients by adding this to `~/.config/yt-dlp/config`:

  ```
  --extractor-args "youtube:player_client=default,tv,web_safari"
  ```

yt-dlp still reads its own config file and plugins when vidl runs it, so
these apply without changing vidl. See the
[yt-dlp PO token guide](https://github.com/yt-dlp/yt-dlp/wiki/PO-Token-Guide).

If the error stays with all of these in place, YouTube blocks the IP address
itself. Residential IPs get flagged too after many requests in a short time,
and vidl adds a size lookup per queued download on top of the downloads. Get
a new IP (with a dynamic address, reconnecting or restarting the router
usually does it; compare `curl -s ipinfo.io/ip` before and after) or wait a
few hours. Afterwards, lower `concurrency` and turn on `wait_enabled` for a
while so the new IP isn't flagged too.

As a last resort, set `cookies_from_browser` to a browser where you're
logged in to YouTube, preferably with a spare account, since YouTube may flag
the account instead. yt-dlp reads the browser's default profile. On macOS,
allow the Keychain prompt for the browser's "Safe Storage" key, or reading
Chrome-based browsers' cookies fails. Downloads that already failed keep
their error until you retry them with `R`.

**`[yt-dlp ffmpeg] not found on PATH`.** Install the missing tool (see
[Requirements](../README.md#requirements)) and check `which yt-dlp ffmpeg`.

**`a running vidl holds … but does not answer on …`.** Another vidl owns the
queue but its socket is gone or unusable (`vidl add` needs AF_UNIX sockets,
which Windows has since Windows 10 version 1803). Add the URLs in that vidl,
or quit it and run the command again.

**A download is stuck or behaves oddly.** Run the same URL with yt-dlp
directly to see whether the problem is the site or vidl.
