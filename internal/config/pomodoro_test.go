package config

import (
	"testing"
	"time"
)

func TestLoadPomodoroDefaults(t *testing.T) {
	t.Setenv("FOCUS_DATA_DIR", t.TempDir())
	t.Setenv("FOCUS_POMODORO_WORK_MINUTES", "")
	t.Setenv("FOCUS_POMODORO_SHORT_BREAK_MINUTES", "")
	t.Setenv("FOCUS_POMODORO_LONG_BREAK_MINUTES", "")
	t.Setenv("FOCUS_POMODORO_BLOCKS_BEFORE_LONG", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PomodoroWork != 25*time.Minute {
		t.Fatalf("PomodoroWork = %v, want 25m", cfg.PomodoroWork)
	}
	if cfg.PomodoroShortBreak != 5*time.Minute {
		t.Fatalf("PomodoroShortBreak = %v, want 5m", cfg.PomodoroShortBreak)
	}
	if cfg.PomodoroLongBreak != 15*time.Minute {
		t.Fatalf("PomodoroLongBreak = %v, want 15m", cfg.PomodoroLongBreak)
	}
	if cfg.PomodoroBlocksBeforeLong != 4 {
		t.Fatalf("PomodoroBlocksBeforeLong = %d, want 4", cfg.PomodoroBlocksBeforeLong)
	}
}

func TestLoadPomodoroOverrides(t *testing.T) {
	t.Setenv("FOCUS_DATA_DIR", t.TempDir())
	t.Setenv("FOCUS_POMODORO_WORK_MINUTES", "50")
	t.Setenv("FOCUS_POMODORO_SHORT_BREAK_MINUTES", "10")
	t.Setenv("FOCUS_POMODORO_LONG_BREAK_MINUTES", "30")
	t.Setenv("FOCUS_POMODORO_BLOCKS_BEFORE_LONG", "6")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PomodoroWork != 50*time.Minute {
		t.Fatalf("PomodoroWork = %v, want 50m", cfg.PomodoroWork)
	}
	if cfg.PomodoroShortBreak != 10*time.Minute {
		t.Fatalf("PomodoroShortBreak = %v, want 10m", cfg.PomodoroShortBreak)
	}
	if cfg.PomodoroLongBreak != 30*time.Minute {
		t.Fatalf("PomodoroLongBreak = %v, want 30m", cfg.PomodoroLongBreak)
	}
	if cfg.PomodoroBlocksBeforeLong != 6 {
		t.Fatalf("PomodoroBlocksBeforeLong = %d, want 6", cfg.PomodoroBlocksBeforeLong)
	}
}

func TestLoadPomodoroGarbage(t *testing.T) {
	t.Setenv("FOCUS_DATA_DIR", t.TempDir())
	vars := []string{
		"FOCUS_POMODORO_WORK_MINUTES",
		"FOCUS_POMODORO_SHORT_BREAK_MINUTES",
		"FOCUS_POMODORO_LONG_BREAK_MINUTES",
		"FOCUS_POMODORO_BLOCKS_BEFORE_LONG",
	}
	for _, v := range vars {
		for _, raw := range []string{"abc", "0", "-5", "2.5"} {
			t.Setenv(v, raw)
			if _, err := Load(); err == nil {
				t.Fatalf("%s=%q: want error, got nil", v, raw)
			}
		}
		t.Setenv(v, "")
	}
}
