package app

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

func TestRevertRequiresConfirmation(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	// r on a dirty file opens the confirmation modal rather than acting.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("expected the confirmation modal to open")
	}
	if cmd != nil {
		t.Error("opening the modal should not run a command yet")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Revert changes?") {
		t.Errorf("expected the revert prompt, got:\n%s", view)
	}

	// Confirming emits ConfirmMsg, which the app turns into the revert command.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a ConfirmMsg command from the modal")
	}
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	next, cmd = m.Update(conf)
	m = next.(*Model)
	if m.confirming {
		t.Error("the modal should close after confirming")
	}
	if cmd == nil {
		t.Error("expected a revert command after confirming")
	}
}

func TestRevertGuardOnUnversioned(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(*Model)
	if m.confirming {
		t.Error("an unversioned file has nothing to revert; no modal should open")
	}
	if cmd != nil {
		t.Error("the revert guard should not run a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "nothing to revert") {
		t.Errorf("expected a guard toast, got:\n%s", view)
	}
}

func TestRevertDirectoryRequiresConfirmation(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	// Single-file revert returns nil on a directory row, so an opened modal here
	// proves the directory fan-out ran.
	selectDirRow(t, m, "src")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("r on a directory row should open the confirmation modal")
	}
	if cmd != nil {
		t.Error("opening the modal should not run a command yet")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Revert changes?") {
		t.Errorf("expected the revert prompt, got:\n%s", view)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	if _, cmd = m.Update(conf); cmd == nil {
		t.Error("expected a revert command after confirming a directory revert")
	}
}

func TestDirectoryRevertPathsSelectsDirtyFiles(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/gone.go", State: svn.StateDeleted},
		{Path: "src/new.txt", State: svn.StateUnversioned},
		{Path: "src/build.log", State: svn.StateIgnored},
		{Path: "docs/readme.md", State: svn.StateModified},
	}
	paths := directoryRevertPaths(fileNode{Name: "src", Path: "src"}, items)

	got := map[string]bool{}
	for _, p := range paths {
		got[p] = true
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 revertable paths under src/, got %d: %v", len(paths), paths)
	}
	if !got["src/a.go"] || !got["src/gone.go"] {
		t.Errorf("expected the modified and deleted files, got %v", paths)
	}
	if got["src/new.txt"] {
		t.Error("an unversioned file has nothing to revert and should be skipped")
	}
	if got["src/build.log"] {
		t.Error("an ignored file should be skipped")
	}
	if got["docs/readme.md"] {
		t.Error("a file outside src/ should be excluded")
	}
}

// TestDirectoryRevertPathsIncludesTheDirectoryItself covers an added directory:
// svn tracks it as a change of its own, but the tree renders it as the directory
// row rather than a leaf, so it has to come along with its children or the
// revert strands it.
func TestDirectoryRevertPathsIncludesTheDirectoryItself(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "mpte", State: svn.StateAdded},
		{Path: "mpte/Makefile", State: svn.StateAdded},
		{Path: "mpte/rust-crate/src/lib.rs", State: svn.StateAdded},
		{Path: "mpte-compute/notes.md", State: svn.StateModified},
	}
	paths := directoryRevertPaths(fileNode{Name: "mpte", Path: "mpte"}, items)

	want := []string{"mpte", "mpte/Makefile", "mpte/rust-crate/src/lib.rs"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths = %v, want %v (the row's own path first)", paths, want)
		}
	}
}

func TestRevertDirectoryNothingToRevert(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/new.txt", State: svn.StateUnversioned},
		{Path: "src/build.log", State: svn.StateIgnored},
	})
	selectDirRow(t, m, "src")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(*Model)
	if m.confirming {
		t.Error("a directory with nothing revertable should not open the modal")
	}
	if cmd != nil {
		t.Error("the guard should not run a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "nothing to revert under src/") {
		t.Errorf("expected a nothing-to-revert toast, got:\n%s", view)
	}
}

func TestDeleteConfirmationCancels(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("d should open the delete confirmation")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Delete file?") {
		t.Errorf("expected the delete prompt, got:\n%s", view)
	}

	// Esc emits DismissMsg; the app closes the modal and runs nothing.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	dis, ok := cmd().(uimsg.DismissMsg)
	if !ok {
		t.Fatalf("expected DismissMsg, got %T", cmd())
	}
	next, cmd = m.Update(dis)
	m = next.(*Model)
	if m.confirming {
		t.Error("the modal should close on cancel")
	}
	if cmd != nil {
		t.Error("cancelling delete should not run a command")
	}
}

