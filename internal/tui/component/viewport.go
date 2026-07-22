package component

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/keymap"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// Viewport is a scrollable, read-only text area. It renders a vertical window
// over its content and scrolls with the arrow and page keys while focused.
type Viewport struct {
	lines   []string
	offset  int
	width   int
	height  int
	focused bool
	theme   theme.Theme
	keys    keymap.KeyMap
}

var (
	_ tui.Component = (*Viewport)(nil)
	_ tui.Sizeable  = (*Viewport)(nil)
	_ tui.Focusable = (*Viewport)(nil)
	_ tui.Themeable = (*Viewport)(nil)
)

// NewViewport builds an empty viewport.
func NewViewport(th theme.Theme, keys keymap.KeyMap) *Viewport {
	return &Viewport{theme: th, keys: keys}
}

// Init implements tui.Component.
func (v *Viewport) Init() tea.Cmd { return nil }

// SetContent replaces the viewport text and resets scrolling to the top.
func (v *Viewport) SetContent(content string) {
	if content == "" {
		v.lines = nil
	} else {
		v.lines = strings.Split(content, "\n")
	}
	v.offset = 0
	v.clampOffset()
}

// SetSize implements tui.Sizeable.
func (v *Viewport) SetSize(width, height int) {
	v.width, v.height = width, height
	v.clampOffset()
}

// Focus implements tui.Focusable.
func (v *Viewport) Focus() { v.focused = true }

// Blur implements tui.Focusable.
func (v *Viewport) Blur() { v.focused = false }

// Focused implements tui.Focusable.
func (v *Viewport) Focused() bool { return v.focused }

// SetTheme implements tui.Themeable.
func (v *Viewport) SetTheme(th theme.Theme) { v.theme = th }

// Update scrolls the viewport while focused.
func (v *Viewport) Update(m tea.Msg) tea.Cmd {
	if !v.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(km, v.keys.Up):
		v.offset--
	case key.Matches(km, v.keys.Down):
		v.offset++
	case key.Matches(km, v.keys.PageUp):
		v.offset -= v.pageStep()
	case key.Matches(km, v.keys.PageDown):
		v.offset += v.pageStep()
	case key.Matches(km, v.keys.Top):
		v.offset = 0
	case key.Matches(km, v.keys.Bottom):
		v.offset = len(v.lines)
	default:
		return nil
	}
	v.clampOffset()
	return nil
}

// View renders the visible window, padded to width×height.
func (v *Viewport) View() string {
	h := v.height
	if h <= 0 {
		h = len(v.lines)
	}
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		idx := v.offset + i
		if idx < len(v.lines) {
			out = append(out, fitLine(v.lines[idx], v.width))
		} else {
			out = append(out, fitLine("", v.width))
		}
	}
	return strings.Join(out, "\n")
}

func (v *Viewport) pageStep() int {
	if v.height > 1 {
		return v.height - 1
	}
	return 1
}

func (v *Viewport) clampOffset() {
	maxOff := len(v.lines) - v.height
	if maxOff < 0 {
		maxOff = 0
	}
	if v.offset > maxOff {
		v.offset = maxOff
	}
	if v.offset < 0 {
		v.offset = 0
	}
}
