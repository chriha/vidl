# Development

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
| `dist`      | Cross-compile plain binaries for macOS, Linux and Windows (arm64, amd64) into `dist/`, with `checksums.txt`. |
| `snapshot`  | Build a local GoReleaser snapshot (archives and Homebrew formula) into `dist/` without publishing. |
| `release`   | Tag and push the next release (see below). |
| `clean`     | Remove `bin/`, `dist/` and coverage output. |

Override the embedded version with `make build VERSION=1.0.0`.

## Releasing

CI runs `make check` on Linux and macOS for every push and pull request,
and vets and builds the Windows target. `make release` tags the next release
from the [Conventional Commits](https://www.conventionalcommits.org) since
the last `v*` tag: a `feat` bumps the minor version, anything else the patch;
the first release is `v1.0.0`. It checks that you are on `main` with a clean
tree, in sync with `origin/main`, that the tag doesn't exist yet, that the CI
workflow succeeded on HEAD and that the `HOMEBREW_TAP_TOKEN` secret exists
(needs `gh`). It then shows the version and the commits and asks before it
creates and pushes an annotated tag.

The pushed tag runs the release workflow: `make check`, then GoReleaser
builds the archives for macOS, Linux and Windows (amd64, arm64) with
`checksums.txt`, publishes the GitHub release with a changelog grouped into
features and fixes, and pushes the formula to the Homebrew tap
[chriha/homebrew-vidl](https://github.com/chriha/homebrew-vidl).

| Variable    | Effect |
| ----------- | ------ |
| `DRY_RUN=1` | show the version and commits, create no tag |
| `MAJOR=1`   | allow a major bump (a `type!:` commit or `BREAKING CHANGE:` footer); refused otherwise, as v2+ needs a `/v2` module path for `go install` to see it |
| `V=vX.Y.Z`  | tag this version instead; must be greater than the last release |
| `SKIP_CI=1` | skip the CI and `HOMEBREW_TAP_TOKEN` checks |

One-time setup: create a fine-grained personal access token with
*Contents: read and write* on the tap repository only, and store it as the
secret of this repository: `gh secret set HOMEBREW_TAP_TOKEN`.

To try vidl without touching your real config and downloads, point it at a
scratch config directory:

```sh
mkdir -p /tmp/vidl-dev/vidl
printf 'download_dir = "/tmp/vidl-dev/downloads"\n' > /tmp/vidl-dev/vidl/config.toml
XDG_CONFIG_HOME=/tmp/vidl-dev ./bin/vidl
```

## Project layout

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
