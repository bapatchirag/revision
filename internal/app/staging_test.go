package app

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
)

func TestStageTargetDecision(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "mod.go", State: svn.StateModified},
		{Path: "staged.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})

	// Cursor starts on the first (path-sorted) item: an unstaged change → stage.
	if act, ok := m.stageTarget(); !ok || act.path != "mod.go" || act.add || !act.stage {
		t.Errorf("mod.go: got %+v (ok=%v), want {mod.go add:false stage:true}", act, ok)
	}

	// Move to the already-staged file → unstage.
	m.files.Update(tea.KeyMsg{Type: tea.KeyDown})
	if act, ok := m.stageTarget(); !ok || act.path != "staged.go" || act.stage {
		t.Errorf("staged.go: got %+v (ok=%v), want {staged.go stage:false}", act, ok)
	}

	// Move to the unversioned file → add and stage in one step.
	m.files.Update(tea.KeyMsg{Type: tea.KeyDown})
	if act, ok := m.stageTarget(); !ok || act.path != "untracked.txt" || !act.add || !act.stage {
		t.Errorf("untracked.txt: got %+v (ok=%v), want {untracked.txt add:true stage:true}", act, ok)
	}
}

func TestSpaceStagesSelectedFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	// Space on a stageable file (Files panel is focused by default) yields a
	// command; it runs svn, so we assert only that it exists, not its result.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected a stage command for a modified file")
	}
}

func TestSpaceAddsUnversionedFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})
	if act, ok := m.stageTarget(); !ok || !act.add {
		t.Errorf("unversioned stage target should svn add first, got %+v (ok=%v)", act, ok)
	}
	// An unversioned file is now addable: space produces an add+stage command.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected an add+stage command for an unversioned file")
	}
}

func TestSpaceIgnoresIgnoredFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "build.log", State: svn.StateIgnored},
	})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd != nil {
		t.Error("an ignored file should not produce a stage command")
	}
}

// selectDirRow parks the Files-panel cursor on the tree row for the named
// directory segment, failing the test when no such directory row exists.
func selectDirRow(t *testing.T, m *Model, name string) {
	t.Helper()
	for i, n := range m.files.Items() {
		if n.Item == nil && n.Name == name {
			m.files.SetIndex(i)
			return
		}
	}
	t.Fatalf("no directory row named %q in the file tree", name)
}

func TestSpaceStagesDirectory(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
		{Path: "readme.md", State: svn.StateModified},
	})
	// The cursor opens on the first file; move it onto the src/ directory row.
	selectDirRow(t, m, "src")
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected a stage command when space is pressed on a directory row")
	}
}

func TestSpaceStagesRootDirectory(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "readme.md", State: svn.StateModified},
	})
	m.files.SetIndex(0) // the synthetic "/" root row covers the whole tree
	root, ok := m.files.Selected()
	if !ok || root.Path != fileTreeRoot {
		t.Fatalf("expected cursor on the / root row, got %+v (ok=%v)", root, ok)
	}
	if acts := directoryStageActions(root, m.fileItems); len(acts) != 2 {
		t.Errorf("root should stage all 2 files, got %d: %+v", len(acts), acts)
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected a stage command when space is pressed on the root row")
	}
}

func TestSpaceOnFullyStagedDirectoryUnstages(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified, Changelist: stagedChangelist},
		{Path: "src/b.go", State: svn.StateModified, Changelist: stagedChangelist},
	})
	selectDirRow(t, m, "src")
	acts := directoryUnstageActions(fileNode{Name: "src", Path: "src"}, m.fileItems)
	if len(acts) != 2 {
		t.Fatalf("expected 2 unstage actions under src/, got %d: %+v", len(acts), acts)
	}
	for _, a := range acts {
		if a.stage || a.add {
			t.Errorf("an unstage action should neither stage nor add, got %+v", a)
		}
	}
	// Everything under src/ is already staged, so pressing space again unstages
	// the whole subtree.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected an unstage command when a fully staged directory is toggled")
	}
}

func TestSpaceOnDirectoryInNamedChangelistsRemovesThem(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "src/b.go", State: svn.StateModified, Changelist: "bugfix"},
	})
	selectDirRow(t, m, "src")
	acts := directoryUnstageActions(fileNode{Name: "src", Path: "src"}, m.fileItems)
	if len(acts) != 2 {
		t.Fatalf("expected 2 removals under src/, got %d: %+v", len(acts), acts)
	}
	for _, a := range acts {
		if a.stage || a.add {
			t.Errorf("a removal should neither stage nor add, got %+v", a)
		}
	}
	// Every file under src/ already belongs to a named changelist; nothing is left
	// to stage, so space removes them all from their changelists.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected a command removing named-changelist files under the directory")
	}
}