func TestDeleteUnversionedWarnsDiskRemoval(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	view := stripANSI(m.View())
	if !strings.Contains(view, "untracked") || !strings.Contains(view, "disk") {
		t.Errorf("expected an unversioned-delete warning, got:\n%s", view)
	}
}

func TestDeleteDirectoryRequiresConfirmation(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateDeleted},
	})
	selectDirRow(t, m, "src")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("d on a directory row should open the delete confirmation")
	}
	if cmd != nil {
		t.Error("opening the modal should not run a command yet")
	}
	// The plural title distinguishes the directory prompt from the single-file one.
	if view := stripANSI(m.View()); !strings.Contains(view, "Delete files?") {
		t.Errorf("expected the directory delete prompt, got:\n%s", view)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	if _, cmd = m.Update(conf); cmd == nil {
		t.Error("expected a delete command after confirming a directory delete")
	}
}

func TestDirectoryDeleteActionsSkipsIgnored(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/new.txt", State: svn.StateUnversioned},
		{Path: "src/build.log", State: svn.StateIgnored},
		{Path: "docs/readme.md", State: svn.StateModified},
	}
	acts := directoryDeleteActions(fileNode{Name: "src", Path: "src"}, items)

	byPath := map[string]deleteAction{}
	for _, a := range acts {
		byPath[a.path] = a
	}
	if len(acts) != 2 {
		t.Fatalf("expected 2 delete actions under src/, got %d: %+v", len(acts), acts)
	}
	if a, ok := byPath["src/a.go"]; !ok || a.unversioned {
		t.Errorf("src/a.go: got %+v (ok=%v), want a versioned delete", a, ok)
	}
	if a, ok := byPath["src/new.txt"]; !ok || !a.unversioned {
		t.Errorf("src/new.txt: got %+v (ok=%v), want an unversioned (disk) delete", a, ok)
	}
	if _, ok := byPath["src/build.log"]; ok {
		t.Error("an ignored file should be skipped")
	}
	if _, ok := byPath["docs/readme.md"]; ok {
		t.Error("a file outside src/ should be excluded")
	}
}

func TestDeleteDirectoryMessageSeparatesDiskRemoval(t *testing.T) {
	n := fileNode{Name: "src", Path: "src"}
	mixed := deleteDirectoryMessage(n, []deleteAction{
		{path: "src/a.go"},
		{path: "src/new.txt", unversioned: true},
	})
	if !strings.Contains(mixed, "scheduled for deletion") || !strings.Contains(mixed, "permanently removed from disk") {
		t.Errorf("mixed message should mention both outcomes, got: %q", mixed)
	}

	versioned := deleteDirectoryMessage(n, []deleteAction{{path: "src/a.go"}})
	if !strings.Contains(versioned, "scheduled for deletion") || strings.Contains(versioned, "disk") {
		t.Errorf("versioned-only message should not warn about disk, got: %q", versioned)
	}

	unversioned := deleteDirectoryMessage(n, []deleteAction{{path: "src/new.txt", unversioned: true}})
	if !strings.Contains(unversioned, "permanently removed from disk") || strings.Contains(unversioned, "scheduled") {
		t.Errorf("unversioned-only message should only warn about disk, got: %q", unversioned)
	}
}

func TestDeleteDirectoryNothingToDelete(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/build.log", State: svn.StateIgnored},
	})
	selectDirRow(t, m, "src")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	if m.confirming {
		t.Error("a directory with only ignored files should not open the modal")
	}
	if cmd != nil {
		t.Error("the guard should not run a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "nothing to delete under src/") {
		t.Errorf("expected a nothing-to-delete toast, got:\n%s", view)
	}
}

func TestUpdateRunsCommand(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	// u (off the Log panel) confirms an update to the latest revision; with no
	// conflicts the single confirm runs the update.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("u should open the update confirmation")
	}
	if cmd != nil {
		t.Error("opening the modal should not run a command yet")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Update working copy?") {
		t.Errorf("expected the update confirm, got:\n%s", view)
	}
	m, cmd = confirmModal(t, m)
	if m.confirming {
		t.Error("the modal should close after confirming")
	}
	if cmd == nil {
		t.Error("confirming should run the update command")
	}
}

func TestUpdateToHeadConflictAsksAgain(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "conflicted.go", State: svn.StateConflicted},
	})
	// Default focus is the Files panel, so u targets HEAD rather than a revision.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "Update working copy?") {
		t.Fatalf("expected the default update confirm, got:\n%s", view)
	}

	// Accepting the first confirm surfaces the conflict warning, not the update.
	m, cmd := confirmModal(t, m)
	if cmd != nil {
		t.Error("the update must not run before the conflict confirm is accepted")
	}
	if !m.confirming {
		t.Fatal("a second confirmation should be open when conflicts exist")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Conflicts present") || !strings.Contains(view, "untouched") {
		t.Errorf("expected the conflict warning, got:\n%s", view)
	}

	// Accepting the conflict warning runs the update.
	m, cmd = confirmModal(t, m)
	if m.confirming {
		t.Error("both modals should be closed after the second confirm")
	}
	if cmd == nil {
		t.Error("expected the update command after the second confirm")
	}
}

