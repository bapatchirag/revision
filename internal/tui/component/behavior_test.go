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