func TestSpaceOnDirectoryWithNothingToToggle(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		// An ignored file can be neither staged nor removed from a changelist, so the
		// directory toggle has nothing to do.
		{Path: "src/build.log", State: svn.StateIgnored},
	})
	selectDirRow(t, m, "src")
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd != nil {
		t.Error("a directory with only ignored files should produce no command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "nothing to stage or unstage under src/") {
		t.Errorf("expected a nothing-to-toggle toast, got:\n%s", view)
	}
}

func TestDirectoryStageActionsSelectsStageableFiles(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/new.txt", State: svn.StateUnversioned},
		{Path: "src/staged.go", State: svn.StateModified, Changelist: stagedChangelist},
		{Path: "src/named.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "src/build.log", State: svn.StateIgnored},
		{Path: "docs/readme.md", State: svn.StateModified},
	}
	acts := directoryStageActions(fileNode{Name: "src", Path: "src"}, items)

	byPath := map[string]stageAction{}
	for _, a := range acts {
		byPath[a.path] = a
	}
	if len(acts) != 2 {
		t.Fatalf("expected 2 stage actions under src/, got %d: %+v", len(acts), acts)
	}
	if a, ok := byPath["src/a.go"]; !ok || a.add || !a.stage {
		t.Errorf("src/a.go: got %+v (ok=%v), want a plain stage", a, ok)
	}
	if a, ok := byPath["src/new.txt"]; !ok || !a.add || !a.stage {
		t.Errorf("src/new.txt: got %+v (ok=%v), want add+stage", a, ok)
	}
	if _, ok := byPath["src/staged.go"]; ok {
		t.Error("an already-staged file should be skipped")
	}
	if _, ok := byPath["src/named.go"]; ok {
		t.Error("a file in a named changelist should be skipped")
	}
	if _, ok := byPath["src/build.log"]; ok {
		t.Error("an ignored file should be skipped")
	}
	if _, ok := byPath["docs/readme.md"]; ok {
		t.Error("a file outside src/ should be excluded")
	}
}

func TestDirectoryUnstageActionsSelectsChangelistFiles(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified, Changelist: stagedChangelist},
		{Path: "src/b.go", State: svn.StateModified},
		{Path: "src/named.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "docs/c.go", State: svn.StateModified, Changelist: stagedChangelist},
	}
	acts := directoryUnstageActions(fileNode{Name: "src", Path: "src"}, items)

	byPath := map[string]stageAction{}
	for _, a := range acts {
		byPath[a.path] = a
	}
	if len(acts) != 2 {
		t.Fatalf("expected 2 unstage actions under src/, got %d: %+v", len(acts), acts)
	}
	if a, ok := byPath["src/a.go"]; !ok || a.stage || a.add {
		t.Errorf("src/a.go (staged): got %+v (ok=%v), want a plain unstage", a, ok)
	}
	if a, ok := byPath["src/named.go"]; !ok || a.stage || a.add {
		t.Errorf("src/named.go (named changelist): got %+v (ok=%v), want a plain unstage", a, ok)
	}
	if _, ok := byPath["src/b.go"]; ok {
		t.Error("a file in no changelist has nothing to remove and should be skipped")
	}
	if _, ok := byPath["docs/c.go"]; ok {
		t.Error("a file outside src/ should be excluded")
	}
}

func TestStagedFileShowsMarker(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "staged.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	if view := stripANSI(m.View()); !strings.Contains(view, "●") {
		t.Errorf("expected a staged marker in the files list, got:\n%s", view)
	}
}

// pressAdd puts the current Files selection under version control, failing the
// test when the keypress produces no command to run.
func pressAdd(t *testing.T, m *Model) (*Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd == nil {
		t.Fatal("expected an add command")
	}
	return next.(*Model), cmd
}

// TestAddVersionsAnUntrackedFileAndNothingElse pins what the key does: it puts
// the file under version control, leaves its changelist alone, and shows both on
// the same frame as the keypress.
func TestAddVersionsAnUntrackedFileAndNothingElse(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})
	m, _ = pressAdd(t, m)

	want := svn.StatusItem{Path: "untracked.txt", State: svn.StateAdded}
	if got := itemState(t, m, "untracked.txt"); got != want {
		t.Errorf("status item = %+v, want %+v — an add must not file it into a changelist", got, want)
	}
	if m.optimistic == nil {
		t.Error("expected the pre-add status to be kept for a rollback")
	}
}

