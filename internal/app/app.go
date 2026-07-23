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
	"github.com/bapatchirag/revision/internal/tui/layout"
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

// stagedChangelist is the SVN changelist name revision uses to emulate a
// staging area: paths in it are "staged" and committed as a unit.
const stagedChangelist = "revision:staged"

// commitEditorID identifies the commit-message editor on emitted messages.
const commitEditorID = "commit"

// confirmModalID identifies the shared confirmation modal on emitted messages.
const confirmModalID = "confirm"

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
	editor *component.TextArea
	modal  *component.Modal
	toast  *component.Toast
	focus  *focus.Manager

	source       mainSource
	diffPath     string
	diffText     string
	logErr       error
	editing      bool
	confirming   bool
	pending      tea.Cmd
	showingToast bool

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
		editor:  component.NewTextArea(commitEditorID, "Commit message", "Enter a commit message…", th, keys),
		modal:   component.NewModal(confirmModalID, "", "", th, keys),
		toast:   component.NewToast(th),
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
		if m.editing {
			m.sizeEditor()
		}
		if m.confirming {
			m.sizeModal()
		}
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

	case stagedMsg:
		if msg.err != nil {
			m.showToast("stage failed: "+msg.err.Error(), component.LevelError)
			return m, nil
		}
		// Reload status so the changelist grouping (and staged marker) refresh.
		return m, loadStatusCmd(m.client)

	case committedMsg:
		if msg.err != nil {
			m.loading = false
			m.showToast("commit failed: "+msg.err.Error(), component.LevelError)
			m.refreshChrome()
			return m, nil
		}
		if msg.revision != "" {
			m.showToast("committed r"+msg.revision, component.LevelSuccess)
		} else {
			m.showToast("commit complete", component.LevelSuccess)
		}
		m.diffPath, m.diffText = "", ""
		m.refreshChrome()
		return m, tea.Batch(loadStatusCmd(m.client), loadLogCmd(m.client))

	case revertedMsg:
		if msg.err != nil {
			m.showToast("revert failed: "+msg.err.Error(), component.LevelError)
			return m, nil
		}
		m.showToast("reverted "+msg.path, component.LevelSuccess)
		m.diffPath, m.diffText = "", ""
		return m, loadStatusCmd(m.client)

	case deletedMsg:
		if msg.err != nil {
			m.showToast("delete failed: "+msg.err.Error(), component.LevelError)
			return m, nil
		}
		m.showToast("deleted "+msg.path, component.LevelSuccess)
		m.diffPath, m.diffText = "", ""
		return m, loadStatusCmd(m.client)

	case updatedMsg:
		if msg.err != nil {
			m.loading = false
			m.showToast("update failed: "+msg.err.Error(), component.LevelError)
			m.refreshChrome()
			return m, nil
		}
		if msg.revision != "" {
			m.showToast("updated to r"+msg.revision, component.LevelSuccess)
		} else {
			m.showToast("update complete", component.LevelSuccess)
		}
		m.diffPath, m.diffText = "", ""
		return m, tea.Batch(loadStatusCmd(m.client), loadLogCmd(m.client))

	case uimsg.SelectedMsg:
		return m, m.handleSelection(msg)

	case uimsg.SubmitMsg:
		if msg.ID == commitEditorID {
			return m, m.submitCommit(msg.Value)
		}
		return m, nil

	case uimsg.ConfirmMsg:
		if msg.ID == confirmModalID {
			m.closeConfirm()
			cmd := m.pending
			m.pending = nil
			return m, cmd
		}
		return m, nil

	case uimsg.DismissMsg:
		switch msg.ID {
		case commitEditorID:
			m.editing = false
			m.editor.Blur()
		case confirmModalID:
			m.closeConfirm()
			m.pending = nil
		}
		return m, nil

	case tea.KeyMsg:
		if m.editing {
			return m, m.editor.Update(msg)
		}
		if m.confirming {
			return m, m.modal.Update(msg)
		}
		m.dismissToast()
		if cmd, handled := m.handleKey(msg); handled {
			return m, cmd
		}
	}

	return m, m.panels[m.focus.Index()].Update(msg)
}

