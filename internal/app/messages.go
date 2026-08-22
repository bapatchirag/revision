package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/shelf"
	"github.com/bapatchirag/revision/internal/sshagent"
	"github.com/bapatchirag/revision/internal/svn"
)

// loadTimeout caps how long a status, diff or history load may run before it is
// abandoned.
const loadTimeout = 30 * time.Second

// loadGen stamps one class of async load so a reply overtaken by a later request
// can be dropped instead of rendered. Each new request bumps the generation and
// abandons the one still in flight; a reply is only applied when it carries the
// current stamp.
type loadGen struct {
	gen    uint64
	cancel context.CancelFunc
}

// next abandons the request in flight — whatever it returns is now superseded —
// and returns the stamp the new request's reply must carry. It is for loads that
// run no svn command and so have no context to cancel.
func (g *loadGen) next() uint64 {
	g.stop()
	g.gen++
	return g.gen
}

// stop abandons the request in flight without opening a new generation, for
// shutdown, where nothing is waiting for a reply.
func (g *loadGen) stop() {
	if g.cancel != nil {
		g.cancel()
		g.cancel = nil
	}
}

// begin starts a new generation, returning the context the request runs under
// and the stamp its reply must carry. The cancel is retained so the request
// after it can abandon this one.
func (g *loadGen) begin(timeout time.Duration) (context.Context, uint64) {
	gen := g.next()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	g.cancel = cancel
	return ctx, gen
}

// stale reports whether a reply stamped gen has been superseded. The zero stamp
// marks an unstamped reply — every load the model issues is stamped from 1 up —
// and is always applied.
func (g *loadGen) stale(gen uint64) bool { return gen != 0 && gen != g.gen }

// loadGens is every class of load the model stamps. diff covers every source
// feeding Main — the working copy, the saved-patch reader and the reject reader
// — since only one of them is on screen; saved and reject stamp the two
// directory scans.
type loadGens struct {
	diff    loadGen
	status  loadGen
	log     loadGen
	rev     loadGen
	revDiff loadGen
	saved   loadGen
	reject  loadGen
	shelf   loadGen
	repos   loadGen
}

// stopAll abandons every load in flight, releasing the context each holds. A
// generation added above is abandoned at shutdown by listing it here.
func (g *loadGens) stopAll() {
	for _, l := range []*loadGen{&g.diff, &g.status, &g.log, &g.rev, &g.revDiff, &g.saved, &g.reject, &g.shelf, &g.repos} {
		l.stop()
	}
}

// superseded reports whether ctx was cancelled by a later request for the same
// data, in which case its reply is dropped. A deadline that simply expired is a
// real failure and still surfaces.
func superseded(ctx context.Context) bool { return errors.Is(ctx.Err(), context.Canceled) }

// statusLoadedMsg carries the result of a successful status refresh.
type statusLoadedMsg struct {
	items []svn.StatusItem
	gen   uint64
}

// errMsg carries an error to surface in the UI.
type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

// diffLoadedMsg carries the result of loading a single file's diff. dir marks a
// diff produced for a directory row — spanning every change beneath it — rather
// than for one file, so the two can be cached apart.
type diffLoadedMsg struct {
	path string
	dir  bool
	diff string
	err  error
	gen  uint64
}

// diffPendingMsg fires once the cursor has rested on a selection for
// diffDebounce, at which point its diff is worth asking svn for. A stamp that
// has been superseded means the cursor moved on, so the load never runs.
type diffPendingMsg struct {
	key diffKey
	gen uint64
}

// diffSavedMsg carries the result of writing a diff to disk, along with the
// destination it was written to (or attempted).
type diffSavedMsg struct {
	path string
	err  error
}

// savedDiffsLoadedMsg carries the patch files found in the configured diff
// output directory, for the Files panel's Diffs view.
type savedDiffsLoadedMsg struct {
	files []savedDiff
	err   error
	gen   uint64
}

// savedDiffDeletedMsg carries the result of removing a saved patch file from the
// diff output directory, along with the file it was asked to remove: path to
// match against the contents on screen, name to report.
type savedDiffDeletedMsg struct {
	path string
	name string
	err  error
}

// savedDiffReadMsg carries the contents of a saved patch file, keyed by the path
// it was read from so the current selection can match it.
type savedDiffReadMsg struct {
	path string
	text string
	err  error
	gen  uint64
}

// rejectsLoadedMsg carries the reject files found beneath the source path, for
// the Files panel's Rejects view.
type rejectsLoadedMsg struct {
	files []rejectFile
	err   error
	gen   uint64
}

// shelvesLoadedMsg carries the change sets found in the working copy's shelf
// store, for the Shelf panel.
type shelvesLoadedMsg struct {
	entries []shelf.Entry
	err     error
	gen     uint64
}

// shelfReadMsg carries the patch a shelved change set holds, keyed by the entry
// it was read from so the current selection can match it.
type shelfReadMsg struct {
	id   string
	text string
	err  error
	gen  uint64
}

// shelveRequest is one capture: the changes to take, what to call them, and the
// revision the working copy was at when they were taken.
type shelveRequest struct {
	name    string
	baseRev string
	items   []svn.StatusItem
}

// shelvedMsg carries the result of taking a set of changes out of the working
// copy. left names what stayed behind because no patch could carry it; entry is
// filled whenever the shelf was written, including when clearing the working
// copy afterwards is what failed.
type shelvedMsg struct {
	entry shelf.Entry
	left  []string
	err   error
}

