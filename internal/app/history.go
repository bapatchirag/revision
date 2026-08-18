package app

import (
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/component"
)

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
		m.gens.log.next()
		m.logLoading = false
		m.applyLogPage(p, nil)
		return m.revisionDetailForSelection()
	}
	// The rows of the page being left stay on screen, dimmed, until the new page
	// lands, so the panel never blanks mid-turn.
	m.logLoading = true
	ctx, gen := m.gens.log.begin(loadTimeout)
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
// the page, how many revisions the page holds, and the page number. Drilled into
// a range's files it counts those instead, since the page behind them is not
// what is being read.
func (m *Model) logFooter() string {
	if m.inRevDrill() {
		index, shown := fileLeafStats(m.revFiles.Items(), m.revFiles.Index())
		return countLabel(index, shown, len(m.revPatch))
	}
	label := countLabel(m.log.Index()+1, len(m.log.Items()), len(m.logEntries))
	if label == "" {
		return ""
	}
	return label + " · " + strconv.Itoa(m.logPage)
}

// logPickMax is how many revisions can be held at once: a diff has two sides.
const logPickMax = 2

// toggleLogPick picks the highlighted revision to be diffed, or unpicks it when
// it is already held. A third pick drops the one picked first, so a second end
// can be moved around without unpicking it each time.
func (m *Model) toggleLogPick() tea.Cmd {
	e, ok := m.log.Selected()
	if !ok || e.Revision == "" {
		return nil
	}
	if i := slices.Index(m.logPicks, e.Revision); i >= 0 {
		m.logPicks = slices.Delete(m.logPicks, i, i+1)
	} else {
		m.logPicks = append(m.logPicks, e.Revision)
		if len(m.logPicks) > logPickMax {
			m.logPicks = m.logPicks[len(m.logPicks)-logPickMax:]
		}
	}
	m.updateBar()
	return nil
}

// isLogPicked reports whether a revision is currently held for a diff.
func (m *Model) isLogPicked(rev string) bool { return slices.Contains(m.logPicks, rev) }

// clearLogPicks drops every held revision, reporting whether there was anything
// to drop so esc can tell whether it was consumed here.
func (m *Model) clearLogPicks() bool {
	if len(m.logPicks) == 0 {
		return false
	}
	m.logPicks = nil
	m.updateBar()
	return true
}

// pickedRange is the range the held revisions describe: one revision on its own
// diffs against its predecessor, two diff literally against each other, lowest
// first so the patch reads forwards however they were picked.
func (m *Model) pickedRange() (revRange, bool) {
	switch len(m.logPicks) {
	case 1:
		return revRange{to: m.logPicks[0]}, true
	case logPickMax:
		a, b := m.logPicks[0], m.logPicks[1]
		if revLess(b, a) {
			a, b = b, a
		}
		return revRange{from: a, to: b}, true
	}
	return revRange{}, false
}

// revLess orders two revisions by number, falling back to a plain string compare
// for anything svn would not have reported.
func revLess(a, b string) bool {
	x, errA := strconv.Atoi(a)
	y, errB := strconv.Atoi(b)
	if errA == nil && errB == nil {
		return x < y
	}
	return a < b
}

// revDiffWarnSpan is how many revisions a range may cover before the diff is
// worth warning about. It is only a warning: the load runs either way, since how
// long svn takes depends on what changed rather than on how far apart the two
// ends are.
const revDiffWarnSpan = 500

// showPickedDiff drills the Log panel into the files the held revisions touched
// and puts their diff on Main. Nothing held is not an error worth a modal — the
// key is simply early — so it says what to press instead.
//
// The tree is empty until the diff lands, since the patch is the only account of
// what the range touched; Main reports the wait meanwhile.
func (m *Model) showPickedDiff() tea.Cmd {
	r, ok := m.pickedRange()
	if !ok {
		m.showToast("pick a revision with v first", component.LevelWarning)
		return nil
	}
	m.revDiff = r
	m.revPatch = nil
	m.revCollapsed = map[string]bool{}
	// Cleared before the push, so the revisions underneath come back unfiltered
	// and the tree opens on everything the range touched.
	m.logFilterHeld = m.filters[panelLog]
	m.setFilter(panelLog, "")
	m.rebuildRevFiles()
	push := m.logViews.PushTitled(r.label(), m.revFiles)
	m.syncMainTitle()
	m.updateBar()
	m.updateMain()
	return tea.Batch(push, m.loadRevDiff(r))
}

// inRevDrill reports whether the Log panel is drilled into the files a range of
// history touched, in which case the revisions underneath are out of reach and
// the keys that act on them do nothing.
func (m *Model) inRevDrill() bool { return m.logViews.Depth() > 0 }

// showingRevDiff reports whether the screen is given over to a range of history:
// the Log panel drilled into one, with Main following it. It is what the keys
// that act on the working copy test, since neither the rows nor the patch on
// screen describe the state they would change.
func (m *Model) showingRevDiff() bool { return m.source == sourceLog && m.inRevDrill() }

