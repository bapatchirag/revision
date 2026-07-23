package component_test

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/keymap"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// TestMain forces the ASCII color profile so golden output is deterministic
// (no ANSI escapes) regardless of the host terminal.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func testTheme() theme.Theme  { return theme.Default() }
func testKeys() keymap.KeyMap { return keymap.Default() }

// harness adapts a tui.Component (whose Update returns only a command) to a
// tea.Model so teatest can drive it as a real program.
type harness struct{ c tui.Component }

func asModel(c tui.Component) harness { return harness{c: c} }

func (h harness) Init() tea.Cmd { return h.c.Init() }

func (h harness) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.QuitMsg); ok {
		return h, tea.Quit
	}
	return h, h.c.Update(msg)
}

func (h harness) View() string { return h.c.View() }

// mustCmd runs cmd and returns its message, failing when cmd is nil.
func mustCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	return cmd()
}

func keyDown() tea.KeyMsg  { return tea.KeyMsg{Type: tea.KeyDown} }
func keyUp() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyUp} }
func keyTab() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyTab} }
func keyEnter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }
func keyEsc() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyEsc} }

func keyBackspace() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyBackspace} }
func keyCtrlS() tea.KeyMsg     { return tea.KeyMsg{Type: tea.KeyCtrlS} }

// runes builds a key message that types the given text.
func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
