package component

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/keymap"
	"github.com/bapatchirag/revision/internal/tui/msg"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// View is one named tab within a Views container: a label plus the component
// that renders that view's data.
type View struct {
	Name    string
	Content tui.Component
}

// Views is a multi-view container: several named views share a single space,
// cycled with [ and ] (PrevView/NextView). Each view can additionally be drilled
// into a stack of unnamed sub-views — detail cascades — pushed by the composing
// layer (Push) in response to enter and popped with esc (Back). It forwards
// focus, size, theme and every other key to whichever component is on top: the
// active view's base content or its deepest pushed sub-view. Hosted components
// therefore need no awareness of the container.
//
// Views renders only the active component; the view names and any drill
// breadcrumb are drawn by the host Panel in its border (see the tabbed
// interface), so a multi-view panel costs no extra content rows.
type Views struct {
	id      string
	views   []View
	stacks  [][]tui.Component // per-view drill stacks of unnamed sub-views
	active  int
	width   int
	height  int
	focused bool
	theme   theme.Theme
	keys    keymap.KeyMap
}

var (
	_ tui.Component = (*Views)(nil)
	_ tui.Sizeable  = (*Views)(nil)
	_ tui.Focusable = (*Views)(nil)
	_ tui.Themeable = (*Views)(nil)
)

// NewViews builds a multi-view container identified by id (used on emitted
// messages) over the given named views. The first view starts active.
func NewViews(id string, views []View, th theme.Theme, keys keymap.KeyMap) *Views {
	return &Views{
		id:     id,
		views:  views,
		stacks: make([][]tui.Component, len(views)),
		theme:  th,
		keys:   keys,
	}
}

// Init forwards to every base view so each is ready before it is first shown.
func (v *Views) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(v.views))
	for _, view := range v.views {
		if view.Content != nil {
			if c := view.Content.Init(); c != nil {
				cmds = append(cmds, c)
			}
		}
	}
	return tea.Batch(cmds...)
}

// Update, while focused, consumes the view-switch keys ([ / ]) and the pop key
// (esc, when drilled in); every other message is forwarded to the component on
// top so it can act as usual.
func (v *Views) Update(m tea.Msg) tea.Cmd {
	if !v.focused || len(v.views) == 0 {
		return nil
	}
	if km, ok := m.(tea.KeyMsg); ok {
		switch {
		case key.Matches(km, v.keys.NextView):
			return v.switchView(v.active + 1)
		case key.Matches(km, v.keys.PrevView):
			return v.switchView(v.active - 1)
		case key.Matches(km, v.keys.Back):
			if v.Depth() > 0 {
				return v.Pop()
			}
			// Nothing to pop: fall through so the base view (or a parent) sees esc.
		}
	}
	if top := v.top(); top != nil {
		return top.Update(m)
	}
	return nil
}

// View renders the active component. The tab labels live in the host Panel's
// border, so nothing is prepended here.
func (v *Views) View() string {
	if top := v.top(); top != nil {
		return top.View()
	}
	return ""
}

// ActiveIndex returns the active view's position.
func (v *Views) ActiveIndex() int { return v.active }

// Tabs returns the container's view names, for the host Panel to inlay into its
// border. ActiveIndex and Depth report which view is active and how deeply it is
// drilled.
func (v *Views) Tabs() []string {
	names := make([]string, len(v.views))
	for i, view := range v.views {
		names[i] = view.Name
	}
	return names
}

// ActiveName returns the active view's name.
func (v *Views) ActiveName() string {
	if len(v.views) == 0 {
		return ""
	}
	return v.views[v.active].Name
}

// Active returns the component currently on top (a pushed sub-view or, when the
// active view is not drilled in, its base content).
func (v *Views) Active() tui.Component { return v.top() }

// Depth reports how many sub-views are stacked on the active view (0 at base).
func (v *Views) Depth() int {
	if len(v.views) == 0 {
		return 0
	}
	return len(v.stacks[v.active])
}

// Push drills the active view into an unnamed sub-view (a detail cascade),
// focusing and sizing it, and returns the sub-view's Init command.
func (v *Views) Push(sub tui.Component) tea.Cmd {
	if len(v.views) == 0 || sub == nil {
		return nil
	}
	v.blurTop()
	v.stacks[v.active] = append(v.stacks[v.active], sub)
	v.focusTop()
	v.sizeTop()
	return sub.Init()
}

// Pop removes the active view's top sub-view, refocusing and resizing whatever
// is beneath, and emits SubViewPoppedMsg. It is a no-op at the base view.
func (v *Views) Pop() tea.Cmd {
	st := v.stacks[v.active]
	if len(st) == 0 {
		return nil
	}
	v.blurTop()
	v.stacks[v.active] = st[:len(st)-1]
	v.focusTop()
	v.sizeTop()
	id, depth := v.id, v.Depth()
	return func() tea.Msg { return msg.SubViewPoppedMsg{ID: id, Depth: depth} }
}

// SetSize stores the container size and sizes the active component, reserving a
// row for the strip when one is shown.
func (v *Views) SetSize(width, height int) {
	v.width, v.height = width, height
	v.sizeTop()
}

// Focus focuses the container and the active component.
func (v *Views) Focus() {
	v.focused = true
	v.focusTop()
}

// Blur removes focus from the container and the active component.
func (v *Views) Blur() {
	v.blurTop()
	v.focused = false
}

// Focused reports whether the container holds focus.
func (v *Views) Focused() bool { return v.focused }

// SetTheme updates the container palette and propagates it to every hosted
// component (base views and every pushed sub-view).
func (v *Views) SetTheme(th theme.Theme) {
	v.theme = th
	for _, view := range v.views {
		if t, ok := view.Content.(tui.Themeable); ok {
			t.SetTheme(th)
		}
	}
	for _, st := range v.stacks {
		for _, sub := range st {
			if t, ok := sub.(tui.Themeable); ok {
				t.SetTheme(th)
			}
		}
	}
}

// switchView moves the active view to i (wrapping), preserving each view's own
// drill stack, and emits ViewSelectedMsg. A lone view has nothing to switch to.
func (v *Views) switchView(i int) tea.Cmd {
	if len(v.views) <= 1 {
		return nil
	}
	n := len(v.views)
	i = ((i % n) + n) % n
	if i == v.active {
		return nil
	}
	v.blurTop()
	v.active = i
	v.focusTop()
	v.sizeTop()
	id, idx, name := v.id, v.active, v.views[v.active].Name
	return func() tea.Msg { return msg.ViewSelectedMsg{ID: id, Index: idx, Name: name} }
}

// top returns the component currently rendered for the active view: the deepest
// pushed sub-view, or the view's base content when nothing is drilled in.
func (v *Views) top() tui.Component {
	if len(v.views) == 0 {
		return nil
	}
	if st := v.stacks[v.active]; len(st) > 0 {
		return st[len(st)-1]
	}
	return v.views[v.active].Content
}

// focusTop focuses the component on top when the container itself is focused.
func (v *Views) focusTop() {
	if !v.focused {
		return
	}
	if f, ok := v.top().(tui.Focusable); ok {
		f.Focus()
	}
}

// blurTop blurs the component on top.
func (v *Views) blurTop() {
	if f, ok := v.top().(tui.Focusable); ok {
		f.Blur()
	}
}

// sizeTop sizes the component on top to the full container area (the tab labels
// live in the host Panel's border, so they cost no content rows).
func (v *Views) sizeTop() {
	if s, ok := v.top().(tui.Sizeable); ok {
		s.SetSize(v.width, v.height)
	}
}
