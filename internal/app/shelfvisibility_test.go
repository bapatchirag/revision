package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bapatchirag/revision/internal/shelf"
	"github.com/bapatchirag/revision/internal/svn"
)

// leakedStatus is the status report svn gives once a user's global-ignores has
// replaced the built-in list and the shelf store stops being ignored.
func leakedStatus() []svn.StatusItem {
	return []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified},
		{Path: shelf.DirName, State: svn.StateUnversioned},
		{Path: shelf.DirName + "/20260819-1/meta.json", State: svn.StateUnversioned},
		{Path: "b.txt", State: svn.StateModified},
	}
}

func TestUnderShelfStoreMatchesTheStoreAndItsContents(t *testing.T) {
	cases := map[string]bool{
		shelf.DirName:                       true,
		shelf.DirName + "/e1":               true,
		shelf.DirName + "/e1/changes.patch": true,
		"src/" + shelf.DirName:              false,
		".#revision-shelves-backup":         false,
		"a.txt":                             false,
		"":                                  false,
	}
	for path, want := range cases {
		if got := underShelfStore(path); got != want {
			t.Errorf("underShelfStore(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestShelfStoreNeverReachesTheStatusModel(t *testing.T) {
	m := loadItems(t, sizedModel(t), leakedStatus())

	for _, it := range m.fileItems {
		if underShelfStore(it.Path) {
			t.Errorf("%q reached the status model; the store is revision's own bookkeeping", it.Path)
		}
	}
	if got := len(m.fileItems); got != 2 {
		t.Errorf("fileItems = %d, want the two real changes", got)
	}
	// The notice names the store, so it has to be out of the way before the
	// panels can be checked for it.
	m.dismissToast()
	if view := stripANSI(m.View()); strings.Contains(view, shelf.DirName) {
		t.Errorf("the store should not be on screen:\n%s", view)
	}
}

func TestShelfStoreIsKeptOutOfEveryChangeSet(t *testing.T) {
	m := loadItems(t, sizedModel(t), leakedStatus())

	// Nothing that acts on the working copy may pick the store up: not shelving,
	// not the changelist buckets the Changelists view is built from.
	for _, it := range shelvableItems(m.fileItems) {
		if underShelfStore(it.Path) {
			t.Errorf("%q is shelvable, want the store left out", it.Path)
		}
	}
	for _, g := range groupChangelists(m.fileItems) {
		for _, it := range g.Items {
			if underShelfStore(it.Path) {
				t.Errorf("%q is in changelist %q, want the store left out", it.Path, g.Name)
			}
		}
	}
}

func TestVisibleShelfStoreIsReportedOnce(t *testing.T) {
	m := loadItems(t, sizedModel(t), leakedStatus())

	got := m.toast.Message()
	if !strings.Contains(got, shelf.DirName) || !strings.Contains(got, "global-ignores") {
		t.Fatalf("toast = %q, want it to name the store and the setting behind it", got)
	}
	if !m.shelfStoreWarned {
		t.Error("the notice should be marked as given")
	}

	// A second reload must not say it again: it is a standing configuration, not
	// an event. The toast keeps its text once dismissed, so what is checked is
	// whether it was raised again.
	m.dismissToast()
	m = loadItems(t, m, leakedStatus())
	if m.showingToast {
		t.Errorf("toast = %q, want the notice given only once a session", m.toast.Message())
	}
}

func TestAnIgnoredShelfStoreIsNotWorthMentioning(t *testing.T) {
	// svn reports nothing about the store under a default configuration, which is
	// the whole point of the name.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}})

	if m.shelfStoreWarned {
		t.Error("nothing leaked, so nothing should have been said")
	}
	if m.showingToast {
		t.Errorf("toast = %q, want no notice", m.toast.Message())
	}
}

func TestRejectScanSkipsTheShelfStore(t *testing.T) {
	root := t.TempDir()
	// A shelved patch that happens to hold a reject-looking name is the store's
	// business, not the working copy's.
	inStore := filepath.Join(root, shelf.DirName, "20260819-1", "untracked", "stale.rej")
	if err := os.MkdirAll(filepath.Dir(inStore), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(inStore, []byte("hunk"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	real := filepath.Join(root, "src", "a.txt.svnpatch.rej")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(real, []byte("hunk"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := scanRejects(root)
	if err != nil {
		t.Fatalf("scanRejects: %v", err)
	}
	if len(got) != 1 || got[0].Rel != "src/a.txt.svnpatch.rej" {
		t.Errorf("scanRejects = %+v, want only the working copy's own reject", got)
	}
}
