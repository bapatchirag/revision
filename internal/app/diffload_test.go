package app

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/svn"
)

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

// TestSupersededDiffReplyIgnored walks the cursor alpha → beta → alpha faster
// than svn answers, then lets the replies land out of order. The one for beta is
// superseded by the time it arrives, so it must be dropped instead of pinning
// Main on a "Loading diff…" it will never be asked to replace.
func TestSupersededDiffReplyIgnored(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
		{Path: "beta.txt", State: svn.StateModified},
	})
	move := func(k tea.KeyType) {
		t.Helper()
		next, cmd := m.Update(tea.KeyMsg{Type: k})
		m = next.(*Model)
		if cmd == nil {
			t.Fatal("expected a selection command after moving the cursor")
		}
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}

	// The status load requested alpha's diff; the two moves supersede it in turn.
	move(tea.KeyDown)
	beta := m.gens.diff.gen
	move(tea.KeyUp)
	if m.gens.diff.gen == beta {
		t.Fatal("expected returning to alpha to issue a fresh diff load")
	}

	next, _ := m.Update(diffLoadedMsg{path: "alpha.txt", diff: "+fresh alpha", gen: m.gens.diff.gen})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "fresh alpha") {
		t.Fatalf("main should show the current selection's diff, got:\n%s", main)
	}

	next, _ = m.Update(diffLoadedMsg{path: "beta.txt", diff: "+stale beta", gen: beta})
	m = next.(*Model)
	main := stripANSI(m.main.View())
	if strings.Contains(main, "stale beta") || strings.Contains(main, "Loading diff") {
		t.Errorf("superseded diff reply should be dropped, got:\n%s", main)
	}
}

// TestDiffCacheServesRevisitedFile walks alpha → beta → alpha and asserts the
// third selection costs no svn command: the session still holds alpha's diff, so
// it renders on the same frame.
func TestDiffCacheServesRevisitedFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
		{Path: "beta.txt", State: svn.StateModified},
	})
	move := func(k tea.KeyType) tea.Cmd {
		t.Helper()
		next, cmd := m.Update(tea.KeyMsg{Type: k})
		m = next.(*Model)
		if cmd == nil {
			t.Fatal("expected a selection command after moving the cursor")
		}
		next, cmd = m.Update(cmd())
		m = next.(*Model)
		return cmd
	}

	// alpha is selected on load; its diff arrives and is cached.
	next, _ := m.Update(diffLoadedMsg{path: "alpha.txt", diff: "@@ -1 +1 @@\n+alpha body"})
	m = next.(*Model)

	// beta has never been looked at, so it has to be fetched.
	if cmd := move(tea.KeyDown); cmd == nil {
		t.Fatal("expected a diff load for a file that has not been seen")
	}
	next, _ = m.Update(diffLoadedMsg{path: "beta.txt", diff: "@@ -1 +1 @@\n+beta body"})
	m = next.(*Model)

	if cmd := move(tea.KeyUp); cmd != nil {
		t.Error("returning to alpha should be answered from the session, with no svn command")
	}
	if main := stripANSI(m.main.View()); !strings.Contains(main, "+alpha body") {
		t.Errorf("main should show the cached diff immediately, got:\n%s", main)
	}
}

// TestCachedDiffSupersedesLoadInFlight covers the load left running when the
// cursor lands on a row the session can answer: its reply is for the row just
// left, so it must be dropped rather than replace what is on screen.
func TestCachedDiffSupersedesLoadInFlight(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
		{Path: "beta.txt", State: svn.StateModified},
	})
	move := func(k tea.KeyType) {
		t.Helper()
		next, cmd := m.Update(tea.KeyMsg{Type: k})
		m = next.(*Model)
		if cmd == nil {
			t.Fatal("expected a selection command after moving the cursor")
		}
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}

	next, _ := m.Update(diffLoadedMsg{path: "alpha.txt", diff: "@@ -1 +1 @@\n+alpha body"})
	m = next.(*Model)

	move(tea.KeyDown) // beta: not cached, so a load is now in flight
	inFlight := m.gens.diff.gen
	move(tea.KeyUp) // alpha: answered from the session before beta replies

	next, _ = m.Update(diffLoadedMsg{path: "beta.txt", diff: "+beta body", gen: inFlight})
	m = next.(*Model)
	main := stripANSI(m.main.View())
	if strings.Contains(main, "beta body") || strings.Contains(main, "Loading diff") {
		t.Errorf("a reply superseded by a cache hit must be dropped, got:\n%s", main)
	}
	if !strings.Contains(main, "+alpha body") {
		t.Errorf("main should still show the cached diff, got:\n%s", main)
	}
}

