package component_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/msg"
)

// TestFitLineExpandsTabs guards the fixed-width layout against raw tabs: a line
// containing a tab (as in svn diff output) must render to exactly the cell width
// with no tab left for the terminal to expand — otherwise the line wraps and the
// whole frame overflows.
func TestFitLineExpandsTabs(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("a\tb")
	v.SetSize(24, 1)

	out := v.View()
	if strings.Contains(out, "\t") {
		t.Fatalf("view must not contain a raw tab: %q", out)
	}
	if !strings.Contains(out, "a"+strings.Repeat(" ", 4)+"b") {
		t.Errorf("tab was not expanded to spaces: %q", out)
	}
	if w := ansi.StringWidth(out); w != 24 {
		t.Errorf("rendered width = %d, want 24", w)
	}
}

func TestListEmitsSelected(t *testing.T) {
	l := component.NewList[string]("files", func(s string) string { return s }, testTheme(), testKeys())
	l.SetItems([]string{"a", "b", "c"})
	l.SetSize(10, 3)
	l.Focus()

	got := mustCmd(t, l.Update(keyDown()))
	sel, ok := got.(msg.SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg, got %T", got)
	}
	if sel.ID != "files" || sel.Index != 1 {
		t.Errorf("got %+v, want {files 1}", sel)
	}
}

func TestListEmitsActivated(t *testing.T) {
	l := component.NewList[string]("files", func(s string) string { return s }, testTheme(), testKeys())
	l.SetItems([]string{"a", "b"})
	l.SetSize(10, 3)
	l.Focus()

	got := mustCmd(t, l.Update(keyEnter()))
	act, ok := got.(msg.ActivatedMsg)
	if !ok {
		t.Fatalf("expected ActivatedMsg, got %T", got)
	}
	if act.ID != "files" || act.Index != 0 {
		t.Errorf("got %+v, want {files 0}", act)
	}
}

func TestListIgnoresInputWhenBlurred(t *testing.T) {
	l := component.NewList[string]("files", func(s string) string { return s }, testTheme(), testKeys())
	l.SetItems([]string{"a", "b"})
	l.SetSize(10, 3)

	if cmd := l.Update(keyDown()); cmd != nil {
		t.Error("blurred list should ignore key input")
	}
	if l.Index() != 0 {
		t.Errorf("cursor moved while blurred: %d", l.Index())
	}
}

func newStringTable() *component.Table[[]string] {
	return component.NewTable[[]string]("log", []component.Column{
		{Title: "Rev", Width: 4},
		{Title: "Message", Width: 0},
	}, func(r []string) []string { return r }, testTheme(), testKeys())
}

func TestTableEmitsSelected(t *testing.T) {
	tb := newStringTable()
	tb.SetItems([][]string{{"r3", "a"}, {"r2", "b"}, {"r1", "c"}})
	tb.SetSize(20, 4)
	tb.Focus()

	got := mustCmd(t, tb.Update(keyDown()))
	sel, ok := got.(msg.SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg, got %T", got)
	}
	if sel.ID != "log" || sel.Index != 1 {
		t.Errorf("got %+v, want {log 1}", sel)
	}
}

func TestTableEmitsActivated(t *testing.T) {
	tb := newStringTable()
	tb.SetItems([][]string{{"r2", "a"}, {"r1", "b"}})
	tb.SetSize(20, 4)
	tb.Focus()

	got := mustCmd(t, tb.Update(keyEnter()))
	act, ok := got.(msg.ActivatedMsg)
	if !ok {
		t.Fatalf("expected ActivatedMsg, got %T", got)
	}
	if act.ID != "log" || act.Index != 0 {
		t.Errorf("got %+v, want {log 0}", act)
	}
}

func TestTableIgnoresInputWhenBlurred(t *testing.T) {
	tb := newStringTable()
	tb.SetItems([][]string{{"r2", "a"}, {"r1", "b"}})
	tb.SetSize(20, 4)

	if cmd := tb.Update(keyDown()); cmd != nil {
		t.Error("blurred table should ignore key input")
	}
	if tb.Index() != 0 {
		t.Errorf("cursor moved while blurred: %d", tb.Index())
	}
}

