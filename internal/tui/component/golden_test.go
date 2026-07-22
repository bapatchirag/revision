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