// TestStatusReloadKeepsDiffOnScreen covers the reload that follows every
// mutation: when it reports the same working copy, the diff already on screen is
// still the right one and must neither blank nor be fetched again.
func TestStatusReloadKeepsDiffOnScreen(t *testing.T) {
	items := []svn.StatusItem{{Path: "alpha.txt", State: svn.StateModified}}
	m := loadItems(t, sizedModel(t), items)
	next, _ := m.Update(diffLoadedMsg{path: "alpha.txt", diff: "@@ -1 +1 @@\n+alpha body"})
	m = next.(*Model)

	next, cmd := m.Update(statusLoadedMsg{items: items})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "+alpha body") {
		t.Errorf("a status reload should not blank the diff on screen, got:\n%s", main)
	}
	if cmd != nil {
		t.Error("expected no diff load after a reload that changed nothing")
	}
}

// TestStatusReloadDropsDiffThatMoved is the other half: when the reload reports
// a file whose state changed, the cached diff no longer describes it, so it
// clears and is fetched afresh.
func TestStatusReloadDropsDiffThatMoved(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	})
	next, _ := m.Update(diffLoadedMsg{path: "alpha.txt", diff: "@@ -1 +1 @@\n+alpha body"})
	m = next.(*Model)

	next, cmd := m.Update(statusLoadedMsg{items: []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified, Changelist: stagedChangelist},
	}})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected the moved file's diff to be loaded again")
	}
	if main := stripANSI(m.main.View()); strings.Contains(main, "+alpha body") {
		t.Errorf("a diff the status moved out from under should not be served, got:\n%s", main)
	}
}

func TestDirectoryDiffLoadsIntoMain(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	// The cursor opens on the first file; move it onto the src/ directory row.
	selectDirRow(t, m, "src")

	// Highlighting a directory schedules a load of its combined diff.
	if cmd := m.diffLoadForSelection(); cmd == nil {
		t.Fatal("expected a diff-load command for the highlighted directory")
	}

	// When it arrives, Main shows the diff of every file under the directory.
	next, _ := m.Update(diffLoadedMsg{
		path: "src",
		dir:  true,
		diff: "Index: src/a.go\n@@ -1 +1 @@\n+alpha\nIndex: src/b.go\n@@ -1 +1 @@\n+beta",
	})
	m = next.(*Model)
	main := stripANSI(m.main.View())
	if !strings.Contains(main, "+alpha") || !strings.Contains(main, "+beta") {
		t.Errorf("main should show the combined directory diff, got:\n%s", main)
	}
}

func TestRootDirectoryDiffCoversWholeWorkingCopy(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "readme.md", State: svn.StateModified},
	})
	m.files.SetIndex(0) // the synthetic "/" root row covers the whole tree
	root, ok := m.files.Selected()
	if !ok || root.Path != fileTreeRoot {
		t.Fatalf("expected cursor on the / root row, got %+v (ok=%v)", root, ok)
	}
	if cmd := m.diffLoadForSelection(); cmd == nil {
		t.Fatal("expected a diff-load command for the root row")
	}

	// The root diff is keyed by the "/" sentinel so the current selection matches
	// it; it spans changes anywhere in the working copy, nested or not.
	next, _ := m.Update(diffLoadedMsg{
		path: fileTreeRoot,
		dir:  true,
		diff: "Index: readme.md\n@@ -1 +1 @@\n+top\nIndex: src/a.go\n@@ -1 +1 @@\n+nested",
	})
	m = next.(*Model)
	main := stripANSI(m.main.View())
	if !strings.Contains(main, "+top") || !strings.Contains(main, "+nested") {
		t.Errorf("root diff should cover the whole working copy, got:\n%s", main)
	}
}

