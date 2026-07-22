package component_test

import (
	"testing"

	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/msg"
)

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
