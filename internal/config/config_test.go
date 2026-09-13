package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/focus-cli/focus/internal/focus"
)

func TestLoadDefault(t *testing.T) {
	t.Setenv("FOCUS_DATA_DIR", "")
	t.Setenv("FOCUS_DEFAULT_MINUTES", "")
	t.Setenv("XDG_DATA_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantDir := filepath.Join(home, ".local", "share", "focus")
	if cfg.DataDir != wantDir {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, wantDir)
	}
	if cfg.DBPath != filepath.Join(wantDir, "focus.db") {
		t.Fatalf("DBPath = %q", cfg.DBPath)
	}
	if cfg.DefaultLength != focus.DefaultLength {
		t.Fatalf("DefaultLength = %v, want %v", cfg.DefaultLength, focus.DefaultLength)
	}
}

func TestLoadXDGDataHome(t *testing.T) {
	base := t.TempDir()
	t.Setenv("FOCUS_DATA_DIR", "")
	t.Setenv("FOCUS_DEFAULT_MINUTES", "")
	t.Setenv("XDG_DATA_HOME", base)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DataDir != filepath.Join(base, "focus") {
		t.Fatalf("DataDir = %q, want XDG-based dir", cfg.DataDir)
	}
}

func TestLoadDataDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FOCUS_DATA_DIR", dir)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DataDir != dir || cfg.DBPath != filepath.Join(dir, "focus.db") {
		t.Fatalf("override not respected: %+v", cfg)
	}
}

func TestLoadCreatesDir0700(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "newdir", "focus")
	t.Setenv("FOCUS_DATA_DIR", dir)
	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("data dir not created: %v", err)
	}
	if !fi.IsDir() {
		t.Fatalf("%q is not a dir", dir)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Fatalf("data dir perm = %o, want 700", perm)
	}
}

func TestLoadDefaultMinutesOverride(t *testing.T) {
	t.Setenv("FOCUS_DATA_DIR", t.TempDir())
	t.Setenv("FOCUS_DEFAULT_MINUTES", "50")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultLength != 50*time.Minute {
		t.Fatalf("DefaultLength = %v, want 50m", cfg.DefaultLength)
	}
}

func TestLoadDefaultMinutesGarbage(t *testing.T) {
	t.Setenv("FOCUS_DATA_DIR", t.TempDir())
	for _, raw := range []string{"abc", "0", "-5", "2.5", ""} {
		if raw == "" {
			continue // empty means unset → default, covered elsewhere
		}
		t.Setenv("FOCUS_DEFAULT_MINUTES", raw)
		if _, err := Load(); err == nil {
			t.Fatalf("FOCUS_DEFAULT_MINUTES=%q: want error, got nil", raw)
		}
	}
}
