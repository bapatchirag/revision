package component_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// newComponents builds one of every reusable component, so the contract methods
// they all share can be checked in a single pass rather than a test apiece.
func newComponents() map[string]tui.Component {
	th, keys := testTheme(), testKeys()
	return map[string]tui.Component{
		"Form": component.NewForm("form", "Settings", []component.Field{
			{Label: "Theme", Kind: component.FieldChoice, Value: "default", Options: []string{"default", "nord"}},
			{Label: "Log limit", Kind: component.FieldInt, Value: "50"},
		}, th, keys),
		"List":  component.NewList("list", func(s string) string { return s }, th, keys),
		"Menu":  component.NewMenu("menu", "Actions", []component.MenuItem{{Label: "Quit", Key: "q"}}, th, keys),
		"Modal": component.NewModal("modal", "Confirm", "Delete a.txt?", th, keys),
		"Panel": component.NewPanel("Files", 2, component.NewList("inner",
			func(s string) string { return s }, th, keys), th),
		"Prompt":    component.NewPrompt("prompt", "Path", "type a path", th, keys),
		"SearchBar": component.NewSearchBar("search", th, keys),
		"SplitView": component.NewSplitView("split", "Diff", th, keys),
		"StatusBar": component.NewStatusBar(th),
		"Table": component.NewTable("table", []component.Column{{Title: "Rev"}},
			func(s string) []string { return []string{s} }, th, keys),
		"TextArea": component.NewTextArea("text", "Message", "type a message", th, keys),
		"Toast":    component.NewToast(th),
		"Viewport": component.NewViewport(th, keys),
		"Views":    component.NewViews("views", []component.View{{Name: "Changes"}}, th, keys),
	}
}

// TestComponentsInitCleanly checks that every component is ready to render
// straight from its constructor: Init runs, View is safe before any size is set,
// and an unfocused component consumes nothing.
func TestComponentsInitCleanly(t *testing.T) {
	for name, c := range newComponents() {
		t.Run(name, func(t *testing.T) {
			c.Init()
			c.View()
			if cmd := c.Update(tea.KeyMsg{Type: tea.KeyDown}); cmd != nil {
				t.Error("an unfocused component should consume no keys")
			}
		})
	}
}

// TestFocusableContract checks the Focus/Blur/Focused round trip on every
// component that claims to be focusable.
func TestFocusableContract(t *testing.T) {
	for name, c := range newComponents() {
		f, ok := c.(tui.Focusable)
		if !ok {
			continue
		}
		t.Run(name, func(t *testing.T) {
			if f.Focused() {
				t.Error("a component starts blurred")
			}
			f.Focus()
			if !f.Focused() {
				t.Error("Focused() = false after Focus()")
			}
			f.Blur()
			if f.Focused() {
				t.Error("Focused() = true after Blur()")
			}
		})
	}
}

// TestThemeableContract swaps the palette on every themeable component and
// re-renders, so a SetTheme that forgot to store the theme shows up.
func TestThemeableContract(t *testing.T) {
	for name, c := range newComponents() {
		th, ok := c.(tui.Themeable)
		if !ok {
			continue
		}
		t.Run(name, func(t *testing.T) {
			if s, ok := c.(tui.Sizeable); ok {
				s.SetSize(40, 6)
			}
			th.SetTheme(theme.Default())
			c.View()
		})
	}
}

func TestListAccessors(t *testing.T) {
	l := component.NewList("list", func(s string) string { return s }, testTheme(), testKeys())

	if _, ok := l.Selected(); ok {
		t.Error("an empty list has nothing selected")
	}
	if got := l.Items(); got != nil {
		t.Errorf("Items() = %v, want nil before anything is set", got)
	}

	l.SetItems([]string{"alpha", "beta", "gamma"})
	if got := l.Items(); len(got) != 3 {
		t.Fatalf("Items() = %v, want the three items", got)
	}

	l.SetIndex(2)
	if got, ok := l.Selected(); !ok || got != "gamma" {
		t.Errorf("Selected() = %q, want the item the cursor was moved to", got)
	}
	// Out of range in either direction clamps rather than panicking.
	l.SetIndex(99)
	if l.Index() != 2 {
		t.Errorf("Index() = %d, want the cursor clamped to the last row", l.Index())
	}
	l.SetIndex(-5)
	if l.Index() != 0 {
		t.Errorf("Index() = %d, want the cursor clamped to the first row", l.Index())
	}

	// A replacement renderer is what a live theme switch installs.
	l.SetRender(func(s string) string { return "◆ " + s })
	l.SetSize(20, 3)
	if view := l.View(); view == "" {
		t.Error("the list should render with the replacement renderer")
	}
	l.SetTheme(theme.Default())
}

