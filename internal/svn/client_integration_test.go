package svn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireSVN(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"svn", "svnadmin"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found on PATH; skipping integration test", bin)
		}
	}
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// setupWC creates a fresh repository and checks out a working copy, returning
// the working-copy path.
func setupWC(t *testing.T) string {
	t.Helper()
	requireSVN(t)
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	wc := filepath.Join(root, "wc")

	mustRun(t, "", "svnadmin", "create", repo)
	mustRun(t, "", "svn", "checkout", "file://"+repo, wc)
	return wc
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationStatusAndInfo(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	writeFile(t, filepath.Join(wc, "committed.txt"), "hello\n")
	mustRun(t, wc, "svn", "add", "committed.txt")
	mustRun(t, wc, "svn", "commit", "-m", "initial")
	mustRun(t, wc, "svn", "update")

	writeFile(t, filepath.Join(wc, "committed.txt"), "hello\nworld\n")
	writeFile(t, filepath.Join(wc, "added.txt"), "new\n")
	mustRun(t, wc, "svn", "add", "added.txt")
	writeFile(t, filepath.Join(wc, "untracked.txt"), "scratch\n")

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.WorkingCopyRoot == "" {
		t.Error("expected non-empty working-copy root")
	}
	if info.Revision == "" {
		t.Error("expected a revision")
	}

	items, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	byPath := make(map[string]StatusItem, len(items))
	for _, it := range items {
		byPath[it.Path] = it
	}

	cases := map[string]FileState{
		"committed.txt": StateModified,
		"added.txt":     StateAdded,
		"untracked.txt": StateUnversioned,
	}
	for path, want := range cases {
		if got := byPath[path].State; got != want {
			t.Errorf("%s state = %s, want %s", path, got, want)
		}
	}
}

func TestIntegrationLogAndDiff(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	writeFile(t, filepath.Join(wc, "committed.txt"), "hello\n")
	mustRun(t, wc, "svn", "add", "committed.txt")
	mustRun(t, wc, "svn", "commit", "-m", "initial import")
	mustRun(t, wc, "svn", "update")

	entries, err := c.Log(ctx, 10)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one log entry")
	}
	if entries[0].Message != "initial import" {
		t.Errorf("latest message = %q, want %q", entries[0].Message, "initial import")
	}
	if len(entries[0].Paths) == 0 {
		t.Error("expected changed paths in verbose log")
	}

	writeFile(t, filepath.Join(wc, "committed.txt"), "hello\nworld\n")
	diff, err := c.Diff(ctx, "committed.txt")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(diff, "+world") {
		t.Errorf("diff missing the added line:\n%s", diff)
	}
}