func TestMenuEmitsActivatedAndDismiss(t *testing.T) {
	items := []component.MenuItem{{Label: "Commit", Key: "c"}, {Label: "Revert", Key: "r"}}
	mn := component.NewMenu("actions", "Actions", items, testTheme(), testKeys())
	mn.Focus()

	mn.Update(keyDown())
	got := mustCmd(t, mn.Update(keyEnter()))
	act, ok := got.(msg.ActivatedMsg)
	if !ok {
		t.Fatalf("expected ActivatedMsg, got %T", got)
	}
	if act.ID != "actions" || act.Index != 1 {
		t.Errorf("got %+v, want {actions 1}", act)
	}

	dismiss := mustCmd(t, mn.Update(keyEsc()))
	if d, ok := dismiss.(msg.DismissMsg); !ok || d.ID != "actions" {
		t.Errorf("expected DismissMsg{actions}, got %#v", dismiss)
	}
}

func TestModalEmitsConfirmAndDismiss(t *testing.T) {
	mo := component.NewModal("confirm", "Delete?", "gone forever", testTheme(), testKeys())
	mo.Focus()

	confirm := mustCmd(t, mo.Update(keyEnter()))
	if c, ok := confirm.(msg.ConfirmMsg); !ok || c.ID != "confirm" {
		t.Errorf("expected ConfirmMsg{confirm}, got %#v", confirm)
	}

	dismiss := mustCmd(t, mo.Update(keyEsc()))
	if d, ok := dismiss.(msg.DismissMsg); !ok || d.ID != "confirm" {
		t.Errorf("expected DismissMsg{confirm}, got %#v", dismiss)
	}
}

func TestModalSetPromptUpdatesView(t *testing.T) {
	mo := component.NewModal("confirm", "Delete file?", "gone forever", testTheme(), testKeys())
	mo.SetSize(40, 0)
	mo.SetPrompt("Revert changes?", "Discard local changes to app.go?")

	view := mo.View()
	if !strings.Contains(view, "Revert changes?") {
		t.Errorf("view should show the new title, got:\n%s", view)
	}
	if !strings.Contains(view, "Discard local changes to app.go?") {
		t.Errorf("view should show the new message, got:\n%s", view)
	}
	if strings.Contains(view, "gone forever") {
		t.Errorf("view should drop the old message, got:\n%s", view)
	}
}

func TestTextAreaEmitsSubmit(t *testing.T) {
	ta := component.NewTextArea("commit", "Commit", "", testTheme(), testKeys())
	ta.SetSize(30, 6)
	ta.Focus()

	ta.Update(runes("hi"))
	got := mustCmd(t, ta.Update(keyCtrlS()))
	sub, ok := got.(msg.SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", got)
	}
	if sub.ID != "commit" || sub.Value != "hi" {
		t.Errorf("got %+v, want {commit hi}", sub)
	}
}

func TestTextAreaEmitsDismiss(t *testing.T) {
	ta := component.NewTextArea("commit", "Commit", "", testTheme(), testKeys())
	ta.SetSize(30, 6)
	ta.Focus()

	got := mustCmd(t, ta.Update(keyEsc()))
	if d, ok := got.(msg.DismissMsg); !ok || d.ID != "commit" {
		t.Errorf("expected DismissMsg{commit}, got %#v", got)
	}
}

func TestTextAreaEditsMultiLine(t *testing.T) {
	ta := component.NewTextArea("commit", "Commit", "", testTheme(), testKeys())
	ta.SetSize(30, 6)
	ta.Focus()

	ta.Update(runes("ab"))
	ta.Update(keyEnter())
	ta.Update(runes("c"))
	if got, want := ta.Value(), "ab\nc"; got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}

	ta.Update(keyBackspace()) // deletes 'c'
	ta.Update(keyBackspace()) // joins the empty second line back onto "ab"
	if got, want := ta.Value(), "ab"; got != want {
		t.Errorf("after backspace value = %q, want %q", got, want)
	}
}

