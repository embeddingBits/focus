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

## Maintaining this file

Keep this file for knowledge useful to almost every future agent session in this project.
Do not repeat what the codebase already shows; point to the authoritative file or command instead.
Prefer rewriting or pruning existing entries over appending new ones.
When updating this file, preserve this bar for all agents and keep entries concise.
