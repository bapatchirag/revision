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

// changelistEditorID identifies the changelist-name prompt on emitted messages.
const changelistEditorID = "changelist"

// filesViewsID identifies the Files panel's multi-view container on emitted
// messages (the Changes / Changelists tabs and their drill-downs).
const filesViewsID = "files-views"

// changelistsListID / changelistFilesID identify the Changelists list and its
// drilled-in file list on emitted selection/activation messages.
const (
	changelistsListID = "changelists"
	changelistFilesID = "changelist-files"
)

// confirmModalID identifies the shared confirmation modal on emitted messages.
const confirmModalID = "confirm"

// helpMenuID identifies the keybindings help menu on emitted messages.
const helpMenuID = "help"

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

	status      *component.Viewport
	files       *component.List[svn.StatusItem]
	changelists *component.List[changelistGroup]
	clFiles     *component.List[svn.StatusItem]
	filesViews  *component.Views
	log         *component.Table[svn.LogEntry]
	main        *component.Viewport

	panels     []*component.Panel
	bar        *component.StatusBar
	editor     *component.TextArea
	nameEditor *component.Prompt
	modal      *component.Modal
	menu       *component.Menu
	toast      *component.Toast
	focus      *focus.Manager

	source       mainSource
	diffPath     string
	diffText     string
	logErr       error
	editing      bool
	naming       bool
	namePath     string
	nameAdd      bool
	drilledCL    string
	commitCL     string
	confirming   bool
	helping      bool
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
	changelists := component.NewList[changelistGroup](changelistsListID, renderChangelistGroup(th), th, keys)
	clFiles := component.NewList[svn.StatusItem](changelistFilesID, renderStatusItem(th), th, keys)
	filesViews := component.NewViews(filesViewsID, []component.View{
		{Name: "Changes", Content: files},
		{Name: "Changelists", Content: changelists},
	}, th, keys)
	logTable := component.NewTable[svn.LogEntry]("log", logColumns(), renderLogRow, th, keys)
	main := component.NewViewport(th, keys)

	panels := []*component.Panel{
		component.NewPanel("Status", 1, status, th),
		component.NewPanel("Files", 2, filesViews, th),
		component.NewPanel("Log", 3, logTable, th),
		component.NewPanel("Main", 0, main, th),
	}

	m := &Model{
		client:      client,
		info:        info,
		theme:       th,
		keys:        keys,
		status:      status,
		files:       files,
		changelists: changelists,
		clFiles:     clFiles,
		filesViews:  filesViews,
		log:         logTable,
		main:        main,
		panels:      panels,
		bar:         component.NewStatusBar(th),
		editor:      component.NewTextArea(commitEditorID, "Commit message", "Enter a commit message…", th, keys),
		nameEditor:  component.NewPrompt(changelistEditorID, "Changelist name", "e.g. feature-x", th, keys),
		modal:       component.NewModal(confirmModalID, "", "", th, keys),
		menu:        component.NewMenu(helpMenuID, "Keybindings", helpMenuItems(), th, keys),
		toast:       component.NewToast(th),
		source:      sourceFiles,
		commitCL:    stagedChangelist,
		loading:     true,
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
		if m.naming {
			m.sizeNameEditor()
		}
		if m.confirming {
			m.sizeModal()
		}
		if m.helping {
			m.sizeMenu()
		}
		return m, nil

	case statusLoadedMsg:
		m.loading = false
		m.err = nil
		m.diffPath, m.diffText = "", ""
		m.files.SetItems(msg.items)
		m.changelists.SetItems(groupChangelists(msg.items))
		m.syncDrill()
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
			m.showToast(failureText("stage", msg.err), component.LevelError)
			return m, nil
		}
		if msg.changelist != "" {
			m.showToast("added "+msg.path+" to "+msg.changelist, component.LevelSuccess)
		}
		// Reload status so the changelist grouping (and staged marker) refresh.
		return m, loadStatusCmd(m.client)

	case committedMsg:
		if msg.err != nil {
			m.loading = false
			m.showToast(failureText("commit", msg.err), component.LevelError)
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
			m.showToast(failureText("revert", msg.err), component.LevelError)
			return m, nil
		}
		m.showToast("reverted "+msg.path, component.LevelSuccess)
		m.diffPath, m.diffText = "", ""
		return m, loadStatusCmd(m.client)

	case deletedMsg:
		if msg.err != nil {
			m.showToast(failureText("delete", msg.err), component.LevelError)
			return m, nil
		}
		m.showToast("deleted "+msg.path, component.LevelSuccess)
		m.diffPath, m.diffText = "", ""
		return m, loadStatusCmd(m.client)

	case updatedMsg:
		if msg.err != nil {
			m.loading = false
			m.showToast(failureText("update", msg.err), component.LevelError)
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

	case uimsg.ActivatedMsg:
		// Enter on a changelist row drills into its files; enter on the help
		// menu is inert (it is a read-only keybindings reference).
		if msg.ID == changelistsListID {
			return m, m.drillChangelist()
		}
		return m, nil

	case uimsg.ViewSelectedMsg:
		if msg.ID == filesViewsID {
			m.updateBar()
			m.updateMain()
			if msg.Name == "Changes" {
				return m, m.diffLoadForSelection()
			}
		}
		return m, nil

	case uimsg.SubViewPoppedMsg:
		if msg.ID == filesViewsID {
			m.drilledCL = ""
			m.updateBar()
			m.updateMain()
		}
		return m, nil

	case uimsg.SubmitMsg:
		switch msg.ID {
		case commitEditorID:
			return m, m.submitCommit(msg.Value)
		case changelistEditorID:
			return m, m.submitChangelist(msg.Value)
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
		case changelistEditorID:
			m.naming = false
			m.nameEditor.Blur()
		case confirmModalID:
			m.closeConfirm()
			m.pending = nil
		}
		return m, nil

	case tea.KeyMsg:
		if m.editing {
			return m, m.editor.Update(msg)
		}
		if m.naming {
			return m, m.nameEditor.Update(msg)
		}
		if m.confirming {
			return m, m.modal.Update(msg)
		}
		if m.helping {
			// Read-only reference: only ? and esc close it; other keys drive the
			// menu (enter/n are inert, handled above).
			if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Back) {
				m.closeHelp()
				return m, nil
			}
			return m, m.menu.Update(msg)
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
	case m.naming:
		view = m.overlayCenter(view, m.nameEditor.View())
	case m.confirming:
		view = m.overlayCenter(view, m.modal.View())
	case m.helping:
		view = m.overlayCenter(view, m.menu.View())
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
	case key.Matches(k, m.keys.Help):
		return m.openHelp(), true
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
	case "n":
		if m.focus.Index() == panelFiles {
			return m.assignChangelist(), true
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

// stageSelected toggles the staged state of the file under the current file
// selection (the Changes view or a drilled-in changelist), returning the command
// that performs the change (or nil when the selection is not stageable).
func (m *Model) stageSelected() tea.Cmd {
	act, ok := m.stageTarget()
	if !ok {
		if it, sel := m.selectedFile(); sel {
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
	stage bool // add to (true) or remove from (false) a changelist
}

// deleteAction describes how a delete keypress should remove one file.
type deleteAction struct {
	path        string
	unversioned bool // remove from disk (untracked) vs. svn delete (versioned)
}

// stageTarget resolves what a stage action would do for the current file
// selection. An unversioned file is added and staged in one step; a file already
// in any changelist (the anonymous staged bucket or a named list) is removed from
// it — space never moves a file between changelists, enforcing one-changelist-
// per-file; an unassigned pending change is added to the staged bucket. It
// returns ok=false when there is no file selected or it cannot be staged.
func (m *Model) stageTarget() (stageAction, bool) {
	it, ok := m.selectedFile()
	if !ok {
		return stageAction{}, false
	}
	switch {
	case it.State == svn.StateUnversioned:
		return stageAction{path: it.Path, add: true, stage: true}, true
	case it.Changelist != "":
		return stageAction{path: it.Path, stage: false}, true
	case stageable(it.State):
		return stageAction{path: it.Path, stage: true}, true
	default:
		return stageAction{}, false
	}
}

// drillChangelist expands the selected changelist into its file list as a
// drill-down sub-view, labeling the panel with the changelist and tracking which
// one is open so a status reload can keep it in sync.
func (m *Model) drillChangelist() tea.Cmd {
	g, ok := m.changelists.Selected()
	if !ok {
		return nil
	}
	m.clFiles.SetItems(g.Items)
	m.drilledCL = g.Name
	cmd := m.filesViews.PushTitled(g.Label(), m.clFiles)
	m.updateBar()
	m.updateMain()
	return tea.Batch(cmd, m.diffLoadForSelection())
}

// submitChangelist closes the name prompt and assigns the selected file to the
// entered changelist, rejecting an empty or reserved name.
func (m *Model) submitChangelist(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	switch name {
	case "":
		m.showToast("changelist name cannot be empty", component.LevelWarning)
		return nil
	case stagedChangelist:
		m.showToast("that changelist name is reserved", component.LevelWarning)
		return nil
	}
	m.naming = false
	m.nameEditor.Blur()
	return assignChangelistCmd(m.client, name, m.namePath, m.nameAdd)
}

// selectedFile returns the file the current Files-panel view points at: the
// Changes list selection, or the selection within a drilled-in changelist. At
// the Changelists overview (a group is selected, not a file) there is no single
// file, so ok is false.
func (m *Model) selectedFile() (svn.StatusItem, bool) {
	if m.filesViewIsChangelists() {
		if m.inChangelistDrill() {
			return m.clFiles.Selected()
		}
		return svn.StatusItem{}, false
	}
	return m.files.Selected()
}

// filesViewIsChangelists reports whether the Files panel's active view is the
// Changelists view.
func (m *Model) filesViewIsChangelists() bool {
	return m.filesViews.ActiveName() == "Changelists"
}

// inChangelistDrill reports whether the Changelists view is drilled into a
// changelist's file list.
func (m *Model) inChangelistDrill() bool {
	return m.filesViewIsChangelists() && m.filesViews.Depth() > 0
}

// assignChangelist opens the changelist-name prompt for the selected file, so it
// can be added to a named changelist. A file already in a named changelist is
// refused (one named changelist per file — unstage it first); files in the
// anonymous staged/unstaged buckets may be moved into a named changelist. A
// state that cannot be staged is refused too. The prompt lists the existing
// named changelists to pick from.
func (m *Model) assignChangelist() tea.Cmd {
	it, ok := m.selectedFile()
	if !ok {
		return nil
	}
	if isNamedChangelist(it.Changelist) {
		m.showToast(it.Path+" already in "+displayCL(it.Changelist)+" — unstage first (space)", component.LevelWarning)
		return nil
	}
	if it.State != svn.StateUnversioned && !stageable(it.State) {
		m.showToast("can't add "+it.Path+" to a changelist ("+it.State.Code()+")", component.LevelWarning)
		return nil
	}
	m.naming = true
	m.namePath = it.Path
	m.nameAdd = it.State == svn.StateUnversioned
	m.nameEditor.Reset()
	m.nameEditor.SetOptions("Existing changelists:", m.namedChangelists())
	m.nameEditor.Focus()
	m.sizeNameEditor()
	return nil
}

// syncDrill refreshes a drilled-in changelist after a status reload: it
// repopulates the file list from the rebuilt groups, or collapses the drill when
// that changelist no longer exists (e.g. its last file was unstaged).
func (m *Model) syncDrill() {
	if !m.filesViewIsChangelists() || m.filesViews.Depth() == 0 {
		return
	}
	for _, g := range m.changelists.Items() {
		if g.Name == m.drilledCL {
			m.clFiles.SetItems(g.Items)
			return
		}
	}
	m.filesViews.Pop()
	m.drilledCL = ""
}

// openCommit opens the commit-message editor for the current commit target: the
// selected changelist when in the Changelists view, otherwise the anonymous
// staged bucket. It refuses an empty target.
func (m *Model) openCommit() tea.Cmd {
	target, label, ok := m.commitTarget()
	if !ok {
		return nil
	}
	if m.countInChangelist(target) == 0 {
		m.showToast("nothing staged in "+label+" — press space to stage files", component.LevelWarning)
		return nil
	}
	m.commitCL = target
	m.editing = true
	m.editor.Reset()
	m.editor.Focus()
	m.sizeEditor()
	return nil
}

// commitTarget resolves which changelist a commit would target. In the
// Changelists view it is the selected (or drilled-in) changelist, refusing the
// default/unstaged group which is not an addressable changelist; everywhere else
// it is the anonymous staged bucket.
func (m *Model) commitTarget() (cl, label string, ok bool) {
	if m.focus.Index() == panelFiles && m.filesViewIsChangelists() {
		if m.inChangelistDrill() {
			if m.drilledCL == "" {
				m.showToast("the (unstaged) group isn't a changelist — stage or name files first", component.LevelWarning)
				return "", "", false
			}
			return m.drilledCL, displayCL(m.drilledCL), true
		}
		g, sel := m.changelists.Selected()
		if !sel {
			return "", "", false
		}
		if !g.Committable() {
			m.showToast("the "+g.Label()+" group isn't a changelist — stage or name files first", component.LevelWarning)
			return "", "", false
		}
		return g.Name, g.Label(), true
	}
	return stagedChangelist, displayCL(stagedChangelist), true
}

// countInChangelist returns how many pending files belong to the named
// changelist.
func (m *Model) countInChangelist(name string) int {
	n := 0
	for _, it := range m.files.Items() {
		if it.Changelist == name {
			n++
		}
	}
	return n
}

// requestRevert asks to discard local changes to the selected file, opening a
// confirmation modal. A clean/unversioned selection has nothing to revert.
func (m *Model) requestRevert() tea.Cmd {
	it, ok := m.selectedFile()
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
	it, ok := m.selectedFile()
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

// openHelp shows the keybindings help menu as a centered overlay.
func (m *Model) openHelp() tea.Cmd {
	m.helping = true
	m.menu.Focus()
	m.sizeMenu()
	return nil
}

// closeHelp hides the help menu.
func (m *Model) closeHelp() {
	m.helping = false
	m.menu.Blur()
}

// showToast displays a transient notice; it stays until the next interaction.
func (m *Model) showToast(text string, level component.Level) {
	m.toast.Show(text, level)
	m.showingToast = true
}

// dismissToast hides the current toast.
func (m *Model) dismissToast() { m.showingToast = false }

// failureText renders an action failure for a toast. An svn authentication
// failure collapses to a short, actionable hint instead of a raw multi-line svn
// error dump.
func failureText(action string, err error) string {
	if svn.IsAuthError(err) {
		return action + " failed: " + svn.AuthHint
	}
	return action + " failed: " + err.Error()
}

// helpMenuItems is the keybindings reference shown by the "?" help menu.
func helpMenuItems() []component.MenuItem {
	return []component.MenuItem{
		{Label: "Stage / unstage", Key: "space"},
		{Label: "Assign changelist", Key: "n"},
		{Label: "Commit staged / changelist", Key: "c"},
		{Label: "Switch file view", Key: "[ / ]"},
		{Label: "Expand changelist", Key: "enter"},
		{Label: "Revert file", Key: "r"},
		{Label: "Delete file", Key: "d"},
		{Label: "Update working copy", Key: "u"},
		{Label: "Refresh", Key: "R"},
		{Label: "Jump to panel", Key: "1 2 3 0"},
		{Label: "Cycle panels", Key: "tab / shift+tab"},
		{Label: "Move up / down", Key: "k / j"},
		{Label: "Jump top / bottom", Key: "g / G"},
		{Label: "Scroll main up / down", Key: "K / J"},
		{Label: "Scroll main left / right", Key: "h / l"},
		{Label: "Line start / end", Key: "home / end"},
		{Label: "Toggle help", Key: "?"},
		{Label: "Quit", Key: "q"},
	}
}

// submitCommit closes the editor and commits the target changelist with the
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
	return commitCmd(m.client, message, m.commitCL)
}

// sizeEditor sizes the commit editor to a centered portion of the screen.
func (m *Model) sizeEditor() {
	w := clamp(m.width*3/5, 40, max(m.width-4, 40))
	h := clamp(m.height/2, 8, max(m.height-4, 8))
	m.editor.SetSize(w, h)
}

// sizeNameEditor sizes the changelist-name prompt (only its width matters; the
// height follows the input and option rows).
func (m *Model) sizeNameEditor() {
	w := clamp(m.width/2, 30, max(m.width-6, 30))
	m.nameEditor.SetSize(w, 0)
}

// namedChangelists returns the existing user-named changelists (excluding the
// anonymous staged/unstaged buckets), for the assign prompt to offer as options.
func (m *Model) namedChangelists() []string {
	var names []string
	for _, g := range m.changelists.Items() {
		if isNamedChangelist(g.Name) {
			names = append(names, g.Name)
		}
	}
	return names
}

// isNamedChangelist reports whether cl is a real user-named changelist, i.e. not
// the empty default group or the anonymous staged bucket.
func isNamedChangelist(cl string) bool {
	return cl != "" && cl != stagedChangelist
}

// sizeModal sizes the confirmation modal to a centered portion of the screen
// (only its width matters; the height follows the wrapped message).
func (m *Model) sizeModal() {
	w := clamp(m.width/2, 34, max(m.width-6, 34))
	m.modal.SetSize(w, 0)
}

// sizeMenu sizes the help menu to a centered portion of the screen (only its
// width matters; the height follows the item count).
func (m *Model) sizeMenu() {
	m.menu.SetSize(clamp(m.width/2, 40, max(m.width-6, 40)), 0)
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
	case "files", changelistFilesID:
		if m.source == sourceFiles {
			m.updateMain()
			return m.diffLoadForSelection()
		}
	case changelistsListID:
		if m.source == sourceFiles {
			m.updateMain()
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
	it, ok := m.selectedFile()
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

	statusHeight := clamp(6, 3, max(bodyHeight-6, 3))
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
	// Only a unified diff carries the one-column +/-/space marker that must stay
	// pinned while the body scrolls horizontally; error/loading placeholders and
	// log/changelist detail have no gutter.
	m.main.SetGutter(0)
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
	if m.filesShowDiff() {
		m.main.SetGutter(1)
	}
	m.main.SetContent(m.filesMain())
}

// filesMain renders the Main content for the Files panel, which depends on its
// active view: the Changelists overview shows a changelist summary, everything
// else (the Changes view or a drilled-in changelist) shows the selected file.
func (m *Model) filesMain() string {
	if m.filesViewIsChangelists() && !m.inChangelistDrill() {
		return m.changelistDetail()
	}
	return m.fileDetail()
}

// filesShowDiff reports whether filesMain currently renders a unified diff — the
// only Main view with a +/-/space gutter to pin. It mirrors the default branch of
// fileDetail: the Files panel is showing files (not the Changelists overview) and
// the selected file is dirty with a non-empty, freshly-loaded diff.
func (m *Model) filesShowDiff() bool {
	if m.filesViewIsChangelists() && !m.inChangelistDrill() {
		return false
	}
	it, ok := m.selectedFile()
	if !ok || !it.State.IsDirty() {
		return false
	}
	return m.diffPath == it.Path && strings.TrimSpace(m.diffText) != ""
}

// changelistDetail summarizes the selected changelist: its label, file count and
// the paths it groups.
func (m *Model) changelistDetail() string {
	g, ok := m.changelists.Selected()
	if !ok {
		return "No changelists yet — stage files (space) or assign one (n)."
	}
	lines := []string{
		"Changelist: " + g.Label(),
		fmt.Sprintf("%d file(s)", len(g.Items)),
		"",
	}
	if g.Committable() {
		lines = append(lines, "enter expand · c commit this changelist", "")
	} else {
		lines = append(lines, "Files in no changelist (committable by default).", "")
	}
	for _, it := range g.Items {
		lines = append(lines, fmt.Sprintf("  %s %s", it.State.Code(), it.Path))
	}
	return strings.Join(lines, "\n")
}

// fileDetail renders the selected file's diff, prefixed by its changelist when
// it belongs to one, or a placeholder while the diff loads or when the state has
// no textual diff.
func (m *Model) fileDetail() string {
	it, ok := m.selectedFile()
	if !ok {
		return "Working copy is clean — no changes."
	}
	var head []string
	if it.Changelist != "" {
		head = append(head, "changelist: "+displayCL(it.Changelist), "")
	}
	switch {
	case !it.State.IsDirty():
		return strings.Join(append(head, "(no textual diff for this state)"), "\n")
	case m.diffPath != it.Path:
		return strings.Join(append(head, "Loading diff…"), "\n")
	case strings.TrimSpace(m.diffText) == "":
		return strings.Join(append(head, "(no changes to display)"), "\n")
	default:
		return strings.Join(append(head, colorizeDiff(m.theme, m.diffText)), "\n")
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
	m.bar.SetLeft(m.barHint())

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

// barHint returns the contextual keybinding hint for the current Files-panel
// view: the Changelists overview and its drill-down each get their own hints,
// the Changes view (and every other panel) get the file-oriented hint.
func (m *Model) barHint() string {
	if m.focus.Index() == panelFiles && m.filesViewIsChangelists() {
		if m.inChangelistDrill() {
			return "space unstage · c commit · esc back · [ ] view · ? help"
		}
		return "enter expand · c commit · [ ] view · n name · ? help"
	}
	return "space stage · n changelist · c commit · r revert · d delete · ? help"
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
