package component

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// Level classifies a toast for coloring.
type Level int

const (
	// LevelInfo is a neutral notice.
	LevelInfo Level = iota
	// LevelSuccess reports a successful action.
	LevelSuccess
	// LevelWarning reports a recoverable problem.
	LevelWarning
	// LevelError reports a failure.
	LevelError
)

// Toast is a small, transient, single-line notice box tinted by level. It is a
// passive display: the composition layer decides when to show or drop it.
type Toast struct {
	message string
	level   Level
	theme   theme.Theme
}

var (
	_ tui.Component = (*Toast)(nil)
	_ tui.Themeable = (*Toast)(nil)
)

// NewToast builds an empty toast.
func NewToast(th theme.Theme) *Toast {
	return &Toast{theme: th}
}

// Init implements tui.Component.
func (t *Toast) Init() tea.Cmd { return nil }

// Update implements tui.Component; the toast is passive.
func (t *Toast) Update(tea.Msg) tea.Cmd { return nil }

// Show sets the toast message and level.
func (t *Toast) Show(message string, level Level) {
	t.message, t.level = message, level
}

// Message returns the current message.
func (t *Toast) Message() string { return t.message }

// SetTheme implements tui.Themeable.
func (t *Toast) SetTheme(th theme.Theme) { t.theme = th }

// View renders the toast as a small colored box. A message may span several
// lines (split on "\n"); the box is sized to the widest line and every line is
// padded to that width so the border stays rectangular.
func (t *Toast) View() string {
	if t.message == "" {
		return ""
	}
	lines := strings.Split(t.message, "\n")
	contentW := 0
	for _, ln := range lines {
		if n := ansi.StringWidth(ln); n > contentW {
			contentW = n
		}
	}
	bs := lipgloss.NewStyle().Foreground(t.color())
	w := contentW + 2 // a space of padding on each side
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, bs.Render(borderTopLeft+strings.Repeat(borderHorizontal, w)+borderTopRight))
	for _, ln := range lines {
		label := " " + ln + strings.Repeat(" ", contentW-ansi.StringWidth(ln)) + " "
		rows = append(rows, bs.Render(borderVertical)+bs.Render(label)+bs.Render(borderVertical))
	}
	rows = append(rows, bs.Render(borderBottomLeft+strings.Repeat(borderHorizontal, w)+borderBottomRight))
	return strings.Join(rows, "\n")
}

func (t *Toast) color() lipgloss.Color {
	switch t.level {
	case LevelSuccess:
		return t.theme.Success
	case LevelWarning:
		return t.theme.Warning
	case LevelError:
		return t.theme.Error
	default:
		return t.theme.Info
	}
}
