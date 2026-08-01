// Package app is the composition layer: it is the only package that knows both
// the SVN domain (internal/svn) and the reusable component library
// (internal/tui/component). It adapts SVN data into components and arranges them
// into the lazygit-style layout.
package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/sshagent"
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
	// panelCmdLog is the command-log panel below Main. It is last in the focus
	// ring and is skipped by Tab while hidden.
	panelCmdLog = 4
)

// stagedChangelist is the SVN changelist name revision uses to emulate a
// staging area: paths in it are "staged" and committed as a unit.
const stagedChangelist = "revision:staged"

// commitEditorID identifies the commit-message editor on emitted messages.
const commitEditorID = "commit"

// changelistEditorID identifies the changelist-name prompt on emitted messages.
const changelistEditorID = "changelist"

// diffNameEditorID identifies the save-diff file-name prompt on emitted messages.
const diffNameEditorID = "diff-name"

// splitDiffID identifies the side-by-side diff overlay on emitted messages.
const splitDiffID = "split-diff"

// passphraseEditorID identifies the SSH passphrase prompt on emitted messages.
const passphraseEditorID = "ssh-passphrase"

// maxPassphraseAttempts is how many wrong passphrases the SSH unlock overlay
// tolerates before giving up and exiting, since the key is required to proceed.
const maxPassphraseAttempts = 3

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

// updateMenuID identifies the startup update prompt on emitted messages.
const updateMenuID = "update"

// settingsFormID identifies the settings editor on emitted messages.
const settingsFormID = "settings"

// searchBarID identifies the panel filter input on emitted messages.
const searchBarID = "search"

// diffDebounce is how long the cursor must rest on a selection before its diff
// is asked of svn, so holding j/k through a tree runs one `svn diff` instead of
// one per row. A diff already cached is shown immediately, debounce or not.
const diffDebounce = 90 * time.Millisecond

// mainSource selects which side panel's selection drives the Main viewport.
type mainSource int

const (
	sourceFiles mainSource = iota
	sourceLog
	sourceStatus
)

// Model is the root Bubble Tea model. It composes reusable components into the
// lazygit layout: a left column (Status + Files) beside a Main viewport, over a
// contextual status bar.
type Model struct {
	client *svn.Client
	info   *svn.Info

	theme theme.Theme
	keys  keymap.KeyMap
	cfg   config.Config

	status      *component.Viewport
	files       *component.List[fileNode]
	changelists *component.List[changelistGroup]
	clFiles     *component.List[fileNode]
	savedDiffs  *component.List[savedDiff]
	filesViews  *component.Views
	log         *component.Table[svn.LogEntry]
	main        *component.Viewport

	// cmdLogView displays cmdLog: the svn commands revision runs and their
	// output, shown in a toggleable panel below Main.
	cmdLogView *component.Viewport
	cmdLog     *commandLog
	cmdLogSeen int64

	panels     []*component.Panel
	bar        *component.StatusBar
	editor     *component.TextArea
	nameEditor *component.Prompt
	diffEditor *component.Prompt
	pathEditor *component.Prompt
	passEditor *component.Prompt
	modal      *component.Modal
	progress   *component.Modal
	menu       *component.Menu
	updateMenu *component.Menu
	form       *component.Form
	toast      *component.Toast
	searchBar  *component.SearchBar
	splitDiff  *component.SplitView
	focus      *focus.Manager

	fileItems        []svn.StatusItem
	collapsedDirs    map[string]bool
	filesInitialized bool
	clItems          []svn.StatusItem
	clCollapsedDirs  map[string]bool
	logEntries       []svn.LogEntry
	// logPage is the 1-based page of history on screen. logAnchors[i] is the
	// revision page i+2 starts after — recorded as each page loads, since a page
	// can only be addressed by a revision from the one before it — and logMore
	// reports whether a further page exists.
	logPage    int
	logAnchors []string
	logMore    bool
	// logRequested records that the first page of history has been asked for, so
	// the prefetch that follows the first status and the load that follows the
	// first look at the Log panel cannot both fetch it. logLoading is true while a
	// page is in flight, which dims the rows of the page being left.
	logRequested bool
	logLoading   bool
	// headRev is the repository's newest revision, read at startup and refreshed
	// whenever the first page of history lands.
	headRev    string
	wcRevision string
	// workDir is the directory revision was launched from (os.Getwd at startup).
	workDir string
	// launchDir is the directory the svn client was pointed at on startup — the
	// -path target, which is the working directory by default. It is the anchor
	// for the "cwd" display scope, kept so the scope can be switched back after
	// the client has been re-rooted at the working copy.
	launchDir string

	// savedDiffItems is every patch file found in the configured output
	// directory, before the Files-panel filter narrows the Diffs view; savedPath
	// and savedText are the file whose contents Main is showing for that view.
	savedDiffItems []savedDiff
	savedDiffsErr  error
	savedPath      string
	savedText      string
	savedErr       bool

	source   mainSource
	diffPath string
	diffText string
	// diffOfDir records that the diff on screen was produced for a directory row
	// rather than a file, so it can be matched to its cache entry after the fact.
	diffOfDir bool
	// session caches what is reusable for the life of the process — diffs, so
	// far. It is in-memory only and is emptied by Close.
	session *sessionStore
	// diffGen, statusGen, logGen and savedGen stamp the loads whose replies can
	// be overtaken by a later request, so a superseded reply is dropped instead
	// of rendered. diffGen covers both diff sources feeding Main — the working
	// copy and the saved-patch reader — since only one of them is on screen.
	diffGen   loadGen
	statusGen loadGen
	logGen    loadGen
	revGen    loadGen
	savedGen  loadGen
	// mainKey identifies the selection mainText was rendered for, so a refresh of
	// what is already on screen can keep the reader's scroll position.
	mainKey  string
	mainText string
	// diffErr records that diffText is a load-failure notice rather than a patch,
	// so it is never written out as one.
	diffErr bool
	// diffSrc is the diff (or the paths to diff) queued while the save prompt
	// asks for a file name, so a reload landing behind the overlay cannot swap
	// out what gets written.
	diffSrc       diffSource
	dirDiff       bool
	hideUntracked bool
	// showCmdLog controls whether the command-log panel below Main is displayed.
	// It defaults to on and is toggled at runtime; it is not persisted.
	showCmdLog bool
	logErr     error
	editing    bool
	naming     bool
	savingDiff bool
	// retargeting is true while the prompt that re-points revision at another
	// source directory is open.
	retargeting bool
	// splitting is true while the side-by-side view of the on-screen diff is
	// floated over the layout.
	splitting    bool
	filtering    bool
	filterPanel  int
	filters      map[int]string
	nameTargets  []changelistTarget
	drilledCL    string
	commitCL     string
	themeBefore  string
	confirming   bool
	helping      bool
	configuring  bool
	needsSSHKey  bool
	unlocking    bool
	adding       bool
	aborting     bool
	passAttempts int
	pending      tea.Cmd
	// updateConflictPrompt stages a second "conflicts will be skipped" confirm shown after the default update confirm.
	updateConflictPrompt string
	// updatingWC is true while an svn update runs, showing the progress modal.
	updatingWC     bool
	updateProgress string
	showingToast   bool
	startupNotice  string

	build        selfupdate.Build
	updating     bool
	updateRel    selfupdate.Release
	updateMethod selfupdate.Method
	updateChosen bool

	width   int
	height  int
	loading bool
	err     error
}

var _ tea.Model = (*Model)(nil)

// New creates the root model for the given client and working-copy info. build
// carries the running binary's provenance so a release build can offer to
// self-update on startup.
func New(client *svn.Client, info *svn.Info, build selfupdate.Build, cfg config.Config) *Model {
	th, _ := theme.ByName(cfg.Theme)
	keys := keymap.Default()

	// m is captured by the log renderer below so a row can be starred when it
	// matches the working-copy revision; it is assigned before any render runs.
	var m *Model

	status := component.NewViewport(th, keys)
	files := component.NewList[fileNode]("files", renderFileNode(th), th, keys)
	changelists := component.NewList[changelistGroup](changelistsListID, renderChangelistGroup(th), th, keys)
	clFiles := component.NewList[fileNode](changelistFilesID, renderFileNode(th), th, keys)
	savedDiffs := component.NewList[savedDiff](savedDiffsListID, renderSavedDiff(th), th, keys)
	filesViews := component.NewViews(filesViewsID, []component.View{
		{Name: "Changes", Content: files},
		{Name: "Changelists", Content: changelists},
		{Name: savedDiffsViewName, Content: savedDiffs},
	}, th, keys)
	logTable := component.NewTable[svn.LogEntry]("log", logColumns(), func(it svn.LogEntry) []string {
		return renderLogRow(it, m.wcRevision, m.logLoading, m.theme)
	}, th, keys)
	main := component.NewViewport(th, keys)
	cmdLogView := component.NewViewport(th, keys)

	panels := []*component.Panel{
		component.NewPanel("Status", 1, status, th),
		component.NewPanel("Files", 2, filesViews, th),
		component.NewPanel("Log", 3, logTable, th),
		component.NewPanel("Main", 0, main, th),
		component.NewPanel("Command Log", 4, cmdLogView, th),
	}

	m = &Model{
		client:          client,
		info:            info,
		theme:           th,
		keys:            keys,
		cfg:             cfg,
		status:          status,
		files:           files,
		changelists:     changelists,
		clFiles:         clFiles,
		savedDiffs:      savedDiffs,
		filesViews:      filesViews,
		log:             logTable,
		main:            main,
		cmdLogView:      cmdLogView,
		cmdLog:          newCommandLog(commandLogLimit),
		panels:          panels,
		bar:             component.NewStatusBar(th),
		editor:          component.NewTextArea(commitEditorID, "Commit message", "Enter a commit message…", th, keys),
		nameEditor:      component.NewPrompt(changelistEditorID, "Changelist name", "e.g. feature-x", th, keys),
		diffEditor:      component.NewPrompt(diffNameEditorID, "Save diff as", "e.g. changes.diff", th, keys),
		pathEditor:      component.NewPrompt(sourcePathID, "Change source path", "e.g. /path/to/working-copy", th, keys),
		passEditor:      component.NewPrompt(passphraseEditorID, "SSH key passphrase", "passphrase for "+cfg.SSHKeyPath, th, keys),
		modal:           component.NewModal(confirmModalID, "", "", th, keys),
		progress:        component.NewModal("update-progress", "", "", th, keys),
		menu:            component.NewMenu(helpMenuID, "Keybindings", helpMenuItems(), th, keys),
		updateMenu:      component.NewMenu(updateMenuID, "Update available", updateMenuItems(), th, keys),
		form:            component.NewForm(settingsFormID, "Settings", settingsFields(cfg, cfg.DirectoryDiff), th, keys),
		toast:           component.NewToast(th),
		searchBar:       component.NewSearchBar(searchBarID, th, keys),
		splitDiff:       component.NewSplitView(splitDiffID, "Side-by-side diff", th, keys),
		collapsedDirs:   map[string]bool{},
		clCollapsedDirs: map[string]bool{},
		filters:         map[int]string{},
		session:         newSessionStore(),
		source:          sourceFiles,
		dirDiff:         cfg.DirectoryDiff,
		hideUntracked:   cfg.HideUntracked,
		showCmdLog:      true,
		needsSSHKey:     info != nil && info.IsOverSSH(),
		commitCL:        stagedChangelist,
		build:           build,
		logPage:         1,
		loading:         true,
	}
	m.passEditor.SetSecret(true)
	m.progress.SetHint("")
	m.menu.SetReadOnly(true)
	if client != nil {
		m.launchDir = client.Dir
	}
	m.retargetDisplay(cfg.DisplayFrom)
	// Mirror user-initiated svn actions into the command log, skipping the
	// read-only queries revision runs automatically. The recorder is set before
	// Init runs any command, so no action is missed.
	if m.client != nil {
		log := m.cmdLog
		m.client.Recorder = func(r svn.CommandRecord) {
			if !loggedCommand(r) {
				return
			}
			log.record(r)
		}
	}
	m.cmdLogView.SetContent(m.renderCommandLog(nil))
	m.focus = focus.New(panels[panelStatus], panels[panelFiles], panels[panelLog], panels[panelMain], panels[panelCmdLog])
	m.focus.Focus(panelFiles)
	m.syncMainTitle()

	if info != nil {
		m.wcRevision = info.Revision
	}
	m.workDir, _ = os.Getwd()
	m.refreshChrome()
	return m
}

// retargetDisplay roots the svn client at the directory the given display scope
// names — the working copy's root for config.DisplayFromRoot, otherwise the
// directory revision was launched in — so status, history and diffs all describe
// the same tree. The client is replaced rather than mutated so any command
// already in flight keeps running against the directory it started with.
func (m *Model) retargetDisplay(scope string) {
	if m.client == nil {
		return
	}
	dir := m.launchDir
	if scope == config.DisplayFromRoot && m.info != nil && m.info.WorkingCopyRoot != "" {
		dir = m.info.WorkingCopyRoot
	}
	if dir == "" || dir == m.client.Dir {
		return
	}
	next := *m.client
	next.Dir = dir
	m.client = &next
}

