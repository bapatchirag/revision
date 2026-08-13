package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

// click is a left button press at a screen cell.
func click(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

// The coordinates below are read off sizedModel's 80x24 screen: the left column
// is 32 wide and stacks Status (rows 0-7), Files (8-14) and Log (15-22); the
// right column holds Main (rows 0-16) over the command log (17-22); the bar has
// the last row. Hard-coding them means the layout cannot drift unnoticed.
func TestClickFocusesThePanelUnderThePointer(t *testing.T) {
	cases := []struct {
		name string
		x, y int
		want int
	}{
		{"status", 5, 2, panelStatus},
		{"files", 5, 10, panelFiles},
		{"log", 5, 18, panelLog},
		{"main", 40, 5, panelMain},
		{"command log", 40, 20, panelCmdLog},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := sizedModel(t)
			next, _ := m.Update(click(tc.x, tc.y))
			m = next.(*Model)
			if got := m.focus.Index(); got != tc.want {
				t.Errorf("click at (%d,%d) focused panel %d, want %d", tc.x, tc.y, got, tc.want)
			}
		})
	}
}

func TestClickBelowThePanelsLeavesFocusAlone(t *testing.T) {
	m := sizedModel(t)
	before := m.focus.Index()
	// Row 23 is the status bar, which belongs to no panel.
	next, _ := m.Update(click(10, 23))
	m = next.(*Model)
	if got := m.focus.Index(); got != before {
		t.Errorf("focus moved to %d, want it left on %d", got, before)
	}
}

func TestHidingTheCommandLogGivesItsRowsToMain(t *testing.T) {
	m := sizedModel(t)
	m, _ = pressRune(t, m, 'x')
	if m.showCmdLog {
		t.Fatal("x should hide the command log")
	}
	next, _ := m.Update(click(40, 20))
	m = next.(*Model)
	if got := m.focus.Index(); got != panelMain {
		t.Errorf("focused panel %d, want Main to have taken the vacated rows", got)
	}
}

func TestOnlyALeftPressMovesFocus(t *testing.T) {
	ignored := map[string]tea.MouseMsg{
		"release":     {X: 5, Y: 18, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft},
		"drag":        {X: 5, Y: 18, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft},
		"wheel":       {X: 5, Y: 18, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown},
		"right click": {X: 5, Y: 18, Action: tea.MouseActionPress, Button: tea.MouseButtonRight},
	}
	for name, msg := range ignored {
		t.Run(name, func(t *testing.T) {
			m := sizedModel(t)
			before := m.focus.Index()
			next, _ := m.Update(msg)
			m = next.(*Model)
			if got := m.focus.Index(); got != before {
				t.Errorf("focus moved to %d, want it left on %d", got, before)
			}
		})
	}
}

func TestClicksAreIgnoredWhileAnOverlayIsOpen(t *testing.T) {
	m := sizedModel(t)
	m, _ = pressRune(t, m, '?')
	if !m.helping {
		t.Fatal("? should open the help menu")
	}
	before := m.focus.Index()
	next, _ := m.Update(click(5, 18))
	m = next.(*Model)
	if !m.helping {
		t.Error("a click should not close the help menu")
	}
	if got := m.focus.Index(); got != before {
		t.Errorf("focus moved to %d behind the overlay, want it left on %d", got, before)
	}
}

func TestClickingAViewNameSelectsItAndItsPanel(t *testing.T) {
	m := sizedModel(t)
	m, _ = pressRune(t, m, ']')
	if name := m.filesViews.ActiveName(); name != "Changelists" {
		t.Fatalf("active files view = %q, want ] to have moved off Changes", name)
	}
	m, _ = pressRune(t, m, '3')
	if m.focus.Index() != panelLog {
		t.Fatal("3 should focus the Log panel")
	}

	next, _ := m.Update(click(tabColumn(t, m, "Changes"), filesTop))
	m = next.(*Model)
	if name := m.filesViews.ActiveName(); name != "Changes" {
		t.Errorf("active files view = %q, want the clicked one", name)
	}
	if got := m.focus.Index(); got != panelFiles {
		t.Errorf("focused panel %d, want the clicked view's panel", got)
	}
}

// filesTop is the screen row of the Files panel's top border, where it inlays
// its view names. Its rows are the lines below it.
const filesTop = 8

// logTop and mainLeft are the Log panel's top border row and the Main panel's
// left border column on the same 80x24 screen.
const (
	logTop   = 15
	mainLeft = 32
)

// TestClickingARowSelectsIt is the pointer's half of "navigate to a row": the
// same SelectedMsg the arrow keys emit, which is what loads the diff for it.
func TestClickingARowSelectsIt(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.go", State: svn.StateModified},
		{Path: "beta.go", State: svn.StateModified},
		{Path: "gamma.go", State: svn.StateModified},
	})
	if m.focus.Index() != panelFiles {
		t.Fatal("the Files panel starts focused")
	}

	// The tree's first row is the working-copy root, so the three files follow it.
	next, cmd := m.Update(click(4, filesTop+4))
	m = next.(*Model)
	n, ok := m.files.Selected()
	if !ok || n.Path != "gamma.go" {
		t.Errorf("selection = %+v (ok=%v), want the clicked row", n, ok)
	}
	sel, ok := cmd().(uimsg.SelectedMsg)
	if !ok {
		t.Fatalf("expected the click to report a selection, got %T", cmd())
	}
	if sel.ID != "files" || sel.Index != 3 {
		t.Errorf("got %+v, want {files 3}", sel)
	}
}

