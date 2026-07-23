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

func TestIntegrationStageAndCommit(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	// Seed two committed files, then modify both.
	writeFile(t, filepath.Join(wc, "a.txt"), "a\n")
	writeFile(t, filepath.Join(wc, "b.txt"), "b\n")
	mustRun(t, wc, "svn", "add", "a.txt", "b.txt")
	mustRun(t, wc, "svn", "commit", "-m", "seed")
	mustRun(t, wc, "svn", "update")
	writeFile(t, filepath.Join(wc, "a.txt"), "a\na2\n")
	writeFile(t, filepath.Join(wc, "b.txt"), "b\nb2\n")

	// Stage only a.txt; status must report it under the staged changelist.
	if err := c.AddToChangelist(ctx, "revision:staged", "a.txt"); err != nil {
		t.Fatalf("AddToChangelist: %v", err)
	}
	byPath := statusByPath(t, c, ctx)
	if got := byPath["a.txt"].Changelist; got != "revision:staged" {
		t.Errorf("a.txt changelist = %q, want revision:staged", got)
	}
	if got := byPath["b.txt"].Changelist; got != "" {
		t.Errorf("b.txt changelist = %q, want empty", got)
	}

	// Commit only the staged changelist; b.txt must stay modified.
	rev, err := c.Commit(ctx, "commit staged", "revision:staged")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if rev == "" {
		t.Error("expected a committed revision number")
	}

	// Regression: the just-committed revision must appear in the log even though
	// only a.txt was committed, so the working-copy root is still at the old
	// revision (a mixed-revision working copy). Log pegs at HEAD to surface it.
	entries, err := c.Log(ctx, 10)
	if err != nil {
		t.Fatalf("Log after commit: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Revision == rev {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("commit r%s missing from log of a mixed-revision working copy: %+v", rev, entries)
	}
	byPath = statusByPath(t, c, ctx)
	if st, ok := byPath["a.txt"]; ok && st.State == StateModified {
		t.Error("a.txt should have been committed, but is still modified")
	}
	if got := byPath["b.txt"].State; got != StateModified {
		t.Errorf("b.txt state = %s, want modified (must be excluded from a staged commit)", got)
	}

	// Unstaging drops changelist membership.
	if err := c.AddToChangelist(ctx, "revision:staged", "b.txt"); err != nil {
		t.Fatalf("AddToChangelist b.txt: %v", err)
	}
	if err := c.RemoveFromChangelist(ctx, "b.txt"); err != nil {
		t.Fatalf("RemoveFromChangelist: %v", err)
	}
	if got := statusByPath(t, c, ctx)["b.txt"].Changelist; got != "" {
		t.Errorf("b.txt changelist = %q after remove, want empty", got)
	}
}

// statusByPath runs Status and indexes the results by path for convenient
// assertions.
func statusByPath(t *testing.T, c *Client, ctx context.Context) map[string]StatusItem {
	t.Helper()
	items, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	byPath := make(map[string]StatusItem, len(items))
	for _, it := range items {
		byPath[it.Path] = it
	}
	return byPath
}

func TestIntegrationAddThenStage(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	// A fresh file starts unversioned, becomes "added" after Add, and can then
	// join the staged changelist.
	writeFile(t, filepath.Join(wc, "fresh.txt"), "hi\n")
	if got := statusByPath(t, c, ctx)["fresh.txt"].State; got != StateUnversioned {
		t.Fatalf("fresh.txt state = %s, want unversioned", got)
	}
	if err := c.Add(ctx, "fresh.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := statusByPath(t, c, ctx)["fresh.txt"].State; got != StateAdded {
		t.Fatalf("fresh.txt state = %s after Add, want added", got)
	}
	if err := c.AddToChangelist(ctx, "revision:staged", "fresh.txt"); err != nil {
		t.Fatalf("AddToChangelist: %v", err)
	}
	if got := statusByPath(t, c, ctx)["fresh.txt"].Changelist; got != "revision:staged" {
		t.Errorf("fresh.txt changelist = %q, want revision:staged", got)
	}
}

func TestIntegrationRevert(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	writeFile(t, filepath.Join(wc, "file.txt"), "one\n")
	mustRun(t, wc, "svn", "add", "file.txt")
	mustRun(t, wc, "svn", "commit", "-m", "seed")
	mustRun(t, wc, "svn", "update")

	// Modify then revert: the working copy returns to its committed state and
	// the file drops out of status entirely.
	writeFile(t, filepath.Join(wc, "file.txt"), "one\ntwo\n")
	if got := statusByPath(t, c, ctx)["file.txt"].State; got != StateModified {
		t.Fatalf("file.txt state = %s, want modified", got)
	}
	if err := c.Revert(ctx, "file.txt"); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if _, ok := statusByPath(t, c, ctx)["file.txt"]; ok {
		t.Error("file.txt should be clean after revert (absent from status)")
	}
	data, err := os.ReadFile(filepath.Join(wc, "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "one\n" {
		t.Errorf("file.txt = %q after revert, want %q", string(data), "one\n")
	}
}

func TestIntegrationDelete(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	writeFile(t, filepath.Join(wc, "doomed.txt"), "bye\n")
	mustRun(t, wc, "svn", "add", "doomed.txt")
	mustRun(t, wc, "svn", "commit", "-m", "seed")
	mustRun(t, wc, "svn", "update")

	// Local modifications must not block the delete (hence --force); the file is
	// then scheduled for removal.
	writeFile(t, filepath.Join(wc, "doomed.txt"), "bye\nnow\n")
	if err := c.Delete(ctx, "doomed.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := statusByPath(t, c, ctx)["doomed.txt"].State; got != StateDeleted {
		t.Errorf("doomed.txt state = %s after delete, want deleted", got)
	}
}

func TestIntegrationRemoveUnversioned(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	scratch := filepath.Join(wc, "scratch.txt")
	writeFile(t, scratch, "temp\n")
	if got := statusByPath(t, c, ctx)["scratch.txt"].State; got != StateUnversioned {
		t.Fatalf("scratch.txt state = %s, want unversioned", got)
	}
	if err := c.RemoveUnversioned("scratch.txt"); err != nil {
		t.Fatalf("RemoveUnversioned: %v", err)
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Errorf("scratch.txt should be gone from disk, stat err = %v", err)
	}
}

func TestIntegrationUpdate(t *testing.T) {
	requireSVN(t)
	ctx := context.Background()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	wc1 := filepath.Join(root, "wc1")
	wc2 := filepath.Join(root, "wc2")

	mustRun(t, "", "svnadmin", "create", repo)
	mustRun(t, "", "svn", "checkout", "file://"+repo, wc1)
	mustRun(t, "", "svn", "checkout", "file://"+repo, wc2)

	// Commit a file from wc1; wc2 has not seen it until it updates.
	writeFile(t, filepath.Join(wc1, "shared.txt"), "hello\n")
	mustRun(t, wc1, "svn", "add", "shared.txt")
	mustRun(t, wc1, "svn", "commit", "-m", "add shared")

	rev, err := New(wc2).Update(ctx)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if rev == "" {
		t.Error("expected a revision number from update")
	}
	if _, err := os.Stat(filepath.Join(wc2, "shared.txt")); err != nil {
		t.Errorf("shared.txt should exist in wc2 after update: %v", err)
	}
}
