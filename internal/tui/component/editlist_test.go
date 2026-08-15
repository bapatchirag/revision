package component_test

import (
	"strings"
	"testing"

	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/msg"
)

// newRuleList is a list of hide rules: two in force, one turned off.
func newRuleList(t *testing.T) *component.EditList {
	t.Helper()
	el := component.NewEditList("rules", "Hide rules", "No rules yet — press a to add one.", testTheme(), testKeys())
	el.SetEntries([]component.EditEntry{
		{Text: "^build/", Enabled: true},
		{Text: `\.class$`, Enabled: true},
		{Text: "^vendor/", Enabled: false},
	})
	el.Focus()
	return el
}

// texts is the entry text in row order, for comparing against a want slice.
func texts(el *component.EditList) []string {
	out := make([]string, 0, len(el.Entries()))
	for _, e := range el.Entries() {
		out = append(out, e.Text)
	}
	return out
}

func TestEditListEmitsSubmitAndDismiss(t *testing.T) {
	el := newRuleList(t)

	submit := mustCmd(t, el.Update(keyCtrlS()))
	sub, ok := submit.(msg.SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", submit)
	}
	if sub.ID != "rules" {
		t.Errorf("SubmitMsg.ID = %q, want rules", sub.ID)
	}

	dismiss := mustCmd(t, el.Update(keyEsc()))
	if d, ok := dismiss.(msg.DismissMsg); !ok || d.ID != "rules" {
		t.Errorf("expected DismissMsg{rules}, got %#v", dismiss)
	}
}

func TestEditListMovesAndToggles(t *testing.T) {
	el := newRuleList(t)

	el.Update(keyDown())
	if el.Index() != 1 {
		t.Fatalf("index after down = %d, want 1", el.Index())
	}
	el.Update(keySpace())
	if el.Entries()[1].Enabled {
		t.Error("space should turn the row under the cursor off")
	}
	el.Update(keySpace())
	if !el.Entries()[1].Enabled {
		t.Error("space should turn the row back on")
	}

	// The cursor is clamped at both ends rather than wrapping.
	el.Update(keyUp())
	el.Update(keyUp())
	if el.Index() != 0 {
		t.Errorf("index at the top = %d, want 0", el.Index())
	}
	for i := 0; i < 5; i++ {
		el.Update(keyDown())
	}
	if el.Index() != 2 {
		t.Errorf("index at the bottom = %d, want 2", el.Index())
	}
}

func TestEditListAddsAnEntry(t *testing.T) {
	el := newRuleList(t)

	el.Update(runes("a"))
	if !el.Editing() {
		t.Fatal("a should open the new row for text entry")
	}
	el.Update(runes("^out/"))
	el.Update(keyEnter())

	if el.Editing() {
		t.Error("enter should close the row")
	}
	want := []string{"^build/", `\.class$`, "^vendor/", "^out/"}
	if got := texts(el); !equalStrings(got, want) {
		t.Errorf("entries = %v, want %v", got, want)
	}
	if !el.Entries()[3].Enabled {
		t.Error("a new rule should be in force")
	}
}

func TestEditListEditsAnEntry(t *testing.T) {
	el := newRuleList(t)

	el.Update(runes("e"))
	if !el.Editing() {
		t.Fatal("e should open the row under the cursor for text entry")
	}
	// The draft starts at the end of the existing text.
	el.Update(runes("dist/"))
	el.Update(keyEnter())

	if got, want := el.Entries()[0].Text, "^build/dist/"; got != want {
		t.Errorf("edited text = %q, want %q", got, want)
	}
}

func TestEditListDeletesAnEntry(t *testing.T) {
	el := newRuleList(t)

	el.Update(keyDown())
	el.Update(runes("d"))

	want := []string{"^build/", "^vendor/"}
	if got := texts(el); !equalStrings(got, want) {
		t.Errorf("entries = %v, want %v", got, want)
	}
	if el.Index() != 1 {
		t.Errorf("index = %d, want 1 (the row that took its place)", el.Index())
	}
}

func TestEditListDropsABlankNewEntry(t *testing.T) {
	el := newRuleList(t)

	// Added then left blank: the row goes away rather than lingering empty.
	el.Update(runes("a"))
	el.Update(keyEnter())
	if got := len(el.Entries()); got != 3 {
		t.Errorf("entry count = %d, want 3 (a blank new row is dropped)", got)
	}

	// Added then abandoned with esc: same outcome, and the list stays open.
	el.Update(runes("a"))
	el.Update(runes("^tmp/"))
	if cmd := el.Update(keyEsc()); cmd != nil {
		t.Error("esc while editing should close the row, not dismiss the list")
	}
	if got := len(el.Entries()); got != 3 {
		t.Errorf("entry count = %d, want 3 (an abandoned new row is dropped)", got)
	}
}

func TestEditListKeepsAnExistingEntryOnCancel(t *testing.T) {
	el := newRuleList(t)

	el.Update(runes("e"))
	el.Update(keyBackspace())
	el.Update(keyEsc())

	if got, want := el.Entries()[0].Text, "^build/"; got != want {
		t.Errorf("text after cancel = %q, want %q (unchanged)", got, want)
	}
	if el.Editing() {
		t.Error("esc should close the row")
	}
}