// shelveAllMsg is what accepting the shelve-everything confirmation sends, so
// the name prompt only opens once the whole working copy has been agreed to.
type shelveAllMsg struct{}

// shelfRestoredMsg carries the result of putting a shelved change set back. res
// describes what svn did with the patch's targets; restored and blocked are the
// unversioned files copied back and the ones whose path was already taken;
// dropped reports that the entry was popped off the shelf afterwards.
type shelfRestoredMsg struct {
	name     string
	res      svn.PatchResult
	restored []string
	blocked  []string
	dropped  bool
	err      error
}

// shelfDroppedMsg carries the result of removing a shelved change set, named as
// the panel showed it.
type shelfDroppedMsg struct {
	id   string
	name string
	err  error
}

// shelfRenamedMsg carries the result of relabelling a shelved change set.
type shelfRenamedMsg struct {
	name string
	err  error
}

// rejectDeletedMsg carries the result of removing a reject file, along with the
// file it was asked to remove: path to match against the contents on screen,
// name to report.
type rejectDeletedMsg struct {
	path string
	name string
	err  error
}

// rejectReadMsg carries the contents of a reject file, keyed by the path it was
// read from so the current selection can match it.
type rejectReadMsg struct {
	path string
	text string
	err  error
	gen  uint64
}

// patchAppliedMsg carries the result of applying a saved patch file to the
// working copy, named as the Diffs view shows it. res describes what svn did
// with the patch's targets; it is empty when the patch was refused before it
// could be applied.
type patchAppliedMsg struct {
	name string
	res  svn.PatchResult
	err  error
}

// mergeLoadedMsg carries a file read for resolution — a conflicted file, or a
// reject paired with the file it was written for — as the decisions it needs.
// rel names the file on screen, so a read that failed can still be reported.
type mergeLoadedMsg struct {
	doc *mergeDoc
	rel string
	err error
}

// mergeWrittenMsg carries the result of writing a resolved file back out and
// clearing what marked it as needing resolution: `svn resolve` for a conflict,
// removing the reject for a reject. aux is the reject that was cleared.
type mergeWrittenMsg struct {
	kind  mergeKind
	rel   string
	aux   string
	count int
	err   error
}

// editedMsg carries the result of opening a file in the configured editor. name
// is the file as it is shown on screen. detached is true when the editor was
// handed the file and left running outside the terminal, so nothing can have
// been saved yet and the working copy need not be re-read.
type editedMsg struct {
	name     string
	detached bool
	err      error
}

// logLoadedMsg carries the result of a `svn log` load. page is the 1-based page
// it was requested for, so a response overtaken by a later page turn can be
// discarded; more reports whether a further page exists.
type logLoadedMsg struct {
	entries []svn.LogEntry
	page    int
	more    bool
	err     error
	gen     uint64
}

// headLoadedMsg carries the repository's newest revision from the one-entry log
// read at startup, which is all the HEAD indicator needs.
type headLoadedMsg struct {
	rev string
	err error
}

// revisionDetailMsg carries the changed paths of one revision, loaded on demand
// because `svn log --verbose` over a whole page is the most expensive call
// revision makes.
type revisionDetailMsg struct {
	rev   string
	paths []svn.ChangedPath
	err   error
	gen   uint64
}

// revisionPendingMsg fires once the cursor has rested on a revision for
// diffDebounce, at which point its changed paths are worth asking svn for. A
// stamp that has been superseded means the cursor moved on, so the load never
// runs.
type revisionPendingMsg struct {
	rev string
	gen uint64
}

// revDiffLoadedMsg carries the diff of a range of history. Errors ride on the
// message rather than tearing down the UI: a range svn will not diff — one
// reaching back past the directory being displayed, say — is a dead end for that
// pair alone.
type revDiffLoadedMsg struct {
	rng  revRange
	diff string
	err  error
	gen  uint64
}

// stagedMsg carries the result of staging or unstaging, for one path or for a
// set of them: outcome names what landed and what svn refused. token identifies
// the optimistic change the model applied ahead of this reply, so a failure can
// be undone; it is zero when the change was not shown in advance.
type stagedMsg struct {
	outcome    batchOutcome
	changelist string // non-empty when a named changelist was assigned
	token      uint64
}

// addedMsg carries the result of putting paths under version control, for one
// path or for a set of them: outcome names what svn took and what it refused.
// token identifies the optimistic change the model applied ahead of this reply,
// so a failure can be undone; it is zero when the change was not shown in
// advance.
type addedMsg struct {
	outcome batchOutcome
	token   uint64
}

// committedMsg carries the result of a commit. token identifies the rows the
// model marked as in flight when the commit was dispatched, so the reply clears
// exactly those; it is zero when nothing was marked.
type committedMsg struct {
	revision string
	token    uint64
	err      error
}

// revertedMsg carries the result of reverting, for one path or for a set of
// them: outcome names what was discarded and what svn refused. token identifies
// the rows marked as in flight for it, as on committedMsg.
type revertedMsg struct {
	outcome batchOutcome
	token   uint64
}

// deletedMsg carries the result of deleting, for one path or for a set of them:
// outcome names what went and what would not. token identifies the rows marked
// as in flight for it, as on committedMsg.
type deletedMsg struct {
	outcome batchOutcome
	token   uint64
}

// updatedMsg carries the result of an `svn update`. toRevision distinguishes an
// update to a revision picked in the Log — which was necessarily on the page on
// screen, so the Log stays there — from a plain update to HEAD, which lands on
// the first page.
type updatedMsg struct {
	revision   string
	toRevision bool
	err        error
}

