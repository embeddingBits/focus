# focus

`focus` is a terminal focus timer: start a countdown on a task, pause when
interrupted, then record what you accomplished and what comes next. Phase 1
scope is a single Linux-first Go binary with local SQLite storage — one
full-screen countdown (`start`), past-session listing (`history`), and
today's totals (`stats`). No servers, no accounts, no network.

## Install

Requires Go 1.24+.

```sh
go build -o focus ./cmd/focus
```

The build is pure Go (modernc.org/sqlite, no cgo), so a static binary works
too:

```sh
CGO_ENABLED=0 go build -o focus ./cmd/focus
```

## Usage

```sh
# 25-minute session (the default) on a task
focus start "write report"

# Explicit length in minutes
focus start "write report" 50

# Past sessions (default 20, --limit N to change)
focus history
focus history --limit 5

# Today's total focused time, session count, and average
focus stats
```

While the full-screen timer runs:

- `p` — pause / resume (paused time is held, not counted)
- `s` — finish; prompts for an accomplishment and a next step
- `q` — abandon; press `q` twice to confirm (the partial session is kept in
  history but excluded from stats)

Piped or redirected output (and `--no-tui`) runs a plain headless countdown
instead, which auto-finishes when time is up:

```sh
focus start "quick" 1 --no-tui
```

Exit codes: `0` ok, `2` usage error (e.g. bad minutes or `--limit`), `1`
runtime failure.

## Data

Sessions live in SQLite at `~/.local/share/focus/focus.db`
(`$XDG_DATA_HOME/focus/focus.db` when `XDG_DATA_HOME` is set). Two
environment overrides:

- `FOCUS_DATA_DIR` — use a different data directory (e.g. a scratch dir for
  trials)
- `FOCUS_DEFAULT_MINUTES` — default session length when minutes are omitted
  (positive integer, default `25`)

## Design facts

- Pause-correct timing: the engine (`internal/focus`) owns pause math, so
  paused intervals never count toward focused time.
- Abandoned sessions persist for honest history but are excluded from stats.
- Timestamps are stored UTC; `stats` buckets "today" by local midnight.

## Repo layout

- `cmd/focus` — thin entry point; commands wire up in `internal/cli`.
- `internal/cli` — cobra command parsing (`start`, `history`, `stats`) and
  exit-code mapping.
- `internal/focus` — session state machine (engine owns pause math).
- `internal/storage` — SQLite store, schema migrations, today's-stats query.
- `internal/config` — data-dir resolution and env overrides.
- `internal/tui` — Bubble Tea countdown timer plus history/stats renders.

## Development

```sh
go test ./...
go vet ./...
gofmt -l .   # must print nothing
```

## Roadmap (direction only, no promises)

- Phase 2: richer history views (filtering, search)
- Phase 3: richer stats (weekly trends, per-task breakdowns)
- Phase 4: tags / projects per session
- Phase 5: configurable defaults and reminders
- Phase 6: import/export for the local database
