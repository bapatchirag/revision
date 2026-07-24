package component_test

import (
	"bytes"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/bapatchirag/revision/internal/tui/component"
)

const finalTimeout = 3 * time.Second

// nudge sends a key that no component acts on, forcing at least one render
// cycle so a static component's frame is guaranteed to be emitted.
func nudge(tm *teatest.TestModel) {
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
}

// finalOutput quits the program and returns everything it rendered over its
// lifetime (initial frame plus every frame produced by the sent messages).
func finalOutput(t *testing.T, tm *teatest.TestModel) []byte {
	t.Helper()
	_ = tm.Quit()
	tm.WaitFinished(t, teatest.WithFinalTimeout(finalTimeout))
	out, err := io.ReadAll(tm.FinalOutput(t))
	if err != nil {
		t.Fatalf("read final output: %v", err)
	}
	return out
}

func assertContains(t *testing.T, out []byte, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !bytes.Contains(out, []byte(w)) {
			t.Errorf("output missing %q\n---\n%s", w, out)
		}
	}
}

func TestTeatestListNavigation(t *testing.T) {
	l := component.NewList[string]("files", func(s string) string { return s }, testTheme(), testKeys())
	l.SetItems([]string{"alpha", "bravo", "charlie"})
	l.SetSize(20, 3)
	l.Focus()

	tm := teatest.NewTestModel(t, asModel(l), teatest.WithInitialTermSize(40, 10))
	tm.Send(keyDown())
	// The final frame shows the cursor moved from alpha to bravo.
	assertContains(t, finalOutput(t, tm), "> bravo")
}

func TestTeatestViewportScroll(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("L0\nL1\nL2\nL3\nL4\nL5")
	v.SetSize(8, 3)
	v.Focus()

	tm := teatest.NewTestModel(t, asModel(v), teatest.WithInitialTermSize(40, 10))
	tm.Send(keyDown())
	tm.Send(keyDown())
	// After scrolling two lines the window shows L2..L4 (L0/L1 scrolled off).
	assertContains(t, finalOutput(t, tm), "L2", "L4")
}

func TestTeatestTableNavigation(t *testing.T) {
	tb := component.NewTable[[]string]("log", []component.Column{
		{Title: "Rev", Width: 5},
		{Title: "Message", Width: 0},
	}, func(r []string) []string { return r }, testTheme(), testKeys())
	tb.SetItems([][]string{{"r3", "alpha"}, {"r2", "bravo"}, {"r1", "charlie"}})
	tb.SetSize(24, 4)
	tb.Focus()

	tm := teatest.NewTestModel(t, asModel(tb), teatest.WithInitialTermSize(40, 10))
	tm.Send(keyDown())
	assertContains(t, finalOutput(t, tm), "> r2", "bravo")
}

func TestTeatestMenuNavigation(t *testing.T) {
	mn := component.NewMenu("actions", "Actions", []component.MenuItem{
		{Label: "Commit", Key: "c"},
		{Label: "Revert", Key: "r"},
	}, testTheme(), testKeys())
	mn.Focus()

	tm := teatest.NewTestModel(t, asModel(mn), teatest.WithInitialTermSize(40, 10))
	tm.Send(keyDown())
	assertContains(t, finalOutput(t, tm), "> Revert")
}

func TestTeatestPanelRenders(t *testing.T) {
	l := component.NewList[string]("files", func(s string) string { return s }, testTheme(), testKeys())
	l.SetItems([]string{"main.go"})
	p := component.NewPanel("Files", 2, l, testTheme())
	p.SetSize(20, 5)
	p.Focus()

	tm := teatest.NewTestModel(t, asModel(p), teatest.WithInitialTermSize(40, 12))
	nudge(tm)
	assertContains(t, finalOutput(t, tm), "Files", "main.go")
}

func TestTeatestStatusBarRenders(t *testing.T) {
	b := component.NewStatusBar(testTheme())
	b.SetLeft("q quit")
	b.SetRight("trunk @ r42")
	b.SetSize(40, 1)

	tm := teatest.NewTestModel(t, asModel(b), teatest.WithInitialTermSize(60, 5))
	nudge(tm)
	assertContains(t, finalOutput(t, tm), "trunk @ r42")
}

func TestTeatestToastRenders(t *testing.T) {
	to := component.NewToast(testTheme())
	to.Show("Committed r128", component.LevelSuccess)

	tm := teatest.NewTestModel(t, asModel(to), teatest.WithInitialTermSize(40, 8))
	nudge(tm)
	assertContains(t, finalOutput(t, tm), "Committed r128")
}

func TestTeatestModalRenders(t *testing.T) {
	mo := component.NewModal("confirm", "Delete file?", "gone forever", testTheme(), testKeys())
	mo.Focus()

	tm := teatest.NewTestModel(t, asModel(mo), teatest.WithInitialTermSize(50, 10))
	nudge(tm)
	assertContains(t, finalOutput(t, tm), "Delete file?")
}

func TestTeatestTextAreaEditing(t *testing.T) {
	ta := component.NewTextArea("commit", "Commit message", "", testTheme(), testKeys())
	ta.SetSize(30, 6)
	ta.Focus()

	tm := teatest.NewTestModel(t, asModel(ta), teatest.WithInitialTermSize(50, 12))
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	assertContains(t, finalOutput(t, tm), "Commit message", "hello")
}

func TestTeatestViewsSwitch(t *testing.T) {
	a := component.NewList[string]("changes", func(s string) string { return s }, testTheme(), testKeys())
	a.SetItems([]string{"alpha"})
	b := component.NewList[string]("staged", func(s string) string { return s }, testTheme(), testKeys())
	b.SetItems([]string{"bravo"})
	vs := component.NewViews("views", []component.View{
		{Name: "Changes", Content: a},
		{Name: "Staged", Content: b},
	}, testTheme(), testKeys())
	p := component.NewPanel("Files", 2, vs, testTheme())
	p.SetSize(30, 6)
	p.Focus()

	tm := teatest.NewTestModel(t, asModel(p), teatest.WithInitialTermSize(40, 10))
	tm.Send(runes("]"))
	// After ] the Staged view is active (border tab) and shows its item.
	assertContains(t, finalOutput(t, tm), "Staged", "bravo")
}

func TestTeatestPromptTypesAndPicks(t *testing.T) {
	p := component.NewPrompt("changelist", "Changelist name", "e.g. feature-x", testTheme(), testKeys())
	p.SetOptions("Existing changelists:", []string{"feature-x", "hotfix"})
	p.SetSize(34, 0)
	p.Focus()

	tm := teatest.NewTestModel(t, asModel(p), teatest.WithInitialTermSize(44, 12))
	tm.Send(runes("docs"))
	assertContains(t, finalOutput(t, tm), "Changelist name", "Existing changelists:", "feature-x", "docs")
}
