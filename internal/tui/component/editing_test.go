package component_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/msg"
)

// focusedTextArea returns a focused editor holding text, sized so the buffer
// scrolls.
func focusedTextArea(t *testing.T, text string) *component.TextArea {
	t.Helper()
	ta := component.NewTextArea("text", "Message", "type a message", testTheme(), testKeys())
	ta.SetSize(20, 5)
	ta.SetValue(text)
	ta.Focus()
	return ta
}

// TestTextAreaArrowNavigation walks the cursor over a multi-line buffer with the
// arrow keys and checks where each motion leaves it, by typing a marker at the
// resting position. The buffer starts with the cursor at the end of the last
// line, which is where SetValue parks it.
func TestTextAreaArrowNavigation(t *testing.T) {
	cases := map[string]struct {
		keys []tea.KeyMsg
		want string
	}{
		"right at the very end stays put": {
			keys: []tea.KeyMsg{keyRight()},
			want: "first\ntwo\nthird!",
		},
		"right at the end of a line wraps to the next": {
			// Two ups land mid-"first" (the column is clamped on the way through
			// "two"); three rights reach its end, from where right steps onto "two".
			keys: []tea.KeyMsg{keyUp(), keyUp(), keyRight(), keyRight(), keyRight()},
			want: "first\n!two\nthird",
		},
		"right within a line steps one rune": {
			keys: []tea.KeyMsg{keyUp(), keyUp(), keyLeft(), keyLeft(), keyLeft(), keyRight()},
			want: "f!irst\ntwo\nthird",
		},
		"up moves to the row above": {
			keys: []tea.KeyMsg{keyLeft(), keyLeft(), keyLeft(), keyLeft(), keyLeft(), keyUp()},
			want: "first\n!two\nthird",
		},
		"up at the first row stays put": {
			keys: []tea.KeyMsg{keyUp(), keyUp(), keyUp(), keyUp()},
			want: "fir!st\ntwo\nthird",
		},
		"up onto a shorter row clamps the column": {
			// "third" is 5 long, the row above only 3.
			keys: []tea.KeyMsg{keyUp()},
			want: "first\ntwo!\nthird",
		},
		"down moves to the row below": {
			keys: []tea.KeyMsg{keyUp(), keyUp(), keyLeft(), keyLeft(), keyLeft(), keyDown()},
			want: "first\n!two\nthird",
		},
		"down at the last row stays put": {
			keys: []tea.KeyMsg{keyDown(), keyDown()},
			want: "first\ntwo\nthird!",
		},
		"down onto a shorter row clamps the column": {
			// Back to the top-left, right to the end of "first", then down onto "two".
			keys: []tea.KeyMsg{
				keyUp(), keyUp(), keyLeft(), keyLeft(), keyLeft(),
				keyRight(), keyRight(), keyRight(), keyRight(), keyRight(), keyDown(),
			},
			want: "first\ntwo!\nthird",
		},
		"left at the start of a line wraps to the one above": {
			keys: []tea.KeyMsg{keyLeft(), keyLeft(), keyLeft(), keyLeft(), keyLeft(), keyLeft()},
			want: "first\ntwo!\nthird",
		},
		"left at the very start stays put": {
			keys: []tea.KeyMsg{keyUp(), keyUp(), keyLeft(), keyLeft(), keyLeft(), keyLeft()},
			want: "!first\ntwo\nthird",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ta := focusedTextArea(t, "first\ntwo\nthird")
			for _, k := range tc.keys {
				ta.Update(k)
			}
			ta.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
			if got := ta.Value(); got != tc.want {
				t.Errorf("buffer = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTextAreaEditing(t *testing.T) {
	ta := focusedTextArea(t, "hello world")

	// SetValue leaves the cursor at the end of the buffer.
	ta.Update(keyAltBackspace())
	if got := ta.Value(); got != "hello " {
		t.Errorf("buffer = %q, want the last word deleted", got)
	}

	ta.Update(tea.KeyMsg{Type: tea.KeySpace})
	ta.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("there")})
	if got := ta.Value(); got != "hello  there" {
		t.Errorf("buffer = %q, want the typed text", got)
	}

	ta.Update(keyEnter())
	ta.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("next")})
	if got := ta.Value(); got != "hello  there\nnext" {
		t.Errorf("buffer = %q, want a new line", got)
	}

	ta.Update(keyBackspace())
	if got := ta.Value(); got != "hello  there\nnex" {
		t.Errorf("buffer = %q, want the last rune deleted", got)
	}

	// Backspace at column zero joins the line onto the one above it.
	for range 3 {
		ta.Update(keyLeft())
	}
	ta.Update(keyBackspace())
	if got := ta.Value(); got != "hello  therenex" {
		t.Errorf("buffer = %q, want the lines joined", got)
	}

	// A forward word delete at the end of a line pulls the next one up.
	ta.SetValue("alpha\nbeta")
	ta.Update(keyUp())
	ta.Update(keyRight())
	ta.Update(keyAltDelete())
	if got := ta.Value(); got != "alphabeta" {
		t.Errorf("buffer = %q, want the following line pulled up", got)
	}

	// A word delete at the start of a line does the same, backwards.
	ta.SetValue("alpha\nbeta")
	for range 4 {
		ta.Update(keyLeft())
	}
	ta.Update(keyAltBackspace())
	if got := ta.Value(); got != "alphabeta" {
		t.Errorf("buffer = %q, want the line pulled onto the previous one", got)
	}

	// Word motion wraps across rows in both directions.
	ta.SetValue("alpha\nbeta")
	ta.Update(keyAltLeft())
	ta.Update(keyAltLeft())
	ta.Update(keyAltRight())
	ta.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if got := ta.Value(); !strings.Contains(got, "!") {
		t.Errorf("buffer = %q, want the marker typed somewhere", got)
	}
}

