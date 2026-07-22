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