// closeRevDiff drops everything the drill produced. The container pops the view
// itself and reports it, so this is the reply to that rather than the way out.
func (m *Model) closeRevDiff() {
	m.revDiff = revRange{}
	m.revPatch = nil
	m.revCollapsed = map[string]bool{}
	m.revFiles.SetItems(nil)
	// The container has already popped, so this lands on the revisions again.
	m.setFilter(panelLog, m.logFilterHeld)
	m.logFilterHeld = ""
	m.syncMainTitle()
	m.updateBar()
	m.updateMain()
}

// rebuildRevFiles re-flattens the range's patch into the drilled-in tree,
// narrowed by the Log-panel filter, honoring its own per-directory fold state
// and keeping the cursor on the same path across the rebuild.
func (m *Model) rebuildRevFiles() {
	path := selectedNodePath(m.revFiles)
	m.revFiles.SetItems(buildPathTree(m.filteredRevPatch(), revFilePath, m.revCollapsed))
	selectNodePath(m.revFiles, path)
}

// filteredRevPatch returns the sections of the range's patch the tree should
// show: those matching the Log-panel filter, or all of them when none is set.
func (m *Model) filteredRevPatch() []patchFile {
	q := parseFilter(m.filters[panelLog], revFileFilterKeys)
	if q.empty() {
		return m.revPatch
	}
	out := make([]patchFile, 0, len(m.revPatch))
	for _, f := range m.revPatch {
		if matchPatchFile(f, q) {
			out = append(out, f)
		}
	}
	return out
}

// matchPatchFile reports whether one file of a range's patch satisfies the
// filter: its state where one was asked for, and its path against the free text.
func matchPatchFile(f patchFile, q filterQuery) bool {
	if v, ok := q.params["state"]; ok && !stateMatches(f.State, v) {
		return false
	}
	return q.text == "" || containsFold(f.Path, q.text)
}

// applyRevPatch cuts a loaded range diff into per-file sections and fills the
// tree from them, opening on the root so Main starts on the whole patch.
func (m *Model) applyRevPatch(diff string) {
	m.revPatch = splitPatchByFile(diff)
	m.rebuildRevFiles()
	m.revFiles.SetIndex(0)
}

// toggleRevCollapse expands or collapses the directory under the drilled-in
// tree's cursor. It is inert on a file leaf.
func (m *Model) toggleRevCollapse() tea.Cmd {
	n, ok := m.revFiles.Selected()
	if !ok || n.Item != nil {
		return nil
	}
	if m.revCollapsed[n.Path] {
		delete(m.revCollapsed, n.Path)
	} else {
		m.revCollapsed[n.Path] = true
	}
	m.rebuildRevFiles()
	m.updateMain()
	return nil
}

// loadRevDiff fetches a range's diff unless the session already holds it.
// Revisions are immutable, so one read stays good and looking at a comparison
// again costs no command.
func (m *Model) loadRevDiff(r revRange) tea.Cmd {
	if e, held := m.session.RevDiff(r); held {
		if !e.failed {
			m.applyRevPatch(e.text)
		}
		return nil
	}
	if span := revSpan(r); span > revDiffWarnSpan {
		m.showToast(r.label()+" covers "+strconv.Itoa(span)+" revisions — this may take a while", component.LevelWarning)
	}
	ctx, gen := m.gens.revDiff.begin(loadTimeout)
	return loadRevDiffCmd(ctx, m.client, r, gen)
}

// revSpan is how many revisions a range covers, or zero when either end is not a
// number.
func revSpan(r revRange) int {
	from, errA := strconv.Atoi(r.from)
	to, errB := strconv.Atoi(r.to)
	if errA != nil || errB != nil {
		return 0
	}
	return to - from
}

// reloadRevDiffIfShown refetches the range on screen, for an explicit refresh
// that has just emptied the session it would otherwise be served from.
func (m *Model) reloadRevDiffIfShown() tea.Cmd {
	if !m.revDiff.set() {
		return nil
	}
	return m.loadRevDiff(m.revDiff)
}

// logPickLabel names the held revisions for the status bar, in the order they
// were picked so it is plain which one a third pick would displace.
func (m *Model) logPickLabel() string {
	revs := make([]string, len(m.logPicks))
	for i, r := range m.logPicks {
		revs[i] = "r" + r
	}
	return "picked " + strings.Join(revs, " ↔ ")
}

// revisionDetailForSelection returns a command to read the changed paths of the
// selected revision. Revisions are immutable, so one the session already holds
// costs no command; a miss is debounced like a diff, so walking the Log panel
// does not spawn an svn process for every row passed over.
func (m *Model) revisionDetailForSelection() tea.Cmd {
	// Whatever is in flight was for the row just left; its reply is of no use
	// now, so it is dropped rather than rendered.
	gen := m.gens.rev.next()
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
