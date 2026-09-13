// Command focus is the terminal focus operating system: a countdown timer
// with accomplishment/next prompts persisted to local SQLite, plus history
// and stats views. Thin entry point: config+storage+commands wire up in
// internal/cli.
package main

import (
	"os"

	"github.com/focus-cli/focus/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
