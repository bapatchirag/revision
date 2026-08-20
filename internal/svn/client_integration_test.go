package svn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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

	entries, _, err := c.LogPage(ctx, "", 10)
	if err != nil {
		t.Fatalf("LogPage: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one log entry")
	}
	if entries[0].Message != "initial import" {
		t.Errorf("latest message = %q, want %q", entries[0].Message, "initial import")
	}
	if len(entries[0].Paths) != 0 {
		t.Error("a log page should not carry changed paths; they come from RevisionDetail")
	}

	head, err := c.HeadRevision(ctx)
	if err != nil {
		t.Fatalf("HeadRevision: %v", err)
	}
	if head != entries[0].Revision {
		t.Errorf("HeadRevision = %q, want %q", head, entries[0].Revision)
	}

	detail, err := c.RevisionDetail(ctx, entries[0].Revision)
	if err != nil {
		t.Fatalf("RevisionDetail: %v", err)
	}
	if len(detail.Paths) == 0 {
		t.Error("expected changed paths in the per-revision detail")
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

// TestIntegrationRevisionDiff pins the two properties the Log panel's diff is
// built on: an added or deleted file is marked by "(nonexistent)" on the side it
// is missing from, and a range is literal — the state at one revision against
// the state at another, not the sum of the commits between them.
func TestIntegrationRevisionDiff(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	// r1 adds a.txt, r2 adds b.txt, r3 modifies a.txt, r4 deletes b.txt.
	writeFile(t, filepath.Join(wc, "a.txt"), "a1\n")
	mustRun(t, wc, "svn", "add", "a.txt")
	mustRun(t, wc, "svn", "commit", "-m", "r1")
	mustRun(t, wc, "svn", "update")

	writeFile(t, filepath.Join(wc, "b.txt"), "b1\n")
	mustRun(t, wc, "svn", "add", "b.txt")
	mustRun(t, wc, "svn", "commit", "-m", "r2")
	mustRun(t, wc, "svn", "update")

	writeFile(t, filepath.Join(wc, "a.txt"), "a1\na2\n")
	mustRun(t, wc, "svn", "commit", "-m", "r3")
	mustRun(t, wc, "svn", "update")

	mustRun(t, wc, "svn", "rm", "b.txt")
	mustRun(t, wc, "svn", "commit", "-m", "r4")
	mustRun(t, wc, "svn", "update")

	added, err := c.DiffRevision(ctx, "2")
	if err != nil {
		t.Fatalf("DiffRevision(2): %v", err)
	}
	if !strings.Contains(added, "--- b.txt\t(nonexistent)") {
		t.Errorf("an added file should be missing from the left side:\n%s", added)
	}

	deleted, err := c.DiffRevision(ctx, "4")
	if err != nil {
		t.Fatalf("DiffRevision(4): %v", err)
	}
	if !strings.Contains(deleted, "+++ b.txt\t(nonexistent)") {
		t.Errorf("a deleted file should be missing from the right side:\n%s", deleted)
	}

	// r2 is the left-hand endpoint, so b.txt is already present there: the range
	// records it being deleted, never added. That is what makes the range literal.
	rng, err := c.DiffRevisions(ctx, "2", "4")
	if err != nil {
		t.Fatalf("DiffRevisions(2, 4): %v", err)
	}
	if !strings.Contains(rng, "+++ b.txt\t(nonexistent)") {
		t.Errorf("the range should delete b.txt:\n%s", rng)
	}
	if strings.Contains(rng, "--- b.txt\t(nonexistent)") {
		t.Errorf("a literal range must not replay the left endpoint's own change:\n%s", rng)
	}
	if !strings.Contains(rng, "+a2") {
		t.Errorf("the range should carry r3's modification:\n%s", rng)
	}

	// Picked the other way round, the same pair must still read forwards.
	reversed, err := c.DiffRevisions(ctx, "4", "2")
	if err != nil {
		t.Fatalf("DiffRevisions(4, 2): %v", err)
	}
	if reversed != rng {
		t.Errorf("a range picked newest-first should match the same range picked oldest-first:\n%s\n---\n%s", reversed, rng)
	}
}

func TestIntegrationLogPaging(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	writeFile(t, filepath.Join(wc, "f.txt"), "0\n")
	mustRun(t, wc, "svn", "add", "f.txt")
	for i := 1; i <= 5; i++ {
		writeFile(t, filepath.Join(wc, "f.txt"), strings.Repeat("x\n", i))
		mustRun(t, wc, "svn", "commit", "-m", "commit "+strconv.Itoa(i))
		mustRun(t, wc, "svn", "update")
	}

	// Walk the whole history two revisions at a time; the pages must tile it
	// exactly, with no repeats and no gaps.
	var walked []string
	anchor := ""
	for page := 1; ; page++ {
		if page > 5 {
			t.Fatal("paging did not terminate")
		}
		entries, more, err := c.LogPage(ctx, anchor, 2)
		if err != nil {
			t.Fatalf("LogPage(%q): %v", anchor, err)
		}
		if len(entries) > 2 {
			t.Fatalf("page %d has %d entries, want at most 2", page, len(entries))
		}
		for _, e := range entries {
			walked = append(walked, e.Revision)
		}
		if !more {
			break
		}
		anchor = entries[len(entries)-1].Revision
	}

	all, more, err := c.LogPage(ctx, "", 0)
	if err != nil {
		t.Fatalf("LogPage unlimited: %v", err)
	}
	if more {
		t.Error("an unlimited fetch must not report a further page")
	}
	if len(walked) != len(all) {
		t.Fatalf("paged through %d revisions, want %d", len(walked), len(all))
	}
	for i, e := range all {
		if walked[i] != e.Revision {
			t.Fatalf("paged revisions = %q, want %q", walked, "the unpaged order")
		}
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
	// revision (a mixed-revision working copy). LogPage pegs at HEAD to surface it.
	entries, _, err := c.LogPage(ctx, "", 10)
	if err != nil {
		t.Fatalf("LogPage after commit: %v", err)
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
	if err := c.RevertPaths(ctx, []string{"file.txt"}); err != nil {
		t.Fatalf("RevertPaths: %v", err)
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

// TestIntegrationRevertPathsClearsAnAddedDirectory pins the case that a revert
// naming one path at a time cannot do. svn refuses to revert a directory
// scheduled for addition at the default depth (E155038), and once the recursive
// revert has taken the directory, every path still queued beneath it is gone
// too: reverting those in turn fails with E155010 more than one level down.
func TestIntegrationRevertPathsClearsAnAddedDirectory(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	if err := os.MkdirAll(filepath.Join(wc, "mpte", "rust-crate", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(wc, "mpte", "Makefile"), "all:\n")
	writeFile(t, filepath.Join(wc, "mpte", "rust-crate", "src", "lib.rs"), "fn main() {}\n")
	mustRun(t, wc, "svn", "add", "mpte")

	var added []string
	for p, it := range statusByPath(t, c, ctx) {
		if it.State == StateAdded {
			added = append(added, p)
		}
	}
	sort.Strings(added)
	want := []string{"mpte", "mpte/Makefile", "mpte/rust-crate", "mpte/rust-crate/src", "mpte/rust-crate/src/lib.rs"}
	if !equalPaths(added, want) {
		t.Fatalf("added = %v, want %v", added, want)
	}

	// The whole set in status order is what a directory-level revert in the app
	// hands over, parent first.
	if err := c.RevertPaths(ctx, added); err != nil {
		t.Fatalf("RevertPaths: %v", err)
	}

	after := statusByPath(t, c, ctx)
	for p, it := range after {
		if it.State == StateAdded {
			t.Errorf("%s is still scheduled for addition after the revert", p)
		}
	}
	if got := after["mpte"].State; got != StateUnversioned {
		t.Errorf("mpte state = %s after the revert, want unversioned", got)
	}
	// The revert un-schedules the add but leaves the tree where it was.
	if _, err := os.Stat(filepath.Join(wc, "mpte", "rust-crate", "src", "lib.rs")); err != nil {
		t.Errorf("the added tree should still be on disk after a revert: %v", err)
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

func TestIntegrationUpdateToRevision(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()

	// Two commits: first.txt lands in r1, second.txt in r2.
	writeFile(t, filepath.Join(wc, "first.txt"), "one\n")
	mustRun(t, wc, "svn", "add", "first.txt")
	mustRun(t, wc, "svn", "commit", "-m", "first")
	writeFile(t, filepath.Join(wc, "second.txt"), "two\n")
	mustRun(t, wc, "svn", "add", "second.txt")
	mustRun(t, wc, "svn", "commit", "-m", "second")

	// Updating back to r1 drops second.txt and reports the revision.
	rev, err := New(wc).UpdateToRevision(ctx, "1")
	if err != nil {
		t.Fatalf("UpdateToRevision: %v", err)
	}
	if rev != "1" {
		t.Errorf("UpdateToRevision() = %q, want %q", rev, "1")
	}
	if _, err := os.Stat(filepath.Join(wc, "second.txt")); !os.IsNotExist(err) {
		t.Errorf("second.txt should be gone after updating to r1, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(wc, "first.txt")); err != nil {
		t.Errorf("first.txt should remain after updating to r1: %v", err)
	}
}

// seedPatch builds a working copy holding a.txt and sub/b.txt, saves a patch of
// a modification to both, then reverts the working copy. It returns the working
// copy and the patch file, which is written outside it so it is not itself a
// working-copy change.
func seedPatch(t *testing.T) (wc, patch string) {
	t.Helper()
	wc = setupWC(t)
	if err := os.Mkdir(filepath.Join(wc, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(wc, "a.txt"), "one\n")
	writeFile(t, filepath.Join(wc, "sub", "b.txt"), "x\n")
	mustRun(t, wc, "svn", "add", "a.txt", "sub")
	mustRun(t, wc, "svn", "commit", "-m", "seed")
	mustRun(t, wc, "svn", "update")

	writeFile(t, filepath.Join(wc, "a.txt"), "ONE\n")
	writeFile(t, filepath.Join(wc, "sub", "b.txt"), "y\n")
	diff, err := New(wc).Diff(context.Background(), "")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	patch = filepath.Join(t.TempDir(), "wc.diff")
	writeFile(t, patch, diff)
	mustRun(t, wc, "svn", "revert", "-R", ".")
	return wc, patch
}

func TestIntegrationPatch(t *testing.T) {
	wc, patch := seedPatch(t)
	ctx := context.Background()
	c := New(wc)

	// The dry run reports both targets and changes nothing on disk.
	dry, err := c.Patch(ctx, patch, true)
	if err != nil {
		t.Fatalf("Patch dry run: %v", err)
	}
	if want := []string{"a.txt", "sub/b.txt"}; !equalPaths(dry.Applied, want) {
		t.Errorf("dry run Applied = %v, want %v", dry.Applied, want)
	}
	if dry.Targets() != 2 {
		t.Errorf("dry run Targets() = %d, want 2", dry.Targets())
	}
	if data, _ := os.ReadFile(filepath.Join(wc, "a.txt")); string(data) != "one\n" {
		t.Fatalf("a.txt = %q after a dry run, want it untouched", string(data))
	}

	// Applying for real leaves both files modified.
	res, err := c.Patch(ctx, patch, false)
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if want := []string{"a.txt", "sub/b.txt"}; !equalPaths(res.Applied, want) {
		t.Errorf("Applied = %v, want %v", res.Applied, want)
	}
	if data, _ := os.ReadFile(filepath.Join(wc, "a.txt")); string(data) != "ONE\n" {
		t.Errorf("a.txt = %q after the patch, want %q", string(data), "ONE\n")
	}
	if got := statusByPath(t, c, ctx)["sub/b.txt"].State; got != StateModified {
		t.Errorf("sub/b.txt state = %s after the patch, want modified", got)
	}
}

// TestIntegrationPatchFromAnotherDirectory is why a patch is checked against the
// directory before it is applied: svn does not refuse one whose paths are
// relative to somewhere else. Run a directory too deep, it creates each missing
// target and rejects the patch's hunks into it.
func TestIntegrationPatchFromAnotherDirectory(t *testing.T) {
	wc, patch := seedPatch(t)
	sub := filepath.Join(wc, "sub")

	if PatchBelongsTo(readString(t, patch), sub) {
		t.Fatal("a patch relative to the working copy's root does not belong to sub")
	}

	dry, err := New(sub).Patch(context.Background(), patch, true)
	if err != nil {
		t.Fatalf("Patch dry run: %v", err)
	}
	if len(dry.Applied) > 0 {
		t.Errorf("Applied = %v, want nothing to go in cleanly from the wrong directory", dry.Applied)
	}
	if len(dry.Conflicted) == 0 {
		t.Error("svn should report conflicts for a patch run from the wrong directory")
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// numbered returns 30 numbered lines with the given 1-based lines replaced. Two
// edits far enough apart in it produce two separate hunks, which is what a
// partly applying patch needs.
func numbered(replace map[int]string) string {
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		if s, ok := replace[i]; ok {
			b.WriteString(s + "\n")
			continue
		}
		b.WriteString(strconv.Itoa(i) + "\n")
	}
	return b.String()
}

// TestIntegrationPatchPartiallyApplies pins down what svn does with a patch that
// only half fits, which is what the app's gate reads: the classification is per
// file, so a file with one rejected hunk comes back conflicted even though its
// other hunk went in, while a file that fits comes back applied.
func TestIntegrationPatchPartiallyApplies(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	writeFile(t, filepath.Join(wc, "f.txt"), numbered(nil))
	writeFile(t, filepath.Join(wc, "g.txt"), "clean\n")
	mustRun(t, wc, "svn", "add", "f.txt", "g.txt")
	mustRun(t, wc, "svn", "commit", "-m", "seed")
	mustRun(t, wc, "svn", "update")

	// A patch touching both ends of f.txt, and all of g.txt.
	writeFile(t, filepath.Join(wc, "f.txt"), numbered(map[int]string{2: "TWO", 28: "TWENTY-EIGHT"}))
	writeFile(t, filepath.Join(wc, "g.txt"), "CLEAN\n")
	diff, err := c.Diff(ctx, "")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	patch := filepath.Join(t.TempDir(), "two-hunks.diff")
	writeFile(t, patch, diff)
	mustRun(t, wc, "svn", "revert", "-R", ".")

	// f.txt then drifts under the second hunk, which can no longer be placed.
	writeFile(t, filepath.Join(wc, "f.txt"), numbered(map[int]string{27: "twenty-seven-CHANGED"}))

	dry, err := c.Patch(ctx, patch, true)
	if err != nil {
		t.Fatalf("Patch dry run: %v", err)
	}
	if !equalPaths(dry.Applied, []string{"g.txt"}) {
		t.Errorf("dry run Applied = %v, want [g.txt]", dry.Applied)
	}
	if !equalPaths(dry.Conflicted, []string{"f.txt"}) {
		t.Errorf("dry run Conflicted = %v, want [f.txt] — one rejected hunk conflicts the file", dry.Conflicted)
	}
	if got := readString(t, filepath.Join(wc, "g.txt")); got != "clean\n" {
		t.Errorf("g.txt = %q after a dry run, want it untouched", got)
	}

	if _, err := c.Patch(ctx, patch, false); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	lines := strings.Split(readString(t, filepath.Join(wc, "f.txt")), "\n")
	if lines[1] != "TWO" {
		t.Errorf("f.txt line 2 = %q, want the hunk that fits to have been applied", lines[1])
	}
	if lines[27] != "28" {
		t.Errorf("f.txt line 28 = %q, want the rejected hunk left out", lines[27])
	}
	rejects, _ := filepath.Glob(filepath.Join(wc, "f.txt*.rej"))
	if len(rejects) == 0 {
		t.Error("svn should have written the rejected hunk out beside f.txt")
	}
}

// TestIntegrationDiffForPatchingCarriesAMovedFile pins the reason
// DiffPathsForPatching exists: a plain diff describes a move as a deletion and
// an empty section, so putting one back would lose the file outright.
func TestIntegrationDiffForPatchingCarriesAMovedFile(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	writeFile(t, filepath.Join(wc, "moved.txt"), "old\n")
	mustRun(t, wc, "svn", "add", "moved.txt")
	mustRun(t, wc, "svn", "commit", "-m", "seed")
	mustRun(t, wc, "svn", "move", "moved.txt", "renamed.txt")

	plain, err := c.DiffPaths(ctx, nil)
	if err != nil {
		t.Fatalf("DiffPaths: %v", err)
	}
	if strings.Contains(plain, "+old") {
		t.Errorf("a plain diff is expected to leave a moved file's content out:\n%s", plain)
	}

	full, err := c.DiffPathsForPatching(ctx, nil)
	if err != nil {
		t.Fatalf("DiffPathsForPatching: %v", err)
	}
	if !strings.Contains(full, "+old") {
		t.Fatalf("DiffPathsForPatching left the moved file's content out:\n%s", full)
	}

	// The patch has to be able to put the move back with nothing else to go on.
	if err := c.RevertPaths(ctx, []string{"."}); err != nil {
		t.Fatalf("RevertPaths: %v", err)
	}
	patch := filepath.Join(t.TempDir(), "move.patch")
	writeFile(t, patch, full)
	if _, err := c.Patch(ctx, patch, false); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := readString(t, filepath.Join(wc, "renamed.txt")); got != "old\n" {
		t.Errorf("renamed.txt = %q, want %q put back by the patch", got, "old\n")
	}
	if _, err := os.Stat(filepath.Join(wc, "moved.txt")); err == nil {
		t.Error("moved.txt should have been taken away again by the patch")
	}
}

// TestIntegrationDiffLeavesBinaryContentOut pins what BinarySkips is for: svn
// will not express a binary file as text, so a diff names it and carries none
// of it.
func TestIntegrationDiffLeavesBinaryContentOut(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	bin := filepath.Join(wc, "logo.bin")
	writeFile(t, bin, "\x00\x01\x02BEFORE\xff\xfe")
	mustRun(t, wc, "svn", "add", "logo.bin")
	mustRun(t, wc, "svn", "commit", "-m", "seed")
	writeFile(t, bin, "\x00\x01\x02AFTER\xff\xfe\xfd")

	patch, err := c.DiffPathsForPatching(ctx, nil)
	if err != nil {
		t.Fatalf("DiffPathsForPatching: %v", err)
	}
	if strings.Contains(patch, "AFTER") {
		t.Errorf("svn is not expected to carry a binary file's content:\n%s", patch)
	}
	got := BinarySkips(patch)
	if len(got) != 1 || got[0] != "logo.bin" {
		t.Errorf("BinarySkips = %v, want [logo.bin] — what the patch cannot put back has to be named", got)
	}
}

// TestIntegrationRevertPathsLeavesAScheduledAddOnDisk pins the trace a revert
// leaves: the add is un-scheduled, but the file stays, so a caller clearing a
// working copy has to remove it itself.
func TestIntegrationRevertPathsLeavesAScheduledAddOnDisk(t *testing.T) {
	wc := setupWC(t)
	ctx := context.Background()
	c := New(wc)

	writeFile(t, filepath.Join(wc, "tracked.txt"), "committed\n")
	mustRun(t, wc, "svn", "add", "tracked.txt")
	mustRun(t, wc, "svn", "commit", "-m", "seed")

	writeFile(t, filepath.Join(wc, "tracked.txt"), "modified\n")
	writeFile(t, filepath.Join(wc, "added.txt"), "brand new\n")
	mustRun(t, wc, "svn", "add", "added.txt")

	if err := c.RevertPaths(ctx, []string{"."}); err != nil {
		t.Fatalf("RevertPaths: %v", err)
	}
	if got := readString(t, filepath.Join(wc, "tracked.txt")); got != "committed\n" {
		t.Errorf("tracked.txt = %q, want the committed content back", got)
	}
	if _, err := os.Stat(filepath.Join(wc, "added.txt")); err != nil {
		t.Errorf("added.txt should still be on disk after a revert: %v", err)
	}
	items, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(items) != 1 || items[0].Path != "added.txt" || items[0].State != StateUnversioned {
		t.Errorf("Status = %+v, want added.txt left behind unversioned", items)
	}
}