func TestUpdateResultShowsToast(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updatedMsg{revision: "7"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "updated to r7") {
		t.Errorf("expected the update toast, got:\n%s", view)
	}
}

func TestRevisionIndicatorTracksUpdate(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	// History (newest first) reveals HEAD as r50; the working copy opens at r42.
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{
		{Revision: "50"}, {Revision: "49"}, {Revision: "42"},
	}})
	m = next.(*Model)
	// The Status panel lists the checked-out revision and HEAD on their own rows.
	if view := stripANSI(m.View()); !strings.Contains(view, "Revision  r42") || !strings.Contains(view, "HEAD      r50") {
		t.Errorf("expected the status panel to show r42 and HEAD r50, got:\n%s", view)
	}

	// Updating to HEAD moves the checked-out revision to r50.
	next, _ = m.Update(updatedMsg{revision: "50"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "Revision  r50") {
		t.Errorf("expected the checked-out revision to track to r50, got:\n%s", view)
	}
}

func TestUpdateShowsProgressModal(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	// History reveals HEAD r50; the working copy opens at r42.
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{
		{Revision: "50"}, {Revision: "42"},
	}})
	m = next.(*Model)

	// u (off the Log panel) confirms a HEAD update; with no conflicts the single
	// confirm dispatches it and raises the progress modal. The exact target is
	// only known once svn reports it, so the box names "the latest revision".
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = next.(*Model)
	m, cmd := confirmModal(t, m)
	if !m.updatingWC {
		t.Fatal("expected the updating overlay after confirming")
	}
	if cmd == nil {
		t.Error("expected the update command to run")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Updating from r42 to the latest revision") {
		t.Errorf("expected the progress modal, got:\n%s", view)
	}

	// While the update runs, keys are swallowed so the panels stay put.
	if _, kc := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}); kc != nil {
		t.Error("keys should be ignored while the update runs")
	}

	// Completion clears the overlay.
	next, _ = m.Update(updatedMsg{revision: "50"})
	m = next.(*Model)
	if m.updatingWC {
		t.Error("the overlay should clear when the update completes")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Updating from") {
		t.Errorf("the progress modal should be gone after completion, got:\n%s", view)
	}
}

func TestUpdateToRevisionProgressShowsTarget(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{
		{Revision: "50"}, {Revision: "42"},
	}})
	m = next.(*Model)
	// Focus the Log panel (r50 is the selected row) and update to it with space.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)
	m, _ = confirmModal(t, m)
	// A specific revision is known up front, so the box names it exactly.
	if view := stripANSI(m.View()); !strings.Contains(view, "Updating from r42 to r50") {
		t.Errorf("expected the exact target revision in the progress modal, got:\n%s", view)
	}
}

func TestUpdateToRevisionConfirms(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{
		{Revision: "42", Author: "alice", Message: "first"},
		{Revision: "41", Author: "bob", Message: "second"},
	}})
	m = next.(*Model)

	// Focus the Log panel (key "3"), then space targets the selected revision.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("space on the Log panel should open the confirmation modal")
	}
	if cmd != nil {
		t.Error("opening the modal should not run a command yet")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Update to revision?") || !strings.Contains(view, "r42") {
		t.Errorf("expected the update-to-revision prompt, got:\n%s", view)
	}

	// Confirming turns into the update command.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a ConfirmMsg command from the modal")
	}
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	next, cmd = m.Update(conf)
	m = next.(*Model)
	if m.confirming {
		t.Error("the modal should close after confirming")
	}
	if cmd == nil {
		t.Error("expected an update command after confirming")
	}
}

func TestUpdateToRevisionNoSelectionWarns(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	// Focus the Log panel; with no history there is nothing to select.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)
	if m.confirming {
		t.Error("no revision selected should not open the modal")
	}
	if cmd != nil {
		t.Error("no revision selected should not run a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no revision selected") {
		t.Errorf("expected a guard toast, got:\n%s", view)
	}
}

// focusLogWithRevision loads a single revision, focuses the Log panel, and
// returns the model ready for an update-to-revision keypress.
func focusLogWithRevision(t *testing.T, items []svn.StatusItem) *Model {
	t.Helper()
	m := loadItems(t, sizedModel(t), items)
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{
		{Revision: "42", Author: "alice", Message: "first"},
	}})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	return next.(*Model)
}

