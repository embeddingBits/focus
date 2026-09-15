# Project agent memory

This file is the project's committed home for project-intrinsic agent knowledge: build, test, release, architecture, and sharp-edge notes that should travel with the code.

- Add durable project-specific notes here as they are discovered through real work.

## focus Phase 1

- Module `github.com/focus-cli/focus`, `go 1.24`; deps: `modernc.org/sqlite`
  (pure Go, no cgo), `github.com/spf13/cobra`. Keep `go mod tidy` clean:
  it prunes unused deps, so re-run it after adding imports (cobra must stay
  at exactly v1.10.1; modernc v1.46.x for the `go 1.24` line).
- Gates: `go build -o /tmp/focus ./cmd/focus` (+ `CGO_ENABLED=0` build),
  `go vet ./...`, `gofmt -l .` clean, `go test ./...` green.
- Frozen contract (workstream A owns, B consumes read-only):
  `internal/focus/session.go` (state machine), `internal/focus/store.go`
  (`SessionRecord` + `Store`), `internal/tui` §10 API (`RunTimer`,
  `RenderHistory`, `RenderStats`). See plan §§5-8 in firstmate data dir.
- File ownership: A owns everything except `internal/tui/*`; B owns
  `internal/tui/*` (currently temporary stubs in `internal/tui/stub.go` —
  delete at integration). Neither stream changes the contract unilaterally.
- DB: `$XDG_DATA_HOME/focus/focus.db` (`FOCUS_DATA_DIR` override), UTC
  RFC3339 timestamps, `StatsToday` uses Go-computed local-midnight bounds.
- Manual check: `FOCUS_DATA_DIR=$(mktemp -d) go run ./cmd/focus start "quick" 1`
  (non-TTY runs a headless countdown; exit codes 0 ok / 2 usage / 1 runtime).

## focus break mode (plain `start` sessions)

- Break editor lives in `internal/tui/timer.go` (`viewBreak`): `b` during a
  `focus start` session opens a fresh 5-minute break (adjustable hh:mm:ss,
  up/down step selected HH/MM/SS field by 1h/1m/1s, left/right move between
  the three fields, clamp no-wrap). Editor stays frozen until enter/`s`
  starts the countdown; a second enter/`s` ends it early, `esc`/`q` cancels.
  Work session auto-pauses underneath; whole break-view wall time folds into
  `pausedTotal` on return, while persisted `Taken` spans timer start→end only.
- Persistence: `internal/cli/root.go` `breakPersistHook` writes
  `Kind='break'` rows via `SessionRecord.Kind` (`internal/focus/store.go`).
  `StatsToday` excludes breaks (`kind='focus'` only); `history` shows a
  `break` mark. `FOCUS_BREAK_SECONDS` overrides the 5-minute default.
- Pomodoro breaks do NOT use the break editor; they run as regular timer
  sessions with the phase label as the task (no `PhaseLabel`/`Break` fields
  in `TimerRequest` since the break join).

## focus pomodoro

- Rotation model: `internal/focus/pomodoro.go` (`PomodoroCycle`, clock-injected
  like `Session`); config in `internal/config` (`FOCUS_POMODORO_*_MINUTES` +
  `FOCUS_POMODORO_BLOCKS_BEFORE_LONG`, classic 25m/5m/15m/4); command in
  `internal/cli/pomodoro.go` (`focus pomodoro "<task>" [--blocks N,
  --work/--short-break/--long-break/--every, --no-tui]`).
- Each work block completes as a normal session row, so history/stats need
  no changes.
- Quick rotation trial: 1-minute env overrides + `--blocks 3 --no-tui`
  (work→short→work→long→work, ~5 min), then `history`/`stats` on the same
  `FOCUS_DATA_DIR`.

## focus heatmap

- Weekday × hour productivity grid: `internal/tui/heatmap.go`
  (`ProductivityGrid`, clock-injected like `Session`; sessions split
  proportionally across overlapped local hours, pause time spread evenly),
  command in `internal/cli/heatmap.go` (`focus heatmap [--days N]`,
  default 30). Breaks/abandoned excluded like stats; GitHub-style single
  □/■ cells on a wide pitch, 4 green shades by quartile of the hottest
  slot. Additive only — frozen Store/TUI contract untouched.

## Maintaining this file

Keep this file for knowledge useful to almost every future agent session in this project.
Do not repeat what the codebase already shows; point to the authoritative file or command instead.
Prefer rewriting or pruning existing entries over appending new ones.
When updating this file, preserve this bar for all agents and keep entries concise.

<!-- graft:start -->
## Graft — repo context graph

This repo is indexed in `graft/`: small linked markdown nodes that explain each
system and carry exact file:line spans, kept in sync with the code through git.

For ANY task here — understanding how something works, finding where code lives,
or scoping a change — get context from the graph before grepping or opening
source files. Re-ask freely (it's cheap) and reuse literal identifiers you
already have (symbol, error string, file name) as the query. New to this repo?
Run `graft map` first — a token-budgeted orientation (dir clusters, hubs,
hotspots), no LLM, no key.

- Run `graft ask "<your question>" --source` → ranked nodes with the relevant
  code spans inlined (each hit's ≤8-line crux by default; `--full` for whole
  definitions when the crux isn't enough). Match the tool to the task shape:
  for understanding or editing, the top node IS the answer — cite its
  `covers:` file:line spans and edit straight from `--source`. For
  exhaustive tasks ("every occurrence / every caller of this pattern"), ranked
  results are top-N, not complete — run `graft grep "<literal>"` instead
  (exhaustive over indexed files, grouped by enclosing symbol), falling back
  to raw `grep -rn` only for unindexed files.
- `graft skeleton <file>` → every definition's signature + span, ~10× cheaper
  than reading the file; use it to skim an API surface.
- `graft callers <symbol>` gives precomputed, exact edges — who calls this.
  Add `--direction out` for what it calls, or `--depth N` to walk
  transitively for the full blast radius. For structural questions, skip
  ranking and use this directly.
- Or browse: `graft/INDEX.md` lists every node; follow the links.
- Monorepos and folders of multiple repos rank fairly across sub-projects —
  hits carry `[scope/]` labels naming which one they're from. Narrow with
  `graft ask "<task>" --in <scope>/` once you know where you're working.

If a returned span is truncated ("+N more lines"), open the file at that exact
range before finalizing. Only open source files when a node genuinely lacks a
needed detail, and then at the exact file:line the node points to — never
re-read whole files.

After big code changes, refresh the graph with `graft build` (deterministic,
no API key, $0).
<!-- graft:end -->
