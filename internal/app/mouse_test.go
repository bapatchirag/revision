package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