// Init loads the initial working-copy status and the revision the repository is
// at, and — on a release build — checks GitHub for a newer version in the
// background. When the working copy is served over svn+ssh, it first ensures the
// configured SSH key is loaded in the agent, deferring the initial load behind
// the passphrase overlay when the key still needs unlocking.
//
// A page of history and the saved-diff scan are not part of startup: neither can
// be seen until a panel that shows them is looked at, so both are deferred.
func (m *Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.needsSSHKey {
		cmds = append(cmds, sshCheckCmd(m.cfg.SSHKeyPath))
	} else {
		cmds = append(cmds, m.beginInitialLoad())
	}
	if m.build.IsRelease() {
		cmds = append(cmds, checkUpdateCmd(m.build))
	}
	if m.startupNotice != "" {
		cmds = append(cmds, startupNoticeCmd(m.startupNotice))
	}
	return tea.Batch(cmds...)
}

// Close ends the session: every svn command still in flight is abandoned, the
// caches are purged and the content the panels retained is released. It is the
// counterpart to New and runs on every exit path, including the one that hands
// off to a self-update. Nothing was written outside the process, so there is
// nothing to clean up beyond memory. It is safe to call more than once.
func (m *Model) Close() {
	m.diffGen.stop()
	m.statusGen.stop()
	m.logGen.stop()
	m.revGen.stop()
	m.savedGen.stop()
	m.session.Close()
	m.clearDiff()
	m.mainKey, m.mainText = "", ""
	m.savedPath, m.savedText, m.savedErr = "", "", false
	m.diffSrc = diffSource{}
	m.fileItems, m.clItems, m.logEntries, m.savedDiffItems = nil, nil, nil, nil
	m.cmdLog.clear()
}

