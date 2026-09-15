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

# 7-day activity heatmap (rolling window incl. today)
focus heatmap
```

While the full-screen timer runs:

- `p` — pause / resume (paused time is held, not counted)
- `s` — finish; prompts for an accomplishment and a next step
- `b` — take a break: opens a 5-minute break editor (up/down step the
  selected HH/MM field, left/right move between fields, enter/`s` start the
  break, `esc`/`q` cancel). The work session stays paused while the break
  counts down. Breaks are saved in history and excluded from stats.
- `q` — abandon; press `q` twice to confirm (the partial session is kept in
  history but excluded from stats)

Piped or redirected output (and `--no-tui`) runs a plain headless countdown
instead, which auto-finishes when time is up:

```sh
focus start "quick" 1 --no-tui
```

## Pomodoro

`focus pomodoro` runs the classic rotation on a task: 25-minute work blocks,
5-minute short breaks, and a 15-minute long break after every 4 work blocks.

```sh
focus pomodoro "write report"              # rotate until you quit
focus pomodoro "write report" --blocks 4   # stop after 4 work blocks
```

Each work block runs the full-screen timer (phase label and block `n of N`
shown, e.g. `Work 2 of 4`) and is saved as a normal session, so `history`
and `stats` keep working. Breaks are rest time: nothing is saved, and `s`
skips to the next work block. Abandoning a work block (`q` twice) or
quitting a break (`q` twice) ends the rotation.

Durations default to the classics and are configurable two ways — flags win
over environment:

```sh
focus pomodoro "write report" --work 50 --short-break 10 --long-break 30 --every 6
FOCUS_POMODORO_WORK_MINUTES=50 FOCUS_POMODORO_SHORT_BREAK_MINUTES=10 \
  FOCUS_POMODORO_LONG_BREAK_MINUTES=30 FOCUS_POMODORO_BLOCKS_BEFORE_LONG=6 \
  focus pomodoro "write report"
```

Piped output (and `--no-tui`) runs plain headless countdowns instead, one
per phase with a `=== Work 1 of 4 (25m) ===` header.

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
- `FOCUS_BREAK_SECONDS` — override the break default length (positive
  integer seconds, default `300` = 5 minutes); useful for quick trials
- `FOCUS_POMODORO_WORK_MINUTES`, `FOCUS_POMODORO_SHORT_BREAK_MINUTES`,
  `FOCUS_POMODORO_LONG_BREAK_MINUTES`, `FOCUS_POMODORO_BLOCKS_BEFORE_LONG` —
  pomodoro rotation (positive integers, defaults `25` / `5` / `15` / `4`);
  the matching `pomodoro` flags (`--work`, `--short-break`, `--long-break`,
  `--every`) win over these when passed

## Design facts

- Pause-correct timing: the engine (`internal/focus`) owns pause math, so
  paused intervals never count toward focused time.
- Abandoned sessions persist for honest history but are excluded from stats.
- Timestamps are stored UTC; `stats` buckets "today" by local midnight.

## Repo layout

- `cmd/focus` — thin entry point; commands wire up in `internal/cli`.
- `internal/cli` — cobra command parsing (`start`, `history`, `stats`, `heatmap`) and
  exit-code mapping.
- `internal/focus` — session state machine (engine owns pause math).
- `internal/storage` — SQLite store, schema migrations, today's-stats query.
- `internal/config` — data-dir resolution and env overrides.
- `internal/tui` — Bubble Tea countdown timer plus history/stats/heatmap renders.

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
