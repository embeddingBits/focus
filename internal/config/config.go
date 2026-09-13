// Package config resolves the data directory, database path, and default
// session length, with environment overrides.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/focus-cli/focus/internal/focus"
)

// Config holds resolved filesystem locations and defaults.
type Config struct {
	DataDir       string // $XDG_DATA_HOME/focus or ~/.local/share/focus
	DBPath        string // DataDir/focus.db
	DefaultLength time.Duration
}

// Load resolves the config, creating DataDir (0700) on demand.
// Overrides: FOCUS_DATA_DIR replaces the data dir; FOCUS_DEFAULT_MINUTES
// replaces the default session length (positive integer minutes).
func Load() (Config, error) {
	dataDir := os.Getenv("FOCUS_DATA_DIR")
	if dataDir == "" {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return Config{}, fmt.Errorf("resolve home dir: %w", err)
			}
			base = filepath.Join(home, ".local", "share")
		}
		dataDir = filepath.Join(base, "focus")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return Config{}, fmt.Errorf("create data dir: %w", err)
	}

	defLen := focus.DefaultLength
	if raw := os.Getenv("FOCUS_DEFAULT_MINUTES"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid FOCUS_DEFAULT_MINUTES %q: must be a positive integer", raw)
		}
		defLen = time.Duration(n) * time.Minute
	}

	return Config{
		DataDir:       dataDir,
		DBPath:        filepath.Join(dataDir, "focus.db"),
		DefaultLength: defLen,
	}, nil
}
