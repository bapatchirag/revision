package component_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/msg"
)

func keySpace() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeySpace} }

// testForm builds a form with one of every field kind for the behavior tests.
func testForm() *component.Form {
	return component.NewForm("settings", "Settings", []component.Field{
		{Label: "Default path", Kind: component.FieldText, Value: ""},
		{Label: "Log limit", Kind: component.FieldInt, Value: "100"},
		{Label: "Editor", Kind: component.FieldText, Value: "vim"},
		{Label: "Theme", Kind: component.FieldChoice, Value: "auto", Options: []string{"auto", "nord", "dracula"}},
		{Label: "Directory diff", Kind: component.FieldBool, Value: "true"},
		{Label: "Hide rules", Kind: component.FieldAction, Value: "3 rules · 2 on"},
	}, testTheme(), testKeys())
}

// onActionField parks the cursor on the action row, the last field.
func onActionField(f *component.Form) {
	for i := 0; i < 5; i++ {
		f.Update(keyDown())
	}
}

func TestFormEmitsSubmitAndDismiss(t *testing.T) {
	f := testForm()
	f.Focus()

	submit := mustCmd(t, f.Update(keyCtrlS()))
	sub, ok := submit.(msg.SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", submit)
	}
	if sub.ID != "settings" {
		t.Errorf("SubmitMsg.ID = %q, want settings", sub.ID)
	}
	// The form carries no single value; callers read the fields with Values().
	if sub.Value != "" {
		t.Errorf("SubmitMsg.Value = %q, want empty", sub.Value)
	}

	dismiss := mustCmd(t, f.Update(keyEsc()))
	if d, ok := dismiss.(msg.DismissMsg); !ok || d.ID != "settings" {
		t.Errorf("expected DismissMsg{settings}, got %#v", dismiss)
	}
}

func TestFormEditsTextField(t *testing.T) {
	f := testForm()
	f.Focus()

	f.Update(runes("~/code"))
	if got := f.Value(0); got != "~/code" {
		t.Fatalf("field 0 = %q, want ~/code", got)
	}
	f.Update(keyBackspace())
	if got := f.Value(0); got != "~/cod" {
		t.Errorf("after backspace field 0 = %q, want ~/cod", got)
	}
}

func TestFormIntFieldRejectsNonDigits(t *testing.T) {
	f := testForm()
	f.Focus()

	f.Update(keyDown()) // move to the Log limit field, cursor parked at the end
	f.Update(runes("5"))
	if got := f.Value(1); got != "1005" {
		t.Fatalf("digit not appended: field 1 = %q, want 1005", got)
	}
	f.Update(runes("x")) // a non-digit is dropped
	if got := f.Value(1); got != "1005" {
		t.Errorf("non-digit changed the int field: %q, want 1005", got)
	}
}

func TestFormTogglesBoolField(t *testing.T) {
	f := testForm()
	f.Focus()
	for i := 0; i < 4; i++ {
		f.Update(keyDown()) // move to the Directory diff field
	}
	f.Update(keySpace())
	if got := f.Value(4); got != "false" {
		t.Fatalf("space did not toggle bool off: %q, want false", got)
	}
	f.Update(keyLeft())
	if got := f.Value(4); got != "true" {
		t.Errorf("left did not toggle bool on: %q, want true", got)
	}
}

func TestFormCyclesChoiceField(t *testing.T) {
	f := testForm()
	f.Focus()
	for i := 0; i < 3; i++ {
		f.Update(keyDown()) // move to the Theme field (auto, nord, dracula)
	}
	f.Update(keyRight())
	if got := f.Value(3); got != "nord" {
		t.Fatalf("right did not advance choice: %q, want nord", got)
	}
	f.Update(keyRight()) // dracula
	f.Update(keyRight()) // wraps back to auto
	if got := f.Value(3); got != "auto" {
		t.Errorf("choice did not wrap forward: %q, want auto", got)
	}
	f.Update(keyLeft()) // wraps back to dracula
	if got := f.Value(3); got != "dracula" {
		t.Errorf("choice did not wrap backward: %q, want dracula", got)
	}
}

