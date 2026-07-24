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
	"github.com/bapatchirag/revision/internal/tui/theme"
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

	// The first item is selected, so its diff lands in Main.
	next, _ := m.Update(diffLoadedMsg{path: "added.txt", diff: "@@ -0,0 +1 @@\n+alpha"})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "+alpha") {
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
	next, _ = m.Update(sel)
	m = next.(*Model)

	// The second item's diff follows the selection into Main.
	next, _ = m.Update(diffLoadedMsg{path: "committed.txt", diff: "@@ -1 +1 @@\n+beta"})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "+beta") {
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
	if !strings.Contains(main, "+new") {
		t.Errorf("main should show the diff, got:\n%s", main)
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

// TestDiffGutterStaysPinnedWhenScrolled proves the Main viewport keeps a unified
// diff's +/- marker column pinned to the left while the body scrolls: after
// scrolling fully right, the added and removed rows still begin with their marker.
func TestDiffGutterStaysPinnedWhenScrolled(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "wide.txt", State: svn.StateModified},
	})
	// Body lines far wider than the Main pane, so the diff scrolls horizontally
	// and an unpinned marker would otherwise slide out of view.
	long := strings.Repeat("abcdefghij", 12) // 120 columns
	next, _ := m.Update(diffLoadedMsg{
		path: "wide.txt",
		diff: "@@ -1 +1 @@\n-" + long + "\n+" + long,
	})
	m = next.(*Model)
	before := stripANSI(m.main.View())

	// Scroll the Main viewport as far right as it goes.
	m.main.Focus()
	m.main.Update(tea.KeyMsg{Type: tea.KeyEnd})
	after := stripANSI(m.main.View())

	if before == after {
		t.Fatal("diff did not scroll horizontally; the gutter cannot be observed")
	}
	var minus, plus bool
	for _, ln := range strings.Split(after, "\n") {
		switch {
		case strings.HasPrefix(ln, "-"):
			minus = true
		case strings.HasPrefix(ln, "+"):
			plus = true
		}
	}
	if !minus || !plus {
		t.Errorf("scrolled diff lost its +/- gutter:\n%s", after)
	}
}

func TestColorizeDiff(t *testing.T) {
	// Emit ANSI so the styling is observable, then restore the Ascii profile the
	// rest of the suite relies on.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	diff := "Index: a.txt\n--- a.txt\t(revision 1)\n+++ a.txt\t(working copy)\n" +
		"@@ -1,2 +1,2 @@\n context\n-old\n+new"
	got := colorizeDiff(theme.Default(), diff)

	// Coloring must only add styling, never alter the underlying text.
	if plain := stripANSI(got); plain != diff {
		t.Fatalf("colorize changed content:\n got: %q\nwant: %q", plain, diff)
	}

	// Metadata, hunk, add and delete lines are colored; context lines are not.
	wantColored := map[string]bool{
		"Index: a.txt":              true,
		"--- a.txt\t(revision 1)":   true,
		"+++ a.txt\t(working copy)": true,
		"@@ -1,2 +1,2 @@":           true,
		"-old":                      true,
		"+new":                      true,
		" context":                  false,
	}
	for _, ln := range strings.Split(got, "\n") {
		plain := stripANSI(ln)
		want, tracked := wantColored[plain]
		if !tracked {
			continue
		}
		if colored := ln != plain; colored != want {
			t.Errorf("line %q colored=%v, want %v", plain, colored, want)
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

func TestCommitResultShowsToast(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(committedMsg{revision: "128"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "committed r128") {
		t.Errorf("expected the commit toast, got:\n%s", view)
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

func TestUpdateRunsCommand(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}); cmd == nil {
		t.Error("u should run an update command")
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

func TestRevertResultShowsToast(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, cmd := m.Update(revertedMsg{path: "modified.go"})
	m = next.(*Model)
	if cmd == nil {
		t.Error("a revert should trigger a status reload")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "reverted modified.go") {
		t.Errorf("expected the revert toast, got:\n%s", view)
	}
}

func TestToastDismissedOnKey(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified},
		{Path: "b.go", State: svn.StateModified},
	})
	next, _ := m.Update(committedMsg{revision: "9"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "committed r9") {
		t.Fatalf("expected the commit toast, got:\n%s", view)
	}
	// Any interaction (here: navigating the Files panel) clears the toast.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	if view := stripANSI(m.View()); strings.Contains(view, "committed r9") {
		t.Errorf("the toast should clear on the next key, got:\n%s", view)
	}
}

func TestModalConfirmGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
	})
	// The cursor opens on the app.go leaf (the tree skips the / root and the
	// internal/ and app/ directory rows), so delete targets the file directly.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestChangesTreeShowsDirectoryTree(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
		{Path: "internal/svn/client.go", State: svn.StateModified},
		{Path: "README.md", State: svn.StateModified},
	})
	// Inspect the built tree rows directly, independent of the panel's visible
	// window: every path segment is its own row and files are basenames.
	var names []string
	for _, n := range m.files.Items() {
		names = append(names, n.Name)
		if n.Item != nil && strings.Contains(n.Name, "/") {
			t.Errorf("file leaf %q should be a basename, not a nested path", n.Name)
		}
	}
	for _, want := range []string{"/", "internal", "app", "svn", "app.go", "client.go", "README.md"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tree rows missing %q, got: %v", want, names)
		}
	}
}

