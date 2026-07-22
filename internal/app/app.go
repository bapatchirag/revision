// Package app is the composition layer: it is the only package that knows both
// the SVN domain (internal/svn) and the reusable component library
// (internal/tui/component). It adapts SVN data into components and arranges them
// into the lazygit-style layout.
package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/focus"
	"github.com/bapatchirag/revision/internal/tui/keymap"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// panel ring indices.
const (
	panelStatus = 0
	panelFiles  = 1
	panelMain   = 2
)

// Model is the root Bubble Tea model. It composes reusable components into the
// lazygit layout: a left column (Status + Files) beside a Main viewport, over a
// contextual status bar.
type Model struct {
	client *svn.Client
	info   *svn.Info

	theme theme.Theme
	keys  keymap.KeyMap

	status *component.Viewport
	files  *component.List[svn.StatusItem]
	main   *component.Viewport

	panels []*component.Panel
	bar    *component.StatusBar
	focus  *focus.Manager

	width   int
	height  int
	loading bool
	err     error
}

var _ tea.Model = (*Model)(nil)

// New creates the root model for the given client and working-copy info.
func New(client *svn.Client, info *svn.Info) *Model {
	th := theme.Default()
	keys := keymap.Default()

	status := component.NewViewport(th, keys)
	files := component.NewList[svn.StatusItem]("files", renderStatusItem(th), th, keys)
	main := component.NewViewport(th, keys)

	panels := []*component.Panel{
		component.NewPanel("Status", 1, status, th),
		component.NewPanel("Files", 2, files, th),
		component.NewPanel("Main", 0, main, th),
	}

	m := &Model{
		client:  client,
		info:    info,
		theme:   th,
		keys:    keys,
		status:  status,
		files:   files,
		main:    main,
		panels:  panels,
		bar:     component.NewStatusBar(th),
		loading: true,
	}
	m.focus = focus.New(panels[panelStatus], panels[panelFiles], panels[panelMain])
	m.focus.Focus(panelFiles)

	m.refreshChrome()
	return m
}

// Init loads the initial working-copy status.
func (m *Model) Init() tea.Cmd {
	return loadStatusCmd(m.client)
}

// Update handles messages, global keys, and forwards the rest to the focused
// panel.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case statusLoadedMsg:
		m.loading = false
		m.err = nil
		m.files.SetItems(msg.items)
		m.refreshChrome()
		return m, nil

	case errMsg:
		m.loading = false
		m.err = msg.err
		m.refreshChrome()
		return m, nil

	case uimsg.SelectedMsg:
		if msg.ID == "files" {
			m.updateMain()
		}
		return m, nil

	case tea.KeyMsg:
		if cmd, handled := m.handleKey(msg); handled {
			return m, cmd
		}
	}

	return m, m.panels[m.focus.Index()].Update(msg)
}

// View renders the full lazygit layout.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	left := lipgloss.JoinVertical(lipgloss.Left,
		m.panels[panelStatus].View(),
		m.panels[panelFiles].View(),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, m.panels[panelMain].View())
	return lipgloss.JoinVertical(lipgloss.Left, body, m.bar.View())
}

// handleKey processes global keys, returning whether the key was consumed.
func (m *Model) handleKey(k tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(k, m.keys.Quit):
		return tea.Quit, true
	case key.Matches(k, m.keys.Refresh):
		m.loading = true
		m.refreshChrome()
		return loadStatusCmd(m.client), true
	case key.Matches(k, m.keys.FocusNext):
		m.focus.Next()
		m.updateBar()
		return nil, true
	case key.Matches(k, m.keys.FocusPrev):
		m.focus.Prev()
		m.updateBar()
		return nil, true
	}

	switch k.String() {
	case "1":
		m.focus.Focus(panelStatus)
		m.updateBar()
		return nil, true
	case "2":
		m.focus.Focus(panelFiles)
		m.updateBar()
		return nil, true
	case "0":
		m.focus.Focus(panelMain)
		m.updateBar()
		return nil, true
	}
	return nil, false
}

// layout sizes the panels and bar for the current terminal dimensions.
func (m *Model) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	const barHeight = 1
	bodyHeight := max(m.height-barHeight, 3)

	leftWidth := clamp(m.width*2/5, 24, m.width-20)
	rightWidth := m.width - leftWidth

	statusHeight := clamp(5, 3, bodyHeight-3)
	filesHeight := bodyHeight - statusHeight

	m.panels[panelStatus].SetSize(leftWidth, statusHeight)
	m.panels[panelFiles].SetSize(leftWidth, filesHeight)
	m.panels[panelMain].SetSize(rightWidth, bodyHeight)
	m.bar.SetSize(m.width, barHeight)
	m.updateMain()
}

// refreshChrome recomputes the derived content in the Status panel, Main panel
// and status bar.
func (m *Model) refreshChrome() {
	m.updateStatus()
	m.updateMain()
	m.updateBar()
}

// updateStatus fills the Status panel with repo/revision/summary info.
func (m *Model) updateStatus() {
	lines := make([]string, 0, 3)
	if m.info != nil {
		lines = append(lines, m.info.URL, "r"+m.info.Revision)
	}
	lines = append(lines, fmt.Sprintf("%d change(s)", len(m.files.Items())))
	m.status.SetContent(strings.Join(lines, "\n"))
}

// updateMain fills the Main panel from the current selection or app state.
func (m *Model) updateMain() {
	switch {
	case m.err != nil:
		m.main.SetContent("Error: " + m.err.Error() + "\n\nPress R to retry.")
		return
	case m.loading && len(m.files.Items()) == 0:
		m.main.SetContent("Loading working-copy status…")
		return
	}

	it, ok := m.files.Selected()
	if !ok {
		m.main.SetContent("Working copy is clean — no changes.")
		return
	}

	lines := []string{
		it.Path,
		"",
		fmt.Sprintf("state:      %s (%s)", it.State, it.State.Code()),
	}
	if it.Changelist != "" {
		lines = append(lines, "changelist: "+it.Changelist)
	}
	lines = append(lines, "", "(diff preview arrives in a later phase)")
	m.main.SetContent(strings.Join(lines, "\n"))
}

// updateBar sets the contextual key hints and right-aligned repo context.
func (m *Model) updateBar() {
	m.bar.SetLeft("1 status · 2 files · 0 main · tab cycle · R refresh · ? help · q quit")

	switch {
	case m.err != nil:
		m.bar.SetRight("error")
	case m.loading:
		m.bar.SetRight("loading…")
	case m.info != nil:
		m.bar.SetRight(fmt.Sprintf("%s @ r%s", m.info.URL, m.info.Revision))
	default:
		m.bar.SetRight("")
	}
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