// View renders the full lazygit layout, floating a transient toast and, while
// active, the commit editor or a confirmation modal over it.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	view := m.baseView()
	if m.showingToast {
		view = m.overlayToast(view)
	}
	switch {
	case m.editing:
		view = m.overlayCenter(view, m.editor.View())
	case m.confirming:
		view = m.overlayCenter(view, m.modal.View())
	}
	return view
}

// overlayCenter floats popup in the middle of the base view.
func (m *Model) overlayCenter(base, popup string) string {
	x := max((m.width-lipgloss.Width(popup))/2, 0)
	y := max((m.height-lipgloss.Height(popup))/2, 0)
	return layout.Overlay(base, popup, x, y)
}

// overlayToast floats the toast in the bottom-right corner, just above the
// status bar.
func (m *Model) overlayToast(base string) string {
	popup := m.toast.View()
	if popup == "" {
		return base
	}
	x := max(m.width-lipgloss.Width(popup)-1, 0)
	y := max(m.height-lipgloss.Height(popup)-1, 0) // 1 row for the status bar
	return layout.Overlay(base, popup, x, y)
}

// baseView renders the lazygit layout: the left column of panels beside Main,
// over the status bar.
func (m *Model) baseView() string {
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
		m.dismissToast()
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
	case " ":
		if m.focus.Index() == panelFiles {
			return m.stageSelected(), true
		}
		return nil, false
	case "c":
		return m.openCommit(), true
	case "r":
		if m.focus.Index() == panelFiles {
			return m.requestRevert(), true
		}
		return nil, false
	case "d":
		if m.focus.Index() == panelFiles {
			return m.requestDelete(), true
		}
		return nil, false
	case "u":
		return m.requestUpdate(), true
	}
	return nil, false
}

// stageSelected toggles the staged state of the file under the Files cursor,
// returning the command that performs the change (or nil when the selection is
// not stageable).
func (m *Model) stageSelected() tea.Cmd {
	act, ok := m.stageTarget()
	if !ok {
		if it, sel := m.files.Selected(); sel {
			m.showToast("can't stage "+it.Path+" ("+it.State.Code()+")", component.LevelWarning)
		}
		return nil
	}
	return stageCmd(m.client, stagedChangelist, act)
}

// stageAction describes how a stage keypress should change one file.
type stageAction struct {
	path  string
	add   bool // svn add first (unversioned → versioned)
	stage bool // add to (true) or remove from (false) the staged changelist
}

// deleteAction describes how a delete keypress should remove one file.
type deleteAction struct {
	path        string
	unversioned bool // remove from disk (untracked) vs. svn delete (versioned)
}

// stageTarget resolves what a stage action would do for the current Files
// selection. An unversioned file is added and staged in one step; a versioned
// pending change toggles its changelist membership. It returns ok=false when
// there is no selection or the selection cannot be staged.
func (m *Model) stageTarget() (stageAction, bool) {
	it, ok := m.files.Selected()
	if !ok {
		return stageAction{}, false
	}
	switch {
	case it.State == svn.StateUnversioned:
		return stageAction{path: it.Path, add: true, stage: true}, true
	case stageable(it.State):
		return stageAction{path: it.Path, stage: it.Changelist != stagedChangelist}, true
	default:
		return stageAction{}, false
	}
}

// openCommit opens the commit-message editor, provided something is staged.
func (m *Model) openCommit() tea.Cmd {
	if m.stagedCount() == 0 {
		m.showToast("nothing staged — press space to stage files", component.LevelWarning)
		return nil
	}
	m.editing = true
	m.editor.Reset()
	m.editor.Focus()
	m.sizeEditor()
	return nil
}