func TestDirectoryDiffDisabledByConfig(t *testing.T) {
	cfg := config.Default()
	cfg.DirectoryDiff = false
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")

	// With directory diffs off globally, highlighting a directory loads nothing.
	if cmd := m.diffLoadForSelection(); cmd != nil {
		t.Fatal("expected no diff-load command while directory diffs are off")
	}
	// Main shows a hint naming the toggle key instead of a diff.
	m.updateMain()
	if main := stripANSI(m.main.View()); !strings.Contains(main, "directory diff off") {
		t.Errorf("expected the directory-diff-off hint, got:\n%s", main)
	}
}

func TestToggleDirDiffRevealsDirectoryDiff(t *testing.T) {
	cfg := config.Default()
	cfg.DirectoryDiff = false
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")

	// Pressing the toggle key schedules the directory diff load.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a diff-load command after toggling directory diffs on")
	}
	// When the diff arrives, Main shows it.
	next, _ = m.Update(diffLoadedMsg{
		path: "src",
		dir:  true,
		diff: "Index: src/a.go\n@@ -1 +1 @@\n+alpha",
	})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "+alpha") {
		t.Errorf("expected the directory diff after toggling on, got:\n%s", main)
	}
}

func TestToggleDirDiffHidesDirectoryDiff(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")
	// Directory diffs are on by default, so the loaded diff shows in Main.
	next, _ := m.Update(diffLoadedMsg{path: "src", dir: true, diff: "Index: src/a.go\n@@ -1 +1 @@\n+alpha"})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "+alpha") {
		t.Fatalf("expected the directory diff to show, got:\n%s", main)
	}

	// Toggling off hides it behind the hint and drops the diff gutter.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = next.(*Model)
	if m.filesShowDiff() {
		t.Error("filesShowDiff() = true after toggling directory diffs off")
	}
	if main := stripANSI(m.main.View()); !strings.Contains(main, "directory diff off") {
		t.Errorf("expected the directory-diff-off hint, got:\n%s", main)
	}
}

func TestHideUntrackedHiddenByConfig(t *testing.T) {
	cfg := config.Default()
	cfg.HideUntracked = true
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
		{Path: "scratch.txt", State: svn.StateUnversioned},
	})
	if fileTreeHasPath(m, "scratch.txt") {
		t.Error("untracked file should be hidden from the Changes tree when hideUntracked is on")
	}
	if !fileTreeHasPath(m, "modified.go") {
		t.Error("tracked file should still show when hideUntracked is on")
	}
}

func TestToggleUntrackedHidesAndShows(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
		{Path: "scratch.txt", State: svn.StateUnversioned},
	})
	// Untracked files show by default.
	if !fileTreeHasPath(m, "scratch.txt") {
		t.Fatal("untracked file should show by default")
	}

	// Pressing the toggle hides untracked files but keeps tracked ones.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = next.(*Model)
	if !m.hideUntracked {
		t.Fatal("pressing U did not enable hide-untracked")
	}
	if fileTreeHasPath(m, "scratch.txt") {
		t.Error("untracked file should be hidden after toggling")
	}
	if !fileTreeHasPath(m, "modified.go") {
		t.Error("tracked file should remain after toggling untracked off")
	}

	// Pressing it again reveals them.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = next.(*Model)
	if m.hideUntracked {
		t.Fatal("pressing U again did not disable hide-untracked")
	}
	if !fileTreeHasPath(m, "scratch.txt") {
		t.Error("untracked file should reappear after toggling back on")
	}
}

func TestToggleUntrackedDoesNotPersistConfig(t *testing.T) {
	// Point config at a throwaway dir so any write would be observable there.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
		{Path: "scratch.txt", State: svn.StateUnversioned},
	})

	// The keybind changes only the session view, never the saved configuration.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = next.(*Model)
	if !m.hideUntracked {
		t.Fatal("pressing U did not enable the session hide-untracked toggle")
	}
	if m.cfg.HideUntracked {
		t.Error("the keybind must not change the in-memory config")
	}
	path, err := config.Path()
	if err != nil {
		t.Fatalf("config.Path: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the keybind wrote config.json; it must not persist the toggle")
	}
}