func TestEditListTypesActionLettersLiterally(t *testing.T) {
	el := newRuleList(t)

	el.Update(runes("a"))
	// a/d/e/j/k drive the list while browsing but are literal text while editing.
	el.Update(runes("adejk"))
	el.Update(keyEnter())

	if got, want := el.Entries()[3].Text, "adejk"; got != want {
		t.Errorf("typed text = %q, want %q", got, want)
	}
}

func TestEditListEditsWithinARow(t *testing.T) {
	el := newRuleList(t)

	el.Update(runes("a"))
	el.Update(runes("abc"))
	el.Update(keyLeft())
	el.Update(runes("X"))
	el.Update(keyHome())
	el.Update(runes("^"))
	el.Update(keyEnd())
	el.Update(runes("$"))
	el.Update(keyEnter())

	if got, want := el.Entries()[3].Text, "^abXc$"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestEditListDeletesAWordWhileEditing(t *testing.T) {
	el := newRuleList(t)

	el.Update(runes("a"))
	el.Update(runes("build dist"))
	el.Update(keyAltBackspace())
	el.Update(keyEnter())

	if got, want := el.Entries()[3].Text, "build"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestEditListMovesByWordWhileEditing(t *testing.T) {
	el := newRuleList(t)

	el.Update(runes("a"))
	el.Update(runes("build dist"))
	el.Update(keyAltLeft()) // to the start of "dist"
	el.Update(runes("X"))
	el.Update(keyAltRight()) // back to the end of it
	el.Update(runes("!"))
	el.Update(keyEnter())

	if got, want := el.Entries()[3].Text, "build Xdist!"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestEditListCutsTheWordAheadWhileEditing(t *testing.T) {
	el := newRuleList(t)

	el.Update(runes("a"))
	el.Update(runes("build dist"))
	el.Update(keyHome())
	el.Update(keyAltDelete())
	el.Update(keyEnter())

	if got, want := el.Entries()[3].Text, "dist"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestEditListRendersWithoutASize(t *testing.T) {
	// The overlay renders once before the layout has sized it, so the box has to
	// fall back to its content width.
	view := newRuleList(t).View()
	for _, want := range []string{"Hide rules", "^build/", "^vendor/", "esc cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("unsized view missing %q:\n%s", want, view)
		}
	}
}

func TestEditListIsInertWhenEmpty(t *testing.T) {
	el := component.NewEditList("rules", "Hide rules", "No rules yet.", testTheme(), testKeys())
	el.Focus()

	for _, k := range []string{"e", "d"} {
		el.Update(runes(k))
	}
	el.Update(keySpace())
	if el.Editing() {
		t.Error("there is nothing to edit in an empty list")
	}
	if got := len(el.Entries()); got != 0 {
		t.Errorf("entry count = %d, want 0", got)
	}
	if view := el.View(); !strings.Contains(view, "No rules yet.") {
		t.Errorf("an empty list should show its placeholder:\n%s", view)
	}
}

func TestEditListIgnoresInputWhenBlurred(t *testing.T) {
	el := newRuleList(t)
	el.Blur()

	if cmd := el.Update(runes("d")); cmd != nil {
		t.Error("a blurred list should ignore key input")
	}
	if got := len(el.Entries()); got != 3 {
		t.Errorf("blurred list acted on input: entry count = %d, want 3", got)
	}
}

func TestEditListSetEntriesCopiesAndResets(t *testing.T) {
	el := newRuleList(t)
	el.Update(keyDown())

	entries := []component.EditEntry{{Text: "^out/", Enabled: true}}
	el.SetEntries(entries)
	if el.Index() != 0 {
		t.Errorf("index = %d, want 0 (SetEntries parks the cursor)", el.Index())
	}

	// The caller's slice must not follow later edits, nor the list the caller's.
	entries[0].Text = "mutated"
	if got := el.Entries()[0].Text; got != "^out/" {
		t.Errorf("entry text = %q, want ^out/ (the entries should be copied)", got)
	}
	el.Update(runes("d"))
	if entries[0].Text != "mutated" || len(entries) != 1 {
		t.Error("editing the list should not reach the caller's slice")
	}
}

func TestEditListWindowsLongLists(t *testing.T) {
	el := component.NewEditList("rules", "Hide rules", "none", testTheme(), testKeys())
	entries := make([]component.EditEntry, 0, 8)
	for _, p := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight"} {
		entries = append(entries, component.EditEntry{Text: p, Enabled: true})
	}
	el.SetEntries(entries)
	// Four rows fit: 8 tall, less the border, the spacer and the hint.
	el.SetSize(30, 8)
	el.Focus()

	if view := el.View(); strings.Contains(view, "five") {
		t.Errorf("row past the window should not render:\n%s", view)
	}
	for i := 0; i < 7; i++ {
		el.Update(keyDown())
	}
	view := el.View()
	if !strings.Contains(view, "eight") {
		t.Errorf("the window should follow the cursor:\n%s", view)
	}
	if strings.Contains(view, "one") {
		t.Errorf("the window should have scrolled past the first row:\n%s", view)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