// Update handles messages, global keys, and forwards the rest to the focused
// panel.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Refresh the command log from any invocations that finished since the last
	// message; every svn command completes by delivering a message here.
	m.syncCommandLog()
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
		if m.savingDiff {
			m.sizeDiffEditor()
		}
		if m.retargeting {
			m.sizeSourcePath()
		}
		if m.splitting {
			m.sizeSplitDiff()
		}
		if m.confirming {
			m.sizeModal()
		}
		if m.helping {
			m.sizeMenu()
		}
		if m.updating {
			m.sizeUpdateMenu()
		}
		if m.configuring {
			m.sizeForm()
		}
		if m.unlocking {
			m.sizeUnlock()
		}
		if m.updatingWC {
			m.sizeProgress()
		}
		return m, nil

	case statusLoadedMsg:
		if m.statusGen.stale(msg.gen) {
			return m, nil
		}
		m.loading = false
		m.err = nil
		m.fileItems = msg.items
		// The status every cached diff was produced against has just been re-read:
		// keep the diffs it still agrees with and re-derive what is on screen from
		// them, so a reload no longer blanks a diff it is about to fetch unchanged.
		m.session.Reconcile(m.diffStamp)
		m.rederiveDiff()
		m.rebuildFileTree()
		m.focusFirstFile()
		m.rebuildChangelists()
		m.syncDrill()
		m.refreshChrome()
		// The first paint is done, so the page of history the Log panel needs can
		// be fetched now without the cost falling on startup.
		return m, tea.Batch(m.diffLoadForSelection(), m.ensureLogPage())

	case logLoadedMsg:
		// A page turn taken while this load was in flight supersedes it.
		if m.logGen.stale(msg.gen) || msg.page != m.logPage {
			return m, nil
		}
		m.logLoading = false
		if msg.err == nil {
			m.session.PutLogPage(
				logKey{anchor: m.logAnchor(msg.page), limit: m.cfg.LogLimit},
				historyPage{entries: msg.entries, more: msg.more},
			)
		}
		m.applyLogPage(historyPage{entries: msg.entries, more: msg.more}, msg.err)
		return m, m.revisionDetailForSelection()

	case headLoadedMsg:
		// The HEAD probe is the only svn command startup runs against the
		// repository, so its failure is what tells the Log panel it is unreachable.
		if msg.err != nil {
			m.logErr = msg.err
			if m.source == sourceLog {
				m.updateMain()
			}
			return m, nil
		}
		m.headRev = msg.rev
		m.updateStatus()
		return m, nil

	case revisionPendingMsg:
		// The cursor rested long enough for this revision's paths to be worth
		// asking svn for.
		if m.revGen.stale(msg.gen) {
			return m, nil
		}
		ctx, gen := m.revGen.begin(loadTimeout)
		return m, loadRevisionDetailCmd(ctx, m.client, msg.rev, gen)

	case revisionDetailMsg:
		// A revision that cannot be described leaves its metadata on screen; only
		// the changed-path list is missing, which is not worth an error for.
		if m.revGen.stale(msg.gen) || msg.err != nil {
			return m, nil
		}
		m.session.PutRevDetail(msg.rev, msg.paths)
		if m.source == sourceLog {
			m.updateMain()
		}
		return m, nil

	case diffPendingMsg:
		// The cursor rested long enough for this diff to be worth asking svn for.
		if m.diffGen.stale(msg.gen) {
			return m, nil
		}
		ctx, gen := m.diffGen.begin(loadTimeout)
		return m, loadDiffCmd(ctx, m.client, msg.key, gen)

	case diffLoadedMsg:
		if m.diffGen.stale(msg.gen) {
			return m, nil
		}
		k := diffKey{path: msg.path, dir: msg.dir}
		e := diffEntry{text: msg.diff, stamp: m.diffStamp(k)}
		if msg.err != nil {
			e.text, e.failed = "Unable to load diff: "+msg.err.Error(), true
		}
		m.session.PutDiff(k, e)
		m.applyDiff(k, e)
		if m.source == sourceFiles {
			m.updateMain()
		}
		return m, nil

	case diffSavedMsg:
		if msg.err != nil {
			m.showToast(failureText("save diff", msg.err), component.LevelError)
			return m, nil
		}
		m.showToast("diff saved to "+msg.path, component.LevelSuccess)
		// The new file belongs in the Diffs view, so re-scan the store.
		return m, m.reloadSavedDiffs()

	case savedDiffsLoadedMsg:
		if m.savedGen.stale(msg.gen) {
			return m, nil
		}
		m.savedDiffsErr = msg.err
		m.savedDiffItems = msg.files
		m.rebuildSavedDiffs()
		if m.source == sourceFiles && m.filesViewIsDiffs() {
			m.updateMain()
			return m, m.savedDiffLoadForSelection()
		}
		return m, nil

	case savedDiffReadMsg:
		if m.diffGen.stale(msg.gen) {
			return m, nil
		}
		m.savedPath = msg.path
		m.savedErr = msg.err != nil
		if msg.err != nil {
			m.savedText = "Unable to read diff: " + msg.err.Error()
		} else {
			m.savedText = msg.text
		}
		if m.source == sourceFiles {
			m.updateMain()
		}
		return m, nil

	case editedMsg:
		if msg.err != nil {
			m.showToast(failureText("open "+msg.name, msg.err), component.LevelError)
			return m, nil
		}
		if msg.detached {
			return m, nil
		}
		// A terminal editor has exited, so the file may have changed: re-read the
		// working copy (which reloads the diff on screen) and the saved-diff store.
		return m, tea.Batch(m.reloadStatus(), m.reloadSavedDiffsIfShown())

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
		return m, m.reloadStatus()

	case committedMsg:
		if msg.err != nil {
			m.loading = false
			m.showToast(failureText("commit", msg.err), component.LevelError)
			m.refreshChrome()
			return m, nil
		}
		if msg.revision != "" {
			m.wcRevision = msg.revision
			m.showToast("committed r"+msg.revision, component.LevelSuccess)
		} else {
			m.showToast("commit complete", component.LevelSuccess)
		}
		m.clearDiff()
		m.refreshChrome()
		// A commit adds a revision at the head of history: show it.
		m.log.GoTop()
		return m, tea.Batch(m.reloadStatus(), m.resetLogPaging())

	case revertedMsg:
		if msg.err != nil {
			m.showToast(failureText("revert", msg.err), component.LevelError)
			return m, nil
		}
		m.showToast("reverted "+msg.path, component.LevelSuccess)
		m.clearDiff()
		return m, m.reloadStatus()

	case deletedMsg:
		if msg.err != nil {
			m.showToast(failureText("delete", msg.err), component.LevelError)
			return m, nil
		}
		m.showToast("deleted "+msg.path, component.LevelSuccess)
		m.clearDiff()
		return m, m.reloadStatus()

	case updatedMsg:
		m.updatingWC = false
		m.updateProgress = ""
		if msg.err != nil {
			m.loading = false
			m.showToast(failureText("update", msg.err), component.LevelError)
			m.refreshChrome()
			return m, nil
		}
		if msg.revision != "" {
			m.wcRevision = msg.revision
			m.showToast("updated to r"+msg.revision, component.LevelSuccess)
		} else {
			m.showToast("update complete", component.LevelSuccess)
		}
		m.clearDiff()
		m.updateStatus()
		m.updateBar()
		// A revision picked in the Log was on the page on screen, so stay there; a
		// plain update lands on HEAD, which is on the first page.
		log := m.reloadLogPage()
		if !msg.toRevision {
			m.log.GoTop()
			log = m.resetLogPaging()
		}
		return m, tea.Batch(m.reloadStatus(), log)

	case updateAvailableMsg:
		// Offer the update only when nothing else is on screen, so the prompt
		// never steals focus from an in-flight commit, confirmation, or menu.
		if !m.overlayActive() {
			m.openUpdate(msg.rel)
		}
		return m, nil

	case startupNoticeMsg:
		// A one-time notice surfaced at launch (e.g. config values reset during
		// reconciliation). It behaves like any toast: it clears on the next key.
		m.showToast(msg.text, component.LevelWarning)
		return m, nil

	case sshCheckedMsg:
		switch {
		case msg.err != nil:
			// The agent is unreachable or ssh-add is missing: there is nothing to
			// unlock and the key is required, so surface the error and quit.
			return m, m.abort("ssh-agent unavailable: " + msg.err.Error())
		case msg.loaded:
			return m, m.beginInitialLoad()
		default:
			m.openUnlock()
			return m, nil
		}

	case sshAddedMsg:
		if !m.unlocking {
			return m, nil
		}
		m.adding = false
		if msg.err != nil {
			if errors.Is(msg.err, sshagent.ErrAgentUnreachable) {
				return m, m.abort("ssh-agent unavailable: " + msg.err.Error())
			}
			m.passAttempts++
			if m.passAttempts >= maxPassphraseAttempts {
				return m, m.abort(fmt.Sprintf("SSH key not added after %d attempts; it is required for this working copy", m.passAttempts))
			}
			m.showToast(fmt.Sprintf("wrong passphrase (%d/%d) — try again", m.passAttempts, maxPassphraseAttempts), component.LevelError)
			m.passEditor.Reset()
			m.passEditor.Focus()
			return m, nil
		}
		m.showToast("SSH key added", component.LevelSuccess)
		m.closeUnlock()
		return m, m.beginInitialLoad()

	case sourceChangedMsg:
		return m, m.applySourceChange(msg)

	case uimsg.SelectedMsg:
		return m, m.handleSelection(msg)

	case uimsg.ActivatedMsg:
		// Enter on a changelist row drills into its files; enter on a directory in
		// the Changes tree or a drilled-in changelist tree collapses/expands it.
		switch msg.ID {
		case changelistsListID:
			return m, m.drillChangelist()
		case "files":
			return m, m.toggleCollapse()
		case changelistFilesID:
			return m, m.toggleClCollapse()
		case updateMenuID:
			return m, m.chooseUpdate(msg.Index)
		}
		return m, nil

	case uimsg.ViewSelectedMsg:
		if msg.ID == filesViewsID {
			m.updateBar()
			m.updateMain()
			switch msg.Name {
			case "Changes":
				return m, m.diffLoadForSelection()
			case savedDiffsViewName:
				// Re-scan on entry so diffs saved (or removed) elsewhere show up.
				return m, tea.Batch(m.reloadSavedDiffs(), m.savedDiffLoadForSelection())
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
		case diffNameEditorID:
			return m, m.submitDiffName(msg.Value)
		case sourcePathID:
			return m, m.submitSourcePath(msg.Value)
		case settingsFormID:
			return m, m.submitSettings()
		case passphraseEditorID:
			return m, m.submitUnlock(msg.Value)
		case searchBarID:
			m.commitFilter()
			return m, nil
		}
		return m, nil

	case uimsg.ConfirmMsg:
		if msg.ID == confirmModalID {
			m.closeConfirm()
			if prompt := m.updateConflictPrompt; prompt != "" {
				// The default update confirm was accepted, but the working copy
				// holds conflicts svn would silently skip: confirm once more,
				// spelling that out, before actually updating.
				m.updateConflictPrompt = ""
				m.openConfirm("Conflicts present — continue?", prompt)
				return m, nil
			}
			cmd := m.pending
			m.pending = nil
			if m.updateProgress != "" {
				// The pending command is an svn update; show the progress modal
				// until it completes (cleared in the updatedMsg handler).
				m.showUpdating()
			}
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
		case diffNameEditorID:
			m.closeDiffName()
		case sourcePathID:
			m.closeSourcePath()
		case splitDiffID:
			m.closeSplitDiff()
		case passphraseEditorID:
			// The key is required and the user declined to unlock it, so exiting is
			// the only sensible outcome; proceeding would leave a UI that cannot
			// reach the repository.
			return m, m.abort("SSH key required: passphrase entry cancelled")
		case confirmModalID:
			m.closeConfirm()
			m.pending = nil
			m.updateConflictPrompt = ""
			m.updateProgress = ""
		case updateMenuID:
			m.closeUpdate()
		case settingsFormID:
			m.closeSettings()
		case searchBarID:
			return m, m.clearFilter()
		}
		return m, nil

	case tea.KeyMsg:
		if m.aborting {
			// A fatal SSH error is on screen; any key quits so the user can retry.
			return m, tea.Quit
		}
		if m.updatingWC {
			// An svn update is running behind the progress modal; ignore keys so
			// they can't disturb the panels until it completes.
			return m, nil
		}
		if m.unlocking {
			// While the entered passphrase is being added, the input is locked so
			// stray keys can't queue another attempt or reach the panels beneath.
			if m.adding {
				return m, nil
			}
			return m, m.passEditor.Update(msg)
		}
		if m.editing {
			return m, m.editor.Update(msg)
		}
		if m.naming {
			return m, m.nameEditor.Update(msg)
		}
		if m.savingDiff {
			return m, m.diffEditor.Update(msg)
		}
		if m.retargeting {
			// Every edit re-lists the directories under the path being typed, so the
			// suggestions follow it. Scrolling the list writes the value too, so its
			// own picks are left alone.
			before := m.pathEditor.Value()
			cmd := m.pathEditor.Update(msg)
			if m.pathEditor.Value() != before && !m.pathEditor.ListFocused() {
				m.refreshSourceOptions()
			}
			return m, cmd
		}
		if m.splitting {
			// The side-by-side view owns the keyboard while open: it scrolls, esc
			// closes it (as a DismissMsg), and the key that opened it closes it too.
			if key.Matches(msg, m.keys.SplitDiff) {
				m.closeSplitDiff()
				return m, nil
			}
			return m, m.splitDiff.Update(msg)
		}
		if m.filtering {
			// The filter input owns the keyboard while open. Every edit re-runs the
			// filter live so the panel updates as the user types; enter and esc are
			// returned by the search bar as Submit/Dismiss and handled above.
			before := m.searchBar.Value()
			cmd := m.searchBar.Update(msg)
			if m.searchBar.Value() != before {
				cmd = tea.Batch(cmd, m.applyFilterLive())
			}
			return m, cmd
		}
		if m.configuring {
			// The settings editor live-previews the palette while its Theme field
			// changes, so scrolling that field re-themes the UI immediately. The
			// choice is only persisted on ctrl+s; esc reverts it via closeSettings.
			before := m.form.Value(themeFieldIndex)
			cmd := m.form.Update(msg)
			if after := m.form.Value(themeFieldIndex); after != before {
				m.previewTheme(after)
			}
			return m, cmd
		}
		if m.confirming {
			return m, m.modal.Update(msg)
		}
		if m.updating {
			// The update prompt captures every key: ↑/↓ move, enter chooses a
			// method, esc dismisses ("don't update this time").
			return m, m.updateMenu.Update(msg)
		}
		if m.helping {
			// Read-only reference: ? and esc close it, every other key is
			// swallowed.
			if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Back) {
				m.closeHelp()
				return m, nil
			}
			return m, nil
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
	if m.aborting {
		// A fatal SSH error: show it centered and wait for the quit keypress.
		return m.overlayCenter(view, m.toast.View())
	}
	if m.showingToast {
		view = m.overlayToast(view)
	}
	switch {
	case m.updatingWC:
		view = m.overlayCenter(view, m.progress.View())
	case m.unlocking:
		view = m.overlayCenter(view, m.passEditor.View())
	case m.editing:
		view = m.overlayCenter(view, m.editor.View())
	case m.naming:
		view = m.overlayCenter(view, m.nameEditor.View())
	case m.savingDiff:
		view = m.overlayCenter(view, m.diffEditor.View())
	case m.retargeting:
		view = m.overlayCenter(view, m.pathEditor.View())
	case m.splitting:
		// The side-by-side view all but fills the screen and is read rather than
		// acted on, so the layout behind it recedes to a single dim color.
		view = m.overlayCenter(layout.Dim(view, m.theme.Muted), m.splitDiff.View())
	case m.confirming:
		view = m.overlayCenter(view, m.modal.View())
	case m.updating:
		view = m.overlayCenter(view, m.updateMenu.View())
	case m.helping:
		view = m.overlayCenter(view, m.menu.View())
	case m.configuring:
		view = m.overlayCenter(view, m.form.View())
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
// over the status bar. When the command log is shown it sits below Main in the
// right column.
func (m *Model) baseView() string {
	m.panels[panelFiles].SetFooter(m.filesFooter())
	m.panels[panelLog].SetFooter(m.logFooter())
	left := lipgloss.JoinVertical(lipgloss.Left,
		m.panels[panelStatus].View(),
		m.panels[panelFiles].View(),
		m.panels[panelLog].View(),
	)
	right := m.panels[panelMain].View()
	if m.showCmdLog {
		right = lipgloss.JoinVertical(lipgloss.Left, right, m.panels[panelCmdLog].View())
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	// While a filter is being typed the search bar takes the status bar's row so
	// the panel content stays fully visible above it.
	bottom := m.bar.View()
	if m.filtering {
		bottom = m.searchBar.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, bottom)
}

// handleKey processes global keys, returning whether the key was consumed.
func (m *Model) handleKey(k tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(k, m.keys.Quit):
		return tea.Quit, true
	case key.Matches(k, m.keys.Refresh):
		m.loading = true
		m.dismissToast()
		// An explicit refresh asks for fresh data: nothing may be answered from
		// the session.
		m.session.Purge()
		m.clearDiff()
		m.refreshChrome()
		return tea.Batch(m.reloadStatus(), m.reloadLogPage(), m.reloadSavedDiffsIfShown()), true
	case key.Matches(k, m.keys.FocusNext):
		m.focusNextPanel()
		return m.afterFocusChange(), true
	case key.Matches(k, m.keys.FocusPrev):
		m.focusPrevPanel()
		return m.afterFocusChange(), true
	case key.Matches(k, m.keys.Settings):
		return m.openSettings(), true
	case key.Matches(k, m.keys.ChangeDir):
		return m.openSourcePath(), true
	case key.Matches(k, m.keys.Filter):
		return m.openFilter(), true
	case key.Matches(k, m.keys.ToggleCmdLog):
		return m.toggleCmdLog(), true
	case key.Matches(k, m.keys.SaveDiff):
		return m.saveDiff(), true
	case key.Matches(k, m.keys.SplitDiff):
		if m.focus.Index() == panelFiles {
			return m.openSplitDiff(), true
		}
		return nil, false
	case key.Matches(k, m.keys.OpenEditor):
		return m.openInEditor(), true
	case key.Matches(k, m.keys.Back):
		// esc clears the focused panel's filter when it has one; otherwise it is
		// left for the panel (e.g. to pop a changelist drill).
		if cmd, cleared := m.clearFocusedFilter(); cleared {
			return cmd, true
		}
		return nil, false
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
	case "4":
		// Focusing the command log reveals it first when hidden, since it cannot
		// be focused while off-screen.
		if !m.showCmdLog {
			m.showCmdLog = true
			m.layout()
		}
		m.focus.Focus(panelCmdLog)
		return m.afterFocusChange(), true
	case "0":
		m.focus.Focus(panelMain)
		return m.afterFocusChange(), true
	case " ":
		switch m.focus.Index() {
		case panelFiles:
			return m.stageSelected(), true
		case panelLog:
			return m.requestUpdateToRevision(), true
		}
		return nil, false
	case "n":
		if m.isSearchPanel(m.focus.Index()) && m.filters[m.focus.Index()] != "" {
			m.jumpMatch(m.focus.Index(), 1)
			return nil, true
		}
		if m.focus.Index() == panelFiles {
			return m.assignChangelist(), true
		}
		if m.focus.Index() == panelLog {
			return m.nextLogPage(), true
		}
		return nil, false
	case "p":
		if m.focus.Index() == panelLog {
			return m.prevLogPage(), true
		}
		return nil, false
	case "N":
		if m.isSearchPanel(m.focus.Index()) && m.filters[m.focus.Index()] != "" {
			m.jumpMatch(m.focus.Index(), -1)
			return nil, true
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
	case "D":
		return m.toggleDirDiff(), true
	case "U":
		return m.toggleUntracked(), true
	}
	return nil, false
}

// stageSelected acts on the current Files-panel selection: on a directory row it
// toggles staging for every file beneath that directory (stage all, then unstage
// all once everything stageable is staged), and on a file leaf it toggles that
// file's staged state (the Changes view or a drilled-in changelist). It returns
// the command that performs the change (or nil when the selection is not
// stageable).
func (m *Model) stageSelected() tea.Cmd {
	if n, items, ok := m.selectedDirectory(); ok {
		return m.stageDirectory(n, items)
	}
	act, ok := m.stageTarget()
	if !ok {
		if it, sel := m.selectedFile(); sel {
			m.showToast("can't stage "+it.Path+" ("+it.State.Code()+")", component.LevelWarning)
		}
		return nil
	}
	return stageCmd(m.client, stagedChangelist, act)
}

// stageDirectory toggles staging for the files beneath the selected directory
// row. While any file can still be staged it stages them all (adding unversioned
// files first); once nothing is left to stage, a second press removes every file
// under the directory from whatever changelist it belongs to — the staged bucket
// or a named one — mirroring how space clears a single file's changelist. A
// directory holding only clean or ignored files has nothing to do and warns
// instead of running svn.
func (m *Model) stageDirectory(n fileNode, items []svn.StatusItem) tea.Cmd {
	if acts := directoryStageActions(n, items); len(acts) > 0 {
		return stageManyCmd(m.client, stagedChangelist, acts)
	}
	if acts := directoryUnstageActions(n, items); len(acts) > 0 {
		return stageManyCmd(m.client, stagedChangelist, acts)
	}
	m.showToast("nothing to stage or unstage under "+dirLabel(n), component.LevelWarning)
	return nil
}

// directoryStageActions builds the stage actions that stage every stageable file
// beneath a directory row: an unversioned file is added and staged, an
// unassigned pending change is staged, and a file already staged or in a named
// changelist is left as it is. items is the tree's source set.
func directoryStageActions(n fileNode, items []svn.StatusItem) []stageAction {
	var acts []stageAction
	for _, it := range filesUnder(n, items) {
		switch {
		case it.State == svn.StateUnversioned:
			acts = append(acts, stageAction{path: it.Path, add: true, stage: true})
		case it.Changelist == "" && stageable(it.State):
			acts = append(acts, stageAction{path: it.Path, stage: true})
		}
	}
	return acts
}

// directoryUnstageActions builds the actions that remove every file beneath a
// directory row from its changelist — the staged bucket or a named changelist
// alike — so a directory-level toggle clears assignments the same way space does
// for a single file. Files in no changelist have nothing to remove. items is the
// tree's source set.
func directoryUnstageActions(n fileNode, items []svn.StatusItem) []stageAction {
	var acts []stageAction
	for _, it := range filesUnder(n, items) {
		if it.Changelist != "" {
			acts = append(acts, stageAction{path: it.Path, stage: false})
		}
	}
	return acts
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

// changelistTarget is one file an assign-to-changelist action moves into a named
// changelist: its path plus whether it must be `svn add`ed first (an unversioned
// file being named directly, without staging it beforehand).
type changelistTarget struct {
	path string
	add  bool // svn add first (unversioned → versioned)
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
// one is open so a status reload can keep it in sync. The files render as the
// same "/"-rooted tree as the Changes view, opening on the first file.
func (m *Model) drillChangelist() tea.Cmd {
	g, ok := m.changelists.Selected()
	if !ok {
		return nil
	}
	m.clItems = m.changelistItems(g.Name)
	m.rebuildClTree()
	if idx := firstFileIndex(m.clFiles.Items()); idx >= 0 {
		m.clFiles.SetIndex(idx)
	}
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
	return assignChangelistCmd(m.client, name, m.nameTargets)
}

// selectedFile returns the file the current Files-panel view points at: the
// Changes tree selection (only when the cursor is on a file leaf, not a
// directory row), or the selection within a drilled-in changelist. At the
// Changelists overview (a group is selected), on a directory row, or in the
// Diffs view (which lists saved patch files, not working-copy files) there is no
// single file, so ok is false.
func (m *Model) selectedFile() (svn.StatusItem, bool) {
	if m.filesViewIsDiffs() {
		return svn.StatusItem{}, false
	}
	if m.filesViewIsChangelists() {
		if m.inChangelistDrill() {
			if n, ok := m.clFiles.Selected(); ok && n.Item != nil {
				return *n.Item, true
			}
		}
		return svn.StatusItem{}, false
	}
	if n, ok := m.files.Selected(); ok && n.Item != nil {
		return *n.Item, true
	}
	return svn.StatusItem{}, false
}

// rebuildFileTree re-flattens the current status items into the Changes tree,
// honoring the remembered per-directory collapse state. The List clamps the
// cursor, so it stays in range as rows appear or disappear.
func (m *Model) rebuildFileTree() {
	m.files.SetItems(buildFileTree(m.filteredStatusItems(m.fileItems), m.collapsedDirs))
}

// focusFirstFile parks the Changes-tree cursor on the first file leaf the first
// time files appear, skipping the leading "/" root and directory rows so the
// panel opens on an actionable file (as it did before the tree). Later reloads
// leave the cursor where the user put it.
func (m *Model) focusFirstFile() {
	if m.filesInitialized {
		return
	}
	if idx := firstFileIndex(m.files.Items()); idx >= 0 {
		m.files.SetIndex(idx)
		m.filesInitialized = true
	}
}

// toggleCollapse expands or collapses the directory under the Changes-tree
// cursor and rebuilds the tree. It is inert on a file leaf or while the Files
// panel shows the Changelists view.
func (m *Model) toggleCollapse() tea.Cmd {
	if m.filesViewIsChangelists() {
		return nil
	}
	n, ok := m.files.Selected()
	if !ok || n.Item != nil {
		return nil
	}
	if m.collapsedDirs[n.Path] {
		delete(m.collapsedDirs, n.Path)
	} else {
		m.collapsedDirs[n.Path] = true
	}
	m.rebuildFileTree()
	m.updateMain()
	return nil
}

// rebuildClTree re-flattens the drilled-in changelist's items into its tree,
// honoring the drill's own per-directory collapse state.
func (m *Model) rebuildClTree() {
	m.clFiles.SetItems(buildFileTree(m.filteredStatusItems(m.clItems), m.clCollapsedDirs))
}

// toggleClCollapse expands or collapses the directory under the drilled-in
// changelist tree's cursor and rebuilds it. It is inert on a file leaf.
func (m *Model) toggleClCollapse() tea.Cmd {
	n, ok := m.clFiles.Selected()
	if !ok || n.Item != nil {
		return nil
	}
	if m.clCollapsedDirs[n.Path] {
		delete(m.clCollapsedDirs, n.Path)
	} else {
		m.clCollapsedDirs[n.Path] = true
	}
	m.rebuildClTree()
	m.updateMain()
	return nil
}

// filesViewIsChangelists reports whether the Files panel's active view is the
// Changelists view.
func (m *Model) filesViewIsChangelists() bool {
	return m.filesViews.ActiveName() == "Changelists"
}

// filesViewIsDiffs reports whether the Files panel's active view is the Diffs
// browser, which lists saved patch files rather than working-copy changes.
func (m *Model) filesViewIsDiffs() bool {
	return m.filesViews.ActiveName() == savedDiffsViewName
}

// inChangelistDrill reports whether the Changelists view is drilled into a
// changelist's file list.
func (m *Model) inChangelistDrill() bool {
	return m.filesViewIsChangelists() && m.filesViews.Depth() > 0
}

// filesFooter returns the position/count indicator inlaid into the Files panel's
// bottom border: the 1-based position within the active view, the number of
// entries shown, and — when a filter or the hide-untracked toggle hides some —
// the full unfiltered count in brackets. It counts file leaves in the Changes
// tree and a drilled-in changelist (ignoring the synthetic root and directory
// rows), changelists in the Changelists overview, and saved patch files in the
// Diffs view, following whichever view is active.
func (m *Model) filesFooter() string {
	hiding := m.hideUntracked || m.filters[panelFiles] != ""
	switch {
	case m.filesViewIsDiffs():
		shown := len(m.savedDiffs.Items())
		return countLabel(m.savedDiffs.Index()+1, shown, len(m.savedDiffItems))
	case m.inChangelistDrill():
		index, shown := fileLeafStats(m.clFiles.Items(), m.clFiles.Index())
		full := shown
		if hiding {
			full = leafCount(buildFileTree(m.clItems, m.clCollapsedDirs))
		}
		return countLabel(index, shown, full)
	case m.filesViewIsChangelists():
		shown := len(m.changelists.Items())
		full := shown
		if hiding {
			full = len(groupChangelists(m.fileItems))
		}
		return countLabel(m.changelists.Index()+1, shown, full)
	default:
		index, shown := fileLeafStats(m.files.Items(), m.files.Index())
		full := shown
		if hiding {
			full = leafCount(buildFileTree(m.fileItems, m.collapsedDirs))
		}
		return countLabel(index, shown, full)
	}
}

// countLabel formats a "position of shown" indicator for a panel's bottom
// border, appending the full count in brackets when hidden entries push it past
// the shown count. index is 1-based (0 when the cursor sits above the first
// entry, such as on the tree root); an empty, fully-unhidden view yields no
// label.
func countLabel(index, shown, full int) string {
	switch {
	case shown == 0 && full == 0:
		return ""
	case shown == 0:
		return fmt.Sprintf("0 of 0 (%d)", full)
	case full > shown:
		return fmt.Sprintf("%d of %d (%d)", index, shown, full)
	default:
		return fmt.Sprintf("%d of %d", index, shown)
	}
}

// assignChangelist opens the changelist-name prompt for the files that will move
// into a named changelist. When any files are staged (in the anonymous staged
// bucket) the whole staged set is named as a unit; otherwise it falls back to
// the single selected file. In that fallback a lone selected file already in a
// named changelist is refused (one named changelist per file — unstage it
// first), as is a state that cannot be staged. The prompt lists the existing
// named changelists to pick from.
func (m *Model) assignChangelist() tea.Cmd {
	targets := m.stagedTargets()
	if len(targets) == 0 {
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
		targets = []changelistTarget{{path: it.Path, add: it.State == svn.StateUnversioned}}
	}
	m.naming = true
	m.nameTargets = targets
	m.nameEditor.Reset()
	m.nameEditor.SetOptions("Existing changelists:", m.namedChangelists())
	m.nameEditor.Focus()
	m.sizeNameEditor()
	return nil
}

// stagedTargets collects every file currently in the anonymous staged bucket as
// changelist targets, so naming a changelist moves the whole staged set as a
// unit. Staged files are already versioned, so in practice none need an
// `svn add` first.
func (m *Model) stagedTargets() []changelistTarget {
	var targets []changelistTarget
	for _, it := range m.fileItems {
		if it.Changelist == stagedChangelist {
			targets = append(targets, changelistTarget{path: it.Path, add: it.State == svn.StateUnversioned})
		}
	}
	return targets
}

// syncDrill refreshes a drilled-in changelist after a status reload: it
// repopulates the file list from the rebuilt groups, or collapses the drill when
// that changelist no longer exists (e.g. its last file was unstaged).
func (m *Model) syncDrill() {
	if !m.filesViewIsChangelists() || m.filesViews.Depth() == 0 {
		return
	}
	if items := m.changelistItems(m.drilledCL); len(items) > 0 {
		m.clItems = items
		m.rebuildClTree()
		return
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
	for _, it := range m.fileItems {
		if it.Changelist == name {
			n++
		}
	}
	return n
}

// requestRevert asks to discard local changes to the current selection, opening a
// confirmation modal. On a directory row it reverts every dirty file beneath it;
// on a file leaf a clean/unversioned selection has nothing to revert.
func (m *Model) requestRevert() tea.Cmd {
	if n, items, ok := m.selectedDirectory(); ok {
		return m.requestRevertDirectory(n, items)
	}
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

// requestRevertDirectory asks to discard local changes to every dirty file
// beneath the selected directory row. A directory with nothing revertable warns
// instead of opening the modal.
func (m *Model) requestRevertDirectory(n fileNode, items []svn.StatusItem) tea.Cmd {
	paths := directoryRevertPaths(n, items)
	if len(paths) == 0 {
		m.showToast("nothing to revert under "+dirLabel(n), component.LevelWarning)
		return nil
	}
	m.pending = revertManyCmd(m.client, paths)
	m.openConfirm("Revert changes?", fmt.Sprintf(
		"Discard local changes to %d files under %s? This cannot be undone.", len(paths), dirLabel(n)))
	return nil
}

// directoryRevertPaths collects the revertable file paths beneath a directory
// row: every versioned pending change, matching the single-file revert guard.
func directoryRevertPaths(n fileNode, items []svn.StatusItem) []string {
	var paths []string
	for _, it := range filesUnder(n, items) {
		if it.State.IsDirty() {
			paths = append(paths, it.Path)
		}
	}
	return paths
}

// requestDelete asks to remove the current selection, opening a confirmation
// modal. On a directory row it removes every deletable file beneath it; on a file
// leaf a versioned file is scheduled for deletion, an unversioned one is removed
// from disk, and ignored files are left alone.
func (m *Model) requestDelete() tea.Cmd {
	if n, items, ok := m.selectedDirectory(); ok {
		return m.requestDeleteDirectory(n, items)
	}
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

// requestDeleteDirectory asks to remove every deletable file beneath the selected
// directory row: versioned files are scheduled for deletion and unversioned ones
// are removed from disk. Ignored files are skipped, so a directory holding only
// ignored files warns instead of opening the modal.
func (m *Model) requestDeleteDirectory(n fileNode, items []svn.StatusItem) tea.Cmd {
	acts := directoryDeleteActions(n, items)
	if len(acts) == 0 {
		m.showToast("nothing to delete under "+dirLabel(n), component.LevelWarning)
		return nil
	}
	m.pending = deleteManyCmd(m.client, acts)
	m.openConfirm("Delete files?", deleteDirectoryMessage(n, acts))
	return nil
}

// directoryDeleteActions builds the delete actions for the files beneath a
// directory row: each versioned file is scheduled for deletion (svn delete) and
// each unversioned one is removed from disk, skipping ignored files the same way
// the single-file delete does.
func directoryDeleteActions(n fileNode, items []svn.StatusItem) []deleteAction {
	var acts []deleteAction
	for _, it := range filesUnder(n, items) {
		if it.State == svn.StateIgnored {
			continue
		}
		acts = append(acts, deleteAction{path: it.Path, unversioned: it.State == svn.StateUnversioned})
	}
	return acts
}

// deleteDirectoryMessage composes the confirmation body for a directory delete,
// separating files merely scheduled for deletion from untracked files that would
// be permanently removed from disk.
func deleteDirectoryMessage(n fileNode, acts []deleteAction) string {
	var scheduled, disk int
	for _, act := range acts {
		if act.unversioned {
			disk++
		} else {
			scheduled++
		}
	}
	var parts []string
	if scheduled > 0 {
		parts = append(parts, fmt.Sprintf("%d files under %s will be scheduled for deletion (removed on the next commit)", scheduled, dirLabel(n)))
	}
	if disk > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked files will be permanently removed from disk — this cannot be undone", disk))
	}
	return strings.Join(parts, "; ") + "."
}

// requestUpdate brings the working copy up to date with the repository's latest
// revision. It confirms first — like the update-to-revision flow — and adds a
// second confirmation when the working copy already holds conflicts svn skips.
func (m *Model) requestUpdate() tea.Cmd {
	m.pending = updateCmd(m.client)
	m.updateConflictPrompt = conflictUpdatePrompt(m.conflictedPaths(), "the latest revision")
	m.updateProgress = updateProgressText(m.wcRevision, "the latest revision")
	m.openConfirm("Update working copy?", "Update the working copy to the latest revision? Uncommitted changes are kept and merged.")
	return nil
}

// requestUpdateToRevision updates the working copy to the revision selected in
// the Log panel. Because this can move the working copy backwards in history, it
// asks for confirmation first; with no revision selected it warns instead.
func (m *Model) requestUpdateToRevision() tea.Cmd {
	entry, ok := m.log.Selected()
	if !ok {
		m.showToast("no revision selected", component.LevelWarning)
		return nil
	}
	m.pending = updateToRevisionCmd(m.client, entry.Revision)
	m.updateConflictPrompt = conflictUpdatePrompt(m.conflictedPaths(), "r"+entry.Revision)
	m.updateProgress = updateProgressText(m.wcRevision, "r"+entry.Revision)
	m.openConfirm("Update to revision?", "Update the working copy to r"+entry.Revision+"? Uncommitted changes are kept and merged.")
	return nil
}

// conflictedPaths returns the working-copy paths currently in a conflicted
// state, from the last-loaded status. svn update skips these — they stay in
// conflict while everything else moves — so they drive the extra confirmation.
func (m *Model) conflictedPaths() []string {
	var paths []string
	for _, it := range m.fileItems {
		if it.State == svn.StateConflicted {
			paths = append(paths, it.Path)
		}
	}
	return paths
}

// conflictUpdatePrompt builds the additional confirmation shown before updating
// when the working copy already holds conflicts, spelling out that svn leaves
// those files untouched and updates the rest to target (e.g. "r42" or "the
// latest revision"). It returns "" when nothing is in conflict, which suppresses
// the extra step.
func conflictUpdatePrompt(conflicts []string, target string) string {
	n := len(conflicts)
	if n == 0 {
		return ""
	}
	subject := "1 file is"
	if n > 1 {
		subject = fmt.Sprintf("%d files are", n)
	}
	return fmt.Sprintf("%s already in conflict and will be left untouched; the rest of the working copy will still update to %s. Continue?", subject, target)
}

// updateProgressText builds the message shown in the progress modal while an svn
// update runs, e.g. "Updating from r38 to r50…". target is a ready-made label:
// "r50" for a specific revision, or "the latest revision" for a HEAD update whose
// exact number svn only reports on completion.
func updateProgressText(from, target string) string {
	fromLabel := "the current revision"
	if from != "" {
		fromLabel = "r" + from
	}
	return fmt.Sprintf("Updating from %s to %s…", fromLabel, target)
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

// overlayActive reports whether any modal, editor, or menu is currently on
// screen, so a background event (like the update check completing) knows not to
// steal focus.
func (m *Model) overlayActive() bool {
	return m.aborting || m.unlocking || m.editing || m.naming || m.savingDiff || m.retargeting || m.splitting || m.confirming || m.helping || m.updating || m.configuring
}

// openUpdate shows the startup update prompt for the given release as a centered
// overlay, titling it with the new version.
func (m *Model) openUpdate(rel selfupdate.Release) {
	m.updateRel = rel
	m.updating = true
	m.updateMenu.SetTitle("Update available: " + rel.Tag)
	m.updateMenu.Focus()
	m.sizeUpdateMenu()
}

// closeUpdate hides the update prompt without applying an update.
func (m *Model) closeUpdate() {
	m.updating = false
	m.updateMenu.Blur()
}

// chooseUpdate handles a selection in the update prompt. The first two items
// record the chosen method and quit so the update runs after the TUI tears down
// (a self-replacing binary cannot be updated cleanly while it is on screen); the
// last item just dismisses the prompt.
func (m *Model) chooseUpdate(index int) tea.Cmd {
	switch index {
	case 0:
		m.updateMethod = selfupdate.MethodCurl
		m.updateChosen = true
		return tea.Quit
	case 1:
		m.updateMethod = selfupdate.MethodGo
		m.updateChosen = true
		return tea.Quit
	default:
		m.closeUpdate()
		return nil
	}
}

// previewTheme applies the named palette to every component without persisting,
// so the settings editor can show each scheme live while its Theme field cycles.
// It re-themes every component, rebuilds the Files list render closures that
// captured the previous palette (so row glyph colors follow the switch), and
// refreshes derived chrome (which re-colorizes the diff via the live theme). An
// unrecognized name resolves to Auto (matching startup), so it is always safe.
func (m *Model) previewTheme(name string) {
	th, _ := theme.ByName(name)
	m.theme = th
	// Pin the color profile before re-theming so the palette (and the diff
	// re-colorized by refreshChrome) renders in the profile the theme expects.
	theme.ApplyColorProfile(name)
	for _, p := range m.panels {
		p.SetTheme(th)
	}
	m.bar.SetTheme(th)
	m.editor.SetTheme(th)
	m.nameEditor.SetTheme(th)
	m.diffEditor.SetTheme(th)
	m.pathEditor.SetTheme(th)
	m.modal.SetTheme(th)
	m.menu.SetTheme(th)
	m.updateMenu.SetTheme(th)
	m.form.SetTheme(th)
	m.toast.SetTheme(th)
	m.splitDiff.SetTheme(th)
	m.files.SetRender(renderFileNode(th))
	m.clFiles.SetRender(renderFileNode(th))
	m.changelists.SetRender(renderChangelistGroup(th))
	m.savedDiffs.SetRender(renderSavedDiff(th))
	m.refreshChrome()
}

// openSettings shows the settings editor as a centered overlay, populating it
// from the current configuration. The directory-diff field is seeded from the
// live runtime state so the form reflects what the user currently sees; the
// hide-untracked field is seeded from the persisted config instead, since its
// keybind is a session-only toggle the editor must not capture.
func (m *Model) openSettings() tea.Cmd {
	m.form.SetFields(settingsFields(m.cfg, m.dirDiff))
	// Remember the active theme so canceling reverts any live Theme-field preview.
	m.themeBefore = m.cfg.Theme
	m.configuring = true
	m.form.Focus()
	m.sizeForm()
	return nil
}

// closeSettings hides the settings editor without saving, reverting any live
// theme preview to the theme active before the editor opened. submitSettings
// re-applies the chosen theme afterward when it persists a change.
func (m *Model) closeSettings() {
	m.previewTheme(m.themeBefore)
	m.configuring = false
	m.form.Blur()
}

// submitSettings reads the edited fields back into the configuration, applies the
// changes that take effect immediately (the theme palette, the directory-diff
// default, the hide-untracked toggle and the display scope), persists the result,
// and closes the editor. A blank or non-positive log limit is ignored so the
// previous value survives; a failed save is non-fatal and surfaced as a toast.
func (m *Model) submitSettings() tea.Cmd {
	vals := m.form.Values()
	// Field order mirrors settingsFields.
	cfg := m.cfg
	if n, err := strconv.Atoi(strings.TrimSpace(vals[0])); err == nil && n > 0 {
		cfg.LogLimit = n
	}
	cfg.Editor = strings.TrimSpace(vals[1])
	if cfg.Editor == "" {
		cfg.Editor = config.Default().Editor
	}
	cfg.Theme = strings.TrimSpace(vals[2])
	cfg.DirectoryDiff = vals[3] == "true"
	cfg.HideUntracked = vals[4] == "true"
	cfg.SSHKeyPath = strings.TrimSpace(vals[5])
	if cfg.SSHKeyPath == "" {
		cfg.SSHKeyPath = config.Default().SSHKeyPath
	}
	cfg.DisplayFrom = strings.TrimSpace(vals[6])
	cfg.DiffOutputDir = strings.TrimSpace(vals[7])

	m.closeSettings()

	themeChanged := cfg.Theme != m.cfg.Theme
	untrackedChanged := cfg.HideUntracked != m.hideUntracked
	displayChanged := cfg.DisplayFrom != m.cfg.DisplayFrom
	diffDirChanged := cfg.DiffOutputDir != m.cfg.DiffOutputDir
	logLimitChanged := cfg.LogLimit != m.cfg.LogLimit
	m.cfg = cfg
	m.dirDiff = cfg.DirectoryDiff
	m.hideUntracked = cfg.HideUntracked
	if themeChanged {
		m.previewTheme(cfg.Theme)
	}
	if untrackedChanged {
		m.rebuildFilesViews()
	}
	// A new display scope re-roots every svn command, so the status and history
	// on screen no longer describe the tree being shown; load them afresh.
	var reload tea.Cmd
	if displayChanged {
		m.retargetDisplay(cfg.DisplayFrom)
		m.session.Purge()
		m.clearDiff()
		m.loading = true
		m.refreshChrome()
		reload = m.beginInitialLoad()
	}
	// A new output directory means a different set of saved diffs to browse; the
	// display scope changes it too, since a blank setting resolves to the root.
	if diffDirChanged || displayChanged {
		m.savedDiffItems, m.savedDiffsErr = nil, nil
		m.savedPath, m.savedText, m.savedErr = "", "", false
		m.rebuildSavedDiffs()
		reload = tea.Batch(reload, m.reloadSavedDiffsIfShown())
	}
	// A new page size puts different revisions on every page, so the anchors
	// reached with the old one no longer address anything.
	if logLimitChanged && !displayChanged {
		m.log.GoTop()
		reload = tea.Batch(reload, m.resetLogPaging())
	}
	if err := config.Save(m.cfg); err != nil {
		m.showToast("couldn't save settings: "+err.Error(), component.LevelWarning)
		return reload
	}
	m.showToast("settings saved", component.LevelSuccess)
	if reload != nil {
		return reload
	}
	m.updateMain()
	if m.source == sourceFiles {
		return m.diffLoadForSelection()
	}
	return nil
}

// PendingUpdate reports the update method the user chose before quitting, if
// any. The command layer runs it once the program has exited.
func (m *Model) PendingUpdate() (selfupdate.Method, bool) {
	return m.updateMethod, m.updateChosen
}

// showToast displays a transient notice; it stays until the next interaction.
func (m *Model) showToast(text string, level component.Level) {
	m.toast.Show(text, level)
	m.showingToast = true
}

// dismissToast hides the current toast.
func (m *Model) dismissToast() { m.showingToast = false }

// SetStartupNotice schedules text to appear as a transient notice as soon as the
// UI is up, used to report configuration conflicts resolved at startup. It must
// be called before the program runs; an empty or blank string is a no-op.
func (m *Model) SetStartupNotice(text string) {
	m.startupNotice = strings.TrimSpace(text)
}

// ConfigValidator returns a config.Validator that reconciles domain-specific
// settings against this build, keeping the config package free of TUI concerns.
// It resets a theme that is no longer available to the default so the new
// default takes precedence, reporting the change so it can be shown to the user.
func ConfigValidator() config.Validator {
	return func(cfg *config.Config) []string {
		name := strings.TrimSpace(cfg.Theme)
		if name == "" {
			return nil
		}
		if _, ok := theme.ByName(name); ok {
			return nil
		}
		def := config.Default().Theme
		cfg.Theme = def
		return []string{fmt.Sprintf("theme %q is no longer available; reset to %q", name, def)}
	}
}

// failureText renders an action failure for a toast. An svn authentication
// failure collapses to a short, actionable hint instead of a raw multi-line svn
// error dump.
func failureText(action string, err error) string {
	if svn.IsAuthError(err) {
		return action + " failed: " + svn.AuthHint
	}
	return action + " failed: " + err.Error()
}

// helpMenuItems is the keybindings reference shown by the "?" help menu. The
// table itself lives in keymap.HelpSections, which also feeds the website's
// keybindings page; this only lays it out as menu rows.
func helpMenuItems() []component.MenuItem {
	sections := keymap.HelpSections()
	items := make([]component.MenuItem, 0, len(sections))
	for _, s := range sections {
		items = append(items, component.MenuSection(s.Title))
		for _, b := range s.Bindings {
			items = append(items, component.MenuItem{Label: b.Action, Key: b.KeyHint()})
		}
	}
	return items
}

// updateMenuItems are the choices shown in the startup update prompt. Their
// order is load-bearing: chooseUpdate maps index 0/1/2 to curl / go / dismiss.
func updateMenuItems() []component.MenuItem {
	return []component.MenuItem{
		{Label: "Update with cURL"},
		{Label: "Update with Go"},
		{Label: "Don't update this time"},
	}
}

// themeFieldIndex is the position of the Theme field within settingsFields; the
// settings editor live-previews the palette when the field at this index
// changes, and submitSettings reads the same position back as the theme.
const themeFieldIndex = 2

// settingsFields builds the settings editor's fields from the configuration, in
// the field order submitSettings relies on. The directory-diff field is seeded
// from the live runtime state (dirDiff) rather than cfg so the form shows what
// the user currently sees. The hide-untracked field, by contrast, is seeded from
// the persisted configuration: its keybind (U) is a session-only view toggle that
// must never reach the saved config, so the editor shows and edits the global
// default independently of any runtime override. Every other field comes straight
// from the persisted configuration.
func settingsFields(cfg config.Config, dirDiff bool) []component.Field {
	return []component.Field{
		{Label: "Log limit", Kind: component.FieldInt, Value: strconv.Itoa(cfg.LogLimit)},
		{Label: "Editor", Kind: component.FieldChoice, Value: cfg.Editor, Options: config.EditorValues()},
		{Label: "Theme", Kind: component.FieldChoice, Value: cfg.Theme, Options: theme.Names()},
		{Label: "Directory diff", Kind: component.FieldBool, Value: strconv.FormatBool(dirDiff)},
		{Label: "Hide untracked", Kind: component.FieldBool, Value: strconv.FormatBool(cfg.HideUntracked)},
		{Label: "SSH key", Kind: component.FieldText, Value: cfg.SSHKeyPath},
		{Label: "Display from", Kind: component.FieldChoice, Value: cfg.DisplayFrom, Options: config.DisplayFromValues()},
		{Label: "Diff output", Kind: component.FieldText, Value: cfg.DiffOutputDir},
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

// sizeDiffEditor sizes the save-diff file-name prompt like the changelist-name
// prompt.
func (m *Model) sizeDiffEditor() {
	w := clamp(m.width/2, 30, max(m.width-6, 30))
	m.diffEditor.SetSize(w, 0)
}

// beginInitialLoad kicks off the loads the first paint depends on: the
// working-copy status, and the single log entry the HEAD indicator needs. It is
// deferred until any required svn+ssh key is unlocked, so those remote
// operations don't fail on a locked key. A page of history is not part of it —
// that follows the first status, or the first look at the Log panel.
func (m *Model) beginInitialLoad() tea.Cmd {
	m.forgetLogPaging()
	m.headRev = ""
	return tea.Batch(m.reloadStatus(), headRevisionCmd(m.client))
}

// submitUnlock adds the configured SSH key to the agent with the entered
// passphrase. It locks the input and shows a processing notice so the wait for
// ssh-add is visible and can't be interrupted; the result arrives on
// sshAddedMsg, which starts the deferred initial load on success.
func (m *Model) submitUnlock(passphrase string) tea.Cmd {
	m.adding = true
	m.passEditor.Blur()
	m.showToast("Adding SSH key…", component.LevelInfo)
	return sshAddCmd(m.cfg.SSHKeyPath, passphrase)
}

// openUnlock shows the SSH passphrase overlay so the configured key can be
// unlocked and added to the agent before the initial load runs.
func (m *Model) openUnlock() {
	m.unlocking = true
	m.adding = false
	m.passEditor.Reset()
	m.passEditor.Focus()
	m.sizeUnlock()
}

// closeUnlock hides the passphrase overlay.
func (m *Model) closeUnlock() {
	m.unlocking = false
	m.adding = false
	m.passEditor.Blur()
}

// abort tears down the passphrase overlay and shows reason plus a quit hint in a
// centered error toast; the next keypress quits. It is used for every
// unrecoverable SSH outcome — an unreachable agent, a cancelled prompt, or too
// many wrong passphrases — since the key is required to proceed.
func (m *Model) abort(reason string) tea.Cmd {
	m.aborting = true
	m.closeUnlock()
	wrapW := clamp(m.width-8, 24, 60)
	wrapped := lipgloss.NewStyle().Width(wrapW).Render(reason)
	m.toast.Show(wrapped+"\n\nPress any key to quit and try again", component.LevelError)
	return nil
}

// sizeUnlock sizes the passphrase prompt (only its width matters).
func (m *Model) sizeUnlock() {
	w := clamp(m.width/2, 30, max(m.width-6, 30))
	m.passEditor.SetSize(w, 0)
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

// showUpdating raises the progress modal for the staged update message and marks
// an update in flight; the updatedMsg handler clears it when svn returns.
func (m *Model) showUpdating() {
	m.updatingWC = true
	m.progress.SetPrompt("", m.updateProgress)
	m.progress.Focus()
	m.sizeProgress()
}

// sizeProgress widths the update-progress modal to fit its one-line message,
// capped to the screen so a narrow terminal wraps instead of overflowing.
func (m *Model) sizeProgress() {
	w := clamp(lipgloss.Width(m.updateProgress)+4, 34, max(m.width-6, 34))
	m.progress.SetSize(w, 0)
}

// sizeMenu sizes the help menu to a centered portion of the screen (only its
// width matters; the height follows the item count). It is laid out in two
// columns so the grouped keybindings fit a short terminal.
func (m *Model) sizeMenu() {
	m.menu.SetColumns(2)
	m.menu.SetSize(clamp(m.width*4/5, 60, max(m.width-6, 60)), 0)
}

// sizeUpdateMenu sizes the startup update prompt like the help menu (width
// only; the height follows the three choices).
func (m *Model) sizeUpdateMenu() {
	m.updateMenu.SetSize(clamp(m.width/2, 40, max(m.width-6, 40)), 0)
}

// sizeForm sizes the settings editor to a centered portion of the screen (only
// its width matters; the height follows the field count).
func (m *Model) sizeForm() {
	m.form.SetSize(clamp(m.width*3/5, 40, max(m.width-4, 40)), 0)
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
	case savedDiffsListID:
		if m.source == sourceFiles {
			m.updateMain()
			return m.savedDiffLoadForSelection()
		}
	case "log":
		if m.source == sourceLog {
			m.updateMain()
			return m.revisionDetailForSelection()
		}
	}
	return nil
}

// afterFocusChange updates which panel drives Main, refreshes the chrome, and
// loads a diff when Main now follows the Files panel.
func (m *Model) afterFocusChange() tea.Cmd {
	switch m.focus.Index() {
	case panelStatus:
		m.source = sourceStatus
	case panelLog:
		m.source = sourceLog
	case panelFiles:
		m.source = sourceFiles
	case panelMain, panelCmdLog:
		// Focusing Main or the command log only scrolls it; keep the current source.
	}
	// History is asked for before Main is rendered, so the first look at the Log
	// panel shows that it is loading rather than that it is empty.
	var cmd tea.Cmd
	if m.source == sourceLog {
		cmd = tea.Batch(m.ensureLogPage(), m.revisionDetailForSelection())
	}
	m.syncMainTitle()
	m.updateBar()
	m.updateMain()
	if m.source == sourceFiles {
		return m.diffLoadForSelection()
	}
	return cmd
}

// focusNextPanel and focusPrevPanel cycle focus like the focus manager but skip
// the command-log panel while it is hidden, so Tab never lands on an off-screen
// panel. It is the only hideable panel and sits at the end of the ring, so a
// single extra step is always enough to pass it.
func (m *Model) focusNextPanel() {
	m.focus.Next()
	if m.focus.Index() == panelCmdLog && !m.showCmdLog {
		m.focus.Next()
	}
}

func (m *Model) focusPrevPanel() {
	m.focus.Prev()
	if m.focus.Index() == panelCmdLog && !m.showCmdLog {
		m.focus.Prev()
	}
}

// syncMainTitle names the Main panel after the focused side panel: the Status
// panel makes it "About", the Files panel "Diff", and the Log panel "Commit
// message". Focusing Main itself leaves the heading unchanged, so it keeps
// naming whichever side panel last drove it.
func (m *Model) syncMainTitle() {
	switch m.focus.Index() {
	case panelStatus:
		m.panels[panelMain].SetTitle("About")
	case panelFiles:
		m.panels[panelMain].SetTitle("Diff")
	case panelLog:
		m.panels[panelMain].SetTitle("Commit message")
	}
}

// reloadStatus, loadDiff, reloadSavedDiffs and readSavedDiff issue the loads
// whose replies can be overtaken by a later request. Each abandons the request
// still in flight and stamps the new one, so only the reply the model is still
// waiting for is applied.
func (m *Model) reloadStatus() tea.Cmd {
	ctx, gen := m.statusGen.begin(loadTimeout)
	return loadStatusCmd(ctx, m.client, gen)
}

// loadDiff puts the diff for k on screen. One the session already holds for the
// working copy's current state is applied on the spot and costs no command; a
// miss is debounced, so passing over a row does not spawn an svn process for it.
func (m *Model) loadDiff(k diffKey) tea.Cmd {
	if e, ok := m.session.Diff(k, m.diffStamp(k)); ok {
		// Whatever is in flight was for the row just left; its reply must not
		// land on top of this one.
		m.diffGen.next()
		m.applyDiff(k, e)
		if m.source == sourceFiles {
			m.updateMain()
		}
		return nil
	}
	gen := m.diffGen.next()
	return tea.Tick(diffDebounce, func(time.Time) tea.Msg {
		return diffPendingMsg{key: k, gen: gen}
	})
}

// diffStamp fingerprints the working-copy state a diff of k would be produced
// from, as the session store's key to whether a cached diff is still good.
func (m *Model) diffStamp(k diffKey) string {
	root := ""
	if m.client != nil {
		root = m.client.Dir
	}
	return diffStampFor(root, m.wcRevision, m.fileItems, k)
}

// applyDiff puts a loaded (or cached) diff on screen, recording what it was
// produced for so a later refresh can be told from a move to another selection.
func (m *Model) applyDiff(k diffKey, e diffEntry) {
	m.diffPath, m.diffOfDir = k.path, k.dir
	m.diffText, m.diffErr = e.text, e.failed
}

// clearDiff drops the diff on screen, so Main falls back to its placeholder
// until a fresh one lands.
func (m *Model) clearDiff() {
	m.diffPath, m.diffText, m.diffErr, m.diffOfDir = "", "", false, false
}

// rederiveDiff re-reads the diff on screen from the session once the status
// behind it has moved: one the session still stands behind stays put, and one it
// dropped clears, so the load that follows fetches it afresh.
func (m *Model) rederiveDiff() {
	if m.diffPath == "" {
		return
	}
	k := diffKey{path: m.diffPath, dir: m.diffOfDir}
	if e, ok := m.session.Diff(k, m.diffStamp(k)); ok {
		m.applyDiff(k, e)
		return
	}
	m.clearDiff()
}

func (m *Model) reloadSavedDiffs() tea.Cmd {
	return loadSavedDiffsCmd(m.diffDir(), m.savedGen.next())
}

// reloadSavedDiffsIfShown re-scans the patch files only while the Diffs view is
// the one on screen. Anywhere else the scan is deferred: that view re-scans
// whenever it is opened, so the disk is never read for a list nobody is looking
// at.
func (m *Model) reloadSavedDiffsIfShown() tea.Cmd {
	if !m.filesViewIsDiffs() {
		return nil
	}
	return m.reloadSavedDiffs()
}

func (m *Model) readSavedDiff(path string) tea.Cmd {
	return readSavedDiffCmd(path, m.diffGen.next())
}

// diffLoadForSelection returns a command to load the diff that Main should show
// for the current Files selection when it is not already loaded. In the Diffs
// view that is the highlighted saved patch file; otherwise a directory row loads
// the combined diff of every change beneath it (the "/" root covers the whole
// working copy) and a file leaf loads its own diff when it is dirty.
func (m *Model) diffLoadForSelection() tea.Cmd {
	if m.filesViewIsDiffs() {
		return m.savedDiffLoadForSelection()
	}
	k, ok := m.diffSelection()
	if !ok || m.diffPath == k.path {
		// There is nothing to fetch, so a load still in flight is for a row that
		// has been left; its reply must not land on this one.
		m.diffGen.next()
		return nil
	}
	return m.loadDiff(k)
}

// diffSelection returns the diff the Files selection calls for, or ok=false when
// it calls for none: no selection, a file with no textual diff, or a directory
// row while directory diffs are off.
func (m *Model) diffSelection() (diffKey, bool) {
	if n, _, ok := m.selectedTreeNode(); ok && n.Item == nil {
		if !m.dirDiff {
			return diffKey{}, false
		}
		return diffKey{path: n.Path, dir: true}, true
	}
	it, ok := m.selectedFile()
	if !ok || !it.State.IsDirty() {
		return diffKey{}, false
	}
	return diffKey{path: it.Path}, true
}

// toggleDirDiff flips whether directory rows show their combined diff. It lets a
// working copy that disables directory diffs globally (config) reveal one on
// demand, and hide it again. It reports the new state with a toast and, when Main
// follows the Files panel, refreshes it — loading the diff if it now needs one.
func (m *Model) toggleDirDiff() tea.Cmd {
	m.dirDiff = !m.dirDiff
	if m.dirDiff {
		m.showToast("directory diff on", component.LevelInfo)
	} else {
		m.showToast("directory diff off", component.LevelInfo)
	}
	if m.source != sourceFiles {
		return nil
	}
	m.updateMain()
	return m.diffLoadForSelection()
}

// toggleUntracked flips whether untracked (unversioned) files are hidden from the
// Changes and diff views. It lets a working copy that shows untracked files
// globally hide the noise on demand, and reveal it again. It rebuilds the Files
// views so the change takes effect immediately, reports the new state with a
// toast and, when Main follows the Files panel, refreshes it — the cursor may
// have moved onto a different file as rows appeared or disappeared.
func (m *Model) toggleUntracked() tea.Cmd {
	m.hideUntracked = !m.hideUntracked
	if m.hideUntracked {
		m.showToast("untracked files hidden", component.LevelInfo)
	} else {
		m.showToast("untracked files shown", component.LevelInfo)
	}
	m.rebuildFilesViews()
	if m.source != sourceFiles {
		return nil
	}
	m.updateMain()
	return m.diffLoadForSelection()
}

// toggleCmdLog flips the command-log panel shown below Main, which mirrors the
// svn actions revision runs. It re-lays out the right column so Main reclaims
// the space when the log is hidden, moves focus to Main if the log was focused
// as it hides, and reports the new state with a toast.
func (m *Model) toggleCmdLog() tea.Cmd {
	m.showCmdLog = !m.showCmdLog
	m.layout()
	var cmd tea.Cmd
	if !m.showCmdLog && m.focus.Index() == panelCmdLog {
		m.focus.Focus(panelMain)
		cmd = m.afterFocusChange()
	}
	if m.showCmdLog {
		m.showToast("command log shown", component.LevelInfo)
	} else {
		m.showToast("command log hidden", component.LevelInfo)
	}
	return cmd
}

// rebuildFilesViews re-flattens every Files-panel view — the Changes tree, the
// Changelists overview, a drilled-in changelist and the saved-diffs browser —
// from the current status items. It is the shared refresh used whenever what
// those views should show changes without a status reload (a filter edit or the
// untracked toggle).
func (m *Model) rebuildFilesViews() {
	m.rebuildFileTree()
	m.rebuildChangelists()
	m.rebuildSavedDiffs()
	if m.inChangelistDrill() {
		m.rebuildClTree()
	}
}

// openFilter opens the filter input for the focused panel, pre-filled with that
// panel's current filter and labeled with the panel name and its available
// parameters. The input captures the keyboard until the user submits (enter,
// keep) or dismisses (esc, clear).
func (m *Model) openFilter() tea.Cmd {
	p := m.focus.Index()
	m.filtering = true
	m.filterPanel = p
	m.searchBar.SetPrefix(filterPrefix(p))
	m.searchBar.SetValue(m.filters[p])
	m.searchBar.SetSize(m.width, 1)
	m.searchBar.Focus()
	return nil
}

// applyFilterLive re-applies the in-progress query to the panel being filtered,
// so it updates as the user types. It returns a command to refresh the Main
// panel when it follows the filtered panel, whose narrowed selection may now
// point at a different row.
func (m *Model) applyFilterLive() tea.Cmd {
	m.setFilter(m.filterPanel, m.searchBar.Value())
	return m.afterFilterChange(m.filterPanel)
}

// afterFilterChange refreshes Main when it is driven by the panel whose filter
// just changed, since narrowing the list clamps the cursor onto a different
// selection. It returns a command to load the newly-selected file's diff when
// one is needed.
func (m *Model) afterFilterChange(p int) tea.Cmd {
	switch p {
	case panelFiles:
		if m.source == sourceFiles {
			m.updateMain()
			return m.diffLoadForSelection()
		}
	case panelLog:
		if m.source == sourceLog {
			m.updateMain()
		}
	}
	return nil
}

// commitFilter closes the filter input, keeping the filter that was applied live
// while typing. For a search panel with no matches it surfaces a toast so the
// user is not left wondering why nothing is highlighted.
func (m *Model) commitFilter() {
	m.filtering = false
	m.searchBar.Blur()
	if q := m.filters[m.filterPanel]; q != "" && m.isSearchPanel(m.filterPanel) && m.searchViewport(m.filterPanel).MatchCount() == 0 {
		m.showToast("no matches for "+q, component.LevelInfo)
	}
	m.updateBar()
}

// clearFilter closes the filter input and removes the filter from the panel it
// was editing (esc while the input is open), returning a command to refresh Main
// when it follows that panel.
func (m *Model) clearFilter() tea.Cmd {
	m.filtering = false
	m.searchBar.Blur()
	p := m.filterPanel
	m.setFilter(p, "")
	m.updateBar()
	return m.afterFilterChange(p)
}

// clearFocusedFilter removes the focused panel's filter when it has one (esc
// while no input is open), returning a command to refresh Main and whether a
// filter was cleared — so the caller can leave esc for the panel (e.g. to pop a
// changelist drill) when there was none.
func (m *Model) clearFocusedFilter() (tea.Cmd, bool) {
	p := m.focus.Index()
	if m.filters[p] == "" {
		return nil, false
	}
	m.setFilter(p, "")
	m.updateBar()
	return m.afterFilterChange(p), true
}

// setFilter records (or clears, when q is blank) the filter for panel p and
// re-renders that panel from its unfiltered source. The Files and Log panels
// filter (rows are removed); the Main and Status viewports search (matching lines
// are highlighted and jumped between, never removed).
func (m *Model) setFilter(p int, q string) {
	if strings.TrimSpace(q) == "" {
		delete(m.filters, p)
	} else {
		m.filters[p] = q
	}
	switch p {
	case panelFiles:
		m.rebuildFilesViews()
	case panelLog:
		m.applyLogFilter()
	case panelStatus:
		m.status.SetSearch(m.filters[panelStatus])
	case panelMain:
		m.main.SetSearch(m.filters[panelMain])
	}
}

// isSearchPanel reports whether panel p is a Viewport that searches (highlights +
// jumps) rather than filters (removes rows).
func (m *Model) isSearchPanel(p int) bool {
	return p == panelMain || p == panelStatus
}

// searchViewport returns the Viewport backing a search panel.
func (m *Model) searchViewport(p int) *component.Viewport {
	if p == panelStatus {
		return m.status
	}
	return m.main
}

// jumpMatch moves a search panel's viewport to its next (dir > 0) or previous
// match and refreshes the footer position; with no matches it explains why with
// a toast.
func (m *Model) jumpMatch(p, dir int) {
	vp := m.searchViewport(p)
	if vp.MatchCount() == 0 {
		m.showToast("no matches for "+m.filters[p], component.LevelInfo)
		return
	}
	if dir < 0 {
		vp.PrevMatch()
	} else {
		vp.NextMatch()
	}
	m.updateBar()
}

// filterPrefix is the muted label shown in the filter input for panel p, naming
// the panel, its behavior (filter vs. search) and its available parameters.
func filterPrefix(p int) string {
	switch p {
	case panelFiles:
		return "filter files (state: cl:)"
	case panelLog:
		return "filter log (rev: user: path: date:)"
	case panelStatus:
		return "search status"
	default:
		return "search main"
	}
}

// filesQuery is the parsed Files-panel filter.
func (m *Model) filesQuery() filterQuery {
	return parseFilter(m.filters[panelFiles], fileFilterKeys)
}

// filteredStatusItems returns the subset of items shown in the Files views: the
// ones matching the Files-panel filter, with untracked (unversioned) files
// dropped while the hide-untracked toggle is on. Items are returned unchanged
// when no filter is set and untracked files are shown.
func (m *Model) filteredStatusItems(items []svn.StatusItem) []svn.StatusItem {
	q := m.filesQuery()
	if q.empty() && !m.hideUntracked {
		return items
	}
	out := make([]svn.StatusItem, 0, len(items))
	for _, it := range items {
		if m.hideUntracked && it.State == svn.StateUnversioned {
			continue
		}
		if matchStatusItem(it, q) {
			out = append(out, it)
		}
	}
	return out
}

// changelistItems returns every working-copy file in the named changelist from
// the full (unfiltered) status set, so a drill snapshot stays independent of any
// Files filter currently narrowing the view.
func (m *Model) changelistItems(name string) []svn.StatusItem {
	var items []svn.StatusItem
	for _, it := range m.fileItems {
		if it.Changelist == name {
			items = append(items, it)
		}
	}
	return items
}

// rebuildChangelists repopulates the Changelists overview from the filtered
// status items.
func (m *Model) rebuildChangelists() {
	m.changelists.SetItems(groupChangelists(m.filteredStatusItems(m.fileItems)))
}

// applyLogFilter repopulates the Log table from the raw revision history under
// the Log-panel filter.
func (m *Model) applyLogFilter() {
	m.log.SetItems(m.filteredLogEntries())
}

// filteredLogEntries returns the revisions matching the Log-panel filter, or all
// of them when no filter is set. The filter only ever sees the page on screen,
// since that is all the Log panel holds.
func (m *Model) filteredLogEntries() []svn.LogEntry {
	q := parseFilter(m.filters[panelLog], logFilterKeys)
	if q.empty() {
		return m.logEntries
	}
	out := make([]svn.LogEntry, 0, len(m.logEntries))
	for _, e := range m.logEntries {
		if matchLogEntry(e, q) {
			out = append(out, e)
		}
	}
	return out
}

// loadLogPage fetches the given 1-based page of history. The anchor it needs was
// recorded when the preceding page loaded, so only a page that has been reached
// before — or the first — can be requested. A page the session already holds is
// rendered on the spot and costs no command, so paging back over ground already
// covered is instant.
func (m *Model) loadLogPage(page int) tea.Cmd {
	m.logPage = page
	m.logRequested = true
	k := logKey{anchor: m.logAnchor(page), limit: m.cfg.LogLimit}
	if p, ok := m.session.LogPage(k); ok {
		// Whatever is in flight was for the page just left; its reply must not
		// land on top of this one.
		m.logGen.next()
		m.logLoading = false
		m.applyLogPage(p, nil)
		return m.revisionDetailForSelection()
	}
	// The rows of the page being left stay on screen, dimmed, until the new page
	// lands, so the panel never blanks mid-turn.
	m.logLoading = true
	ctx, gen := m.logGen.begin(loadTimeout)
	return loadLogCmd(ctx, m.client, k.anchor, page, m.cfg.LogLimit, gen)
}

// logAnchor returns the revision the given 1-based page starts after, recorded
// when the page before it loaded. The first page needs none.
func (m *Model) logAnchor(page int) string {
	if page <= 1 || page-2 >= len(m.logAnchors) {
		return ""
	}
	return m.logAnchors[page-2]
}

// applyLogPage puts a page of history — freshly loaded or served from the
// session — on screen, refreshing the Status panel and bar with the HEAD the
// first page reveals.
func (m *Model) applyLogPage(p historyPage, err error) {
	m.logErr = err
	m.logEntries = p.entries
	m.logMore = p.more
	m.recordLogAnchor()
	m.applyLogFilter()
	if m.logPage == 1 && len(p.entries) > 0 {
		m.headRev = p.entries[0].Revision
	}
	m.updateStatus()
	m.updateBar()
	if m.source == sourceLog {
		m.updateMain()
	}
}

// ensureLogPage fetches the page of history the Log panel needs, once. It is
// called both when the first status lands and when the panel is first looked at,
// whichever comes first; the second caller finds the page already asked for and
// does nothing.
func (m *Model) ensureLogPage() tea.Cmd {
	if m.logRequested {
		return nil
	}
	return m.loadLogPage(m.logPage)
}

// resetLogPaging returns the Log panel to the first page and reloads it, for
// when what is being logged changes.
func (m *Model) resetLogPaging() tea.Cmd {
	m.forgetLogPaging()
	return m.loadLogPage(1)
}

// forgetLogPaging drops everything the old history was addressed by: the page on
// screen, the anchors reached from it and the pages the session held.
func (m *Model) forgetLogPaging() {
	m.logAnchors = nil
	m.logMore = false
	m.logPage = 1
	m.logRequested = false
	m.session.PurgeLogPages()
}

// recordLogAnchor remembers the revision the page on screen ends on, which is
// what the page after it must be anchored to.
func (m *Model) recordLogAnchor() {
	if !m.logMore || len(m.logEntries) == 0 {
		return
	}
	last := m.logEntries[len(m.logEntries)-1].Revision
	for len(m.logAnchors) < m.logPage {
		m.logAnchors = append(m.logAnchors, "")
	}
	m.logAnchors[m.logPage-1] = last
}

// reloadLogPage refetches the page on screen, keeping the cursor where it is. It
// is an explicit refresh, so the cached pages are dropped rather than served.
func (m *Model) reloadLogPage() tea.Cmd {
	m.session.PurgeLogPages()
	return m.loadLogPage(m.logPage)
}

// nextLogPage and prevLogPage turn the Log panel a page at a time. Turning to an
// unrelated set of revisions starts at the top, unlike a refetch of the same
// page, which keeps the selection.
func (m *Model) nextLogPage() tea.Cmd {
	if !m.logMore {
		m.showToast("no older revisions", component.LevelWarning)
		return nil
	}
	m.log.GoTop()
	return m.loadLogPage(m.logPage + 1)
}

func (m *Model) prevLogPage() tea.Cmd {
	if m.logPage <= 1 {
		m.showToast("already on the first page", component.LevelWarning)
		return nil
	}
	m.log.GoTop()
	return m.loadLogPage(m.logPage - 1)
}

// logFooter is the Log panel's position indicator: where the cursor sits within
// the page, how many revisions the page holds, and the page number.
func (m *Model) logFooter() string {
	label := countLabel(m.log.Index()+1, len(m.log.Items()), len(m.logEntries))
	if label == "" {
		return ""
	}
	return label + " · " + strconv.Itoa(m.logPage)
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

	// Tall enough for the Status panel's six rows plus its border.
	statusHeight := clamp(8, 3, max(bodyHeight-6, 3))
	rest := bodyHeight - statusHeight
	filesHeight := rest / 2
	logHeight := rest - filesHeight

	m.panels[panelStatus].SetSize(leftWidth, statusHeight)
	m.panels[panelFiles].SetSize(leftWidth, filesHeight)
	m.panels[panelLog].SetSize(leftWidth, logHeight)

	// The right column is Main alone, or Main above the command log when it is
	// shown; the split keeps the two columns the same overall height.
	mainHeight := bodyHeight
	if m.showCmdLog {
		cmdLogHeight := clamp(bodyHeight/4, 6, 12)
		if cmdLogHeight > bodyHeight-3 {
			cmdLogHeight = max(bodyHeight-3, 0)
		}
		mainHeight = bodyHeight - cmdLogHeight
		m.panels[panelCmdLog].SetSize(rightWidth, cmdLogHeight)
	}
	m.panels[panelMain].SetSize(rightWidth, mainHeight)
	m.bar.SetSize(m.width, barHeight)
	m.searchBar.SetSize(m.width, barHeight)
	m.updateMain()
}

// refreshChrome recomputes the derived content in the Status panel, Main panel
// and status bar.
func (m *Model) refreshChrome() {
	m.updateStatus()
	m.updateMain()
	m.updateBar()
}

// updateStatus fills the Status panel with the working copy's locations and
// revision state: the working-copy root, the source path revision operates on,
// the current working directory, the branch it is checked out from, and the
// checked-out and HEAD revision numbers.
// A value that is not yet known is omitted so the panel only lists facts.
func (m *Model) updateStatus() {
	lines := make([]string, 0, 6)
	bold := lipgloss.NewStyle().Bold(true)
	add := func(label, value string) {
		if value != "" {
			lines = append(lines, bold.Render(fmt.Sprintf("%-8s", label))+"  "+value)
		}
	}
	if m.info != nil {
		add("Root", m.info.WorkingCopyRoot)
	}
	if m.client != nil {
		add("Source", m.client.Dir)
	}
	add("CWD", m.workDir)
	if m.info != nil {
		add("Branch", m.info.Branch())
	}
	if m.wcRevision != "" {
		add("Revision", "r"+m.wcRevision)
	}
	if head := m.headRevision(); head != "" {
		add("HEAD", "r"+head)
	}
	m.status.SetContent(strings.Join(lines, "\n"))
}

// headRevision returns the repository's latest revision: read at startup by the
// one-entry HEAD probe, and refreshed whenever the first page of history lands.
// It is empty until that probe answers.
func (m *Model) headRevision() string { return m.headRev }

// updateMain fills the Main panel from whichever side panel currently drives it.
// The Main and Status panels are searched (not filtered): any active search
// re-highlights against the new content inside the Viewport, so nothing is set
// here beyond the content itself.
//
// Content identical to what is displayed is not written at all, and a refresh of
// the same selection keeps the reader's scroll position; only a move to another
// selection starts back at the top.
func (m *Model) updateMain() {
	// Only a unified diff carries the one-column +/-/space marker that must stay
	// pinned while the body scrolls horizontally; mainContent sets the gutter for
	// that case, and this baseline clears it for every other view.
	m.main.SetGutter(0)
	content := m.mainContent()
	key := m.mainSelectionKey()
	switch {
	case key == m.mainKey && content == m.mainText:
		return
	case key == m.mainKey:
		m.main.SetContentPreservingScroll(content)
	default:
		m.main.SetContent(content)
	}
	m.mainKey, m.mainText = key, content
}

// mainSelectionKey identifies what Main is showing — the driving panel and the
// row selected in it — so a reload of the same subject can be told from a move
// to a different one.
func (m *Model) mainSelectionKey() string {
	switch m.source {
	case sourceStatus:
		return "status"
	case sourceLog:
		e, _ := m.log.Selected()
		return "log:" + e.Revision
	}
	switch {
	case m.filesViewIsDiffs():
		d, _ := m.savedDiffs.Selected()
		return "saved:" + d.Path
	case m.filesViewIsChangelists() && !m.inChangelistDrill():
		g, _ := m.changelists.Selected()
		return "changelist:" + g.Name
	}
	n, _, _ := m.selectedTreeNode()
	return "files:" + n.Path
}

// mainContent computes the raw Main text for the current state, setting the diff
// gutter as a side effect when it renders a unified diff.
func (m *Model) mainContent() string {
	if m.source == sourceStatus {
		return m.statusDetail()
	}
	// The Diffs view browses patch files on local disk, so it stays readable
	// while the working copy is still loading or has failed to load.
	if m.source != sourceFiles || !m.filesViewIsDiffs() {
		switch {
		case m.err != nil:
			return "Error: " + m.err.Error() + "\n\nPress R to retry."
		case m.loading && len(m.fileItems) == 0:
			return "Loading working-copy status…"
		}
	}
	if m.source == sourceLog {
		return m.logDetail()
	}
	if m.filesShowDiff() {
		m.main.SetGutter(1)
	}
	return m.filesMain()
}

// filesMain renders the Main content for the Files panel, which depends on its
// active view: the Diffs browser shows the highlighted saved patch file, the
// Changelists overview shows a changelist summary, a directory row in the
// Changes tree shows the combined diff beneath it, and everything else (a file
// in the Changes tree or a drilled-in changelist) shows the selected file.
func (m *Model) filesMain() string {
	if m.filesViewIsDiffs() {
		return m.savedDiffDetail()
	}
	if m.filesViewIsChangelists() && !m.inChangelistDrill() {
		return m.changelistDetail()
	}
	if n, _, ok := m.selectedTreeNode(); ok && n.Item == nil {
		return m.directoryDetail(n)
	}
	return m.fileDetail()
}

// selectedTreeNode returns the tree row under the active Files-panel cursor —
// from the Changes tree, or a drilled-in changelist tree — together with the
// item set that tree was built from (used to stage a directory's files). It
// reports ok=false at the Changelists overview, where the selection is a
// changelist group rather than a tree row, and in the Diffs view, where it is a
// saved patch file.
func (m *Model) selectedTreeNode() (fileNode, []svn.StatusItem, bool) {
	if m.filesViewIsDiffs() {
		return fileNode{}, nil, false
	}
	if m.filesViewIsChangelists() {
		if m.inChangelistDrill() {
			n, ok := m.clFiles.Selected()
			return n, m.clItems, ok
		}
		return fileNode{}, nil, false
	}
	n, ok := m.files.Selected()
	return n, m.fileItems, ok
}

// selectedDirectory returns the highlighted Files-panel row when it is a
// directory row (Item == nil), along with the status items backing its view, so
// directory-level actions can fan out over filesUnder. ok is false on a file leaf
// or the Changelists overview.
func (m *Model) selectedDirectory() (fileNode, []svn.StatusItem, bool) {
	if n, items, ok := m.selectedTreeNode(); ok && n.Item == nil {
		return n, items, true
	}
	return fileNode{}, nil, false
}

// filesShowDiff reports whether filesMain currently renders a unified diff — the
// only Main view with a +/-/space gutter to pin. It mirrors the diff branches of
// savedDiffDetail, directoryDetail and fileDetail: the Files panel is showing a
// saved patch file that has been read, or working-copy files (not the
// Changelists overview) whose selected directory row, or dirty file leaf, has a
// non-empty, freshly-loaded diff.
func (m *Model) filesShowDiff() bool {
	if m.filesViewIsDiffs() {
		d, ok := m.savedDiffs.Selected()
		return ok && !m.savedErr && m.savedPath == d.Path && strings.TrimSpace(m.savedText) != ""
	}
	if m.filesViewIsChangelists() && !m.inChangelistDrill() {
		return false
	}
	if n, _, ok := m.selectedTreeNode(); ok && n.Item == nil {
		return m.dirDiff && m.diffPath == n.Path && strings.TrimSpace(m.diffText) != ""
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

// directoryDetail renders the combined diff of every change beneath a selected
// directory row (the "/" root covers the whole working copy). It mirrors
// fileDetail: a placeholder shows while the diff loads or when the directory has
// no textual changes. When directory diffs are toggled off it shows only a hint
// naming the key that reveals the diff.
func (m *Model) directoryDetail(n fileNode) string {
	if !m.dirDiff {
		return "(directory diff off — press " + m.keys.ToggleDirDiff.Help().Key + " to show it)"
	}
	switch {
	case m.diffPath != n.Path:
		return "Loading diff…"
	case strings.TrimSpace(m.diffText) == "":
		return "(no textual changes under this directory)"
	default:
		return colorizeDiff(m.theme, m.diffText)
	}
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
// revision. The changed paths cost their own `svn log --verbose`, so they are
// filled in from the session when that load has landed and the rest of the
// detail is shown without waiting for it.
func (m *Model) logDetail() string {
	entry, ok := m.log.Selected()
	if !ok {
		switch {
		case m.logLoading:
			return "Loading history…"
		case m.logErr != nil:
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
	if paths, ok := m.session.RevDetail(entry.Revision); ok && len(paths) > 0 {
		lines = append(lines, "", "Changed paths:")
		for _, p := range paths {
			lines = append(lines, fmt.Sprintf("  %s %s", p.Action, p.Path))
		}
	}
	return strings.Join(lines, "\n")
}

// revisionDetailForSelection returns a command to read the changed paths of the
// selected revision. Revisions are immutable, so one the session already holds
// costs no command; a miss is debounced like a diff, so walking the Log panel
// does not spawn an svn process for every row passed over.
func (m *Model) revisionDetailForSelection() tea.Cmd {
	// Whatever is in flight was for the row just left; its reply is of no use
	// now, so it is dropped rather than rendered.
	gen := m.revGen.next()
	entry, ok := m.log.Selected()
	if !ok || entry.Revision == "" {
		return nil
	}
	if _, held := m.session.RevDetail(entry.Revision); held {
		return nil
	}
	rev := entry.Revision
	return tea.Tick(diffDebounce, func(time.Time) tea.Msg {
		return revisionPendingMsg{rev: rev, gen: gen}
	})
}

// updateBar sets the contextual key hints and the right-aligned load state.
func (m *Model) updateBar() {
	m.bar.SetHints(m.barHints())

	switch {
	case m.err != nil:
		m.bar.SetRight("error")
	case m.loading:
		m.bar.SetRight("loading…")
	default:
		m.bar.SetRight("")
	}
}

// barHints returns the key hints for the focused panel — and, for the Files
// panel, its active view — in the order the bar should drop them when they do
// not all fit. Only keys that act on that panel are listed; the global ones live
// in the help menu, so a panel that only scrolls (the command log) gets no hints
// and leaves the bar empty. When the focused panel has an active filter or
// search (and the input is closed) it is described first, so the user can see
// it, jump between search matches, and clear it.
func (m *Model) barHints() []string {
	p := m.focus.Index()
	hints := m.panelHints(p)
	if len(hints) == 0 {
		return nil
	}
	hints = append(hints, "? help")
	if q := m.filters[p]; q != "" && !m.filtering {
		if m.isSearchPanel(p) {
			return append(m.searchHints(p, q), hints...)
		}
		return append([]string{"filter: " + q, "esc clear"}, hints...)
	}
	return hints
}

// panelHints are the keys panel p responds to, most useful first.
func (m *Model) panelHints(p int) []string {
	switch p {
	case panelStatus:
		// The Status panel is where the source path is shown, so it is also where
		// the key that changes it is worth advertising.
		return []string{"/ search", "P source path"}
	case panelMain:
		return []string{"/ search"}
	case panelFiles:
		return m.filesHints()
	case panelLog:
		return []string{"space update to rev", "n/p page", "c commit", "/ filter"}
	}
	// The command log only scrolls; it has no actions of its own.
	return nil
}

// filesHints are the Files panel's keys for its active view: the saved-diff
// browser, a drilled-in changelist, the Changelists overview, or the Changes
// tree.
func (m *Model) filesHints() []string {
	switch {
	case m.filesViewIsDiffs():
		return []string{"e open", "/ filter", "[ ] view"}
	case m.inChangelistDrill():
		return []string{"space unstage", "c commit", "esc back", "[ ] view"}
	case m.filesViewIsChangelists():
		return []string{"enter expand", "n name", "c commit", "[ ] view"}
	}
	return []string{"space stage", "n changelist", "c commit", "r revert", "d delete", "[ ] view"}
}

// searchHints describe the active search on a Viewport panel: the query, the
// current match position (or that there are none), and the jump/clear keys.
func (m *Model) searchHints(p int, q string) []string {
	vp := m.searchViewport(p)
	if vp.MatchCount() == 0 {
		return []string{"search: " + q, "no matches", "esc clear"}
	}
	where := fmt.Sprintf("%d matches", vp.MatchCount())
	if pos := vp.CurrentMatch(); pos > 0 {
		where = fmt.Sprintf("%d/%d", pos, vp.MatchCount())
	}
	return []string{"search: " + q, where, "n next", "N prev", "esc clear"}
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