func TestEnterCollapsesDirectory(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
		{Path: "internal/svn/client.go", State: svn.StateModified},
	})
	if view := stripANSI(m.View()); !strings.Contains(view, "app.go") {
		t.Fatalf("expected file leaves visible before collapse, got:\n%s", view)
	}

	// The cursor opens on the first file; move it onto the internal/ directory row.
	for i, n := range m.files.Items() {
		if n.Name == "internal" {
			m.files.SetIndex(i)
			break
		}
	}

	// Enter emits an ActivatedMsg the model turns into a collapse toggle.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg command from enter on a directory")
	}
	act, ok := cmd().(uimsg.ActivatedMsg)
	if !ok {
		t.Fatalf("expected ActivatedMsg, got %T", cmd())
	}
	next, _ := m.Update(act)
	m = next.(*Model)

	// Collapsing internal/ hides its descendants but keeps the directory row.
	view := stripANSI(m.View())
	if strings.Contains(view, "app.go") || strings.Contains(view, "client.go") {
		t.Errorf("collapsing internal/ should hide its descendants, got:\n%s", view)
	}
	if !strings.Contains(view, "internal/") {
		t.Errorf("the collapsed directory row should remain, got:\n%s", view)
	}
}

func TestHelpMenuOpensAndCloses(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})

	// "?" floats the keybindings menu over the layout.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	if !m.helping {
		t.Fatal("expected the help menu to open on ?")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Keybindings", "Stage / unstage", "space", "Quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("help view missing %q\n---\n%s", want, view)
		}
	}

	// While help is open, other keys are captured by the menu — q must not quit.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Error("q should not quit while the help menu is open")
		}
	}
	if !m.helping {
		t.Error("the help menu should stay open on a non-dismiss key")
	}

	// enter must NOT close the help menu — it is a read-only reference.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the resulting ActivatedMsg
		m = next.(*Model)
	}
	if !m.helping {
		t.Error("enter should not close the help menu")
	}

	// esc closes the help menu.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if m.helping {
		t.Error("the help menu should close after esc")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Keybindings") {
		t.Error("the layout should return after closing help")
	}
}

