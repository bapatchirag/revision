package app

import (
	"strings"
	"testing"

	"github.com/bapatchirag/revision/internal/svn"
)

func TestRevertedStatus(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "keep.txt", State: svn.StateModified},
		{Path: "src/added.txt", State: svn.StateAdded, Changelist: "revision:staged"},
		{Path: "src/copied.txt", State: svn.StateAdded, Copied: true},
		{Path: "src/deleted.txt", State: svn.StateDeleted},
		{Path: "src/edited.txt", State: svn.StateModified},
		{Path: "src/missing.txt", State: svn.StateMissing},
		{Path: "src/replaced.txt", State: svn.StateReplaced},
	}
	done := []string{
		"src/added.txt", "src/copied.txt", "src/deleted.txt",
		"src/edited.txt", "src/missing.txt", "src/replaced.txt",
	}
	got, changed := revertedStatus(items, done)
	if !changed {
		t.Fatal("a revert over six rows must report a change")
	}
	want := []svn.StatusItem{
		{Path: "keep.txt", State: svn.StateModified},
		{Path: "src/added.txt", State: svn.StateUnversioned, PropState: svn.StateNone},
	}
	if len(got) != len(want) {
		t.Fatalf("revertedStatus = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestRevertedStatusDropsWhatAnUntrackedDirectoryHides pins the shape a naive
// rewrite gets wrong: reverting an added tree leaves the whole thing on disk
// untracked, and svn does not look inside an untracked directory — so it reports
// the head of the tree and nothing under it, versioned or not.
func TestRevertedStatusDropsWhatAnUntrackedDirectoryHides(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "tree", State: svn.StateAdded},
		{Path: "tree/a.txt", State: svn.StateAdded},
		{Path: "tree/nested/b.txt", State: svn.StateAdded},
		{Path: "tree/scratch.log", State: svn.StateUnversioned},
	}
	got, _ := revertedStatus(items, []string{"tree", "tree/a.txt", "tree/nested/b.txt"})
	if len(got) != 1 || got[0].Path != "tree" || got[0].State != svn.StateUnversioned {
		t.Errorf("revertedStatus = %+v, want the one untracked row at the head of the tree", got)
	}
}

// TestRevertedStatusUnlinksTheHalfOfAMoveLeftBehind pins what svn does to the
// row the revert did not name: the link goes with the scheduled add, and a row
// still pointing at it would name a file that is no longer going anywhere.
func TestRevertedStatusUnlinksTheHalfOfAMoveLeftBehind(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.txt", State: svn.StateDeleted, MovedTo: "src/renamed.txt"},
		{Path: "src/renamed.txt", State: svn.StateAdded, Copied: true, MovedFrom: "src/a.txt"},
	}
	got, changed := revertedStatus(items, []string{"src/renamed.txt"})
	if !changed {
		t.Fatal("unlinking the surviving half is a change")
	}
	want := svn.StatusItem{Path: "src/a.txt", State: svn.StateDeleted}
	if len(got) != 1 || got[0] != want {
		t.Errorf("revertedStatus = %+v, want just %+v", got, want)
	}
}

func TestRevertedStatusLeavesAnUntouchedStatusAlone(t *testing.T) {
	items := []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}}
	if _, changed := revertedStatus(items, nil); changed {
		t.Error("a revert that discarded nothing changes nothing")
	}
	if _, changed := revertedStatus(items, []string{"b.txt"}); changed {
		t.Error("a revert of a path svn status never reported changes nothing")
	}
}

// TestARevertChangesTheTreeBeforeTheReadConfirmsIt is the whole point of
// settling: the rows go as the toast appears, not when the svn process that
// confirms them finally answers.
func TestARevertChangesTheTreeBeforeTheReadConfirmsIt(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/added.txt", State: svn.StateAdded},
		{Path: "src/edited.txt", State: svn.StateModified},
		{Path: "src/kept.txt", State: svn.StateModified},
	})
	client, _ := recordingClient(t)
	m.client = client

	// The reply is handled but its reload command is deliberately never run.
	next, cmd := m.Update(revertedMsg{
		outcome: batchOutcome{done: []string{"src/added.txt", "src/edited.txt"}},
	})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("a revert must still ask svn to confirm what it settled")
	}

	var got []string
	for _, it := range m.fileItems {
		got = append(got, it.Path+":"+string(it.State))
	}
	want := "src/added.txt:unversioned src/kept.txt:modified"
	if strings.Join(got, " ") != want {
		t.Fatalf("fileItems = %q, want %q", strings.Join(got, " "), want)
	}
	if view := stripANSI(m.View()); strings.Contains(view, "edited.txt") {
		t.Errorf("the reverted file is still on screen:\n%s", view)
	}
}

// TestARevertLeavesARefusedPathOnScreen pins the other half: only the paths svn
// reported it discarded are settled, so a row it refused stays as it was until
// the read says otherwise.
func TestARevertLeavesARefusedPathOnScreen(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.txt", State: svn.StateModified},
		{Path: "src/b.txt", State: svn.StateModified},
	})
	client, _ := recordingClient(t)
	m.client = client

	out := batchOutcome{done: []string{"src/a.txt"}}
	out.fail("src/b.txt", errSkipped)
	next, _ := m.Update(revertedMsg{outcome: out})
	m = next.(*Model)

	if len(m.fileItems) != 1 || m.fileItems[0].Path != "src/b.txt" {
		t.Errorf("fileItems = %+v, want only the refused path left", m.fileItems)
	}
}