// TestAddRollsBackWhenSvnRefuses is the other half of the optimistic update: a
// path svn would not take goes back to untracked rather than sitting on screen
// as added.
func TestAddRollsBackWhenSvnRefuses(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})
	m, _ = pressAdd(t, m)
	token := m.optimisticTok

	next, cmd := m.Update(addedMsg{outcome: singleOutcome("untracked.txt", errors.New("locked")), token: token})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a status reload after a refused add")
	}
	if got := itemState(t, m, "untracked.txt"); got.State != svn.StateUnversioned {
		t.Errorf("status item = %+v, want the row put back as untracked", got)
	}
	if m.optimistic != nil {
		t.Error("a settled change should no longer hold a rollback snapshot")
	}
}

// TestAddRefusesAnythingAlreadyTracked pins the guard: only untracked paths are
// addable, so a modified file and an ignored one both say so and run nothing.
func TestAddRefusesAnythingAlreadyTracked(t *testing.T) {
	for _, tc := range []struct {
		name string
		item svn.StatusItem
		code string
	}{
		{"modified", svn.StatusItem{Path: "mod.go", State: svn.StateModified}, "M"},
		{"ignored", svn.StatusItem{Path: "build.log", State: svn.StateIgnored}, "I"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := loadItems(t, sizedModel(t), []svn.StatusItem{tc.item})
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
			if cmd != nil {
				t.Fatal("expected no add command for an already-tracked path")
			}
			m = next.(*Model)
			want := "can't add " + tc.item.Path + " (" + tc.code + ")"
			if view := stripANSI(m.View()); !strings.Contains(view, want) {
				t.Errorf("expected a %q toast, got:\n%s", want, view)
			}
			if m.optimistic != nil {
				t.Error("a refused keypress must not move the model")
			}
		})
	}
}

// TestAddOnADirectoryRowVersionsEverythingUntracked covers the directory fan-out
// and, with it, that the tracked files beneath are left where they are.
func TestAddOnADirectoryRowVersionsEverythingUntracked(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/new1.txt", State: svn.StateUnversioned},
		{Path: "src/new2.txt", State: svn.StateUnversioned},
	})
	selectDirRow(t, m, "src")
	m, _ = pressAdd(t, m)

	for _, p := range []string{"src/new1.txt", "src/new2.txt"} {
		if got := itemState(t, m, p); got.State != svn.StateAdded {
			t.Errorf("%s = %+v, want it scheduled for addition", p, got)
		}
	}
	if got := itemState(t, m, "src/a.go"); got.State != svn.StateModified {
		t.Errorf("src/a.go = %+v, want the tracked file left alone", got)
	}
}

func TestAddOnADirectoryWithNothingUntracked(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/build.log", State: svn.StateIgnored},
	})
	selectDirRow(t, m, "src")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd != nil {
		t.Fatal("a directory with nothing untracked should produce no command")
	}
	if view := stripANSI(next.(*Model).View()); !strings.Contains(view, "nothing to add under src/") {
		t.Errorf("expected a nothing-to-add toast, got:\n%s", view)
	}
}

func TestDirectoryAddPathsSelectsUntrackedPaths(t *testing.T) {
	items := []svn.StatusItem{
		// svn reports an untracked directory as one entry; when it also reports
		// children, the entry renders as the directory row rather than a leaf, so
		// the row's own path has to be named or svn is asked to add files whose
		// parent it does not track yet.
		{Path: "src", State: svn.StateUnversioned},
		{Path: "src/new.txt", State: svn.StateUnversioned},
		{Path: "src/mod.go", State: svn.StateModified},
		{Path: "src/build.log", State: svn.StateIgnored},
		{Path: "docs/other.txt", State: svn.StateUnversioned},
	}
	got := directoryAddPaths(fileNode{Name: "src", Path: "src"}, items)

	want := []string{"src", "src/new.txt"}
	if len(got) != len(want) {
		t.Fatalf("directoryAddPaths = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("directoryAddPaths = %q, want %q (the row's own entry first, so svn adds the parent before its children)", got, want)
		}
	}
}

// TestDirectoryAddPathsCoversTheRootRow pins that the synthetic "/" row means the
// whole tree, as it does for staging and reverting.
func TestDirectoryAddPathsCoversTheRootRow(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/new.txt", State: svn.StateUnversioned},
		{Path: "readme.md", State: svn.StateUnversioned},
		{Path: "mod.go", State: svn.StateModified},
	}
	if got := directoryAddPaths(fileNode{Name: "/", Path: fileTreeRoot}, items); len(got) != 2 {
		t.Errorf("root row = %q, want both untracked files across the tree", got)
	}
}

// TestAddSkipsRowsAlreadyWaitingOnSvn keeps the key in line with the rest: a row
// in flight is not acted on a second time.
func TestAddSkipsRowsAlreadyWaitingOnSvn(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})
	m.markPending(pendingDelete, 7, []string{"untracked.txt"})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}); cmd != nil {
		t.Error("a row waiting on svn should produce no add command")
	}
}
