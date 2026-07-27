package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	if !def.DirectoryDiff {
		t.Error("Default().DirectoryDiff = false, want true")
	}
	if def.SSHKeyPath != "~/.ssh/id_rsa" {
		t.Errorf("Default().SSHKeyPath = %q, want %q", def.SSHKeyPath, "~/.ssh/id_rsa")
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
		SSHKeyPath:  "/home/me/.ssh/id_ed25519",
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

func TestLoadDisablesDirectoryDiff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"directoryDiff":false}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: unexpected error %v", err)
	}
	if got.DirectoryDiff {
		t.Error("DirectoryDiff = true, want false (disabled on disk)")
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

func TestLoadDefaultsSSHKeyWhenBlank(t *testing.T) {
	// An empty or whitespace-only sshKeyPath on disk must resolve to the default
	// key location, while an explicit key is preserved verbatim.
	cases := map[string]struct {
		json string
		want string
	}{
		"empty":      {`{"sshKeyPath":""}`, Default().SSHKeyPath},
		"whitespace": {`{"sshKeyPath":"   "}`, Default().SSHKeyPath},
		"absent":     {`{}`, Default().SSHKeyPath},
		"explicit":   {`{"sshKeyPath":"/home/me/.ssh/work_ed25519"}`, "/home/me/.ssh/work_ed25519"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tc.json), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			got, err := loadFrom(path)
			if err != nil {
				t.Fatalf("loadFrom: unexpected error %v", err)
			}
			if got.SSHKeyPath != tc.want {
				t.Errorf("SSHKeyPath = %q, want %q", got.SSHKeyPath, tc.want)
			}
		})
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

func TestReconcileCreatesDefaultFileWhenMissing(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	path := filepath.Join(xdg, "revision", "config.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("precondition: config should not exist yet, stat err = %v", err)
	}

	got, rec, err := Reconcile(nil)
	if err != nil {
		t.Fatalf("Reconcile: unexpected error %v", err)
	}
	if got != Default() {
		t.Errorf("Reconcile() = %+v, want Default() %+v", got, Default())
	}
	if !rec.Created || rec.Updated || len(rec.Conflicts) != 0 {
		t.Errorf("Reconciliation = %+v, want Created only", rec)
	}

	// The first run must persist the defaults so the user has a file to edit.
	onDisk, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom after Reconcile: %v", err)
	}
	if onDisk != Default() {
		t.Errorf("persisted config = %+v, want Default() %+v", onDisk, Default())
	}
}

func TestReconcileMergesNewKeysIntoExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// A file written by an older build that predates most keys; only editor is
	// set. Reconcile must fill in the keys the newer schema adds and rewrite the
	// file, while preserving the value the user chose.
	if err := os.WriteFile(path, []byte(`{"editor":"nano"}`), 0o644); err != nil {
		t.Fatalf("write sparse config: %v", err)
	}

	got, rec, err := reconcileAt(path, nil)
	if err != nil {
		t.Fatalf("reconcileAt: unexpected error %v", err)
	}
	if got.Editor != "nano" {
		t.Errorf("Editor = %q, want %q (existing value must be preserved)", got.Editor, "nano")
	}
	if !rec.Updated {
		t.Error("Reconciliation.Updated = false, want true (missing keys must trigger a rewrite)")
	}
	if len(rec.Conflicts) != 0 {
		t.Errorf("Conflicts = %v, want none (merging keys is silent)", rec.Conflicts)
	}
	if note := rec.Notice(); note != "" {
		t.Errorf("Notice() = %q, want empty (merging keys is silent)", note)
	}

	// Every schema key must now be present on disk so the file is self-documenting.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config after reconcile: %v", err)
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(data, &present); err != nil {
		t.Fatalf("parse rewritten config: %v", err)
	}
	for _, key := range schemaKeys() {
		if _, ok := present[key]; !ok {
			t.Errorf("rewritten config missing key %q", key)
		}
	}
}

func TestReconcileLeavesCompleteValidFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// A complete, valid document (all schema keys, all values valid) must not be
	// rewritten: there is nothing to update.
	if err := saveTo(path, Default()); err != nil {
		t.Fatalf("saveTo: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	_, rec, err := reconcileAt(path, nil)
	if err != nil {
		t.Fatalf("reconcileAt: unexpected error %v", err)
	}
	if rec.Updated || rec.Created || len(rec.Conflicts) != 0 {
		t.Errorf("Reconciliation = %+v, want zero value (nothing to do)", rec)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config after reconcile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("config was rewritten: before %q, after %q", before, after)
	}
}

func TestReconcileResetsInvalidValueWithConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// All keys present, but logLimit is non-positive: an explicit invalid value
	// that conflicts with the schema and must revert to the default.
	const onDisk = `{"defaultPath":"","logLimit":0,"editor":"","theme":"auto","directoryDiff":true}`
	if err := os.WriteFile(path, []byte(onDisk), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, rec, err := reconcileAt(path, nil)
	if err != nil {
		t.Fatalf("reconcileAt: unexpected error %v", err)
	}
	if got.LogLimit != Default().LogLimit {
		t.Errorf("LogLimit = %d, want default %d", got.LogLimit, Default().LogLimit)
	}
	if !rec.Updated {
		t.Error("Reconciliation.Updated = false, want true")
	}
	if len(rec.Conflicts) != 1 {
		t.Fatalf("Conflicts = %v, want exactly one", rec.Conflicts)
	}
	if note := rec.Notice(); note == "" || !strings.Contains(note, "logLimit") {
		t.Errorf("Notice() = %q, want a message mentioning logLimit", note)
	}

	// The repaired value must be persisted, not just returned in memory.
	reloaded, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom after reconcile: %v", err)
	}
	if reloaded.LogLimit != Default().LogLimit {
		t.Errorf("persisted LogLimit = %d, want default %d", reloaded.LogLimit, Default().LogLimit)
	}
}

func TestReconcileRunsValidatorForDomainConflicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	const onDisk = `{"defaultPath":"","logLimit":100,"editor":"","theme":"retired","directoryDiff":true}`
	if err := os.WriteFile(path, []byte(onDisk), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// A validator standing in for the app's theme check: any theme other than the
	// default is treated as no longer available and reset to the default.
	validate := func(cfg *Config) []string {
		if cfg.Theme == Default().Theme {
			return nil
		}
		was := cfg.Theme
		cfg.Theme = Default().Theme
		return []string{"theme " + was + " reset"}
	}

	got, rec, err := reconcileAt(path, validate)
	if err != nil {
		t.Fatalf("reconcileAt: unexpected error %v", err)
	}
	if got.Theme != Default().Theme {
		t.Errorf("Theme = %q, want default %q", got.Theme, Default().Theme)
	}
	if len(rec.Conflicts) != 1 || !rec.Updated {
		t.Errorf("Reconciliation = %+v, want one conflict and Updated", rec)
	}
	reloaded, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom after reconcile: %v", err)
	}
	if reloaded.Theme != Default().Theme {
		t.Errorf("persisted Theme = %q, want default %q", reloaded.Theme, Default().Theme)
	}
}

func TestReconcileMalformedJSONLeavesFileIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	const bad = "{not json"
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	got, rec, err := reconcileAt(path, nil)
	if err == nil {
		t.Fatal("reconcileAt(malformed): expected an error, got nil")
	}
	if got != Default() {
		t.Errorf("reconcileAt(malformed) = %+v, want Default()", got)
	}
	if rec.Updated || rec.Created {
		t.Errorf("Reconciliation = %+v, want zero value (must not touch an unparseable file)", rec)
	}
	// A file we could not parse must be left intact for the user to fix.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config after reconcile: %v", err)
	}
	if string(data) != bad {
		t.Errorf("malformed config was modified: got %q, want %q", string(data), bad)
	}
}

func TestReconciliationNotice(t *testing.T) {
	if note := (Reconciliation{}).Notice(); note != "" {
		t.Errorf("Notice() with no conflicts = %q, want empty", note)
	}
	one := Reconciliation{Conflicts: []string{"logLimit 0 is invalid; reset to 100"}}
	if note := one.Notice(); note != "config: logLimit 0 is invalid; reset to 100" {
		t.Errorf("Notice() with one conflict = %q", note)
	}
	many := Reconciliation{Conflicts: []string{"a", "b", "c"}}
	if note := many.Notice(); note != "config: 3 settings reset to defaults" {
		t.Errorf("Notice() with many conflicts = %q", note)
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
