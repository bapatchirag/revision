package app

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/muesli/termenv"

	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

func sizedModel(t *testing.T) *Model {
	t.Helper()
	m := New(nil, &svn.Info{URL: "https://svn.example.com/repo/trunk", Revision: "42"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func loadItems(t *testing.T, m *Model, items []svn.StatusItem) *Model {
	t.Helper()
	next, _ := m.Update(statusLoadedMsg{items: items})
	return next.(*Model)
}

func TestModelRendersStatus(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded, Changelist: "revision:staged"},
		{Path: "committed.txt", State: svn.StateModified},
	})

	view := stripANSI(m.View())
	for _, want := range []string{"added.txt", "committed.txt", "svn.example.com", "r42", "2 change(s)"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n---\n%s", want, view)
		}
	}
}

func TestModelEmptyState(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	if view := stripANSI(m.View()); !strings.Contains(view, "clean") {
		t.Errorf("expected clean message, got:\n%s", view)
	}
}

func TestModelShowsError(t *testing.T) {
	m := sizedModel(t)
	next, _ := m.Update(errMsg{err: errors.New("kaboom")})
	m = next.(*Model)

	if view := stripANSI(m.View()); !strings.Contains(view, "kaboom") {
		t.Errorf("expected error in view, got:\n%s", view)
	}
}

func TestModelQuit(t *testing.T) {
	m := sizedModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected a command from quit key")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("expected tea.QuitMsg from quit key")
	}
}

func TestSelectionUpdatesMain(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded},
		{Path: "committed.txt", State: svn.StateModified},
	})

	if main := m.main.View(); !strings.Contains(main, "added.txt") {
		t.Fatalf("main should start on the first item, got:\n%s", main)
	}

	// Down is forwarded to the focused Files panel, which emits SelectedMsg.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatal("expected a SelectedMsg command after moving down")
	}
	sel, ok := cmd().(uimsg.SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg, got %T", cmd())
	}
	next, _ := m.Update(sel)
	m = next.(*Model)

	if main := m.main.View(); !strings.Contains(main, "committed.txt") {
		t.Errorf("main should follow selection to the second item, got:\n%s", main)
	}
}

func TestFileDiffLoadsIntoMain(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "committed.txt", State: svn.StateModified},
	})
	// Before the diff arrives the Main panel shows a loading placeholder.
	if main := stripANSI(m.main.View()); !strings.Contains(main, "Loading diff") {
		t.Errorf("expected a loading placeholder, got:\n%s", main)
	}

	next, _ := m.Update(diffLoadedMsg{path: "committed.txt", diff: "@@ -1 +1 @@\n-old\n+new"})
	m = next.(*Model)
	main := stripANSI(m.main.View())
	if !strings.Contains(main, "committed.txt") || !strings.Contains(main, "+new") {
		t.Errorf("main should show the file header and diff, got:\n%s", main)
	}
}

func TestStaleDiffIgnoredForOtherFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "committed.txt", State: svn.StateModified},
	})
	// A diff for a file that is no longer selected must not replace Main.
	next, _ := m.Update(diffLoadedMsg{path: "other.txt", diff: "+stale"})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); strings.Contains(main, "+stale") {
		t.Errorf("main should ignore a diff for an unselected file, got:\n%s", main)
	}
}

func TestDiffWithTabsDoesNotOverflowWidth(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded},
	})
	// svn diff output is full of tabs; no rendered row may exceed the terminal
	// width, or it wraps and the whole frame overflows (panes appear to resize).
	next, _ := m.Update(diffLoadedMsg{
		path: "added.txt",
		diff: "Index: added.txt\n--- added.txt\t(nonexistent)\n+++ added.txt\t(working copy)\n@@ -0,0 +1 @@\n+new",
	})
	m = next.(*Model)

	for i, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w != 80 {
			t.Errorf("line %d width = %d, want 80: %q", i, w, stripANSI(line))
		}
	}
}

func TestLogPanelSelectionUpdatesMain(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(logLoadedMsg{entries: []svn.LogEntry{
		{Revision: "42", Author: "alice", Message: "first commit"},
		{Revision: "41", Author: "bob", Message: "second commit"},
	}})
	m = next.(*Model)

	// The Log panel renders history even while unfocused.
	if view := stripANSI(m.View()); !strings.Contains(view, "r42") || !strings.Contains(view, "alice") {
		t.Errorf("view missing log history, got:\n%s", view)
	}

	// Focusing the Log panel (key "3") points Main at the log selection.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "r42") || !strings.Contains(main, "first commit") {
		t.Errorf("main should show the first revision detail, got:\n%s", main)
	}

	// Moving down updates Main to the next revision.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	sel, ok := cmd().(uimsg.SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg, got %T", cmd())
	}
	next, _ = m.Update(sel)
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "second commit") {
		t.Errorf("main should follow the log selection, got:\n%s", main)
	}
}

func TestModelGoldenLayout(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded, Changelist: "revision:staged"},
		{Path: "modified.go", State: svn.StateModified},
		{Path: "gone.txt", State: svn.StateDeleted},
	})
	golden.RequireEqual(t, []byte(m.View()))
}

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
	// An unversioned file is now addable: space produces an add+stage command.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected an add+stage command for an unversioned file")
	}
	if act, ok := m.stageTarget(); !ok || !act.add {
		t.Errorf("unversioned stage target should svn add first, got %+v (ok=%v)", act, ok)
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

func TestStagedFileShowsMarker(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "staged.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	if view := stripANSI(m.View()); !strings.Contains(view, "●") {
		t.Errorf("expected a staged marker in the files list, got:\n%s", view)
	}
}

func TestCommitRequiresStagedFiles(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if cmd != nil {
		t.Error("commit with nothing staged should not run a command")
	}
	if m.editing {
		t.Error("the editor should not open with nothing staged")
	}
}

func TestCommitEditorOpensAndSubmits(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if !m.editing {
		t.Fatal("the editor should open with a staged file")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Commit message") {
		t.Errorf("expected the commit editor to overlay the layout, got:\n%s", view)
	}

	// Type a message; ctrl+s makes the editor emit SubmitMsg.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("do it")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected a SubmitMsg command from the editor")
	}
	sub, ok := cmd().(uimsg.SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", cmd())
	}
	if sub.Value != "do it" {
		t.Errorf("submitted value = %q, want %q", sub.Value, "do it")
	}

	// Handing the SubmitMsg back closes the editor and yields a commit command.
	next, cmd = m.Update(sub)
	m = next.(*Model)
	if m.editing {
		t.Error("the editor should close after submit")
	}
	if cmd == nil {
		t.Error("expected a commit command after submit")
	}
}

func TestCommitEditorCancels(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)

	// Esc emits DismissMsg, which the app handles to close the editor.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.editing {
		t.Error("the editor should close on cancel")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Commit message") {
		t.Error("the layout should return after cancelling the editor")
	}
}

func TestCommitResultShowsInBar(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(committedMsg{revision: "128"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "committed r128") {
		t.Errorf("expected the commit notice in the status bar, got:\n%s", view)
	}
}

func TestCommitEditorGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Fix status parsing")})
	golden.RequireEqual(t, []byte(m.View()))
}
