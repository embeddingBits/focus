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
	// Pomodoro rotation: work/short-break/long-break lengths and how many
	// work blocks form one set (long break cadence).
	PomodoroWork             time.Duration
	PomodoroShortBreak       time.Duration
	PomodoroLongBreak        time.Duration
	PomodoroBlocksBeforeLong int
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

	pomWork, err := positiveMinutes("FOCUS_POMODORO_WORK_MINUTES", focus.DefaultPomodoroWork)
	if err != nil {
		return Config{}, err
	}
	pomShort, err := positiveMinutes("FOCUS_POMODORO_SHORT_BREAK_MINUTES", focus.DefaultPomodoroShortBreak)
	if err != nil {
		return Config{}, err
	}
	pomLong, err := positiveMinutes("FOCUS_POMODORO_LONG_BREAK_MINUTES", focus.DefaultPomodoroLongBreak)
	if err != nil {
		return Config{}, err
	}
	pomEvery, err := positiveInt("FOCUS_POMODORO_BLOCKS_BEFORE_LONG", focus.DefaultBlocksBeforeLong)
	if err != nil {
		return Config{}, err
	}

	return Config{
		DataDir:                  dataDir,
		DBPath:                   filepath.Join(dataDir, "focus.db"),
		DefaultLength:            defLen,
		PomodoroWork:             pomWork,
		PomodoroShortBreak:       pomShort,
		PomodoroLongBreak:        pomLong,
		PomodoroBlocksBeforeLong: pomEvery,
	}, nil
}

// positiveMinutes resolves an env override holding positive integer minutes;
// empty means def. Garbage or non-positive values are usage errors.
func positiveMinutes(name string, def time.Duration) (time.Duration, error) {
	n, err := positiveInt(name, 0)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return def, nil
	}
	return time.Duration(n) * time.Minute, nil
}

// positiveInt resolves an env override holding a positive integer; empty
// means def (which may itself be 0 to signal "unset").
func positiveInt(name string, def int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid %s %q: must be a positive integer", name, raw)
	}
	return n, nil
}
