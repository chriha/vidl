# Harness config

Read by the `/harness:*` skills and briefs (see the plugin's CONVENTIONS.md). Owned by this project — edit freely.
To change a whole brief/adapter instead, copy it to `.claude/harness/<same path>` (it replaces the plugin file).

## Tracker
tracker: github
forge: github
<!-- labels/priority scheme differing from the adapter's defaults go here -->

## Commands
test: `make check`   <!-- gofmt check + go vet + race tests; docs/PLAN.md requires it before every commit -->
static: `make vet`
format: `make fmt-check`   <!-- must print nothing; fix only changed files: `gofmt -w <files>` -->
build: `go build ./...`
setup-worktree:    <!-- none; bin/ is gitignored and built per worktree -->
test-affected: `go test -count=1 {packages}`

## Scripts
test_file_glob: `**/*_test.go`
red_check_test_cmd: `go test -count=1`   <!-- red-check appends the packages of the changed test files -->
exclude_from_size: `:(exclude,glob)**/*_test.go` `:!go.sum`

## Branch & commits
branch types: fix|feature|refactor|chore
branch: `<type>/<ticket>-<slug>`, lowercase, ≤ 60 chars after the type
commit types: feat|fix|docs|style|refactor|perf|test|chore|ci
scope: optional, one word
max subject length: 72
ticket id in commit subject: no
Co-Authored-By: Claude <noreply@anthropic.com>

## Plan-first triggers
Extend the generic gate in `briefs/implementer.md`. Plan first when any of these hold:
- blast radius: an exported function with many callers, a change to a core interface, a change in the Bubble Tea message flow
- hard to undo: config schema (`config.toml`), on-disk queue/history format (`queue.json`), a documented CLI flag or key binding, the yt-dlp argument/progress contract
- multiple real designs (new package vs existing, sync vs background work)
- needs sequencing (several PRs, deprecation period)
- data loss involved (deleting partial/downloaded files, clearing history)

## Self-check
Before every commit — none of:
- invented functions or methods (verify they exist)
- swallowed errors or `_ =` added to silence one (surface them in the UI footer or as a toast)
- a helper duplicating an existing one (grep first)
- a goroutine without a way to stop it; a data race on shared state (go through the owning loop/messages)
- UI imports in `internal/ytdlp` or `internal/queue` (UI talks to the manager only through its public methods and snapshots)
- shelling out with an unquoted/unvalidated string
- `//nolint` or skipped tests to make a check pass
- `go get` / `go mod tidy` / hand-edited `go.mod`/`go.sum` (a dependency change is an open question)
- behaviour change without updating `README.md` (keybinding table, roadmap)

## Test conventions
- Table-driven; goal-first (specify behavior, edge cases, error paths), one behavior per test, assert behavior not internals.
- Deterministic and independent: `t.TempDir()`, `t.Setenv`, `t.Helper()`; no `time.Sleep`, real clock, network, real yt-dlp, or the user's real config dir without a fake; no package-level shared mutable state or order dependence.
- UI state transitions: call `Update` with key messages on a model backed by a `queue.Manager` with a fake runner and `MemStore`; assert on model state, not rendered strings.
- Manual end-to-end runs use an isolated `XDG_CONFIG_HOME` and download dir (see docs/PLAN.md § Verifying changes) — never the user's real config or downloads.
- Unit vs integration level should fit the behavior under test; cover error paths and boundary values when the diff adds logic that warrants them.

## Never
- Change dependencies without approval.
- Add `//nolint`, skip tests, or loosen a check to get green.
- Run `gofmt` over the whole tree in a feature diff — format only the files you changed.
