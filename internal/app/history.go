package app

import (
	"strconv"
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