// updateAvailableMsg is emitted when the startup check finds a newer release
// than the running binary. It is only produced on release builds.
type updateAvailableMsg struct {
	rel selfupdate.Release
}

// startupNoticeMsg carries a one-time message to display as a toast once the UI
// is up. It is used to surface configuration conflicts resolved at startup.
type startupNoticeMsg struct{ text string }

// sshCheckedMsg carries whether the configured SSH key is already loaded in the
// agent. A non-nil err means the agent could not be reached (or ssh-add is
// missing), so there is nothing to unlock.
type sshCheckedMsg struct {
	loaded bool
	err    error
}

// sshAddedMsg carries the result of adding the SSH key to the agent.
type sshAddedMsg struct{ err error }

// sourceOrigin records which prompt asked for a source probe, so the reply can
// name what it changed and hand a rejected directory back to the prompt it was
// typed into.
type sourceOrigin int

const (
	fromSourcePath sourceOrigin = iota
	fromRepoSwitch
)

// label names what the probed directory becomes, for the toast confirming it.
func (o sourceOrigin) label() string {
	if o == fromRepoSwitch {
		return "repository"
	}
	return "source path"
}

// sourceChangedMsg carries the result of probing a candidate source directory:
// the client rooted at it and the working-copy info read from there, or the
// error that rules the directory out.
type sourceChangedMsg struct {
	client *svn.Client
	info   *svn.Info
	from   sourceOrigin
	err    error
}

// probeSourceCmd asks svn, off the UI goroutine, whether client's directory is a
// working copy, so the session is only re-rooted on a directory known to be one.
func probeSourceCmd(client *svn.Client, from sourceOrigin) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		info, err := client.Info(ctx)
		return sourceChangedMsg{client: client, info: info, from: from, err: err}
	}
}

// reposFoundMsg carries the working copies a scan turned up.
type reposFoundMsg struct {
	repos []string
	gen   uint64
}

// scanReposCmd walks roots off the UI goroutine, so the switch-repository prompt
// opens at once and fills in when the scan lands.
func scanReposCmd(ctx context.Context, roots []string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		return reposFoundMsg{repos: discoverRepos(ctx, roots), gen: gen}
	}
}

// startupNoticeCmd emits a startupNoticeMsg so a launch-time notice is shown
// through the normal toast path once the program is running.
func startupNoticeCmd(text string) tea.Cmd {
	return func() tea.Msg { return startupNoticeMsg{text: text} }
}

// sshCheckCmd checks, off the UI goroutine, whether the configured SSH key is
// already loaded in the running ssh-agent.
func sshCheckCmd(keyPath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		loaded, err := sshagent.KeyLoaded(ctx, keyPath)
		return sshCheckedMsg{loaded: loaded, err: err}
	}
}

// sshAddCmd adds the configured SSH key to the agent, off the UI goroutine,
// using the entered passphrase to decrypt it.
func sshAddCmd(keyPath, passphrase string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return sshAddedMsg{err: sshagent.AddKey(ctx, keyPath, passphrase)}
	}
}

// checkUpdateCmd asks GitHub, off the UI goroutine, whether a newer release
// exists. It emits updateAvailableMsg only when one does; a development build,
// an up-to-date binary, or any network/parse failure yields no message so the
// check can never disrupt startup. The answer is remembered between launches,
// so starting revision repeatedly does not repeatedly call the API.
func checkUpdateCmd(build selfupdate.Build) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		rel, newer, err := selfupdate.CheckCached(ctx, build, updateCheckPath())
		if err != nil || !newer {
			return nil
		}
		return updateAvailableMsg{rel: rel}
	}
}

// updateCheckPath is where the startup check remembers its last answer. An
// unresolvable config directory yields "", which turns the memo off rather than
// failing the check.
func updateCheckPath() string {
	dir, err := config.Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "update-check.json")
}

// loadStatusCmd runs `svn status` off the UI goroutine and reports the result.
// A run abandoned in favour of a later status load reports nothing, so a
// cancellation the model asked for never surfaces as a failure.
func loadStatusCmd(ctx context.Context, client *svn.Client, gen uint64) tea.Cmd {
	return func() tea.Msg {
		items, err := client.Status(ctx)
		if superseded(ctx) {
			return nil
		}
		if err != nil {
			return errMsg{err}
		}
		return statusLoadedMsg{items: items, gen: gen}
	}
}

// loadDiffCmd runs `svn diff <path>` off the UI goroutine. A file path diffs that
// file; a directory path diffs every change beneath it. The synthetic "/" root
// (fileTreeRoot) maps to the empty path so it diffs the whole working copy, while
// the message stays keyed by the original path so the current selection can match
// it. Diff failures are carried on the message rather than promoted to a fatal
// error so a single undiffable path never tears down the UI.
func loadDiffCmd(ctx context.Context, client *svn.Client, k diffKey, gen uint64) tea.Cmd {
	target := k.path
	if target == fileTreeRoot {
		target = ""
	}
	return func() tea.Msg {
		diff, err := client.Diff(ctx, target)
		return diffLoadedMsg{path: k.path, dir: k.dir, diff: diff, err: err, gen: gen}
	}
}

// diffDirPerm and diffFilePerm are the permissions saved diffs are created with:
// the standard user-owned rwx/rw a shell redirect would produce, since a diff of
// a working copy the user can already read holds nothing more sensitive.
const (
	diffDirPerm  = 0o755
	diffFilePerm = 0o644
)