// tabColumn returns the column that border row starts name at. Every rune in it
// is one cell wide, so a rune offset is a column.
func tabColumn(t *testing.T, m *Model, name string) int {
	t.Helper()
	rows := strings.Split(stripANSI(m.View()), "\n")
	// The right-hand column shares the row, so only the Files panel is searched.
	border := []rune(rows[filesTop])[:32]
	for i := range border {
		if strings.HasPrefix(string(border[i:]), name) {
			return i
		}
	}
	t.Fatalf("the Files panel should name %q in its border, got %q", name, string(border))
	return 0
}

// doubleClick sends two presses on one cell, close enough together to count.
func doubleClick(t *testing.T, m *Model, x, y int) (*Model, tea.Cmd) {
	t.Helper()
	next, _ := m.Update(click(x, y))
	next, cmd := next.(*Model).Update(click(x, y))
	return next.(*Model), cmd
}

// fileRow returns the screen row the Changes tree draws path on.
func fileRow(t *testing.T, m *Model, path string) int {
	t.Helper()
	for i, n := range m.files.Items() {
		if n.Path == path {
			return filesTop + 1 + i
		}
	}
	t.Fatalf("no row for %q in the Changes tree", path)
	return 0
}

func TestDoubleClickingADirectoryFoldsIt(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})

	m, _ = doubleClick(t, m, 4, fileRow(t, m, "src"))
	if !m.collapsedDirs["src"] {
		t.Fatal("a double click on a directory row should fold it")
	}
	m, _ = doubleClick(t, m, 4, fileRow(t, m, "src"))
	if m.collapsedDirs["src"] {
		t.Error("a second double click should unfold it again")
	}
}

func TestDoubleClickingAFileOpensTheEditor(t *testing.T) {
	cfg := config.Default()
	cfg.Editor = config.EditorVim
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
	})
	// With nothing on PATH the editor cannot be resolved, so the attempt says so
	// instead of launching one — which names the file it was going to open.
	fakeBins(t)

	m, _ = doubleClick(t, m, 4, fileRow(t, m, "src/a.go"))
	if view := stripANSI(m.View()); !strings.Contains(view, "can't open src/a.go") {
		t.Errorf("a double click on a file should open it in the editor, got:\n%s", view)
	}
}

func TestDoubleClickingARevisionAsksToUpdateToIt(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{
		{Revision: "42", Author: "alice", Message: "first"},
		{Revision: "41", Author: "bob", Message: "second"},
	}})
	m = next.(*Model)

	// The Log panel's first row is its header, so r41 is the second row under it.
	m, _ = doubleClick(t, m, 4, logTop+2)
	if !m.confirming {
		t.Fatal("a double click on a revision should ask before moving the working copy")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Update to revision?") || !strings.Contains(view, "r41") {
		t.Errorf("expected the prompt to name the double-clicked revision, got:\n%s", view)
	}
}

func TestDoubleClickingAChangelistOpensIt(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, _ := m.Update(click(tabColumn(t, m, "Changelist"), filesTop))
	m = next.(*Model)
	if !m.filesViewIsChangelists() {
		t.Fatal("the Changelists view should be showing")
	}

	m, _ = doubleClick(t, m, 4, filesTop+1)
	if m.filesViews.Depth() != 1 {
		t.Errorf("drill depth = %d, want the double-clicked changelist opened", m.filesViews.Depth())
	}
}

func TestDoubleClickingADiffLineAimsTheEditorAtIt(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	next, _ := m.Update(diffLoadedMsg{path: "src/a.go", diff: scrollableDiff()})
	m = next.(*Model)

	// Row 8 of the diff is the second hunk's header, which starts at file line 201.
	m, _ = doubleClick(t, m, mainLeft+2, 1+8)
	if got := m.main.Cursor(); got != 8 {
		t.Fatalf("the diff cursor is on row %d, want the double-clicked one", got)
	}
	if got := editLine(t, m); got != 201 {
		t.Errorf("line = %d, want 201 — the hunk the click landed in", got)
	}
}

func TestTwoSlowClicksAreNotADoubleClick(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
	})
	row := fileRow(t, m, "src")

	next, _ := m.Update(click(4, row))
	m = next.(*Model)
	// The pause a reader takes between looking at two rows.
	m.lastClick.at = time.Now().Add(-2 * doubleClickWindow)
	next, _ = m.Update(click(4, row))
	m = next.(*Model)

	if m.collapsedDirs["src"] {
		t.Error("two clicks a pause apart are two clicks, not a double click")
	}
}
