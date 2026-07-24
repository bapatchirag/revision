package component_test

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/tui/component"
)

func TestGoldenList(t *testing.T) {
	l := component.NewList[string]("files", func(s string) string { return s }, testTheme(), testKeys())
	l.SetItems([]string{"alpha.txt", "bravo.txt", "charlie.txt"})
	l.SetSize(20, 5)
	l.Focus()
	golden.RequireEqual(t, []byte(l.View()))
}

func TestGoldenListScrolled(t *testing.T) {
	l := component.NewList[string]("files", func(s string) string { return s }, testTheme(), testKeys())
	l.SetItems([]string{"one", "two", "three", "four", "five", "six"})
	l.SetSize(12, 3)
	l.Focus()
	// Move the cursor past the bottom of the window to force scrolling.
	for i := 0; i < 4; i++ {
		l.Update(keyDown())
	}
	golden.RequireEqual(t, []byte(l.View()))
}

func TestGoldenViewport(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("first line\nsecond line\nthird line")
	v.SetSize(20, 5)
	golden.RequireEqual(t, []byte(v.View()))
}

func TestGoldenViewportHScroll(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("the quick brown fox jumps over the lazy dog and keeps running")
	v.SetSize(20, 3)
	v.Focus()
	// Scroll right to expose the horizontal window past the left edge.
	for i := 0; i < 8; i++ {
		v.Update(keyRight())
	}
	golden.RequireEqual(t, []byte(v.View()))
}

func TestGoldenViewportScrollbars(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("the quick brown fox jumps over the lazy dog\npack my box with five dozen liquor jugs\nhow vexingly quick daft zebras jump\nthe five boxing wizards jump quickly\nsphinx of black quartz judge my vow\njackdaws love my big sphinx of quartz")
	v.SetSize(24, 4)
	v.Focus()
	// Scroll on both axes so each thumb sits away from the origin.
	for i := 0; i < 3; i++ {
		v.Update(keyDown())
	}
	for i := 0; i < 6; i++ {
		v.Update(keyRight())
	}
	golden.RequireEqual(t, []byte(v.View()))
}

func TestGoldenTable(t *testing.T) {
	tb := component.NewTable[[]string]("log", []component.Column{
		{Title: "Rev", Width: 6},
		{Title: "Author", Width: 8},
		{Title: "Message", Width: 0},
	}, func(r []string) []string { return r }, testTheme(), testKeys())
	tb.SetItems([][]string{
		{"r3", "alice", "Add diff viewport"},
		{"r2", "bob", "Fix status parsing"},
		{"r1", "alice", "Initial import"},
	})
	tb.SetSize(40, 6)
	tb.Focus()
	golden.RequireEqual(t, []byte(tb.View()))
}

func TestGoldenTableScrolled(t *testing.T) {
	tb := component.NewTable[[]string]("log", []component.Column{
		{Title: "Rev", Width: 4},
		{Title: "Message", Width: 0},
	}, func(r []string) []string { return r }, testTheme(), testKeys())
	tb.SetItems([][]string{
		{"r6", "six"}, {"r5", "five"}, {"r4", "four"},
		{"r3", "three"}, {"r2", "two"}, {"r1", "one"},
	})
	tb.SetSize(20, 4) // header + 3 body rows
	tb.Focus()
	// Move the cursor past the bottom of the window to force scrolling.
	for i := 0; i < 4; i++ {
		tb.Update(keyDown())
	}
	golden.RequireEqual(t, []byte(tb.View()))
}

func TestGoldenListHScroll(t *testing.T) {
	l := component.NewList[string]("files", func(s string) string { return s }, testTheme(), testKeys())
	l.SetItems([]string{
		"internal/tui/component/viewport.go",
		"internal/tui/component/list.go",
		"internal/app/app.go",
	})
	l.SetSize(20, 5)
	l.Focus()
	for i := 0; i < 8; i++ {
		l.Update(keyRight())
	}
	golden.RequireEqual(t, []byte(l.View()))
}

func TestGoldenTableHScroll(t *testing.T) {
	tb := component.NewTable[[]string]("log", []component.Column{
		{Title: "Rev", Width: 4},
		{Title: "Message", Width: 0},
	}, func(r []string) []string { return r }, testTheme(), testKeys())
	tb.SetItems([][]string{
		{"r3", "add horizontal scrolling to the list and table panels"},
		{"r2", "reuse the viewport scroll helpers"},
		{"r1", "initial import"},
	})
	tb.SetSize(24, 5)
	tb.Focus()
	for i := 0; i < 8; i++ {
		tb.Update(keyRight())
	}
	golden.RequireEqual(t, []byte(tb.View()))
}

func TestGoldenStatusBar(t *testing.T) {
	b := component.NewStatusBar(testTheme())
	b.SetLeft("2 files · tab cycle · q quit")
	b.SetRight("trunk @ r42")
	b.SetSize(50, 1)
	golden.RequireEqual(t, []byte(b.View()))
}

func TestGoldenPanel(t *testing.T) {
	l := component.NewList[string]("files", func(s string) string { return s }, testTheme(), testKeys())
	l.SetItems([]string{"main.go", "app.go", "list.go"})
	p := component.NewPanel("Files", 2, l, testTheme())
	p.SetSize(24, 7)
	p.Focus()
	golden.RequireEqual(t, []byte(p.View()))
}

func TestGoldenPanelUnfocused(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("https://svn.example.com/repo/trunk\nr42\n3 change(s)")
	p := component.NewPanel("Status", 1, v, testTheme())
	p.SetSize(30, 5)
	golden.RequireEqual(t, []byte(p.View()))
}