// saveDiffCmd generates the diff of the given paths and writes it into dir as
// name, off the UI goroutine. An empty path set diffs the whole working copy.
// The run is marked as a user action, so — unlike the diffs revision loads to
// fill Main — it is reported in the command log.
func saveDiffCmd(client *svn.Client, paths []string, dir, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(svn.WithUserAction(context.Background()), 60*time.Second)
		defer cancel()
		diff, err := client.DiffPaths(ctx, paths)
		if err != nil {
			return diffSavedMsg{path: filepath.Join(dir, name), err: err}
		}
		return writeDiff(dir, name, diff)
	}
}

// writeDiffCmd writes a patch already in hand into dir as name, off the UI
// goroutine. Unlike saveDiffCmd it runs no svn command, so nothing about it
// reaches the command log — there is no invocation behind it to report.
func writeDiffCmd(diff, dir, name string) tea.Cmd {
	return func() tea.Msg { return writeDiff(dir, name, diff) }
}

// loadSavedDiffsCmd lists the patch files already saved in dir, off the UI
// goroutine, for the Diffs view to browse.
func loadSavedDiffsCmd(dir string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		files, err := scanSavedDiffs(dir)
		return savedDiffsLoadedMsg{files: files, err: err, gen: gen}
	}
}

// loadShelvesCmd lists the change sets in the working copy's shelf store, off
// the UI goroutine, for the Shelf panel to browse.
func loadShelvesCmd(dir string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		entries, err := shelf.Scan(dir)
		return shelvesLoadedMsg{entries: entries, err: err, gen: gen}
	}
}

// readShelfCmd reads a shelved change set's patch off the UI goroutine so it can
// be shown in Main. A read failure is carried on the message rather than
// promoted to a fatal error, so an unreadable entry never tears down the UI.
func readShelfCmd(dir, id string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		text, err := shelf.ReadPatch(dir, id)
		return shelfReadMsg{id: id, text: text, err: err, gen: gen}
	}
}

// shelveCmd takes a set of changes out of the working copy, off the UI
// goroutine.
func shelveCmd(client *svn.Client, dir string, req shelveRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(svn.WithUserAction(context.Background()), batchTimeout(len(req.items)))
		defer cancel()
		entry, left, err := captureShelf(ctx, client, dir, req)
		return shelvedMsg{entry: entry, left: left, err: err}
	}
}

// captureShelf writes a set of changes into the shelf store and then clears them
// from the working copy.
//
// The order is the whole safety of the feature: the patch is produced and the
// entry written to disk *before* anything is reverted, so a capture that fails
// part way through leaves the working copy as it was rather than with the
// changes gone and nowhere left to find them.
//
// What svn will not put in a patch is left alone rather than lost: a binary
// file's content never reaches a diff, an unversioned directory or symlink has
// no bytes of its own to copy, and a file svn refuses to diff at all is dropped
// from the capture rather than allowed to sink it. None of those are reverted or
// removed, and all are named in the returned list so the caller can say what
// stayed behind.
func captureShelf(ctx context.Context, client *svn.Client, dir string, req shelveRequest) (shelf.Entry, []string, error) {
	var versioned, untracked []svn.StatusItem
	for _, it := range req.items {
		if it.State == svn.StateUnversioned {
			untracked = append(untracked, it)
			continue
		}
		versioned = append(versioned, it)
	}

	var patch string
	var undiffable []string
	if len(versioned) > 0 {
		var err error
		if patch, undiffable, err = patchFor(ctx, client, itemPaths(versioned)); err != nil {
			return shelf.Entry{}, nil, err
		}
	}

	skip := make(map[string]bool, len(undiffable))
	for _, p := range undiffable {
		skip[p] = true
	}
	binaries := svn.BinarySkips(patch)
	for _, p := range binaries {
		skip[p] = true
	}
	left := append(append([]string{}, binaries...), undiffable...)

	entry := shelf.Entry{Name: req.name, BaseRevision: req.baseRev, SkippedBinary: binaries}
	for _, it := range versioned {
		if skip[it.Path] {
			continue
		}
		entry.Files = append(entry.Files, shelf.FileRec{
			Path:       it.Path,
			State:      string(it.State),
			Changelist: it.Changelist,
		})
	}

	var payloads []shelf.Payload
	for _, it := range untracked {
		src := filepath.Join(client.Dir, it.Path)
		if info, err := os.Lstat(src); err != nil || !info.Mode().IsRegular() {
			left = append(left, it.Path)
			continue
		}
		payloads = append(payloads, shelf.Payload{Rel: it.Path, Src: src})
	}

	if len(entry.Files) == 0 && len(payloads) == 0 {
		return shelf.Entry{}, left, errors.New("nothing here can be put in a patch")
	}

	saved, err := shelf.Save(dir, entry, patch, payloads)
	if err != nil {
		return shelf.Entry{}, left, err
	}

	// The entry is on disk from here, so anything that fails below has left the
	// changes recoverable rather than lost.
	return saved, left, clearWorkingCopy(ctx, client, versioned, payloads, skip)
}

// patchFor produces the patch a shelf entry carries, and names the paths that
// could not go into it.
//
// svn diffs its targets in one pass and gives up on all of them at the first it
// cannot read, so a single unreadable file would otherwise cost the whole
// capture. When that happens the paths are asked for one at a time and the ones
// svn still refuses are dropped — a per-file diff concatenates into a patch just
// as svn would have written it. A failure that is about the working copy rather
// than any one file is passed straight back: nothing here would fare better.
func patchFor(ctx context.Context, client *svn.Client, paths []string) (string, []string, error) {
	patch, err := client.DiffPathsForPatching(ctx, paths)
	if err == nil {
		return patch, nil, nil
	}
	if svn.BlocksEveryPath(err) {
		return "", nil, err
	}
	var b strings.Builder
	var dropped []string
	for _, p := range paths {
		one, oneErr := client.DiffPathsForPatching(ctx, []string{p})
		if oneErr != nil {
			dropped = append(dropped, p)
			continue
		}
		b.WriteString(one)
	}
	if len(dropped) == len(paths) {
		return "", nil, err
	}
	return b.String(), dropped, nil
}