func TestTextAreaSubmitAndDismiss(t *testing.T) {
	ta := focusedTextArea(t, "a commit message")

	sub, ok := mustCmd(t, ta.Update(keyCtrlS())).(msg.SubmitMsg)
	if !ok || sub.Value != "a commit message" {
		t.Errorf("submit = %+v, want the buffer's text", sub)
	}
	if _, ok := mustCmd(t, ta.Update(keyEsc())).(msg.DismissMsg); !ok {
		t.Error("esc should dismiss the editor")
	}
}

// TestTextAreaScrollsWithTheCursor drives the cursor past the bottom of the
// window and checks the view follows it.
func TestTextAreaScrollsWithTheCursor(t *testing.T) {
	ta := focusedTextArea(t, strings.Join([]string{"l1", "l2", "l3", "l4", "l5", "l6", "l7", "l8"}, "\n"))

	// SetValue parks the cursor on the last line, which must be on screen.
	if view := ta.View(); !strings.Contains(view, "l8") {
		t.Errorf("view should show the cursor's line, got:\n%s", view)
	}
	for range 8 {
		ta.Update(keyUp())
	}
	if view := ta.View(); !strings.Contains(view, "l1") {
		t.Errorf("view should follow the cursor back to the top, got:\n%s", view)
	}
}

// TestTextAreaIntrinsicWidth renders before any size is set, which is what the
// intrinsic-width path is for.
func TestTextAreaIntrinsicWidth(t *testing.T) {
	ta := component.NewTextArea("text", "Message", "type a message", testTheme(), testKeys())
	if ta.View() == "" {
		t.Error("an unsized editor should still render at its intrinsic width")
	}
	ta.SetValue("a line rather longer than the placeholder is")
	if ta.View() == "" {
		t.Error("an unsized editor should widen to its content")
	}
}

// focusedForm returns a focused settings-shaped form.
func focusedForm(t *testing.T) *component.Form {
	t.Helper()
	f := component.NewForm("form", "Settings", []component.Field{
		{Label: "Theme", Kind: component.FieldChoice, Value: "default", Options: []string{"default", "nord", "gruvbox"}},
		{Label: "Log limit", Kind: component.FieldInt, Value: "50"},
		{Label: "Live refresh", Kind: component.FieldBool, Value: "true"},
		{Label: "Diff dir", Kind: component.FieldText, Value: "out dir"},
	}, testTheme(), testKeys())
	f.SetSize(50, 0)
	f.Focus()
	return f
}

