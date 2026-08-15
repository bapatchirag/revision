package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// withoutHome removes every clue to a home directory, so os.UserHomeDir fails
// and the error paths of Dir, Path, Load, Save and Reconcile are reached.
func withoutHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
}

func TestDirFailsWithoutAHomeDirectory(t *testing.T) {
	withoutHome(t)
	if _, err := Dir(); err == nil {
		t.Error("Dir() should fail when the home directory cannot be located")
	}
	if _, err := Path(); err == nil {
		t.Error("Path() should carry Dir()'s failure")
	}
}

func TestLoadFallsBackToDefaultsWhenThePathIsUnknown(t *testing.T) {
	withoutHome(t)
	cfg, err := Load()
	if err == nil {
		t.Error("Load() should report that it could not locate the config file")
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Error("Load() should still return usable defaults so the app can start")
	}
}

func TestSaveAndReconcileFailWhenThePathIsUnknown(t *testing.T) {
	withoutHome(t)
	if err := Save(Default()); err == nil {
		t.Error("Save() should report that it could not locate the config file")
	}
	cfg, rec, err := Reconcile(nil)
	if err == nil {
		t.Error("Reconcile() should report that it could not locate the config file")
	}
	if !reflect.DeepEqual(cfg, Default()) || rec.Created || rec.Updated || len(rec.Conflicts) != 0 {
		t.Error("Reconcile() should still return usable defaults so the app can start")
	}
}

func TestSaveToReportsAnUncreatableDirectory(t *testing.T) {
	// A file where the config directory should be: MkdirAll can never succeed.
	blocker := filepath.Join(t.TempDir(), "revision")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	err := saveTo(filepath.Join(blocker, fileName), Default())
	if err == nil || !strings.Contains(err.Error(), "create config dir") {
		t.Errorf("err = %v, want the directory failure reported", err)
	}
}

func TestSaveToReportsAnUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := saveTo(filepath.Join(dir, fileName), Default())
	if err == nil || !strings.Contains(err.Error(), "create temp config") {
		t.Errorf("err = %v, want the temp-file failure reported", err)
	}
}

// TestSaveToLeavesTheOldConfigWhenTheWriteFails checks the point of the atomic
// write: a failure never leaves a truncated config behind.
func TestSaveToLeavesTheOldConfigWhenTheWriteFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	const existing = "{\"theme\": \"nord\"}\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if err := saveTo(path, Default()); err == nil {
		t.Fatal("expected the write to fail")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the existing config should still be readable: %v", err)
	}
	if string(got) != existing {
		t.Errorf("config = %q, want the original left intact", got)
	}
}