// clearWorkingCopy takes the captured changes back out of the working copy, once
// the shelf entry holding them is safely on disk.
//
// Every part of it is attempted. A file the revert would not discard no longer
// stops the unversioned payloads from being cleared, and a payload that will not
// go no longer hides the rest — what did not come away is named instead, all of
// it, since those files are what the working copy still holds.
func clearWorkingCopy(
	ctx context.Context, client *svn.Client,
	versioned []svn.StatusItem, payloads []shelf.Payload, skip map[string]bool,
) error {
	var revert []string
	for _, it := range versioned {
		if !skip[it.Path] {
			revert = append(revert, it.Path)
		}
	}
	out := revertOutcome(client.RevertPaths(ctx, revert))
	reverted := make(map[string]bool, len(out.done))
	for _, p := range out.done {
		reverted[p] = true
	}

	var stuck []string
	for _, it := range versioned {
		// A revert un-schedules an add but leaves the file where it was.
		if it.State == svn.StateAdded && reverted[it.Path] {
			if err := client.RemoveUnversioned(it.Path); err != nil {
				stuck = append(stuck, it.Path)
			}
		}
	}
	for _, p := range payloads {
		if err := client.RemoveUnversioned(p.Rel); err != nil {
			stuck = append(stuck, p.Rel)
		}
	}

	var parts []string
	if err := out.err(); err != nil {
		parts = append(parts, err.Error())
	}
	if len(stuck) > 0 {
		parts = append(parts, "could not clear "+strings.Join(stuck, ", "))
	}
	if len(parts) == 0 {
		return nil
	}
	return errors.New(strings.Join(parts, "; "))
}

// readSavedDiffCmd reads a saved patch file off the UI goroutine so it can be
// shown in Main. A read failure is carried on the message rather than promoted
// to a fatal error, so an unreadable file never tears down the UI.
func readSavedDiffCmd(path string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		b, err := os.ReadFile(path)
		return savedDiffReadMsg{path: path, text: string(b), err: err, gen: gen}
	}
}

// restoreShelfCmd puts a shelved change set back into the working copy, off the
// UI goroutine, and pops the entry off the shelf when pop is set.
//
// The patch goes through the same gate a saved one does — was it taken from
// here, and does svn say any of it would land — before anything is written. A
// patch that does not pass does not stop the unversioned files from going back:
// the two halves of an entry are independent, and one that svn will not apply
// says nothing about files it never sees. Only a restore that came back whole
// drops the entry: while a hunk is still sitting in a .rej file, or an
// unversioned file could not be put back, the shelf is the only remaining copy
// of what did not make it.
func restoreShelfCmd(client *svn.Client, dir string, e shelf.Entry, pop bool) tea.Cmd {
	return func() tea.Msg {
		name := shelfLabel(e)
		res, patchErr := applyShelfPatch(client, dir, e)
		restored, blocked, err := shelf.Restore(dir, e.ID, client.Dir)
		msg := shelfRestoredMsg{name: name, res: res, restored: restored, blocked: blocked}
		if msg.err = errors.Join(patchErr, err); msg.err != nil {
			return msg
		}

		ctx, cancel := context.WithTimeout(svn.WithUserAction(context.Background()), batchTimeout(len(e.Files)))
		defer cancel()
		if msg.err = replayChangelists(ctx, client, e.Files); msg.err != nil {
			return msg
		}

		if pop && len(res.Conflicted) == 0 && len(res.Skipped) == 0 && len(blocked) == 0 {
			if err := shelf.Drop(dir, e.ID); err != nil {
				msg.err = err
				return msg
			}
			msg.dropped = true
		}
		return msg
	}
}

// applyShelfPatch runs an entry's patch against the working copy. An entry that
// carries only unversioned files has no patch to apply, which is not a failure.
func applyShelfPatch(client *svn.Client, dir string, e shelf.Entry) (svn.PatchResult, error) {
	path, err := shelf.PatchPath(dir, e.ID)
	if err != nil {
		return svn.PatchResult{}, err
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return svn.PatchResult{}, err
	}
	if strings.TrimSpace(string(text)) == "" {
		return svn.PatchResult{}, nil
	}
	if !svn.PatchBelongsTo(string(text), client.Dir) {
		return svn.PatchResult{}, fmt.Errorf(
			"none of the files it changes are in %s — it was shelved from another directory", client.Dir)
	}
	ctx, cancel := context.WithTimeout(svn.WithUserAction(context.Background()), 60*time.Second)
	defer cancel()
	dry, err := client.Patch(ctx, path, true)
	if err != nil {
		return svn.PatchResult{}, err
	}
	if err := patchTrialErr(dry); err != nil {
		return svn.PatchResult{}, err
	}
	return client.Patch(ctx, path, false)
}