// confirmModal presses enter on the open confirmation modal and feeds the
// resulting ConfirmMsg back into the model, returning the follow-up command.
func confirmModal(t *testing.T, m *Model) (*Model, tea.Cmd) {
	t.Helper()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("the modal should emit a command on enter")
	}
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	next, out := m.Update(conf)
	return next.(*Model), out
}

func TestUpdateToRevisionConflictAsksAgain(t *testing.T) {
	m := focusLogWithRevision(t, []svn.StatusItem{
		{Path: "conflicted.go", State: svn.StateConflicted},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "Update to revision?") {
		t.Fatalf("expected the default update confirm, got:\n%s", view)
	}

	// Accepting the first confirm surfaces the conflict warning rather than
	// running the update.
	m, cmd := confirmModal(t, m)
	if cmd != nil {
		t.Error("the update must not run before the conflict confirm is accepted")
	}
	if !m.confirming {
		t.Fatal("a second confirmation should be open when conflicts exist")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Conflicts present") || !strings.Contains(view, "untouched") {
		t.Errorf("expected the conflict warning, got:\n%s", view)
	}

	// Accepting the conflict warning finally runs the update.
	m, cmd = confirmModal(t, m)
	if m.confirming {
		t.Error("both modals should be closed after the second confirm")
	}
	if cmd == nil {
		t.Error("expected the update command after the second confirm")
	}
}

func TestUpdateToRevisionNoConflictSkipsSecondConfirm(t *testing.T) {
	m := focusLogWithRevision(t, []svn.StatusItem{
		{Path: "clean.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)

	// With no conflicts the single confirm runs the update directly.
	m, cmd := confirmModal(t, m)
	if m.confirming {
		t.Error("a clean working copy should not open a second confirmation")
	}
	if cmd == nil {
		t.Error("expected the update command straight after the only confirm")
	}
}

func TestUpdateToRevisionConflictCancelAborts(t *testing.T) {
	m := focusLogWithRevision(t, []svn.StatusItem{
		{Path: "conflicted.go", State: svn.StateConflicted},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)
	m, _ = confirmModal(t, m) // accept the first confirm to reach the warning
	if !m.confirming {
		t.Fatal("expected the conflict confirmation to be open")
	}

	// Cancelling the conflict warning drops the staged update entirely.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should emit a dismiss command")
	}
	dis, ok := cmd().(uimsg.DismissMsg)
	if !ok {
		t.Fatalf("expected DismissMsg, got %T", cmd())
	}
	next, _ = m.Update(dis)
	m = next.(*Model)
	if m.confirming {
		t.Error("cancelling should close the modal")
	}
	if m.pending != nil {
		t.Error("cancelling must clear the pending update")
	}
	if m.updateConflictPrompt != "" {
		t.Error("cancelling must clear the staged conflict prompt")
	}
}

func TestRevertResultShowsToast(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, cmd := m.Update(revertedMsg{outcome: singleOutcome("modified.go", nil)})
	m = next.(*Model)
	if cmd == nil {
		t.Error("a revert should trigger a status reload")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "reverted modified.go") {
		t.Errorf("expected the revert toast, got:\n%s", view)
	}
}

// TestPartialRevertStillReloadsStatus pins what a fan-out failure leaves behind:
// a revert acts on each path on its own, so a run that refused one has still
// discarded the changes to the rest. Reporting the failure and stopping there
// leaves the Files panel showing files that are no longer modified.
func TestPartialRevertStillReloadsStatus(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "gone.go", State: svn.StateModified},
		{Path: "stuck.go", State: svn.StateModified},
	})
	var out batchOutcome
	out.ok("gone.go")
	out.add("stuck.go", errors.New("svn: E155010"))

	next, cmd := m.Update(revertedMsg{outcome: out})
	m = next.(*Model)

	if cmd == nil {
		t.Fatal("the paths that did revert have to be re-read, or the panel goes stale")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "stuck.go") {
		t.Errorf("expected the refused path named, got:\n%s", view)
	}
	if !strings.Contains(view, "reverted 1 file") {
		t.Errorf("expected what did land reported too, got:\n%s", view)
	}
}

// TestPartialDeleteStillReloadsStatus is the same guarantee for a delete.
func TestPartialDeleteStillReloadsStatus(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "gone.go", State: svn.StateModified},
		{Path: "stuck.go", State: svn.StateModified},
	})
	var out batchOutcome
	out.ok("gone.go")
	out.add("stuck.go", errors.New("svn: E155007"))

	if _, cmd := m.Update(deletedMsg{outcome: out}); cmd == nil {
		t.Fatal("the paths that were deleted have to be re-read, or the panel goes stale")
	}
}