func TestGoldenModal(t *testing.T) {
	mo := component.NewModal("confirm", "Delete file?", "src/main.go will be removed.", testTheme(), testKeys())
	mo.Focus()
	golden.RequireEqual(t, []byte(mo.View()))
}

func TestGoldenModalSized(t *testing.T) {
	mo := component.NewModal("confirm", "Delete file?", "", testTheme(), testKeys())
	mo.SetPrompt("Revert changes?", "Discard local changes to internal/app/app.go? This cannot be undone.")
	mo.SetSize(40, 0)
	mo.Focus()
	golden.RequireEqual(t, []byte(mo.View()))
}

func TestGoldenToast(t *testing.T) {
	to := component.NewToast(testTheme())
	to.Show("Committed r128", component.LevelSuccess)
	golden.RequireEqual(t, []byte(to.View()))
}

func TestGoldenMenu(t *testing.T) {
	mn := component.NewMenu("actions", "Actions", []component.MenuItem{
		{Label: "Stage file", Key: "space"},
		{Label: "Commit", Key: "c"},
		{Label: "Revert", Key: "r"},
	}, testTheme(), testKeys())
	mn.Focus()
	golden.RequireEqual(t, []byte(mn.View()))
}

func TestGoldenTextArea(t *testing.T) {
	ta := component.NewTextArea("commit", "Commit message", "Describe your change…", testTheme(), testKeys())
	ta.SetValue("Fix status parsing\n\nHandle the changelist grouping.")
	ta.SetSize(40, 8)
	ta.Focus()
	golden.RequireEqual(t, []byte(ta.View()))
}

func TestGoldenTextAreaPlaceholder(t *testing.T) {
	ta := component.NewTextArea("commit", "Commit message", "Describe your change…", testTheme(), testKeys())
	ta.SetSize(40, 6)
	golden.RequireEqual(t, []byte(ta.View()))
}

func TestGoldenViews(t *testing.T) {
	changes := component.NewList[string]("changes", func(s string) string { return s }, testTheme(), testKeys())
	changes.SetItems([]string{"app.go", "views.go", "keymap.go"})
	staged := component.NewList[string]("staged", func(s string) string { return s }, testTheme(), testKeys())
	staged.SetItems([]string{"views.go"})
	vs := component.NewViews("files-views", []component.View{
		{Name: "Changes", Content: changes},
		{Name: "Staged", Content: staged},
	}, testTheme(), testKeys())
	// A multi-view container renders its tabs in the host Panel's border.
	p := component.NewPanel("Files", 2, vs, testTheme())
	p.SetSize(34, 7)
	p.Focus()
	golden.RequireEqual(t, []byte(p.View()))
}

func TestGoldenViewsDrilled(t *testing.T) {
	base := component.NewList[string]("log", func(s string) string { return s }, testTheme(), testKeys())
	base.SetItems([]string{"r42 add views", "r41 fix parse"})
	vs := component.NewViews("log-views", []component.View{
		{Name: "Log", Content: base},
	}, testTheme(), testKeys())
	p := component.NewPanel("Log", 3, vs, testTheme())
	p.SetSize(34, 7)
	p.Focus()
	// Drill into an unnamed sub-view (a revision's changed paths); the Panel
	// border gains a breadcrumb chevron.
	sub := component.NewList[string]("log-paths", func(s string) string { return s }, testTheme(), testKeys())
	sub.SetItems([]string{"A component/views.go", "M internal/app/app.go"})
	vs.Push(sub)
	golden.RequireEqual(t, []byte(p.View()))
}

func TestGoldenViewsDrilledTitled(t *testing.T) {
	base := component.NewList[string]("files", func(s string) string { return s }, testTheme(), testKeys())
	base.SetItems([]string{"feature-x (2)", "(staged) (1)"})
	vs := component.NewViews("files-views", []component.View{
		{Name: "Changes", Content: base},
		{Name: "Changelists", Content: base},
	}, testTheme(), testKeys())
	p := component.NewPanel("Files", 2, vs, testTheme())
	p.SetSize(34, 7)
	p.Focus()
	// Drill into a titled sub-view: the Panel border shows just that title in
	// place of the tabs (no chevron).
	sub := component.NewList[string]("cl-files", func(s string) string { return s }, testTheme(), testKeys())
	sub.SetItems([]string{"M app.go", "A views.go"})
	vs.PushTitled("feature-x", sub)
	golden.RequireEqual(t, []byte(p.View()))
}

func TestGoldenPrompt(t *testing.T) {
	p := component.NewPrompt("changelist", "Changelist name", "e.g. feature-x", testTheme(), testKeys())
	p.SetOptions("Existing changelists:", []string{"feature-x", "hotfix"})
	p.SetValue("feat")
	p.SetSize(40, 0)
	p.Focus()
	golden.RequireEqual(t, []byte(p.View()))
}

func TestGoldenPromptNoOptions(t *testing.T) {
	p := component.NewPrompt("changelist", "Changelist name", "e.g. feature-x", testTheme(), testKeys())
	p.SetSize(40, 0)
	p.Focus()
	golden.RequireEqual(t, []byte(p.View()))
}

func TestGoldenPromptListFocused(t *testing.T) {
	p := component.NewPrompt("changelist", "Changelist name", "e.g. feature-x", testTheme(), testKeys())
	p.SetOptions("Existing changelists:", []string{"feature-x", "hotfix"})
	p.SetSize(40, 0)
	p.Focus()
	// Tab into the list, then scroll to the second option; the input mirrors the
	// highlighted option and the hint switches to the list-mode variant.
	p.Update(keyTab())
	p.Update(keyDown())
	golden.RequireEqual(t, []byte(p.View()))
}