// replayChangelists puts the restored files back into the changelists they were
// shelved from, which no patch records. A file the patch did not bring back —
// one shelved as a deletion, or one svn could not place — is passed over rather
// than named to svn, which would only reject a path that is not there. Every
// other file is attempted, so one changelist svn will not assign does not leave
// the files after it unassigned as well.
func replayChangelists(ctx context.Context, client *svn.Client, files []shelf.FileRec) error {
	var out batchOutcome
	for _, f := range files {
		if f.Changelist == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(client.Dir, filepath.FromSlash(f.Path))); err != nil {
			continue
		}
		out.add(f.Path, client.AddToChangelist(ctx, f.Changelist, f.Path))
	}
	return out.err()
}

// dropShelfCmd removes a shelved change set off the UI goroutine. The store is
// files on disk, not working-copy state, so nothing svn knows about is involved.
func dropShelfCmd(dir, id, name string) tea.Cmd {
	return func() tea.Msg {
		return shelfDroppedMsg{id: id, name: name, err: shelf.Drop(dir, id)}
	}
}

// renameShelfCmd relabels a shelved change set off the UI goroutine, leaving
// everything it captured untouched.
func renameShelfCmd(dir, id, name string) tea.Cmd {
	return func() tea.Msg {
		return shelfRenamedMsg{name: name, err: shelf.Rename(dir, id, name)}
	}
}

// deleteSavedDiffCmd removes a saved patch file from the diff output directory
// off the UI goroutine. The store is plain files on disk, not working-copy
// state, so this is an os.Remove rather than anything svn knows about.
func deleteSavedDiffCmd(path, name string) tea.Cmd {
	return func() tea.Msg {
		return savedDiffDeletedMsg{path: path, name: name, err: os.Remove(path)}
	}
}

// loadRejectsCmd walks dir for the rejects a patch left behind, off the UI
// goroutine, for the Rejects view to browse.
func loadRejectsCmd(dir string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		files, err := scanRejects(dir)
		return rejectsLoadedMsg{files: files, err: err, gen: gen}
	}
}

// readRejectCmd reads a reject file off the UI goroutine so it can be shown in
// Main. A read failure is carried on the message rather than promoted to a fatal
// error, so an unreadable file never tears down the UI.
func readRejectCmd(path string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		b, err := os.ReadFile(path)
		return rejectReadMsg{path: path, text: string(b), err: err, gen: gen}
	}
}

// deleteRejectCmd removes a reject file off the UI goroutine. svn ignores
// rejects, so like the saved-diff store this is an os.Remove rather than
// anything svn knows about.
func deleteRejectCmd(path, name string) tea.Cmd {
	return func() tea.Msg {
		return rejectDeletedMsg{path: path, name: name, err: os.Remove(path)}
	}
}

// applyPatchCmd applies a saved patch file to the working copy rooted at dir,
// off the UI goroutine. Nothing is changed until two questions are answered:
// whether the patch was taken from dir at all, and whether svn says any of it
// would land there. A patch that passes both is applied for whatever it is
// worth — svn takes the hunks that fit and writes the rest out beside their
// targets as .rej files. Both runs are marked as user actions, so the command
// log shows the trial as well as the patch itself.
func applyPatchCmd(client *svn.Client, path, name, dir string) tea.Cmd {
	return func() tea.Msg {
		text, err := os.ReadFile(path)
		if err != nil {
			return patchAppliedMsg{name: name, err: err}
		}
		if !svn.PatchBelongsTo(string(text), dir) {
			return patchAppliedMsg{name: name, err: fmt.Errorf(
				"none of the files it changes are in %s — it was taken from another directory", dir)}
		}
		ctx, cancel := context.WithTimeout(svn.WithUserAction(context.Background()), 60*time.Second)
		defer cancel()
		dry, err := client.Patch(ctx, path, true)
		if err != nil {
			return patchAppliedMsg{name: name, err: err}
		}
		if err := patchTrialErr(dry); err != nil {
			return patchAppliedMsg{name: name, err: err}
		}
		res, err := client.Patch(ctx, path, false)
		return patchAppliedMsg{name: name, res: res, err: err}
	}
}

// patchTrialErr reads a dry run's result and returns why the patch is not worth
// applying, or nil when any of it would land. A patch that only partly fits is
// still worth having: svn applies the hunks it can and leaves the rest in .rej
// files to be worked through by hand. One where nothing lands at all leaves
// nothing but those rejects, so it is refused instead.
func patchTrialErr(res svn.PatchResult) error {
	switch {
	case res.Targets() == 0:
		return errors.New("svn found nothing in it to apply")
	case len(res.Applied) > 0:
		return nil
	case len(res.Skipped) == res.Targets():
		return fmt.Errorf("svn cannot find %s", batchLabel(len(res.Skipped), res.Skipped[0]))
	}
	return errors.New("not one of its changes applies here")
}

// loadConflictCmd reads a conflicted file off the UI goroutine and breaks it
// into the decisions its markers describe. The file is read rather than taken
// from anything already on screen, so what is resolved is the file as it stands.
func loadConflictCmd(path, rel string) tea.Cmd {
	return func() tea.Msg {
		b, err := os.ReadFile(path)
		if err != nil {
			return mergeLoadedMsg{rel: rel, err: err}
		}
		return mergeLoadedMsg{doc: conflictDoc(path, rel, string(b)), rel: rel}
	}
}

