package main

import "runtime/debug"

// resolveVersion returns v unless it's the "dev" default, in which case it
// falls back to the module version recorded by `go install module@version`.
func resolveVersion(v string, info *debug.BuildInfo, ok bool) string {
	if v != "dev" || !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return v
	}
	return info.Main.Version
}

// installHint tells the user how to install the missing tools on goos.
func installHint(goos string) string {
	switch goos {
	case "darwin":
		return "install with: brew install yt-dlp ffmpeg"
	case "windows":
		return "install with: winget install yt-dlp.yt-dlp Gyan.FFmpeg"
	default:
		return "install them with your package manager (see the README)"
	}
}
