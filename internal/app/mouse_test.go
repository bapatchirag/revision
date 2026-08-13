package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
