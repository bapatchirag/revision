package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	def := Default()
	if def.LogLimit <= 0 {
		t.Errorf("Default().LogLimit = %d, want positive", def.LogLimit)
	}
	if def.Theme != "auto" {
		t.Errorf("Default().Theme = %q, want %q", def.Theme, "auto")
	}
}

func TestDirUsesXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir(): unexpected error %v", err)
	}
	want := filepath.Join(xdg, "revision")
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestDirFallsBackToHomeConfig(t *testing.T) {
	// An empty XDG_CONFIG_HOME is treated as unset, so Dir must resolve to
	// ~/.config/revision — the same location on both macOS and Linux.
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir(): unexpected error %v", err)
	}
	want := filepath.Join(home, ".config", "revision")
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestPathEndsWithConfigJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path(): unexpected error %v", err)
	}
	want := filepath.Join(home, ".config", "revision", "config.json")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom(missing): unexpected error %v", err)
	}
	if got != Default() {
		t.Errorf("loadFrom(missing) = %+v, want Default() %+v", got, Default())
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Config{
		DefaultPath: "/work/trunk",
		LogLimit:    50,
		Editor:      "vim",
		Theme:       "solarized",
	}

	if err := saveTo(path, want); err != nil {
		t.Fatalf("saveTo: unexpected error %v", err)
	}
	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: unexpected error %v", err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestLoadFillsDefaultsForAbsentKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// Only one key is present; every other field must come from Default().
	if err := os.WriteFile(path, []byte(`{"editor":"nano"}`), 0o644); err != nil {
		t.Fatalf("write partial config: %v", err)
	}

	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: unexpected error %v", err)
	}
	if got.Editor != "nano" {
		t.Errorf("Editor = %q, want %q", got.Editor, "nano")
	}
	if got.LogLimit != Default().LogLimit {
		t.Errorf("LogLimit = %d, want default %d", got.LogLimit, Default().LogLimit)
	}
	if got.Theme != Default().Theme {
		t.Errorf("Theme = %q, want default %q", got.Theme, Default().Theme)
	}
}

func TestLoadNormalizesInvalidValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"logLimit":0,"theme":"  "}`), 0o644); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: unexpected error %v", err)
	}
	if got.LogLimit != Default().LogLimit {
		t.Errorf("LogLimit = %d, want normalized to default %d", got.LogLimit, Default().LogLimit)
	}
	if got.Theme != Default().Theme {
		t.Errorf("Theme = %q, want normalized to default %q", got.Theme, Default().Theme)
	}
}

func TestLoadInvalidJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	got, err := loadFrom(path)
	if err == nil {
		t.Fatal("loadFrom(malformed): expected an error, got nil")
	}
	if got != Default() {
		t.Errorf("loadFrom(malformed) = %+v, want Default() %+v", got, Default())
	}
}

func TestSaveCreatesNestedDirAndLeavesNoTempFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "revision")
	path := filepath.Join(dir, "config.json")

	if err := saveTo(path, Default()); err != nil {
		t.Fatalf("saveTo: unexpected error %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read config dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("config dir contents = %v, want exactly [config.json] (no temp leftovers)", names)
	}
}

func TestSaveWritesIndentedJSONWithTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := saveTo(path, Default()); err != nil {
		t.Fatalf("saveTo: unexpected error %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Error("written config should end with a trailing newline")
	}
	// Two-space indentation is the marker of MarshalIndent output, which keeps
	// the on-disk file human-editable.
	if !bytes.Contains(data, []byte("\n  \"")) {
		t.Error("written config should be indented (human-editable)")
	}
}