// selectPath parks the Files cursor on a file leaf and returns the command the
// move asked for, so a test can assert what the selection does or does not load.
func selectPath(t *testing.T, m *Model, path string) tea.Cmd {
	t.Helper()
	for i, n := range m.files.Items() {
		if n.Item != nil && n.Path == path {
			m.files.SetIndex(i)
			return m.diffLoadForSelection()
		}
	}
	t.Fatalf("no file leaf for %q", path)
	return nil
}

// TestMovedSourceShowsWhereItWentInsteadOfADeletion covers the half of a move
// svn reports as deleted. Its diff is the whole file as removed lines whatever
// else is true of it, which reads as the file being destroyed, so Main names the
// destination instead — and never spends an svn run fetching the diff it drops.
func TestMovedSourceShowsWhereItWentInsteadOfADeletion(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src.txt", State: svn.StateDeleted, MovedTo: "sub/dest.txt"},
	})
	if cmd := selectPath(t, m, "src.txt"); cmd != nil {
		t.Error("the source half's diff is never shown, so it must not be loaded")
	}
	main := stripANSI(m.main.View())
	if !strings.Contains(main, "moved to sub/dest.txt") {
		t.Errorf("main should name where the file went, got:\n%s", main)
	}
	if !strings.Contains(main, "the file moved") {
		t.Errorf("main should say why it is withholding the deletion diff, got:\n%s", main)
	}
	if m.filesShowDiff() {
		t.Error("filesShowDiff() = true for a row showing no diff; the gutter would pin nothing")
	}
}

// TestMovedDestinationKeepsItsDiff is the other half. A move the file was edited
// after diffs against what was copied, and that delta appears nowhere else — the
// source half shows nothing of it — so it has to survive.
func TestMovedDestinationKeepsItsDiff(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "dest.txt", State: svn.StateAdded, Copied: true, MovedFrom: "src.txt"},
	})
	if cmd := selectPath(t, m, "dest.txt"); cmd == nil {
		t.Fatal("the destination half's diff carries the post-move edit, so it must be loaded")
	}
	next, _ := m.Update(diffLoadedMsg{path: "dest.txt", diff: "@@ -1 +1,2 @@\n old\n+edited"})
	m = next.(*Model)

	main := stripANSI(m.main.View())
	if !strings.Contains(main, "moved from src.txt") {
		t.Errorf("main should name where the file came from, got:\n%s", main)
	}
	if !strings.Contains(main, "+edited") {
		t.Errorf("main should still show the edit made after the move, got:\n%s", main)
	}
	if !m.filesShowDiff() {
		t.Error("filesShowDiff() = false while a diff is on screen; the gutter would not be pinned")
	}
}

// TestPureMoveReadsAsUnchangedRatherThanEmpty covers a move with no edit after
// it. svn still writes the "Index:" block for the file it was asked about, so
// the diff is non-empty text carrying no change — which a blank-text check would
// let through to Main as two meaningless header lines.
func TestPureMoveReadsAsUnchangedRatherThanEmpty(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "dest.txt", State: svn.StateAdded, Copied: true, MovedFrom: "src.txt"},
	})
	selectPath(t, m, "dest.txt")
	headerOnly := "Index: dest.txt\n===================================================================\n"
	next, _ := m.Update(diffLoadedMsg{path: "dest.txt", diff: headerOnly})
	m = next.(*Model)

	main := stripANSI(m.main.View())
	if !strings.Contains(main, "unchanged from src.txt") {
		t.Errorf("an unedited move should say the content is unchanged, got:\n%s", main)
	}
	if strings.Contains(main, "Index: dest.txt") {
		t.Errorf("the bare header should not be shown as though it were a diff, got:\n%s", main)
	}
	if m.filesShowDiff() {
		t.Error("filesShowDiff() = true for a header-only patch; the gutter would pin nothing")
	}
}