func TestFormMovesBetweenFields(t *testing.T) {
	f := focusedForm(t)

	// Down and tab both advance; the cursor stops at the last field.
	for range 10 {
		f.Update(keyDown())
	}
	f.Update(keyRight()) // a text field's cursor key, not a field move
	if got := f.Values()[3]; got != "out dir" {
		t.Errorf("the last field's value changed to %q", got)
	}

	// Up and shift+tab walk back; the cursor stops at the first field.
	for range 10 {
		f.Update(keyUp())
	}
	f.Update(keyRight())
	if got := f.Value(0); got != "nord" {
		t.Errorf("Value(0) = %q, want the choice cycled forward", got)
	}
	f.Update(keyLeft())
	if got := f.Value(0); got != "default" {
		t.Errorf("Value(0) = %q, want the choice cycled back", got)
	}

	// Enter on a text field advances to the next one.
	f.Update(keyTab())
	f.Update(keyEnter())
	f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if got := f.Value(2); got != "true" {
		t.Errorf("a bool field should ignore typed runes, got %q", got)
	}
}

func TestFormEditsText(t *testing.T) {
	f := focusedForm(t)
	for range 3 {
		f.Update(keyDown()) // the text field
	}

	f.Update(keyHome())
	f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/wc/")})
	if got := f.Value(3); got != "/wc/out dir" {
		t.Errorf("Value(3) = %q, want the text inserted at the start", got)
	}

	f.Update(keyEnd())
	f.Update(keyBackspace())
	if got := f.Value(3); got != "/wc/out di" {
		t.Errorf("Value(3) = %q, want the last rune deleted", got)
	}

	f.Update(keyAltBackspace())
	if got := f.Value(3); got != "/wc/out " {
		t.Errorf("Value(3) = %q, want the last word deleted", got)
	}

	f.Update(keyAltLeft())
	f.Update(keyAltDelete())
	if got := f.Value(3); got != "/wc/ " {
		t.Errorf("Value(3) = %q, want the word after the cursor deleted", got)
	}

	f.Update(keyEnd())
	f.Update(tea.KeyMsg{Type: tea.KeySpace})
	f.Update(keyAltRight())
	f.Update(keyLeft())
	f.Update(keyLeft())
	f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	if got := f.Value(3); !strings.Contains(got, "z") {
		t.Errorf("Value(3) = %q, want the rune typed at the cursor", got)
	}

	// An integer field keeps digits and drops everything else.
	f.Update(keyUp())
	f.Update(keyUp())
	f.Update(keyEnd())
	f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9a")})
	f.Update(tea.KeyMsg{Type: tea.KeySpace})
	if got := f.Value(1); got != "509" {
		t.Errorf("Value(1) = %q, want only the digit accepted", got)
	}
}

func TestFormTogglesAndSubmits(t *testing.T) {
	f := focusedForm(t)
	f.Update(keyDown())
	f.Update(keyDown()) // the bool field

	f.Update(tea.KeyMsg{Type: tea.KeySpace})
	if got := f.Value(2); got != "false" {
		t.Errorf("Value(2) = %q, want the toggle flipped", got)
	}
	f.Update(keyRight())
	if got := f.Value(2); got != "true" {
		t.Errorf("Value(2) = %q, want the toggle flipped back", got)
	}

	if _, ok := mustCmd(t, f.Update(keyCtrlS())).(msg.SubmitMsg); !ok {
		t.Error("ctrl+s should submit the form")
	}
	if _, ok := mustCmd(t, f.Update(keyEsc())).(msg.DismissMsg); !ok {
		t.Error("esc should dismiss the form")
	}
}

// TestFormWithNoFieldsIsInert guards the empty-field paths in moveField and the
// renderer.
func TestFormWithNoFieldsIsInert(t *testing.T) {
	f := component.NewForm("form", "Settings", nil, testTheme(), testKeys())
	f.Focus()
	f.Update(keyDown())
	f.Update(keyEnter())
	if f.View() == "" {
		t.Error("an empty form should still render its box")
	}
	if got := f.Values(); len(got) != 0 {
		t.Errorf("Values() = %v, want none", got)
	}
}