func TestTextAreaIgnoresInputWhenBlurred(t *testing.T) {
	ta := component.NewTextArea("commit", "Commit", "", testTheme(), testKeys())
	ta.SetSize(30, 6)

	if cmd := ta.Update(runes("x")); cmd != nil {
		t.Error("blurred editor should ignore key input")
	}
	if ta.Value() != "" {
		t.Errorf("blurred editor should not change, got %q", ta.Value())
	}
}

func newStringViews() (*component.Views, *component.List[string], *component.List[string]) {
	a := component.NewList[string]("changes", func(s string) string { return s }, testTheme(), testKeys())
	a.SetItems([]string{"a1", "a2"})
	b := component.NewList[string]("staged", func(s string) string { return s }, testTheme(), testKeys())
	b.SetItems([]string{"b1"})
	vs := component.NewViews("views", []component.View{
		{Name: "Changes", Content: a},
		{Name: "Staged", Content: b},
	}, testTheme(), testKeys())
	vs.SetSize(24, 6)
	vs.Focus()
	return vs, a, b
}

func TestViewsSwitchesWithBrackets(t *testing.T) {
	vs, a, b := newStringViews()
	if vs.ActiveIndex() != 0 || vs.ActiveName() != "Changes" {
		t.Fatalf("expected Changes active, got %d/%q", vs.ActiveIndex(), vs.ActiveName())
	}

	got := mustCmd(t, vs.Update(runes("]")))
	sel, ok := got.(msg.ViewSelectedMsg)
	if !ok {
		t.Fatalf("expected ViewSelectedMsg, got %T", got)
	}
	if sel.ID != "views" || sel.Index != 1 || sel.Name != "Staged" {
		t.Errorf("got %+v, want {views 1 Staged}", sel)
	}
	if vs.ActiveIndex() != 1 {
		t.Errorf("active view = %d, want 1", vs.ActiveIndex())
	}
	if a.Focused() || !b.Focused() {
		t.Error("switching should blur the old view and focus the new one")
	}

	// [ wraps back to the first view.
	mustCmd(t, vs.Update(runes("[")))
	if vs.ActiveIndex() != 0 {
		t.Errorf("after [, active view = %d, want 0", vs.ActiveIndex())
	}
}

func TestViewsForwardsToActive(t *testing.T) {
	vs, a, _ := newStringViews()
	// A navigation key reaches the active view, which emits SelectedMsg.
	got := mustCmd(t, vs.Update(keyDown()))
	sel, ok := got.(msg.SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg from the active view, got %T", got)
	}
	if sel.ID != "changes" || sel.Index != 1 {
		t.Errorf("got %+v, want {changes 1}", sel)
	}
	if a.Index() != 1 {
		t.Errorf("active view cursor = %d, want 1", a.Index())
	}
}

func TestViewsDrillPushPop(t *testing.T) {
	vs, _, _ := newStringViews()
	if vs.Depth() != 0 {
		t.Fatalf("expected base depth 0, got %d", vs.Depth())
	}

	sub := component.NewList[string]("detail", func(s string) string { return s }, testTheme(), testKeys())
	sub.SetItems([]string{"d1", "d2"})
	vs.Push(sub)
	if vs.Depth() != 1 {
		t.Fatalf("after push, depth = %d, want 1", vs.Depth())
	}
	if vs.ActiveName() != "Changes" {
		t.Errorf("drilling should keep the named view active, got %q", vs.ActiveName())
	}
	if !sub.Focused() {
		t.Error("the pushed sub-view should be focused")
	}

	// esc pops back out and emits SubViewPoppedMsg.
	got := mustCmd(t, vs.Update(keyEsc()))
	pop, ok := got.(msg.SubViewPoppedMsg)
	if !ok {
		t.Fatalf("expected SubViewPoppedMsg, got %T", got)
	}
	if pop.ID != "views" || pop.Depth != 0 {
		t.Errorf("got %+v, want {views 0}", pop)
	}
	if vs.Depth() != 0 {
		t.Errorf("after pop, depth = %d, want 0", vs.Depth())
	}
}