func TestFormTypesNavLettersLiterally(t *testing.T) {
	f := testForm()
	f.Focus()

	// j/k/h/l are navigation elsewhere; in a text field they are literal text.
	f.Update(runes("jkhl"))
	if got := f.Value(0); got != "jkhl" {
		t.Fatalf("nav letters not typed literally: field 0 = %q", got)
	}
	// ↓ (by key type) is what moves between fields.
	f.Update(keyDown())
	f.Update(runes("9"))
	if got := f.Value(1); got != "1009" {
		t.Errorf("field 1 = %q, want 1009 (↓ should have moved fields)", got)
	}
	if got := f.Value(0); got != "jkhl" {
		t.Errorf("field 0 changed unexpectedly: %q", got)
	}
}

func TestFormActionFieldEmitsActivated(t *testing.T) {
	f := testForm()
	f.Focus()
	onActionField(f)

	for _, k := range []tea.KeyMsg{keyEnter(), keySpace()} {
		activated := mustCmd(t, f.Update(k))
		act, ok := activated.(msg.ActivatedMsg)
		if !ok {
			t.Fatalf("expected ActivatedMsg, got %T", activated)
		}
		if act.ID != "settings" || act.Index != 5 {
			t.Errorf("got %+v, want {settings 5}", act)
		}
	}
}

func TestFormActionFieldIsNotEdited(t *testing.T) {
	f := testForm()
	f.Focus()
	onActionField(f)

	f.Update(runes("typed"))
	f.Update(keyBackspace())
	if got, want := f.Value(5), "3 rules · 2 on"; got != want {
		t.Errorf("action field = %q, want %q (it is a read-only summary)", got, want)
	}
}

func TestFormSetValueReplacesASummary(t *testing.T) {
	f := testForm()

	f.SetValue(5, "4 rules · 4 on")
	if got, want := f.Value(5), "4 rules · 4 on"; got != want {
		t.Errorf("action field = %q, want %q", got, want)
	}
	// An index no field occupies is ignored rather than panicking.
	f.SetValue(-1, "x")
	f.SetValue(99, "x")
	if got := len(f.Values()); got != 6 {
		t.Errorf("Values() len = %d, want 6", got)
	}

	// Replacing the active field's value leaves the edit column at its end, so
	// typing continues after the new text rather than inside it.
	f.Focus()
	f.SetValue(0, "~/code")
	f.Update(runes("!"))
	if got, want := f.Value(0), "~/code!"; got != want {
		t.Errorf("field 0 = %q, want %q", got, want)
	}
}

func TestFormValuesReflectEdits(t *testing.T) {
	f := testForm()
	f.Focus()

	f.Update(runes("/tmp"))
	vals := f.Values()
	if len(vals) != 6 {
		t.Fatalf("Values() len = %d, want 6", len(vals))
	}
	if vals[0] != "/tmp" {
		t.Errorf("Values()[0] = %q, want /tmp", vals[0])
	}
	if vals[2] != "vim" {
		t.Errorf("Values()[2] = %q, want vim (untouched)", vals[2])
	}
}

func TestFormIgnoresInputWhenBlurred(t *testing.T) {
	f := testForm() // not focused

	if cmd := f.Update(runes("x")); cmd != nil {
		t.Error("blurred form should ignore key input")
	}
	if f.Value(0) != "" {
		t.Errorf("blurred form should not change, got %q", f.Value(0))
	}
}

func TestFormHintMatchesActiveField(t *testing.T) {
	f := testForm()
	f.SetSize(60, 0)
	f.Focus()

	if v := f.View(); !strings.Contains(v, "ctrl+s save") {
		t.Errorf("text-field hint missing:\n%s", v)
	}
	for i := 0; i < 3; i++ {
		f.Update(keyDown()) // Theme (choice)
	}
	if v := f.View(); !strings.Contains(v, "change") {
		t.Errorf("choice-field hint missing:\n%s", v)
	}
	f.Update(keyDown()) // Directory diff (bool)
	if v := f.View(); !strings.Contains(v, "toggle") {
		t.Errorf("bool-field hint missing:\n%s", v)
	}
	f.Update(keyDown()) // Hide rules (action)
	if v := f.View(); !strings.Contains(v, "enter open") {
		t.Errorf("action-field hint missing:\n%s", v)
	}
}

func TestFormViewShowsFieldsAndValues(t *testing.T) {
	f := testForm()
	f.SetSize(60, 0)
	f.Focus()

	view := f.View()
	for _, want := range []string{"Settings", "Default path", "Log limit", "Editor", "Theme", "Directory diff", "vim", "100", "Hide rules", "3 rules · 2 on"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n---\n%s", want, view)
		}
	}
}