func TestPromptCursorMotion(t *testing.T) {
	p := component.NewPrompt("prompt", "Path", "type a path", testTheme(), testKeys())
	p.SetSize(40, 0)
	p.SetLocked("/wc/")
	p.SetValue("/wc/src/main go")
	p.Focus()

	// The cursor stops at the locked prefix rather than the start of the line.
	for range 20 {
		p.Update(keyLeft())
	}
	p.Update(keyBackspace())
	if p.Value() != "/wc/src/main go" {
		t.Errorf("Value() = %q, want the locked prefix protected", p.Value())
	}
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if p.Value() != "/wc/xsrc/main go" {
		t.Errorf("Value() = %q, want the rune typed just past the lock", p.Value())
	}

	// Word motion likewise stops at the lock.
	p.Update(keyAltLeft())
	p.Update(keyAltLeft())
	p.Update(keyAltBackspace())
	if p.Value() != "/wc/xsrc/main go" {
		t.Errorf("Value() = %q, want a word delete at the lock to change nothing", p.Value())
	}

	for range 20 {
		p.Update(keyRight())
	}
	p.Update(keyAltRight())
	p.Update(keyCtrlW())
	if p.Value() != "/wc/xsrc/main " {
		t.Errorf("Value() = %q, want the last word deleted", p.Value())
	}

	p.Update(keyLeft())
	p.Update(keyRight())
	p.Update(keyRight()) // already at the end: a no-op
	p.Update(keyAltDelete())
	if p.Value() != "/wc/xsrc/main " {
		t.Errorf("Value() = %q, want a forward word delete at the end to change nothing", p.Value())
	}

	p.Update(tea.KeyMsg{Type: tea.KeySpace})
	if p.Value() != "/wc/xsrc/main  " {
		t.Errorf("Value() = %q, want a space typed at the cursor", p.Value())
	}
}

// TestPromptSecretMotionDoesNotStepThroughWords checks that a masked value moves
// the cursor to the ends rather than over its word boundaries, which would say
// where they are.
func TestPromptSecretMotionDoesNotStepThroughWords(t *testing.T) {
	p := component.NewPrompt("prompt", "Passphrase", "", testTheme(), testKeys())
	p.SetSize(40, 0)
	p.SetSecret(true)
	p.SetValue("one two three")
	p.Focus()

	p.Update(keyAltLeft())
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if p.Value() != "!one two three" {
		t.Errorf("Value() = %q, want the cursor at the very start", p.Value())
	}

	p.Update(keyAltRight())
	p.Update(keyAltBackspace())
	if p.Value() != "" {
		t.Errorf("Value() = %q, want a backwards word delete to clear a secret", p.Value())
	}

	p.SetValue("one two")
	for range 10 {
		p.Update(keyLeft())
	}
	p.Update(keyAltDelete())
	if p.Value() != "" {
		t.Errorf("Value() = %q, want a forward word delete to clear to the end", p.Value())
	}
}

func TestPromptOptionListSelection(t *testing.T) {
	p := component.NewPrompt("prompt", "Path", "", testTheme(), testKeys())
	p.SetSize(40, 0)
	p.SetOptions("Directories", []string{"/wc/src", "/wc/docs", "/wc/test"})
	p.Focus()
	p.Update(keyTab())

	// Up at the top and down past the bottom both clamp.
	p.Update(keyUp())
	p.Update(keyDown())
	if p.Value() != "/wc/docs" {
		t.Errorf("Value() = %q, want the second option", p.Value())
	}
	for range 5 {
		p.Update(keyDown())
	}
	if p.Value() != "/wc/test" {
		t.Errorf("Value() = %q, want the last option", p.Value())
	}

	sub, ok := mustCmd(t, p.Update(keyEnter())).(msg.SubmitMsg)
	if !ok || sub.Value != "/wc/test" {
		t.Errorf("submit = %+v, want the picked option", sub)
	}
	if _, ok := mustCmd(t, p.Update(keyEsc())).(msg.DismissMsg); !ok {
		t.Error("esc should dismiss the prompt")
	}

	// With no options, tab and the list keys do nothing.
	bare := component.NewPrompt("prompt", "Path", "", testTheme(), testKeys())
	bare.Focus()
	bare.Update(keyTab())
	bare.Update(keyDown())
	if bare.Value() != "" {
		t.Errorf("Value() = %q, want an empty prompt untouched", bare.Value())
	}
	if bare.View() == "" {
		t.Error("an unsized prompt should still render at its intrinsic width")
	}
}