func TestHelpMenuTogglesClosedWithQuestionMark(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	if !m.helping {
		t.Fatal("? should open the help menu")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	if m.helping {
		t.Error("? should toggle the help menu closed")
	}
}

func TestAuthFailureShowsHint(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	authErr := errors.New("svn commit: E170001: No more credentials or we tried too many times.")
	next, _ := m.Update(committedMsg{err: authErr})
	m = next.(*Model)

	view := stripANSI(m.View())
	if !strings.Contains(view, "authentication required") {
		t.Errorf("expected an auth hint toast, got:\n%s", view)
	}
	if strings.Contains(view, "E170001") {
		t.Errorf("the raw svn error should be replaced by the hint, got:\n%s", view)
	}
}

func TestHelpMenuGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestChangelistGrouping(t *testing.T) {
	groups := groupChangelists([]svn.StatusItem{
		{Path: "z.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "a.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "b.go", State: svn.StateModified},
		{Path: "c.go", State: svn.StateModified, Changelist: "alpha"},
	})
	// Named changelists first (alphabetical), then staged, then the unstaged default.
	want := []string{"alpha", "feature", "(staged)", "(unstaged)"}
	if len(groups) != len(want) {
		t.Fatalf("want %d groups, got %d: %+v", len(want), len(groups), groups)
	}
	for i, w := range want {
		if groups[i].Label() != w {
			t.Errorf("group %d = %q, want %q", i, groups[i].Label(), w)
		}
	}
	if !groups[0].Committable() {
		t.Error("a named changelist should be committable")
	}
	if groups[3].Committable() {
		t.Error("the unstaged default group should not be committable")
	}
}

func TestFilesViewSwitchesToChangelists(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "b.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	// Files panel is focused by default; ] cycles to the Changelists view.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver ViewSelectedMsg
		m = next.(*Model)
	}
	if name := m.filesViews.ActiveName(); name != "Changelists" {
		t.Fatalf("active files view = %q, want Changelists", name)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "feature") || !strings.Contains(view, "(staged)") {
		t.Errorf("the changelists view should list the groups, got:\n%s", view)
	}
}

func TestAssignChangelistPromptAndSubmit(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "mod.go", State: svn.StateModified},
	})
	// n opens the changelist-name prompt.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("n should open the changelist-name prompt")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Changelist name") {
		t.Errorf("expected the name prompt, got:\n%s", view)
	}

	// Type a name; enter submits it (single-line input).
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature-x")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a SubmitMsg from the name prompt")
	}
	sub, ok := cmd().(uimsg.SubmitMsg)
	if !ok || sub.ID != changelistEditorID {
		t.Fatalf("expected a changelist SubmitMsg, got %T (%+v)", cmd(), cmd())
	}
	if sub.Value != "feature-x" {
		t.Errorf("submitted name = %q, want feature-x", sub.Value)
	}

	next, cmd = m.Update(sub)
	m = next.(*Model)
	if m.naming {
		t.Error("the prompt should close after submit")
	}
	if cmd == nil {
		t.Error("expected an assign command after submit")
	}
}

func TestAssignChangelistAllowsStagedFile(t *testing.T) {
	// A file in the anonymous staged bucket can be moved into a named changelist.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "staged.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("a staged file should be assignable to a named changelist")
	}
}

func TestAssignChangelistNamesAllStagedFiles(t *testing.T) {
	// Naming a changelist while several files are staged moves the whole staged
	// set as a unit, not just the highlighted file; an unstaged file is left out.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "b.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "c.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("n should open the changelist-name prompt when files are staged")
	}
	got := map[string]bool{}
	for _, tgt := range m.nameTargets {
		got[tgt.path] = true
	}
	if len(m.nameTargets) != 2 || !got["a.go"] || !got["b.go"] {
		t.Errorf("nameTargets = %+v, want exactly the staged files a.go and b.go", m.nameTargets)
	}
}

func TestAssignChangelistFallsBackToSelectedFile(t *testing.T) {
	// With nothing staged, naming still targets just the selected file so the
	// single-file workflow keeps working.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "lone.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("n should open the prompt for the selected file when nothing is staged")
	}
	if len(m.nameTargets) != 1 || m.nameTargets[0].path != "lone.go" {
		t.Errorf("nameTargets = %+v, want just lone.go", m.nameTargets)
	}
}

func TestAssignChangelistOffersExistingNames(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "loose.go", State: svn.StateModified},
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "b.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	view := stripANSI(m.View())
	if !strings.Contains(view, "Existing changelists:") || !strings.Contains(view, "feature") {
		t.Errorf("the prompt should list existing named changelists, got:\n%s", view)
	}
	// The anonymous buckets are not offered as pickable names.
	if strings.Contains(view, "(staged)") || strings.Contains(view, "(unstaged)") {
		t.Errorf("anonymous buckets should not appear as options, got:\n%s", view)
	}
}

func TestAssignChangelistGuardsNamedChangelist(t *testing.T) {
	// A file already in a *named* changelist cannot be reassigned (unstage first).
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if m.naming {
		t.Error("a file already in a named changelist should not open the prompt")
	}
	if cmd != nil {
		t.Error("the guard should not produce a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "already in") {
		t.Errorf("expected an already-assigned guard toast, got:\n%s", view)
	}
}

func TestAssignChangelistCancels(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "mod.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	// Esc emits DismissMsg, which the app handles to close the prompt.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.naming {
		t.Error("the prompt should close on cancel")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Changelist name") {
		t.Error("the layout should return after cancelling the prompt")
	}
}