func TestViewsIgnoresInputWhenBlurred(t *testing.T) {
	vs, a, _ := newStringViews()
	vs.Blur()

	if cmd := vs.Update(runes("]")); cmd != nil {
		t.Error("a blurred container should ignore the view-switch key")
	}
	if vs.ActiveIndex() != 0 {
		t.Errorf("active view changed while blurred: %d", vs.ActiveIndex())
	}
	if cmd := vs.Update(keyDown()); cmd != nil {
		t.Error("a blurred container should not forward navigation")
	}
	if a.Index() != 0 {
		t.Errorf("active view cursor moved while blurred: %d", a.Index())
	}
}

func TestViewsRendersContentWithoutStrip(t *testing.T) {
	only := component.NewList[string]("log", func(s string) string { return s }, testTheme(), testKeys())
	only.SetItems([]string{"r42", "r41"})
	vs := component.NewViews("log-views", []component.View{{Name: "Log", Content: only}}, testTheme(), testKeys())
	vs.SetSize(20, 4)
	vs.Focus()

	// Views renders only the active component; the tab labels live in the host
	// Panel's border, so the container output is exactly the wrapped view.
	if got, want := vs.View(), only.View(); got != want {
		t.Errorf("Views should render only its content:\n got: %q\nwant: %q", got, want)
	}
	// [ / ] are inert with a single view.
	if cmd := vs.Update(runes("]")); cmd != nil {
		t.Error("a single-view container should not switch on ]")
	}

	// Drilling swaps the content for the sub-view but still adds no strip.
	sub := component.NewList[string]("detail", func(s string) string { return s }, testTheme(), testKeys())
	sub.SetItems([]string{"x"})
	vs.Push(sub)
	if got, want := vs.View(), sub.View(); got != want {
		t.Errorf("a drilled Views should render only the sub-view:\n got: %q\nwant: %q", got, want)
	}
	if vs.Depth() != 1 {
		t.Errorf("expected depth 1 after push, got %d", vs.Depth())
	}
}

func TestViewsLocksSwitchWhileDrilled(t *testing.T) {
	vs, _, _ := newStringViews()
	sub := component.NewList[string]("detail", func(s string) string { return s }, testTheme(), testKeys())
	sub.SetItems([]string{"d1"})
	vs.Push(sub)

	// While drilled in, [ / ] must not switch the active view.
	if cmd := vs.Update(runes("]")); cmd != nil {
		t.Error("] should be inert while drilled into a sub-view")
	}
	if cmd := vs.Update(runes("[")); cmd != nil {
		t.Error("[ should be inert while drilled into a sub-view")
	}
	if vs.ActiveIndex() != 0 {
		t.Errorf("active view changed while drilled: %d", vs.ActiveIndex())
	}
	// esc still pops out, and then switching works again.
	mustCmd(t, vs.Update(keyEsc()))
	if vs.Depth() != 0 {
		t.Fatalf("esc should pop the sub-view, depth = %d", vs.Depth())
	}
	if cmd := vs.Update(runes("]")); cmd == nil {
		t.Error("] should switch again once popped back to the base view")
	}
}

func TestViewsCrumbTitle(t *testing.T) {
	vs, _, _ := newStringViews()
	if vs.CrumbTitle() != "" {
		t.Errorf("base view should have no crumb title, got %q", vs.CrumbTitle())
	}
	sub := component.NewList[string]("detail", func(s string) string { return s }, testTheme(), testKeys())
	sub.SetItems([]string{"d1"})
	vs.PushTitled("feature-x", sub)
	if vs.CrumbTitle() != "feature-x" {
		t.Errorf("drilled crumb title = %q, want feature-x", vs.CrumbTitle())
	}
	mustCmd(t, vs.Update(keyEsc()))
	if vs.CrumbTitle() != "" {
		t.Errorf("crumb title should clear after popping, got %q", vs.CrumbTitle())
	}
}

