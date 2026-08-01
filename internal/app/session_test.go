package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bapatchirag/revision/internal/svn"
)

// stampRoot writes files into a temp directory and returns it, so the on-disk
// half of a stamp — size and modification time — has something real to read.
func stampRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// stamper fingerprints keys against one snapshot of a working copy: the files on
// disk under root, its revision, and what svn status reported for it.
func stamper(root, rev string, items []svn.StatusItem) func(diffKey) string {
	return func(k diffKey) string { return diffStampFor(root, rev, items, k) }
}

// cachedDiff stores text for k under the given snapshot and reports whether the
// store still serves it under another.
func cachedDiff(s *sessionStore, k diffKey, stamp func(diffKey) string) bool {
	_, ok := s.Diff(k, stamp(k))
	return ok
}

func TestSessionKeepsDiffWhenNothingMoved(t *testing.T) {
	root := stampRoot(t, map[string]string{"a.txt": "alpha", "b.txt": "beta"})
	items := []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified},
		{Path: "b.txt", State: svn.StateModified},
	}
	before := stamper(root, "42", items)

	s := newSessionStore()
	k := diffKey{path: "a.txt"}
	s.PutDiff(k, diffEntry{text: "@@ alpha @@", stamp: before(k)})

	// The same status over the same files: a reload must not cost a re-fetch.
	if !cachedDiff(s, k, before) {
		t.Fatal("an unchanged file's diff should survive a status reload")
	}
}

func TestSessionDropsDiffWhenStatusMoves(t *testing.T) {
	root := stampRoot(t, map[string]string{"a.txt": "alpha", "b.txt": "beta"})
	items := []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified},
		{Path: "b.txt", State: svn.StateModified},
	}
	before := stamper(root, "42", items)

	s := newSessionStore()
	kept := diffKey{path: "b.txt"}
	moved := diffKey{path: "a.txt"}
	s.PutDiff(kept, diffEntry{text: "@@ beta @@", stamp: before(kept)})
	s.PutDiff(moved, diffEntry{text: "@@ alpha @@", stamp: before(moved)})

	// a.txt is staged; b.txt is untouched.
	staged := []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified, Changelist: stagedChangelist},
		{Path: "b.txt", State: svn.StateModified},
	}
	after := stamper(root, "42", staged)

	if cachedDiff(s, moved, after) {
		t.Error("a file whose status moved should have been dropped")
	}
	if !cachedDiff(s, kept, after) {
		t.Error("a file nothing happened to should have been kept")
	}
}

func TestSessionDropsDiffWhenFileLeavesStatus(t *testing.T) {
	root := stampRoot(t, map[string]string{"a.txt": "alpha"})
	items := []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}}
	before := stamper(root, "42", items)

	s := newSessionStore()
	k := diffKey{path: "a.txt"}
	s.PutDiff(k, diffEntry{text: "@@ alpha @@", stamp: before(k)})

	// A revert leaves the file clean, so svn status stops reporting it.
	after := stamper(root, "42", nil)
	if cachedDiff(s, k, after) {
		t.Error("a reverted file's diff should have been dropped")
	}
}

