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

// toastFrame is what the box adds around its content on each axis: a border
// column plus a padding space on either side horizontally, two border rows
// vertically.
const (
	toastFrameH = 4
	toastFrameV = 2
)

// toastBreakpoints are the characters the message may wrap after, beyond the
// spaces ansi.Wrap always breaks on. Errors are mostly paths and URLs, which
// have no spaces to break at but plenty of separators.
const toastBreakpoints = "/:,;"

// Toast is a small, transient, single-line notice box tinted by level. It is a
// passive display: the composition layer decides when to show or drop it.
type Toast struct {
	message string
	level   Level
	width   int
	height  int
	theme   theme.Theme
}

var (
	_ tui.Component = (*Toast)(nil)
	_ tui.Sizeable  = (*Toast)(nil)
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

// SetSize implements tui.Sizeable. The dimensions are a ceiling rather than a
// fixed frame: the box still hugs a message that fits within them.
func (t *Toast) SetSize(width, height int) { t.width, t.height = width, height }

// SetTheme implements tui.Themeable.
func (t *Toast) SetTheme(th theme.Theme) { t.theme = th }

// View renders the toast as a small colored box. A message may span several
// lines (split on "\n"); the box is sized to the widest line and every line is
// padded to that width so the border stays rectangular.
func (t *Toast) View() string {
	if t.message == "" {
		return ""
	}
	lines := t.fit()
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

// fit breaks the message into the lines the box will hold. Unsized (width ≤ 0)
// it is used as written and the box grows to whatever it needs; sized, it wraps
// to the available width and, once out of rows, drops the rest behind an
// ellipsis. Without this a long svn error draws a box wider than the terminal,
// which then wraps it and breaks the border.
func (t *Toast) fit() []string {
	if t.width <= 0 {
		return strings.Split(t.message, "\n")
	}
	inner := max(t.width-toastFrameH, 1)
	lines := strings.Split(ansi.Wrap(t.message, inner, toastBreakpoints), "\n")
	rows := t.height - toastFrameV
	if t.height <= 0 || len(lines) <= rows {
		return lines
	}
	rows = max(rows, 1)
	lines = lines[:rows]
	lines[rows-1] = ansi.Truncate(lines[rows-1], max(inner-1, 1), "") + "…"
	return lines
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