// requestRevert asks to discard local changes to the selected file, opening a
// confirmation modal. A clean/unversioned selection has nothing to revert.
func (m *Model) requestRevert() tea.Cmd {
	it, ok := m.files.Selected()
	if !ok {
		return nil
	}
	if !it.State.IsDirty() {
		m.showToast("nothing to revert in "+it.Path, component.LevelWarning)
		return nil
	}
	m.pending = revertCmd(m.client, it.Path)
	m.openConfirm("Revert changes?", "Discard local changes to "+it.Path+"? This cannot be undone.")
	return nil
}

// requestDelete asks to remove the selected file, opening a confirmation modal.
// A versioned file is scheduled for deletion; an unversioned one is removed from
// disk. Ignored files are left alone.
func (m *Model) requestDelete() tea.Cmd {
	it, ok := m.files.Selected()
	if !ok {
		return nil
	}
	if it.State == svn.StateIgnored {
		m.showToast("can't delete ignored "+it.Path, component.LevelWarning)
		return nil
	}
	act := deleteAction{path: it.Path, unversioned: it.State == svn.StateUnversioned}
	message := it.Path + " will be scheduled for deletion (removed on the next commit)."
	if act.unversioned {
		message = "Permanently delete untracked " + it.Path + " from disk? This cannot be undone."
	}
	m.pending = deleteCmd(m.client, act)
	m.openConfirm("Delete file?", message)
	return nil
}

// requestUpdate brings the working copy up to date with the repository.
func (m *Model) requestUpdate() tea.Cmd {
	m.loading = true
	m.refreshChrome()
	return updateCmd(m.client)
}

// openConfirm arms the shared modal with a prompt and shows it; the pending
// command runs when the user confirms.
func (m *Model) openConfirm(title, message string) {
	m.confirming = true
	m.modal.SetPrompt(title, message)
	m.modal.Focus()
	m.sizeModal()
}

// closeConfirm hides the confirmation modal.
func (m *Model) closeConfirm() {
	m.confirming = false
	m.modal.Blur()
}

// showToast displays a transient notice; it stays until the next interaction.
func (m *Model) showToast(text string, level component.Level) {
	m.toast.Show(text, level)
	m.showingToast = true
}

// dismissToast hides the current toast.
func (m *Model) dismissToast() { m.showingToast = false }

// submitCommit closes the editor and commits the staged changelist with the
// entered message, rejecting an empty message.
func (m *Model) submitCommit(message string) tea.Cmd {
	if strings.TrimSpace(message) == "" {
		m.showToast("commit message cannot be empty", component.LevelWarning)
		return nil
	}
	m.editing = false
	m.editor.Blur()
	m.loading = true
	m.refreshChrome()
	return commitCmd(m.client, message, stagedChangelist)
}

// stagedCount returns how many files are currently staged.
func (m *Model) stagedCount() int {
	n := 0
	for _, it := range m.files.Items() {
		if it.Changelist == stagedChangelist {
			n++
		}
	}
	return n
}

// sizeEditor sizes the commit editor to a centered portion of the screen.
func (m *Model) sizeEditor() {
	w := clamp(m.width*3/5, 40, max(m.width-4, 40))
	h := clamp(m.height/2, 8, max(m.height-4, 8))
	m.editor.SetSize(w, h)
}

// sizeModal sizes the confirmation modal to a centered portion of the screen
// (only its width matters; the height follows the wrapped message).
func (m *Model) sizeModal() {
	w := clamp(m.width/2, 34, max(m.width-6, 34))
	m.modal.SetSize(w, 0)
}

// stageable reports whether a working-copy state can be added to the staged
// changelist as-is. Only versioned, pending changes qualify. Unversioned files
// are handled separately by stageTarget (svn add + stage); ignored and missing
// paths are excluded (missing needs `svn rm` first).
func stageable(s svn.FileState) bool {
	switch s {
	case svn.StateModified, svn.StateAdded, svn.StateDeleted, svn.StateReplaced, svn.StateConflicted, svn.StateMerged:
		return true
	default:
		return false
	}
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
	m.bar.SetLeft("space stage · c commit · r revert · d delete · u update · R refresh · q quit")

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
