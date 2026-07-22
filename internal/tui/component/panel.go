package component

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// Panel is a bordered, titled, optionally-numbered container that wraps a child
// component. It highlights its border when focused, mirroring lazygit's side
// panels. Focus, size and theme are forwarded to the child.
type Panel struct {
	title   string
	number  int // 0 means no number badge
	child   tui.Component
	theme   theme.Theme
	width   int
	height  int
	focused bool
}

var (
	_ tui.Component = (*Panel)(nil)
	_ tui.Sizeable  = (*Panel)(nil)
	_ tui.Focusable = (*Panel)(nil)
	_ tui.Themeable = (*Panel)(nil)
)

// NewPanel wraps child in a titled border. A number greater than zero renders a
// "[n]" badge in the title, matching lazygit's numbered panels.
func NewPanel(title string, number int, child tui.Component, th theme.Theme) *Panel {
	return &Panel{title: title, number: number, child: child, theme: th}
}

// Init forwards to the child.
func (p *Panel) Init() tea.Cmd {
	if p.child == nil {
		return nil
	}
	return p.child.Init()
}

// Update forwards the message to the child, which acts only while focused.
func (p *Panel) Update(msg tea.Msg) tea.Cmd {
	if p.child == nil {
		return nil
	}
	return p.child.Update(msg)
}

// View renders the child inside the titled border.
func (p *Panel) View() string {
	innerW := p.width - 2
	innerH := p.height - 2
	content := ""
	if p.child != nil {
		content = p.child.View()
	}
	return box(content, p.titleText(), innerW, innerH, p.theme, p.focused)
}

func (p *Panel) titleText() string {
	if p.number > 0 {
		return fmt.Sprintf("[%d] %s", p.number, p.title)
	}
	return p.title
}

// SetSize sets the panel's outer size and propagates the inner size to the
// child when it is sizeable.
func (p *Panel) SetSize(width, height int) {
	p.width, p.height = width, height
	if s, ok := p.child.(tui.Sizeable); ok {
		iw, ih := width-2, height-2
		if iw < 0 {
			iw = 0
		}
		if ih < 0 {
			ih = 0
		}
		s.SetSize(iw, ih)
	}
}

// Focus focuses the panel and its child (when focusable).
func (p *Panel) Focus() {
	p.focused = true
	if f, ok := p.child.(tui.Focusable); ok {
		f.Focus()
	}
}

// Blur removes focus from the panel and its child.
func (p *Panel) Blur() {
	p.focused = false
	if f, ok := p.child.(tui.Focusable); ok {
		f.Blur()
	}
}

// Focused reports whether the panel currently holds focus.
func (p *Panel) Focused() bool { return p.focused }

// SetTheme updates the panel and child palette.
func (p *Panel) SetTheme(th theme.Theme) {
	p.theme = th
	if t, ok := p.child.(tui.Themeable); ok {
		t.SetTheme(th)
	}
}