// loadRejectMergeCmd reads a reject and the file it was written for off the UI
// goroutine, and works out which of its hunks still have somewhere to go. A
// reject whose target has since been deleted has nothing to resolve against.
func loadRejectMergeCmd(rejPath, rejRel string) tea.Cmd {
	targetPath, targetRel := rejectTarget(rejPath), rejectTarget(rejRel)
	return func() tea.Msg {
		rej, err := os.ReadFile(rejPath)
		if err != nil {
			return mergeLoadedMsg{rel: rejRel, err: err}
		}
		target, err := os.ReadFile(targetPath)
		if err != nil {
			return mergeLoadedMsg{rel: targetRel, err: err}
		}
		return mergeLoadedMsg{
			doc: rejectDoc(rejPath, targetRel, string(rej), targetPath, string(target)),
			rel: targetRel,
		}
	}
}

// writeMergeCmd writes a resolved file back out off the UI goroutine and then
// clears what marked it as needing resolution: `svn resolve --accept working`
// for a conflict, which takes the file as it now stands and drops the artifacts
// beside it, or removing the reject whose hunks have just been dealt with. The
// resolve is marked as a user action, so it shows up in the command log.
func writeMergeCmd(client *svn.Client, d *mergeDoc) tea.Cmd {
	kind, path, rel, aux := d.kind, d.path, d.rel, d.aux
	text, count := d.merged(), len(d.regions)
	return func() tea.Msg {
		done := mergeWrittenMsg{kind: kind, rel: rel, aux: aux, count: count}
		if err := os.WriteFile(path, []byte(text), diffFilePerm); err != nil {
			done.err = err
			return done
		}
		if kind == mergeReject {
			done.err = os.Remove(aux)
			return done
		}
		ctx, cancel := context.WithTimeout(svn.WithUserAction(context.Background()), 30*time.Second)
		defer cancel()
		done.err = client.Resolve(ctx, path)
		return done
	}
}

// writeDiff writes diff into dir as name, creating dir (and any missing parent)
// when it does not exist yet. A trailing newline is ensured so the result is a
// well-formed patch. The destination rides on the message either way, so success
// can name it and failure can say which path it was.
func writeDiff(dir, name, diff string) diffSavedMsg {
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(dir, diffDirPerm); err != nil {
		return diffSavedMsg{path: path, err: err}
	}
	if !strings.HasSuffix(diff, "\n") {
		diff += "\n"
	}
	if err := os.WriteFile(path, []byte(diff), diffFilePerm); err != nil {
		return diffSavedMsg{path: path, err: err}
	}
	return diffSavedMsg{path: path}
}

// loadLogCmd runs `svn log` off the UI goroutine for one page of history: anchor
// is the revision the previous page ended on (empty for the first page) and page
// is the 1-based number the result belongs to. Errors are carried on the message
// so history-load failures stay confined to the Log panel.
func loadLogCmd(ctx context.Context, client *svn.Client, anchor string, page, limit int, gen uint64) tea.Cmd {
	return func() tea.Msg {
		entries, more, err := client.LogPage(ctx, anchor, limit)
		return logLoadedMsg{entries: entries, page: page, more: more, err: err, gen: gen}
	}
}

// headRevisionCmd reads just the newest revision off the UI goroutine, so the
// Status panel's HEAD indicator is correct from the first paint without a page
// of history being fetched for it.
func headRevisionCmd(client *svn.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()
		rev, err := client.HeadRevision(ctx)
		return headLoadedMsg{rev: rev, err: err}
	}
}

// loadRevisionDetailCmd reads one revision's changed paths off the UI goroutine.
// Errors are carried on the message so a revision that cannot be described
// leaves the metadata already on screen intact.
func loadRevisionDetailCmd(ctx context.Context, client *svn.Client, rev string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		entry, err := client.RevisionDetail(ctx, rev)
		return revisionDetailMsg{rev: rev, paths: entry.Paths, err: err, gen: gen}
	}
}

// loadRevDiffCmd reads the diff over a range of history off the UI goroutine. A
// range with no start is the single revision it ends at, which svn diffs against
// its own predecessor.
func loadRevDiffCmd(ctx context.Context, client *svn.Client, r revRange, gen uint64) tea.Cmd {
	return func() tea.Msg {
		var (
			diff string
			err  error
		)
		if r.from == "" {
			diff, err = client.DiffRevision(ctx, r.to)
		} else {
			diff, err = client.DiffRevisions(ctx, r.from, r.to)
		}
		return revDiffLoadedMsg{rng: r, diff: diff, err: err, gen: gen}
	}
}

// stageCmd applies a single stage action off the UI goroutine.
func stageCmd(client *svn.Client, changelist string, act stageAction, token uint64) tea.Cmd {
	return stageManyCmd(client, changelist, []stageAction{act}, token)
}

// stageManyCmd applies stage actions off the UI goroutine: `svn add` for the
// files that must be versioned first, then the changelist assignments and
// removals.
//
// It takes three invocations for the whole set rather than one or two per file.
// Every action is still attempted and judged on its own: a file svn refuses is
// recorded and the rest go on without it, so one path that cannot be staged no
// longer decides how far down the list the keypress got.
func stageManyCmd(client *svn.Client, changelist string, acts []stageAction, token uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), batchTimeout(len(acts)))
		defer cancel()
		return stagedMsg{outcome: applyStages(ctx, client, changelist, acts), token: token}
	}
}

