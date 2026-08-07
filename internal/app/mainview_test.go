package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

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

// TestDiffRefreshKeepsMainScroll re-delivers the diff already on screen, as a
// reload of the same selection does, and asserts Main stays where the user
// scrolled to instead of snapping back to the top.
func TestDiffRefreshKeepsMainScroll(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "committed.txt", State: svn.StateModified},
	})
	body := make([]string, 40)
	for i := range body {
		body[i] = fmt.Sprintf("+line%02d", i)
	}
	diff := "@@ -1 +1 @@\n" + strings.Join(body, "\n")

	next, _ := m.Update(diffLoadedMsg{path: "committed.txt", diff: diff})
	m = next.(*Model)

	// Scroll Main down a page from the Main panel itself.
	m.focus.Focus(panelMain)
	m.afterFocusChange()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = next.(*Model)
	scrolled := stripANSI(m.main.View())
	if strings.Contains(scrolled, "+line00") {
		t.Fatalf("expected Main to have scrolled past the top, got:\n%s", scrolled)
	}

	// The same diff arriving again is a refresh of what is on screen.
	next, _ = m.Update(diffLoadedMsg{path: "committed.txt", diff: diff})
	m = next.(*Model)
	if got := stripANSI(m.main.View()); got != scrolled {
		t.Errorf("refresh moved Main; want the scrolled view\n--- before ---\n%s\n--- after ---\n%s", scrolled, got)
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

func BenchmarkUpdateMain(b *testing.B) {
	items := make([]svn.StatusItem, 0, 200)
	for i := 0; i < 200; i++ {
		items = append(items, svn.StatusItem{
			Path:  fmt.Sprintf("internal/pkg%02d/file%03d.go", i%10, i),
			State: svn.StateModified,
		})
	}
	m := loadItems(b, sizedModel(b), items)
	m.diffPath, m.diffText = items[0].Path, diffFixture(500)
	m.files.SetIndex(firstFileIndex(m.files.Items()))
	m.updateMain()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.updateMain()
	}
}

func TestLogPanelSelectionUpdatesMain(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{
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

func TestStatusPanelShowsAbout(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	// Widen so the full project URLs fit without horizontal scrolling.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(*Model)
	// Focusing the Status panel (1) turns Main into the about screen.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = next.(*Model)

	view := stripANSI(m.View())
	for _, want := range []string{
		"bapatchirag.github.io/revision",
		"revision/issues",
		"revision/releases",
		"Chirag Bapat",
		"Press S",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("about screen missing %q\n---\n%s", want, view)
		}
	}
}

func TestStatusPanelAboutGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}