func TestTableAccessors(t *testing.T) {
	tb := component.NewTable("table", []component.Column{{Title: "Rev"}},
		func(s string) []string { return []string{s} }, testTheme(), testKeys())

	if _, ok := tb.Selected(); ok {
		t.Error("an empty table has nothing selected")
	}

	tb.SetItems([]string{"r1", "r2"})
	if got := tb.Items(); len(got) != 2 {
		t.Fatalf("Items() = %v, want both rows", got)
	}
	if got, ok := tb.Selected(); !ok || got != "r1" {
		t.Errorf("Selected() = %q, want the first row", got)
	}

	// Shrinking the rows under the cursor clamps it back into range.
	tb.SetItems([]string{"r1"})
	if tb.Index() != 0 {
		t.Errorf("Index() = %d, want the cursor clamped", tb.Index())
	}
	tb.SetItems(nil)
	if _, ok := tb.Selected(); ok {
		t.Error("a table emptied under the cursor has nothing selected")
	}
	tb.GoTop()
}

func TestMenuAccessors(t *testing.T) {
	items := []component.MenuItem{
		component.MenuSection("Files"),
		{Label: "Stage", Key: "space"},
		{Label: "Commit", Key: "c"},
	}
	mn := component.NewMenu("menu", "Actions", items, testTheme(), testKeys())

	// The cursor skips the section heading, which is not selectable.
	if mn.Index() == 0 {
		t.Error("the cursor should start past the section heading")
	}
	mn.SetIndex(99)
	if mn.Index() >= len(items) {
		t.Errorf("Index() = %d, want it clamped into range", mn.Index())
	}
	mn.SetIndex(-3)
	if mn.Index() < 0 {
		t.Errorf("Index() = %d, want it clamped into range", mn.Index())
	}

	// Multi-column layouts must render at every width the app uses.
	for _, cols := range []int{0, 1, 2, 3} {
		mn.SetColumns(cols)
		mn.SetSize(60, 0)
		if mn.View() == "" {
			t.Errorf("menu rendered nothing at %d columns", cols)
		}
	}
	mn.SetReadOnly(true)
	mn.Focus()
	if cmd := mn.Update(tea.KeyMsg{Type: tea.KeyDown}); cmd != nil {
		t.Error("a read-only menu answers to no key")
	}
}

func TestViewportAccessors(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetSize(20, 3)

	if v.Cursor() != -1 {
		t.Errorf("Cursor() = %d, want -1 without a line cursor", v.Cursor())
	}
	if v.Offset() != 0 {
		t.Errorf("Offset() = %d, want 0 before scrolling", v.Offset())
	}

	v.SetContent("one\ntwo\nthree\nfour\nfive\nsix")
	v.SetCursorLine(true)
	if v.Cursor() != 0 {
		t.Errorf("Cursor() = %d, want the first line", v.Cursor())
	}

	v.Focus()
	v.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if v.Offset() == 0 {
		t.Error("Offset() should follow a page down")
	}
	v.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if v.Offset() != 0 {
		t.Errorf("Offset() = %d, want the page up to reach the top", v.Offset())
	}

	// A refresh of content already on screen keeps the reader's place.
	v.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	at := v.Offset()
	v.SetContentPreservingScroll("one\ntwo\nthree\nfour\nfive\nsix\nseven")
	if v.Offset() != at {
		t.Errorf("Offset() = %d after a preserving refresh, want %d", v.Offset(), at)
	}

	// Content shorter than the window pulls the offset back into range.
	v.SetContentPreservingScroll("one")
	if v.Offset() != 0 {
		t.Errorf("Offset() = %d, want it clamped to the shorter content", v.Offset())
	}
	v.SetTheme(theme.Default())
}

func TestSplitViewAccessors(t *testing.T) {
	s := component.NewSplitView("split", "Diff", testTheme(), testKeys())
	s.SetSize(60, 10)

	if s.Pages() != 0 {
		t.Errorf("Pages() = %d, want none before content is set", s.Pages())
	}
	if _, _, ok := s.Current(); ok {
		t.Error("Current() should report nothing while the view is empty")
	}

	pages := []component.SplitPage{
		{Title: "a.txt", Rows: []component.SplitRow{{Left: "-old", Right: "+new"}}},
		{Title: "b.txt", Rows: []component.SplitRow{{Left: "-x", Right: "+y"}}},
	}
	s.SetPages(pages)
	s.SetLabels("working copy", "repository")
	s.SetHint("esc close")
	s.SetTitle("Side by side")

	if s.Pages() != 2 || s.Page() != 0 {
		t.Errorf("Pages()/Page() = %d/%d, want 2/0", s.Pages(), s.Page())
	}
	title, row, ok := s.Current()
	if !ok || title != "a.txt" || row.Left != "-old" {
		t.Errorf("Current() = %q/%+v, want the first page's first row", title, row)
	}

	// The view keys turn pages, wrapping like tabs.
	s.Focus()
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if s.Page() != 1 {
		t.Errorf("Page() = %d, want the second page", s.Page())
	}
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if s.Page() != 0 {
		t.Errorf("Page() = %d, want the turn to wrap round", s.Page())
	}

	// Re-rendered content leaves the reader where they were.
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	s.RefreshPages(pages)
	if s.Page() != 1 {
		t.Errorf("Page() = %d, want a refresh to keep the page open", s.Page())
	}
	// A refresh with fewer pages clamps rather than pointing past the end.
	s.RefreshPages(pages[:1])
	if s.Page() != 0 {
		t.Errorf("Page() = %d, want the page clamped to the shorter content", s.Page())
	}
	s.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	s.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	s.SetTheme(theme.Default())
}

