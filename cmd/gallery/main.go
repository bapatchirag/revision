// Command gallery renders each reusable TUI component in isolation, so every
// widget can be eyeballed on its own (make run-gallery). Switch components with
// Tab / [ / ] or the number keys; other keys are forwarded to the focused
// component so you can drive it.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/keymap"
	"github.com/bapatchirag/revision/internal/tui/layout"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

const sampleText = `Index: internal/app/app.go
===================================================================
--- internal/app/app.go   (revision 128)
+++ internal/app/app.go   (working copy)
@@ -18,6 +18,7 @@
 	status *component.Viewport
 	files  *component.List[svn.StatusItem]
+	log    *component.Table[svn.LogEntry]
 	main   *component.Viewport

@@ -40,7 +41,7 @@
-	// diff preview arrives in a later phase
+	m.main.SetContent(m.fileDetail())

 The Viewport scrolls read-only text such as diffs. Use j/k or the
 arrow keys to move a line at a time and PgUp/PgDn (J/K) by a page.`

type demo struct {
	name string
	comp tui.Component
}

type model struct {
	demos  []demo
	idx    int
	width  int
	height int
	keys   keymap.KeyMap
}

func newModel() model {
	th := theme.Default()
	keys := keymap.Default()

	list := component.NewList[string]("gallery-list", func(s string) string { return s }, th, keys)
	list.SetItems([]string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf"})
	list.Focus()

	vp := component.NewViewport(th, keys)
	vp.SetContent(sampleText)
	vp.Focus()

	table := component.NewTable[[]string]("gallery-log", []component.Column{
		{Title: "Rev", Width: 6},
		{Title: "Author", Width: 10},
		{Title: "Date", Width: 10},
		{Title: "Message", Width: 0},
	}, func(r []string) []string { return r }, th, keys)
	table.SetItems([][]string{
		{"r128", "alice", "2026-07-20", "Add diff viewport + log table"},
		{"r127", "bob", "2026-07-19", "Fix status xml parsing"},
		{"r126", "alice", "2026-07-18", "Introduce component gallery"},
		{"r125", "carol", "2026-07-17", "Initial import"},
	})
	table.Focus()

	bar := component.NewStatusBar(th)
	bar.SetLeft("j/k move · enter select · q quit")
	bar.SetRight("gallery @ demo")

	inner := component.NewList[string]("panel-list", func(s string) string { return s }, th, keys)
	inner.SetItems([]string{"README.md", "cmd/gallery/main.go", "PLAN.md", "LOG.md"})
	inner.Focus()
	panel := component.NewPanel("Files", 2, inner, th)
	panel.Focus()

	changes := component.NewList[string]("gallery-changes", func(s string) string { return s }, th, keys)
	changes.SetItems([]string{"internal/app/app.go", "internal/tui/component/views.go", "PLAN.md"})
	staged := component.NewList[string]("gallery-staged", func(s string) string { return s }, th, keys)
	staged.SetItems([]string{"internal/tui/component/views.go"})
	views := component.NewViews("gallery-views", []component.View{
		{Name: "Changes", Content: changes},
		{Name: "Staged", Content: staged},
	}, th, keys)
	viewsPanel := component.NewPanel("Files", 2, views, th)
	viewsPanel.Focus()

	modal := component.NewModal("gallery-modal", "Delete file?", "src/main.go will be removed.", th, keys)
	modal.Focus()

	toast := component.NewToast(th)
	toast.Show("Committed r128", component.LevelSuccess)

	menu := component.NewMenu("gallery-menu", "Actions", []component.MenuItem{
		{Label: "Stage file", Key: "space"},
		{Label: "Commit", Key: "c"},
		{Label: "Revert", Key: "r"},
		{Label: "Refresh", Key: "R"},
	}, th, keys)
	menu.Focus()

	editor := component.NewTextArea("gallery-editor", "Commit message", "Describe your change…", th, keys)
	editor.SetValue("Add reusable TextArea component\n\nEmits SubmitMsg on ctrl+s; edits multi-line text.")
	editor.Focus()

	return model{
		keys: keys,
		demos: []demo{
			{"List[string]", list},
			{"Viewport (diff)", vp},
			{"Table (log)", table},
			{"StatusBar", bar},
			{"Panel + List", panel},
			{"Views (tabs)", viewsPanel},
			{"Modal", modal},
			{"Toast", toast},
			{"Menu", menu},
			{"TextArea", editor},
		},
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeDemos()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "]", "right":
			m.idx = (m.idx + 1) % len(m.demos)
			return m, nil
		case "shift+tab", "[", "left":
			m.idx = (m.idx - 1 + len(m.demos)) % len(m.demos)
			return m, nil
		}
		if n := int(msg.String()[0] - '0'); len(msg.String()) == 1 && n >= 1 && n <= len(m.demos) {
			m.idx = n - 1
			return m, nil
		}
		// Forward everything else to the focused demo; ignore its command.
		_ = m.demos[m.idx].comp.Update(msg)
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	title := lipgloss.NewStyle().Bold(true).Render("revision component gallery")
	name := lipgloss.NewStyle().Bold(true).Render(m.demos[m.idx].name)
	header := fmt.Sprintf("%s — %s (%d/%d)", title, name, m.idx+1, len(m.demos))
	footer := fmt.Sprintf("tab/[/] switch · 1-%d jump · other keys drive the component · q quit", len(m.demos))

	bodyHeight := m.height - 4
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	body := layout.Center(m.width, bodyHeight, m.demos[m.idx].comp.View())

	return strings.Join([]string{header, "", body, footer}, "\n")
}

func (m *model) resizeDemos() {
	demoW := min(m.width-6, 70)
	demoH := min(m.height-8, 16)
	if demoW < 10 {
		demoW = 10
	}
	if demoH < 3 {
		demoH = 3
	}
	for _, d := range m.demos {
		if s, ok := d.comp.(tui.Sizeable); ok {
			s.SetSize(demoW, demoH)
		}
	}
}

func main() {
	if _, err := tea.NewProgram(newModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gallery:", err)
		os.Exit(1)
	}
}