// applyStages carries out a set of stage actions in as few invocations as it
// can: one `svn add` for everything that has to be versioned first, one
// `svn changelist` for everything joining the changelist, one more for
// everything leaving it.
//
// The add has to go first, and separately: a file being staged straight from
// unversioned is not something svn will changelist until it is added. A path the
// add was refused for is left out of the assignment that follows, as it was when
// each path was taken on its own.
func applyStages(ctx context.Context, client *svn.Client, changelist string, acts []stageAction) batchOutcome {
	refused := make(map[string]error, len(acts))
	record := func(errs []svn.PathError) {
		for _, pe := range errs {
			refused[pe.Path] = pe.Err
		}
	}

	var toAdd []string
	for _, act := range acts {
		if act.add {
			toAdd = append(toAdd, act.path)
		}
	}
	record(client.AddPaths(ctx, toAdd))

	var toStage, toUnstage []string
	for _, act := range acts {
		switch {
		case refused[act.path] != nil:
		case act.stage:
			toStage = append(toStage, act.path)
		default:
			toUnstage = append(toUnstage, act.path)
		}
	}
	record(client.AddToChangelistPaths(ctx, changelist, toStage))
	record(client.RemoveFromChangelistPaths(ctx, toUnstage))

	var out batchOutcome
	for _, act := range acts {
		out.add(act.path, refused[act.path])
	}
	return out
}

// addManyCmd puts paths under version control off the UI goroutine, in one
// invocation rather than one per path. Every path is judged on its own:
// AddPaths falls back to a path at a time when the batch aborts, so one path
// svn refuses never decides how far down the set the keypress got.
//
// A directory in the set takes everything under it with it, so a path already
// covered that way is named a second time for nothing — but `svn add --force`
// passes over what is already versioned rather than refusing it, which is what
// lets the set be sent as written.
func addManyCmd(client *svn.Client, paths []string, token uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), batchTimeout(len(paths)))
		defer cancel()
		refused := make(map[string]error, len(paths))
		for _, pe := range client.AddPaths(ctx, paths) {
			refused[pe.Path] = pe.Err
		}
		var out batchOutcome
		for _, p := range paths {
			out.add(p, refused[p])
		}
		return addedMsg{outcome: out, token: token}
	}
}

// commitCmd commits the staged changelist off the UI goroutine.
func commitCmd(client *svn.Client, message, changelist string, token uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		rev, err := client.Commit(ctx, message, changelist)
		return committedMsg{revision: rev, token: token, err: err}
	}
}

// assignChangelistCmd moves every target into the named changelist off the UI
// goroutine, running `svn add` first for any previously unversioned file. Every
// target is attempted, and the result rides on stagedMsg carrying the changelist
// name so the app can confirm the assignment.
func assignChangelistCmd(client *svn.Client, name string, targets []changelistTarget, token uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), batchTimeout(len(targets)))
		defer cancel()
		acts := make([]stageAction, 0, len(targets))
		for _, t := range targets {
			acts = append(acts, stageAction{path: t.path, add: t.add, stage: true})
		}
		return stagedMsg{outcome: applyStages(ctx, client, name, acts), changelist: name, token: token}
	}
}

// batchLabel summarizes a fan-out for a success toast: the sole path when one
// file was touched, otherwise an "N files" count.
func batchLabel(n int, first string) string {
	if n == 1 {
		return first
	}
	return fmt.Sprintf("%d files", n)
}

// revertCmd discards local modifications to a single path off the UI goroutine.
func revertCmd(client *svn.Client, path string, token uint64) tea.Cmd {
	return revertManyCmd(client, []string{path}, token)
}

// revertManyCmd discards local modifications to several paths off the UI
// goroutine. It takes one invocation for the lot rather than one per path, since
// reverting a directory takes its children with it and a later target under one
// already reverted fails; RevertPaths falls back to a path at a time when that
// invocation aborts, so what one path is refused for never costs the others.
func revertManyCmd(client *svn.Client, paths []string, token uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), batchTimeout(len(paths)))
		defer cancel()
		return revertedMsg{outcome: revertOutcome(client.RevertPaths(ctx, paths)), token: token}
	}
}

// deleteCmd deletes a single path off the UI goroutine.
func deleteCmd(client *svn.Client, act deleteAction, token uint64) tea.Cmd {
	return deleteManyCmd(client, []deleteAction{act}, token)
}

// deleteManyCmd deletes paths off the UI goroutine: each versioned path is
// scheduled for removal (svn delete) and each unversioned one is removed from
// disk. Every path is attempted, so one svn refuses does not strand the rest.
//
// Both ways of deleting recurse, so a path a directory in the same set already
// covers is not named again: the directory took it, and asking for it a second
// time fails with E155007 for something that is already gone. It still counts
// towards what the delete achieved, since the directory's verdict is its own.
func deleteManyCmd(client *svn.Client, acts []deleteAction, token uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), batchTimeout(len(acts)))
		defer cancel()
		lead := make(map[string]deleteAction, len(acts))
		paths := make([]string, 0, len(acts))
		for _, act := range acts {
			lead[act.path] = act
			paths = append(paths, act.path)
		}
		var out batchOutcome
		for _, cv := range svn.CoverPaths(paths) {
			act := lead[cv.Lead]
			var err error
			if act.unversioned {
				err = client.RemoveUnversioned(act.path)
			} else {
				err = client.Delete(ctx, act.path)
			}
			for _, p := range cv.Paths {
				out.add(p, err)
			}
		}
		return deletedMsg{outcome: out, token: token}
	}
}

// updateCmd brings the working copy up to date off the UI goroutine.
func updateCmd(client *svn.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		rev, err := client.Update(ctx)
		return updatedMsg{revision: rev, err: err}
	}
}

// updateToRevisionCmd moves the working copy to a specific revision off the UI
// goroutine.
func updateToRevisionCmd(client *svn.Client, rev string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		r, err := client.UpdateToRevision(ctx, rev)
		return updatedMsg{revision: r, toRevision: true, err: err}
	}
}