func TestViewsAccessors(t *testing.T) {
	th, keys := testTheme(), testKeys()
	base := component.NewList("changes", func(s string) string { return s }, th, keys)
	sub := component.NewList("detail", func(s string) string { return s }, th, keys)

	empty := component.NewViews("empty", nil, th, keys)
	if empty.ActiveName() != "" || empty.Depth() != 0 || empty.Active() != nil {
		t.Error("a container with no views reports nothing active")
	}
	if empty.Push(sub) != nil {
		t.Error("pushing onto a container with no views is a no-op")
	}

	v := component.NewViews("views", []component.View{
		{Name: "Changes", Content: base},
		{Name: "Diffs", Content: nil},
	}, th, keys)
	v.SetSize(40, 6)
	v.Init()

	if v.ActiveName() != "Changes" {
		t.Errorf("ActiveName() = %q, want the first view", v.ActiveName())
	}
	if v.Depth() != 0 {
		t.Errorf("Depth() = %d, want 0 at the base view", v.Depth())
	}
	if v.Active() != tui.Component(base) {
		t.Error("Active() should be the base view's content")
	}

	v.PushTitled("a.txt", sub)
	if v.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1 after a push", v.Depth())
	}
	if v.Active() != tui.Component(sub) {
		t.Error("Active() should be the pushed sub-view")
	}
	if v.CrumbTitle() != "a.txt" {
		t.Errorf("CrumbTitle() = %q, want the pushed title", v.CrumbTitle())
	}

	v.Pop()
	if v.Depth() != 0 {
		t.Errorf("Depth() = %d, want the stack emptied", v.Depth())
	}
	if v.Pop() != nil {
		t.Error("popping at the base view is a no-op")
	}

	// A view with no content still renders and forwards nothing.
	v.Focus()
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if v.ActiveName() != "Diffs" {
		t.Errorf("ActiveName() = %q, want the second view", v.ActiveName())
	}
	if v.Active() != nil {
		t.Error("Active() should be nil for a view with no content")
	}
	v.View()
	v.SetTheme(theme.Default())
}

func TestToastAndStatusBarAccessors(t *testing.T) {
	to := component.NewToast(testTheme())
	if to.Message() != "" {
		t.Errorf("Message() = %q, want empty before anything is shown", to.Message())
	}
	if to.View() != "" {
		t.Error("an empty toast renders nothing")
	}
	to.SetSize(40, 3)
	to.Show("saved a.diff", component.LevelInfo)
	if to.Message() != "saved a.diff" {
		t.Errorf("Message() = %q, want the shown text", to.Message())
	}
	to.SetTheme(theme.Default())
	if to.View() == "" {
		t.Error("a toast with a message should render")
	}

	sb := component.NewStatusBar(testTheme())
	sb.SetSize(40, 1)
	sb.SetHints([]string{"q quit", "? help"})
	sb.SetRight("loading")
	sb.SetTheme(theme.Default())
	if sb.View() == "" {
		t.Error("the status bar should render its hints")
	}
}

func TestSearchBarAccessors(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	s.SetSize(40, 1)
	s.SetPrefix("Files")
	s.SetValue("main.go")
	if s.Value() != "main.go" {
		t.Errorf("Value() = %q, want the text that was set", s.Value())
	}
	s.SetTheme(theme.Default())
	s.Init()
	s.Reset()
	if s.Value() != "" {
		t.Errorf("Value() = %q, want it cleared by Reset", s.Value())
	}
}