func TestChangelistDrillExpandsAndCollapses(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)

	// enter drills into the selected changelist.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from the changelists list")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() == 0 {
		t.Fatal("enter should drill into the changelist")
	}
	if m.drilledCL != "feature" {
		t.Errorf("drilled changelist = %q, want feature", m.drilledCL)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "a.go") {
		t.Errorf("the drill should list the changelist's files, got:\n%s", view)
	}

	// esc collapses back out.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a SubViewPoppedMsg on esc")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() != 0 {
		t.Error("esc should collapse the drill")
	}
	if m.drilledCL != "" {
		t.Error("the drilled changelist should be cleared on collapse")
	}
}

func TestChangelistDrillShowsTree(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "internal/svn/client.go", State: svn.StateModified, Changelist: "feature"},
	})
	// Switch to Changelists and drill into "feature".
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from the changelists list")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() == 0 {
		t.Fatal("expected to be drilled into the changelist")
	}

	// The drill renders the same "/"-rooted tree: a root row, directory rows, and
	// basename leaves (never a full nested path on one row).
	var names []string
	for _, n := range m.clFiles.Items() {
		names = append(names, n.Name)
		if n.Item != nil && strings.Contains(n.Name, "/") {
			t.Errorf("drill file leaf %q should be a basename", n.Name)
		}
	}
	for _, want := range []string{"/", "internal", "app", "svn", "app.go", "client.go"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("drill tree missing %q, got: %v", want, names)
		}
	}

	// Enter on the internal/ directory row collapses it, hiding its descendants.
	for i, n := range m.clFiles.Items() {
		if n.Name == "internal" {
			m.clFiles.SetIndex(i)
			break
		}
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from enter on a drill directory")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	for _, n := range m.clFiles.Items() {
		if n.Name == "app.go" || n.Name == "client.go" {
			t.Errorf("collapsing internal/ in the drill should hide its files, got: %v", m.clFiles.Items())
		}
	}
}

func TestCommitChangelistFromView(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if !m.editing {
		t.Fatal("c should open the commit editor for the selected changelist")
	}
	if m.commitCL != "feature" {
		t.Errorf("commit target = %q, want feature", m.commitCL)
	}
}

func TestCommitUnstagedGroupRefused(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified}, // no changelist → (unstaged)
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if m.editing {
		t.Error("committing the unstaged group should be refused")
	}
	if cmd != nil {
		t.Error("no command should run for the unstaged group")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "isn't a changelist") {
		t.Errorf("expected a refusal toast, got:\n%s", view)
	}
}

func TestNamedChangelistFileShowsAccentMarker(t *testing.T) {
	// A named-changelist file is marked in the Changes view (distinct from the
	// staged bucket's marker), so both render the dot.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "feature.go", State: svn.StateModified, Changelist: "feature"},
	})
	if view := stripANSI(m.View()); !strings.Contains(view, "●") {
		t.Errorf("expected a changelist marker in the files list, got:\n%s", view)
	}
}

func TestChangelistsViewGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded, Changelist: "revision:staged"},
		{Path: "feature.go", State: svn.StateModified, Changelist: "feature-x"},
		{Path: "loose.txt", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestChangelistDrillLocksViewSwitch(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	// Switch to Changelists, then drill in.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() == 0 {
		t.Fatal("expected to be drilled into the changelist")
	}

	// While drilled, [ and ] must not switch the Files view.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if name := m.filesViews.ActiveName(); name != "Changelists" {
		t.Errorf("view switched while drilled (now %q); it should stay locked", name)
	}
	if m.filesViews.Depth() == 0 {
		t.Error("the drill should remain open while view switching is locked")
	}
}

func TestChangelistDrillHeaderGolden(t *testing.T) {
	// Expanding a changelist labels the panel header with just the changelist
	// name (no tabs, no chevron).
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "feature.go", State: svn.StateModified, Changelist: "feature-x"},
		{Path: "other.go", State: svn.StateAdded, Changelist: "feature-x"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}