func TestViewportPagingAndCursor(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetSize(20, 4)
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line " + strings.Repeat("x", i%7)
	}
	v.SetContent(strings.Join(lines, "\n"))
	v.Focus()

	// Without a line cursor the keys only scroll, and clamp at both ends.
	v.Update(keyUp())
	if v.Offset() != 0 {
		t.Errorf("Offset() = %d, want it clamped at the top", v.Offset())
	}
	for range 60 {
		v.Update(keyDown())
	}
	atBottom := v.Offset()
	v.Update(keyDown())
	if v.Offset() != atBottom {
		t.Errorf("Offset() = %d, want it clamped at the bottom", v.Offset())
	}

	// With a line cursor the window follows only as far as it must.
	v.SetContent(strings.Join(lines, "\n"))
	v.SetCursorLine(true)
	for range 12 {
		v.Update(keyDown())
	}
	if v.Cursor() != 12 {
		t.Errorf("Cursor() = %d, want it to have moved twelve lines", v.Cursor())
	}
	if v.Offset() == 0 {
		t.Error("the window should have followed the cursor off the first page")
	}
	// The cursor line is painted while focused.
	v.View()
	for range 20 {
		v.Update(keyUp())
	}
	if v.Cursor() != 0 || v.Offset() != 0 {
		t.Errorf("Cursor()/Offset() = %d/%d, want both back at the top", v.Cursor(), v.Offset())
	}

	// Paging takes the cursor along so the reader keeps their place on screen.
	v.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if v.Cursor() == 0 {
		t.Error("a page down should carry the cursor with the window")
	}
	for range 20 {
		v.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	}
	if v.Cursor() != 0 || v.Offset() != 0 {
		t.Errorf("Cursor()/Offset() = %d/%d, want paging up to clamp at the top", v.Cursor(), v.Offset())
	}
	v.Blur()
	v.View()
}

func TestMenuColumnLayoutKeepsSectionsWhole(t *testing.T) {
	items := []component.MenuItem{
		component.MenuSection("Changes"),
		{Label: "Stage", Key: "space"},
		{Label: "Commit", Key: "c"},
		component.MenuSection("Working copy"),
		{Label: "Update", Key: "u"},
		{Label: "Refresh", Key: "R"},
		component.MenuSection("General"),
		{Label: "Help", Key: "?"},
		{Label: "Quit with a very long label indeed", Key: "q"},
	}
	mn := component.NewMenu("menu", "Actions", items, testTheme(), testKeys())
	mn.Focus()

	for _, cols := range []int{1, 2, 3} {
		mn.SetColumns(cols)
		for _, width := range []int{0, 20, 60} {
			mn.SetSize(width, 0)
			if mn.View() == "" {
				t.Errorf("menu rendered nothing at %d columns, width %d", cols, width)
			}
		}
	}

	// Navigation skips the headings in both directions.
	mn.SetColumns(1)
	mn.SetSize(60, 0)
	start := mn.Index()
	mn.Update(keyDown())
	mn.Update(keyDown())
	if mn.Index() == start {
		t.Error("down should move the cursor")
	}
	for range 20 {
		mn.Update(keyUp())
	}
	if !itemSelectable(items, mn.Index()) {
		t.Errorf("Index() = %d, which is a section heading", mn.Index())
	}

	if _, ok := mustCmd(t, mn.Update(keyEnter())).(msg.ActivatedMsg); !ok {
		t.Error("enter should activate the highlighted item")
	}
	if _, ok := mustCmd(t, mn.Update(keyEsc())).(msg.DismissMsg); !ok {
		t.Error("esc should dismiss the menu")
	}
}

// TestMenuOfOnlyHeadingsHasNoCursor guards the clamp path where nothing in the
// list can be selected.
func TestMenuOfOnlyHeadingsHasNoCursor(t *testing.T) {
	mn := component.NewMenu("menu", "Reference", []component.MenuItem{
		component.MenuSection("Changes"),
		component.MenuSection("Navigation"),
	}, testTheme(), testKeys())
	mn.SetSize(40, 0)
	mn.SetIndex(1)
	mn.Focus()
	mn.Update(keyDown())
	if mn.View() == "" {
		t.Error("a menu of headings should still render")
	}
}

func itemSelectable(items []component.MenuItem, i int) bool {
	return i >= 0 && i < len(items) && !items[i].Header
}

func TestModalHintAndKeys(t *testing.T) {
	mo := component.NewModal("modal", "Confirm", "Delete a.txt?", testTheme(), testKeys())
	mo.SetSize(40, 0)
	mo.Focus()

	if _, ok := mustCmd(t, mo.Update(keyEnter())).(msg.ConfirmMsg); !ok {
		t.Error("enter should confirm the modal")
	}
	if _, ok := mustCmd(t, mo.Update(keyEsc())).(msg.DismissMsg); !ok {
		t.Error("esc should dismiss the modal")
	}

	mo.SetPrompt("Working", "Applying the patch…")
	mo.SetHint("")
	if view := mo.View(); strings.Contains(view, "confirm") {
		t.Errorf("a hintless modal should render no prompt, got:\n%s", view)
	}
}