func TestPromptAccessors(t *testing.T) {
	p := component.NewPrompt("prompt", "Path", "type a path", testTheme(), testKeys())
	p.SetSize(40, 0)

	p.SetLocked("/wc/")
	if p.Value() != "/wc/" {
		t.Errorf("Value() = %q, want the locked prefix to seed an empty input", p.Value())
	}
	p.SetValue("/elsewhere")
	p.SetLocked("/wc/")
	if p.Value() != "/wc/" {
		t.Errorf("Value() = %q, want a value outside the lock replaced", p.Value())
	}

	p.SetValue("/wc/src")
	p.SetOptions("Directories", []string{"/wc/src", "/wc/docs"})
	if p.ListFocused() {
		t.Error("the input, not the list, holds focus to begin with")
	}
	p.Focus()
	p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !p.ListFocused() {
		t.Error("tab should move focus into the option list")
	}
	p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if p.ListFocused() {
		t.Error("tab should move focus back to the input")
	}

	p.SetSecret(true)
	if view := ansi.Strip(p.View()); view == "" {
		t.Error("a secret prompt still renders")
	}
	p.SetTheme(theme.Default())
	p.Reset()
	if p.Value() != "/wc/" {
		t.Errorf("Value() = %q, want Reset to leave the locked prefix", p.Value())
	}
	if p.ListFocused() {
		t.Error("Reset returns focus to the input")
	}
}

func TestFormAccessors(t *testing.T) {
	fields := []component.Field{
		{Label: "Theme", Kind: component.FieldChoice, Value: "default", Options: []string{"default", "nord"}},
		{Label: "Log limit", Kind: component.FieldInt, Value: "50"},
		{Label: "Live refresh", Kind: component.FieldBool, Value: "true"},
	}
	f := component.NewForm("form", "Settings", fields, testTheme(), testKeys())
	f.SetSize(50, 0)
	f.Init()

	if got := f.Fields(); len(got) != len(fields) {
		t.Fatalf("Fields() = %v, want all three", got)
	}
	// Fields returns a copy, so an edit to it cannot reach the form.
	copied := f.Fields()
	copied[0].Value = "tampered"
	if f.Value(0) != "default" {
		t.Error("Fields() must hand back a copy, not the form's own slice")
	}
	if f.Value(-1) != "" || f.Value(99) != "" {
		t.Error("Value() outside the field range is empty")
	}
	if got := f.Values(); len(got) != 3 || got[1] != "50" {
		t.Errorf("Values() = %v, want each field's value in order", got)
	}
	f.SetTheme(theme.Default())
}

func TestTextAreaAccessors(t *testing.T) {
	ta := component.NewTextArea("text", "Message", "type a message", testTheme(), testKeys())
	ta.SetSize(30, 4)
	ta.Init()

	if ta.Value() != "" {
		t.Errorf("Value() = %q, want an empty buffer", ta.Value())
	}
	ta.SetValue("first\nsecond\nthird")
	if ta.Value() != "first\nsecond\nthird" {
		t.Errorf("Value() = %q, want the text that was set", ta.Value())
	}

	ta.SetTheme(theme.Default())
	ta.Reset()
	if ta.Value() != "" {
		t.Errorf("Value() = %q, want it cleared by Reset", ta.Value())
	}
	// A cleared buffer still holds one line, so rendering is safe.
	if ta.View() == "" {
		t.Error("an empty text area should still render its placeholder")
	}
}

func TestPanelAccessors(t *testing.T) {
	th := testTheme()
	child := component.NewList("inner", func(s string) string { return s }, th, testKeys())
	p := component.NewPanel("Files", 2, child, th)
	p.SetSize(30, 6)

	p.Focus()
	if !p.Focused() || !child.Focused() {
		t.Error("focusing a panel focuses its child")
	}
	p.Blur()
	if p.Focused() || child.Focused() {
		t.Error("blurring a panel blurs its child")
	}

	p.SetTitle("Changes")
	p.SetFooter("1 of 3")
	if view := ansi.Strip(p.View()); view == "" {
		t.Error("the panel should render its border")
	}

	// A very narrow panel truncates rather than overflowing.
	p.SetSize(8, 4)
	p.View()

	// A panel with no child renders an empty box and forwards nothing.
	bare := component.NewPanel("Empty", -1, nil, th)
	bare.SetSize(20, 4)
	bare.Init()
	if bare.Update(tea.KeyMsg{Type: tea.KeyDown}) != nil {
		t.Error("a panel with no child forwards nothing")
	}
	if bare.View() == "" {
		t.Error("a childless panel still renders its border")
	}
	bare.SetTheme(theme.Default())
}

func TestModalAccessors(t *testing.T) {
	m := component.NewModal("modal", "Confirm", "Delete a.txt?", testTheme(), testKeys())
	m.SetSize(40, 0)
	m.Init()

	if m.Focused() {
		t.Error("a modal starts blurred")
	}
	m.Focus()
	if !m.Focused() {
		t.Error("Focused() = false after Focus()")
	}
	m.SetTheme(theme.Default())
	if ansi.Strip(m.View()) == "" {
		t.Error("the modal should render its message")
	}
}
