package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/focus"
	"github.com/bapatchirag/revision/internal/tui/keymap"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// Model is the root Bubble Tea model. It composes reusable components into the
// lazygit layout: a left column (Status + Files) beside a Main viewport, over a
// contextual status bar.
type Model struct {
	client *svn.Client
	info   *svn.Info

	theme theme.Theme
	// themeName is the palette currently applied, which the persisted config's
	// name does not track while the settings editor previews another one.
	themeName string
	keys      keymap.KeyMap
	cfg       config.Config

	status      *component.Viewport
	files       *component.List[fileNode]
	changelists *component.List[changelistGroup]
	clFiles     *component.List[fileNode]
	savedDiffs  *component.List[savedDiff]
	rejects     *component.List[rejectNode]
	filesViews  *component.Views
	log         *component.Table[svn.LogEntry]
	main        *component.Viewport

	// cmdLogView displays cmdLog: the svn commands revision runs and their
	// output, shown in a toggleable panel below Main.
	cmdLogView *component.Viewport
	cmdLog     *commandLog
	cmdLogSeen int64

	panels      []*component.Panel
	bar         *component.StatusBar
	editor      *component.TextArea
	nameEditor  *component.Prompt
	diffEditor  *component.Prompt
	pathEditor  *component.Prompt
	repoEditor  *component.Prompt
	passEditor  *component.Prompt
	modal       *component.Modal
	progress    *component.Modal
	menu        *component.Menu
	updateMenu  *component.Menu
	form        *component.Form
	rulesEditor *component.EditList
	toast       *component.Toast
	searchBar   *component.SearchBar
	splitDiff   *component.SplitView
	mergeView   *component.SplitView
	focus       *focus.Manager

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

	// rejectItems is every reject file found beneath the source path, before the
	// Files-panel filter narrows the Rejects view; rejectPath and rejectText are
	// the file whose contents Main is showing for that view, and rejectCollapsed
	// the directories folded shut in its tree.
	rejectItems     []rejectFile
	rejectsErr      error
	rejectPath      string
	rejectText      string
	rejectErr       bool
	rejectCollapsed map[string]bool

	source   mainSource
	diffPath string
	diffText string
	// diffOfDir records that the diff on screen was produced for a directory row
	// rather than a file, so it can be matched to its cache entry after the fact.
	diffOfDir bool
	// session caches what is reusable for the life of the process — diffs, so
	// far. It is in-memory only and is emptied by Close.
	session *sessionStore
	// optimistic holds the status svn last reported while a change already shown
	// on screen is still in flight, so a failure can put it back; optimisticTok
	// stamps each such change so a reply can be matched to the state it belongs to.
	optimistic    *optimisticState
	optimisticTok uint64
	// gens stamps the loads whose replies can be overtaken by a later request, so
	// a superseded reply is dropped instead of rendered.
	gens loadGens
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
	// liveRefresh is whether the working copy is being watched in the background.
	// It is seeded from the configuration and toggled at runtime, kept apart from
	// cfg so a session-only toggle is never persisted.
	liveRefresh bool
	// watchGen stamps the live-refresh poller, so a tick from one that has been
	// stopped or restarted is dropped rather than acted on. watchTrackedFP and
	// watchFullFP are the last fingerprints taken at each depth, watchFullDue when
	// the next full scan is affordable and watchScanOff that the working copy is
	// too large for one at all; watchEvery is the interval the poller is running
	// at (backed off after a failure), watchQueued a change seen while the screen
	// was busy, and watchFailed that the failure has already been reported.
	watchGen       uint64
	watchTrackedFP string
	watchFullFP    string
	watchFullDue   time.Time
	watchScanOff   bool
	watchEvery     time.Duration
	watchQueued    bool
	watchFailed    bool
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
	// switchingRepo is true while the prompt that re-points revision at another
	// working copy is open, with repos the checkouts its list was built from.
	// scanningRepos is true while the walk that finds them is still running.
	switchingRepo bool
	scanningRepos bool
	repos         []string
	// splitting is true while the side-by-side view of the on-screen diff is
	// floated over the layout.
	splitting bool
	// merging is true while the resolution overlay is up, with mergeDoc the file
	// it is deciding: a conflicted file, or a reject against its target.
	merging     bool
	mergeDoc    *mergeDoc
	filtering   bool
	filterPanel int
	filters     map[int]string
	nameTargets []changelistTarget
	drilledCL   string
	commitCL    string
	themeBefore string
	confirming  bool
	helping     bool
	configuring bool
	// editingRules is true while the hide-rules editor is open over the settings
	// editor, with rulesDraft the rules it is editing. The draft only reaches the
	// configuration when the settings editor is saved.
	editingRules bool
	rulesDraft   []config.HideRule
	needsSSHKey  bool
	unlocking    bool
	adding       bool
	aborting     bool
	passAttempts int
	pending      tea.Cmd
	// pendingOps marks the paths a revert, delete or commit is running on, so
	// those rows read as in flight rather than done; pendingHold is what the
	// confirmation prompt on screen will mark once accepted, and pendingTok stamps
	// each action so its reply clears only its own rows.
	pendingOps  map[string]pendingOp
	pendingHold *pendingHold
	pendingTok  uint64
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
	// deferredUpdate holds a release the prompt could not be shown for yet. The
	// check runs once per session, so a dropped one would never come back.
	deferredUpdate *selfupdate.Release

	width   int
	height  int
	loading bool
	err     error
	// lastClick is the left click a following one is judged against, for
	// spotting a double click.
	lastClick clickAt
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
	// The pending lookup reads through m, which is assigned below before any row
	// is rendered, so a row marked in flight dims without a rebuild.
	pending := func(n fileNode) int { return m.pendingCount(n) }
	files := component.NewList[fileNode]("files", renderFileNode(th, pending), th, keys)
	changelists := component.NewList[changelistGroup](changelistsListID, renderChangelistGroup(th), th, keys)
	clFiles := component.NewList[fileNode](changelistFilesID, renderFileNode(th, pending), th, keys)
	savedDiffs := component.NewList[savedDiff](savedDiffsListID, renderSavedDiff(th), th, keys)
	rejects := component.NewList[rejectNode](rejectsListID, renderRejectNode(th), th, keys)
	filesViews := component.NewViews(filesViewsID, []component.View{
		{Name: "Changes", Content: files},
		{Name: "Changelists", Content: changelists},
		{Name: savedDiffsViewName, Content: savedDiffs},
		{Name: rejectsViewName, Content: rejects},
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
		themeName:       cfg.Theme,
		keys:            keys,
		cfg:             cfg,
		status:          status,
		files:           files,
		changelists:     changelists,
		clFiles:         clFiles,
		savedDiffs:      savedDiffs,
		rejects:         rejects,
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
		repoEditor:      component.NewPrompt(repoSwitchID, "Switch repository", "e.g. /path/to/working-copy", th, keys),
		passEditor:      component.NewPrompt(passphraseEditorID, "SSH key passphrase", "passphrase for "+cfg.SSHKeyPath, th, keys),
		modal:           component.NewModal(confirmModalID, "", "", th, keys),
		progress:        component.NewModal("update-progress", "", "", th, keys),
		menu:            component.NewMenu(helpMenuID, "Keybindings", helpMenuItems(), th, keys),
		updateMenu:      component.NewMenu(updateMenuID, "Update available", updateMenuItems(), th, keys),
		form:            component.NewForm(settingsFormID, "Settings", settingsFields(cfg, cfg.DirectoryDiff), th, keys),
		rulesEditor:     component.NewEditList(hideRulesEditorID, "Hide rules", "No rules yet — press a to add one.", th, keys),
		toast:           component.NewToast(th),
		searchBar:       component.NewSearchBar(searchBarID, th, keys),
		splitDiff:       component.NewSplitView(splitDiffID, "Side-by-side diff", th, keys),
		mergeView:       component.NewSplitView(mergeViewID, "Resolve", th, keys),
		collapsedDirs:   map[string]bool{},
		clCollapsedDirs: map[string]bool{},
		rejectCollapsed: map[string]bool{},
		filters:         map[int]string{},
		pendingOps:      map[string]pendingOp{},
		session:         newSessionStore(),
		source:          sourceFiles,
		dirDiff:         cfg.DirectoryDiff,
		hideUntracked:   cfg.HideUntracked,
		liveRefresh:     cfg.LiveRefresh,
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
	m.gens.stopAll()
	m.stopWatch()
	m.session.Close()
	m.clearDiff()
	m.mainKey, m.mainText = "", ""
	m.savedPath, m.savedText, m.savedErr = "", "", false
	m.rejectPath, m.rejectText, m.rejectErr = "", "", false
	m.diffSrc = diffSource{}
	m.fileItems, m.clItems, m.logEntries, m.savedDiffItems = nil, nil, nil, nil
	m.rejectItems = nil
	m.dropOptimistic()
	m.pendingOps, m.pendingHold = nil, nil
	m.cmdLog.clear()
}

// PendingUpdate reports the update method the user chose before quitting and
// the release it was chosen for, if any. The command layer runs it once the
// program has exited.
func (m *Model) PendingUpdate() (selfupdate.Method, selfupdate.Release, bool) {
	return m.updateMethod, m.updateRel, m.updateChosen
}

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

// beginInitialLoad kicks off the loads the first paint depends on: the
// working-copy status, and the single log entry the HEAD indicator needs. It is
// deferred until any required svn+ssh key is unlocked, so those remote
// operations don't fail on a locked key. A page of history is not part of it —
// that follows the first status, or the first look at the Log panel.
func (m *Model) beginInitialLoad() tea.Cmd {
	m.forgetLogPaging()
	m.headRev = ""
	return tea.Batch(m.reloadStatus(), headRevisionCmd(m.client), m.startWatch())
}
