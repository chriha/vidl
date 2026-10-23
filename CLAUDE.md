# vidl

A Bubble Tea terminal UI for downloading videos through yt-dlp, with a persistent download queue. User docs: `README.md`;
architecture, conventions and remaining milestones: `docs/PLAN.md` (read its "Lessons learned" before touching `ytdlp` or `queue`).

## Commands
<!-- keep in sync with .claude/harness.md § Commands -->
- `make check` — gofmt check, go vet, race tests; must pass before every commit
- `make test` / `make race` — tests only
- `make build` — binary into `bin/vidl`; `make run ARGS="<url> ..."`
- `make help` — all targets

## Layout
- `cmd/vidl` — entry point, flags (`--version`, `--help`)
- `internal/ytdlp` — yt-dlp process wrapper: probing, format selection, progress parsing, interruptible runs (per-OS `proc_*.go`)
- `internal/queue` — download manager (concurrency, pause/resume/cancel/retry, reorder) and persistence (`store.go`)
- `internal/config` — TOML config under `$XDG_CONFIG_HOME/vidl`
- `internal/ui` — Bubble Tea model, key bindings, formatting

## Conventions
- Short doc comments on exported identifiers; comments only where the code isn't self-explanatory; small focused files.
- `ytdlp` and `queue` never import UI code; the UI uses the manager's public methods and snapshots only.
- Never swallow errors; surface them in the UI footer or as a toast.
- Behaviour changes update `README.md` (keybinding table, roadmap).
- Tests and manual runs never touch the real config dir or downloads (isolated `XDG_CONFIG_HOME`, fake runner, `MemStore`).

## Git
- Branches: `<type>/<id>-<slug>`; Conventional Commits; PRs closed via the tracker's closing keyword. Agents open drafts and never merge or mark ready.

## Agent workflow
Ticket pipeline via the `harness` plugin: `/harness:ticket`, `/harness:work`, `/harness:review`, `/harness:work-on-comments`.
Project config: `.claude/harness.md`; overrides: `.claude/harness/`.