// TestSessionDropsDiffWhenFileEdited covers the change svn status cannot see: an
// already-modified file edited again is still reported as "modified", so only
// the on-disk size and modification time give it away.
func TestSessionDropsDiffWhenFileEdited(t *testing.T) {
	root := stampRoot(t, map[string]string{"a.txt": "alpha"})
	items := []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}}
	stamp := stamper(root, "42", items)

	s := newSessionStore()
	k := diffKey{path: "a.txt"}
	s.PutDiff(k, diffEntry{text: "@@ alpha @@", stamp: stamp(k)})

	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("alpha and then some"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cachedDiff(s, k, stamp) {
		t.Error("an edited file's diff should have been dropped")
	}

	// Same size, later timestamp: the modification time alone must be enough.
	s.PutDiff(k, diffEntry{text: "@@ alpha @@", stamp: stamp(k)})
	if err := os.WriteFile(path, []byte("ALPHA AND THEN SOME"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if cachedDiff(s, k, stamp) {
		t.Error("a same-size edit should still have been caught by the modification time")
	}
}

func TestSessionDropsDirectoryDiffWhenAnythingBeneathMoves(t *testing.T) {
	root := stampRoot(t, map[string]string{"src/a.go": "alpha", "doc/b.md": "beta"})
	items := []svn.StatusItem{
		{Path: "doc/b.md", State: svn.StateModified},
		{Path: "src/a.go", State: svn.StateModified},
	}
	before := stamper(root, "42", items)

	s := newSessionStore()
	src := diffKey{path: "src", dir: true}
	doc := diffKey{path: "doc", dir: true}
	wcRoot := diffKey{path: fileTreeRoot, dir: true}
	for _, k := range []diffKey{src, doc, wcRoot} {
		s.PutDiff(k, diffEntry{text: "@@ combined @@", stamp: before(k)})
	}

	// A new file appears under src/ — no existing item changed, but the combined
	// diff of src/ (and of the whole working copy) did.
	grown := append(items, svn.StatusItem{Path: "src/c.go", State: svn.StateUnversioned})
	after := stamper(root, "42", grown)

	if cachedDiff(s, src, after) {
		t.Error("src/ gained a file; its combined diff should have been dropped")
	}
	if cachedDiff(s, wcRoot, after) {
		t.Error(`the "/" root spans the whole working copy; its diff should have been dropped`)
	}
	if !cachedDiff(s, doc, after) {
		t.Error("nothing under doc/ moved; its diff should have been kept")
	}
}

// TestSessionForgetsFailureAfterTTL keeps a failed diff from being retried on
// every keypress, while still letting a transient failure clear itself.
func TestSessionForgetsFailureAfterTTL(t *testing.T) {
	root := stampRoot(t, map[string]string{"a.txt": "alpha"})
	items := []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}}
	stamp := stamper(root, "42", items)

	s := newSessionStore()
	k := diffKey{path: "a.txt"}
	s.PutDiff(k, diffEntry{text: "Unable to load diff: kaboom", failed: true, stamp: stamp(k)})
	if !cachedDiff(s, k, stamp) {
		t.Fatal("a fresh failure should be remembered, so the path is not re-run on every keypress")
	}

	// Planted straight into the cache: PutDiff would re-date the expiry.
	s.diffs.Put(k, diffEntry{
		text:    "Unable to load diff: kaboom",
		failed:  true,
		stamp:   stamp(k),
		expires: time.Now().Add(-time.Second),
	})
	if cachedDiff(s, k, stamp) {
		t.Error("an expired failure should be forgotten so the diff is tried again")
	}
}

// TestStatusLoadLeavesTheCacheAlone locks in what makes a routine reload cheap:
// with live refresh polling, a status arrival must not walk the cache. Entries
// are revalidated as they are read instead, so what survives is unchanged and a
// stale entry is still never served.
func TestStatusLoadLeavesTheCacheAlone(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
		{Path: "beta.txt", State: svn.StateModified},
	})
	next, _ := m.Update(diffLoadedMsg{path: "alpha.txt", diff: "@@ -1 +1 @@\n+alpha body"})
	m = next.(*Model)
	beta := diffKey{path: "beta.txt"}
	m.session.PutDiff(beta, diffEntry{text: "@@ -1 +1 @@\n+beta body", stamp: m.diffStamp(beta)})

	// beta is staged; nothing about alpha moved.
	next, _ = m.Update(statusLoadedMsg{items: []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
		{Path: "beta.txt", State: svn.StateModified, Changelist: stagedChangelist},
	}})
	m = next.(*Model)

	if n := m.session.diffs.Len(); n != 2 {
		t.Errorf("session holds %d diffs after a status load, want both left where they were", n)
	}
	alpha := diffKey{path: "alpha.txt"}
	if _, ok := m.session.Diff(alpha, m.diffStamp(alpha)); !ok {
		t.Error("a path the status did not touch should still be served")
	}
	if _, ok := m.session.Diff(beta, m.diffStamp(beta)); ok {
		t.Error("a path the status moved should be dropped as it is read")
	}
}

func TestSessionCloseIsIdempotent(t *testing.T) {
	root := stampRoot(t, map[string]string{"a.txt": "alpha"})
	items := []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}}
	stamp := stamper(root, "42", items)

	s := newSessionStore()
	k := diffKey{path: "a.txt"}
	s.PutDiff(k, diffEntry{text: "@@ alpha @@", stamp: stamp(k)})

	s.Close()
	s.Close()
	if s.diffs.Len() != 0 {
		t.Errorf("diffs.Len() = %d after Close, want 0", s.diffs.Len())
	}
	if cachedDiff(s, k, stamp) {
		t.Error("a closed session should hold nothing")
	}
}