func TestPromptEmitsSubmitAndDismiss(t *testing.T) {
	p := component.NewPrompt("changelist", "Changelist name", "e.g. feature-x", testTheme(), testKeys())
	p.Focus()
	p.Update(runes("feature-x"))

	submit := mustCmd(t, p.Update(keyEnter()))
	sub, ok := submit.(msg.SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", submit)
	}
	if sub.ID != "changelist" || sub.Value != "feature-x" {
		t.Errorf("got %+v, want {changelist feature-x}", sub)
	}

	dismiss := mustCmd(t, p.Update(keyEsc()))
	if d, ok := dismiss.(msg.DismissMsg); !ok || d.ID != "changelist" {
		t.Errorf("expected DismissMsg{changelist}, got %#v", dismiss)
	}
}

func TestPromptTabTogglesListAndScrolls(t *testing.T) {
	p := component.NewPrompt("changelist", "Changelist name", "", testTheme(), testKeys())
	p.SetOptions("Existing:", []string{"alpha", "beta"})
	p.Focus()

	// In the input field, up/down are inert — they only scroll the list.
	p.Update(keyDown())
	if p.Value() != "" {
		t.Errorf("up/down should be inert in the input field, value = %q", p.Value())
	}
	// Tab moves focus into the list and highlights the first option.
	p.Update(keyTab())
	if p.Value() != "alpha" {
		t.Errorf("tab should enter the list and pick the first option, value = %q", p.Value())
	}
	// Down/up scroll the list.
	p.Update(keyDown())
	if p.Value() != "beta" {
		t.Errorf("down should scroll to the next option, value = %q", p.Value())
	}
	p.Update(keyUp())
	if p.Value() != "alpha" {
		t.Errorf("up should scroll back, value = %q", p.Value())
	}
	// Tab returns to the input field, where typing edits the value again.
	p.Update(keyTab())
	p.Update(runes("!"))
	if p.Value() != "alpha!" {
		t.Errorf("tab should return to the input field for editing, value = %q", p.Value())
	}
	// Enter submits the current value.
	submit := mustCmd(t, p.Update(keyEnter()))
	if sub, ok := submit.(msg.SubmitMsg); !ok || sub.Value != "alpha!" {
		t.Errorf("expected SubmitMsg{alpha!}, got %#v", submit)
	}
}

func TestPromptTabInertWithoutOptions(t *testing.T) {
	p := component.NewPrompt("changelist", "Changelist name", "", testTheme(), testKeys())
	p.Focus()
	p.Update(runes("feat"))
	// With no options, tab does nothing and typing continues to edit.
	p.Update(keyTab())
	p.Update(runes("ure"))
	if p.Value() != "feature" {
		t.Errorf("tab should be inert without options, value = %q", p.Value())
	}
}

func TestPromptTypesNavLettersLiterally(t *testing.T) {
	p := component.NewPrompt("changelist", "Changelist name", "", testTheme(), testKeys())
	p.Focus()
	// j/k/y/n are literal text in the input, not navigation/confirm keys.
	p.Update(runes("jkyn"))
	if p.Value() != "jkyn" {
		t.Errorf("value = %q, want jkyn (nav letters should be literal)", p.Value())
	}
}

func TestPromptIgnoresInputWhenBlurred(t *testing.T) {
	p := component.NewPrompt("changelist", "Changelist name", "", testTheme(), testKeys())
	if cmd := p.Update(runes("x")); cmd != nil {
		t.Error("a blurred prompt should ignore key input")
	}
	if p.Value() != "" {
		t.Errorf("blurred prompt captured input: %q", p.Value())
	}
}
