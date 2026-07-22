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

const sampleText = `revision is a lazygit-style TUI for Subversion.

The Viewport component scrolls read-only text such as diffs and
file detail. Use j/k or the arrow keys to move a line at a time
and PgUp/PgDn (J/K) to move a page at a time.

Line 1
Line 2
Line 3
Line 4
Line 5
Line 6
Line 7
Line 8`

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

	bar := component.NewStatusBar(th)
	bar.SetLeft("j/k move · enter select · q quit")
	bar.SetRight("gallery @ demo")

	inner := component.NewList[string]("panel-list", func(s string) string { return s }, th, keys)
	inner.SetItems([]string{"README.md", "cmd/gallery/main.go", "PLAN.md", "LOG.md"})
	inner.Focus()
	panel := component.NewPanel("Files", 2, inner, th)
	panel.Focus()

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

	return model{
		keys: keys,
		demos: []demo{
			{"List[string]", list},
			{"Viewport", vp},
			{"StatusBar", bar},
			{"Panel + List", panel},
			{"Modal", modal},
			{"Toast", toast},
			{"Menu", menu},
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
	footer := "tab/[/] switch · 1-7 jump · other keys drive the component · q quit"

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
