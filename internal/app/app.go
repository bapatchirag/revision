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
	panelLog    = 2
	panelMain   = 3
)

// mainSource selects which side panel's selection drives the Main viewport.
type mainSource int

const (
	sourceFiles mainSource = iota
	sourceLog
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
	log    *component.Table[svn.LogEntry]
	main   *component.Viewport

	panels []*component.Panel
	bar    *component.StatusBar
	focus  *focus.Manager

	source   mainSource
	diffPath string
	diffText string
	logErr   error

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
	logTable := component.NewTable[svn.LogEntry]("log", logColumns(), renderLogRow, th, keys)
	main := component.NewViewport(th, keys)

	panels := []*component.Panel{
		component.NewPanel("Status", 1, status, th),
		component.NewPanel("Files", 2, files, th),
		component.NewPanel("Log", 3, logTable, th),
		component.NewPanel("Main", 0, main, th),
	}

	m := &Model{
		client:  client,
		info:    info,
		theme:   th,
		keys:    keys,
		status:  status,
		files:   files,
		log:     logTable,
		main:    main,
		panels:  panels,
		bar:     component.NewStatusBar(th),
		source:  sourceFiles,
		loading: true,
	}
	m.focus = focus.New(panels[panelStatus], panels[panelFiles], panels[panelLog], panels[panelMain])
	m.focus.Focus(panelFiles)

	m.refreshChrome()
	return m
}

// Init loads the initial working-copy status and revision history.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(loadStatusCmd(m.client), loadLogCmd(m.client))
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
		m.diffPath, m.diffText = "", ""
		m.files.SetItems(msg.items)
		m.refreshChrome()
		return m, m.diffLoadForSelection()

	case logLoadedMsg:
		m.logErr = msg.err
		m.log.SetItems(msg.entries)
		if m.source == sourceLog {
			m.updateMain()
		}
		return m, nil

	case diffLoadedMsg:
		m.diffPath = msg.path
		if msg.err != nil {
			m.diffText = "Unable to load diff: " + msg.err.Error()
		} else {
			m.diffText = msg.diff
		}
		if m.source == sourceFiles {
			m.updateMain()
		}
		return m, nil

	case errMsg:
		m.loading = false
		m.err = msg.err
		m.refreshChrome()
		return m, nil

	case uimsg.SelectedMsg:
		return m, m.handleSelection(msg)

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
		m.panels[panelLog].View(),
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
		m.diffPath, m.diffText = "", ""
		m.refreshChrome()
		return tea.Batch(loadStatusCmd(m.client), loadLogCmd(m.client)), true
	case key.Matches(k, m.keys.FocusNext):
		m.focus.Next()
		return m.afterFocusChange(), true
	case key.Matches(k, m.keys.FocusPrev):
		m.focus.Prev()
		return m.afterFocusChange(), true
	}

	switch k.String() {
	case "1":
		m.focus.Focus(panelStatus)
		return m.afterFocusChange(), true
	case "2":
		m.focus.Focus(panelFiles)
		return m.afterFocusChange(), true
	case "3":
		m.focus.Focus(panelLog)
		return m.afterFocusChange(), true
	case "0":
		m.focus.Focus(panelMain)
		return m.afterFocusChange(), true
	}
	return nil, false
}

// handleSelection re-renders Main when the selection that drives it changes, and
// loads the diff for a newly selected file.
func (m *Model) handleSelection(sel uimsg.SelectedMsg) tea.Cmd {
	switch sel.ID {
	case "files":
		if m.source == sourceFiles {
			m.updateMain()
			return m.diffLoadForSelection()
		}
	case "log":
		if m.source == sourceLog {
			m.updateMain()
		}
	}
	return nil
}

// afterFocusChange updates which panel drives Main, refreshes the chrome, and
// loads a diff when Main now follows the Files panel.
func (m *Model) afterFocusChange() tea.Cmd {
	switch m.focus.Index() {
	case panelLog:
		m.source = sourceLog
	case panelMain:
		// Focusing Main only scrolls it; keep the current source.
	default:
		m.source = sourceFiles
	}
	m.updateBar()
	m.updateMain()
	if m.source == sourceFiles {
		return m.diffLoadForSelection()
	}
	return nil
}

// diffLoadForSelection returns a command to load the diff of the selected file
// when it is dirty and not already loaded.
func (m *Model) diffLoadForSelection() tea.Cmd {
	it, ok := m.files.Selected()
	if !ok || !it.State.IsDirty() || m.diffPath == it.Path {
		return nil
	}
	return loadDiffCmd(m.client, it.Path)
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

	statusHeight := clamp(5, 3, max(bodyHeight-6, 3))
	rest := bodyHeight - statusHeight
	filesHeight := rest / 2
	logHeight := rest - filesHeight

	m.panels[panelStatus].SetSize(leftWidth, statusHeight)
	m.panels[panelFiles].SetSize(leftWidth, filesHeight)
	m.panels[panelLog].SetSize(leftWidth, logHeight)
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

// updateMain fills the Main panel from whichever side panel currently drives it.
func (m *Model) updateMain() {
	switch {
	case m.err != nil:
		m.main.SetContent("Error: " + m.err.Error() + "\n\nPress R to retry.")
		return
	case m.loading && len(m.files.Items()) == 0:
		m.main.SetContent("Loading working-copy status…")
		return
	}
	if m.source == sourceLog {
		m.main.SetContent(m.logDetail())
		return
	}
	m.main.SetContent(m.fileDetail())
}

// fileDetail renders the selected file's header followed by its diff, or a
// placeholder while the diff loads or when the state has no textual diff.
func (m *Model) fileDetail() string {
	it, ok := m.files.Selected()
	if !ok {
		return "Working copy is clean — no changes."
	}
	head := []string{
		it.Path,
		fmt.Sprintf("state: %s (%s)", it.State, it.State.Code()),
	}
	if it.Changelist != "" {
		head = append(head, "changelist: "+it.Changelist)
	}
	head = append(head, "")
	switch {
	case !it.State.IsDirty():
		return strings.Join(append(head, "(no textual diff for this state)"), "\n")
	case m.diffPath != it.Path:
		return strings.Join(append(head, "Loading diff…"), "\n")
	case strings.TrimSpace(m.diffText) == "":
		return strings.Join(append(head, "(no changes to display)"), "\n")
	default:
		return strings.Join(head, "\n") + "\n" + m.diffText
	}
}

// logDetail renders the metadata, message and changed paths of the selected
// revision.
func (m *Model) logDetail() string {
	entry, ok := m.log.Selected()
	if !ok {
		if m.logErr != nil {
			return "Unable to load history: " + m.logErr.Error()
		}
		return "No revision history."
	}
	author := entry.Author
	if author == "" {
		author = "(none)"
	}
	lines := []string{"r" + entry.Revision, "author: " + author}
	if !entry.Date.IsZero() {
		lines = append(lines, "date:   "+entry.Date.Format("2006-01-02 15:04"))
	}
	lines = append(lines, "", entry.Message)
	if len(entry.Paths) > 0 {
		lines = append(lines, "", "Changed paths:")
		for _, p := range entry.Paths {
			lines = append(lines, fmt.Sprintf("  %s %s", p.Action, p.Path))
		}
	}
	return strings.Join(lines, "\n")
}

// updateBar sets the contextual key hints and right-aligned repo context.
func (m *Model) updateBar() {
	m.bar.SetLeft("1 status · 2 files · 3 log · 0 main · tab cycle · R refresh · ? help · q quit")

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
