package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/component"
)

// loadEvent handles the replies to the reads that fill the model: working-copy
// status, history, revision detail, diffs, the two local file stores, and the
// document the resolution overlay is opened on. Each reply carries the stamp of
// the request it answers, so one overtaken by a later request is dropped. It
// reports whether it owned the message.
func (m *Model) loadEvent(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case statusLoadedMsg:
		if m.gens.status.stale(msg.gen) {
			return nil, true
		}
		m.loading = false
		m.err = nil
		items, leaked := withoutShelfStore(msg.items)
		m.fileItems = items
		m.noteShelfStoreVisible(leaked)
		// The poller watches the paths svn reports, so the rows this reload added are
		// the new baseline rather than a change of their own.
		m.rebaseWatch()
		// svn has reported the real status, so anything shown ahead of it is either
		// confirmed by this or superseded; the snapshot behind it is now two steps back.
		m.dropOptimistic()
		// Re-derive what is on screen from the session: a cached diff the status
		// still stands behind stays put, so a reload no longer blanks a diff it is
		// about to fetch unchanged. Nothing else in the cache is touched — every
		// entry is re-checked when it is read, which keeps a routine reload cheap no
		// matter how much has been browsed.
		m.rederiveDiff()
		m.rebuildFileTree()
		m.focusFirstFile()
		m.rebuildChangelists()
		m.syncDrill()
		m.refreshChrome()
		// The first paint is done, so the page of history the Log panel needs can
		// be fetched now without the cost falling on startup.
		return tea.Batch(m.diffLoadForSelection(), m.ensureLogPage()), true

	case logLoadedMsg:
		// A page turn taken while this load was in flight supersedes it.
		if m.gens.log.stale(msg.gen) || msg.page != m.logPage {
			return nil, true
		}
		m.logLoading = false
		if msg.err == nil {
			m.session.PutLogPage(
				logKey{anchor: m.logAnchor(msg.page), limit: m.cfg.LogLimit},
				historyPage{entries: msg.entries, more: msg.more},
			)
		}
		m.applyLogPage(historyPage{entries: msg.entries, more: msg.more}, msg.err)
		return m.revisionDetailForSelection(), true

	case headLoadedMsg:
		// The HEAD probe is the only svn command startup runs against the
		// repository, so its failure is what tells the Log panel it is unreachable.
		if msg.err != nil {
			m.logErr = msg.err
			if m.source == sourceLog {
				m.updateMain()
			}
			return nil, true
		}
		m.headRev = msg.rev
		m.updateStatus()
		return nil, true

	case revisionPendingMsg:
		// The cursor rested long enough for this revision's paths to be worth
		// asking svn for.
		if m.gens.rev.stale(msg.gen) {
			return nil, true
		}
		ctx, gen := m.gens.rev.begin(loadTimeout)
		return loadRevisionDetailCmd(ctx, m.client, msg.rev, gen), true

	case revisionDetailMsg:
		// A revision that cannot be described leaves its metadata on screen; only
		// the changed-path list is missing, which is not worth an error for.
		if m.gens.rev.stale(msg.gen) || msg.err != nil {
			return nil, true
		}
		m.session.PutRevDetail(msg.rev, msg.paths)
		if m.source == sourceLog {
			m.updateMain()
		}
		return nil, true

	case revDiffLoadedMsg:
		if m.gens.revDiff.stale(msg.gen) {
			return nil, true
		}
		e := revDiffEntry{text: msg.diff}
		if msg.err != nil {
			e.text, e.failed = "Unable to load diff: "+msg.err.Error(), true
		}
		m.session.PutRevDiff(msg.rng, e)
		if !e.failed {
			m.applyRevPatch(e.text)
		}
		if m.source == sourceLog {
			m.updateMain()
		}
		return nil, true

	case diffPendingMsg:
		// The cursor rested long enough for this diff to be worth asking svn for.
		if m.gens.diff.stale(msg.gen) {
			return nil, true
		}
		ctx, gen := m.gens.diff.begin(loadTimeout)
		return loadDiffCmd(ctx, m.client, msg.key, gen), true

	case diffLoadedMsg:
		if m.gens.diff.stale(msg.gen) {
			return nil, true
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
		return nil, true

	case savedDiffsLoadedMsg:
		if m.gens.saved.stale(msg.gen) {
			return nil, true
		}
		m.savedDiffsErr = msg.err
		m.savedDiffItems = msg.files
		m.rebuildSavedDiffs()
		if m.source == sourceFiles && m.filesViewIsDiffs() {
			m.updateMain()
			return m.savedDiffLoadForSelection(), true
		}
		return nil, true

	case savedDiffReadMsg:
		if m.gens.diff.stale(msg.gen) {
			return nil, true
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
		return nil, true

	case shelvesLoadedMsg:
		if m.gens.shelf.stale(msg.gen) {
			return nil, true
		}
		m.shelfErr = msg.err
		m.shelfItems = msg.entries
		m.rebuildShelves()
		if m.source == sourceShelf {
			m.updateMain()
			return m.shelfLoadForSelection(), true
		}
		return nil, true

	case shelfReadMsg:
		if m.gens.diff.stale(msg.gen) {
			return nil, true
		}
		m.shelfID = msg.id
		m.shelfReadErr = msg.err != nil
		if msg.err != nil {
			m.shelfText = "Unable to read shelf: " + msg.err.Error()
		} else {
			m.shelfText = msg.text
		}
		if m.source == sourceShelf {
			m.updateMain()
		}
		return nil, true

	case rejectsLoadedMsg:
		if m.gens.reject.stale(msg.gen) {
			return nil, true
		}
		m.rejectsErr = msg.err
		m.rejectItems = msg.files
		m.rebuildRejects()
		if m.source == sourceFiles && m.filesViewIsRejects() {
			m.updateMain()
			return m.rejectLoadForSelection(), true
		}
		return nil, true

	case rejectReadMsg:
		if m.gens.diff.stale(msg.gen) {
			return nil, true
		}
		m.rejectPath = msg.path
		m.rejectErr = msg.err != nil
		if msg.err != nil {
			m.rejectText = "Unable to read reject: " + msg.err.Error()
		} else {
			m.rejectText = msg.text
		}
		if m.source == sourceFiles {
			m.updateMain()
		}
		return nil, true

	case mergeLoadedMsg:
		if msg.err != nil {
			m.showToast(failureText("read "+msg.rel, msg.err), component.LevelError)
			return nil, true
		}
		if len(msg.doc.regions) == 0 {
			m.showToast(mergeNothingToDo(msg.doc), component.LevelWarning)
			return nil, true
		}
		if msg.doc.unplaced > 0 {
			// The rest are still worth working through, so this is said in passing
			// rather than in place of the overlay.
			m.showToast(fmt.Sprintf("%d hunks no longer fit %s and are left in the reject",
				msg.doc.unplaced, msg.doc.rel), component.LevelWarning)
		}
		m.showMerge(msg.doc)
		return nil, true
	}
	return nil, false
}
