package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
)

// logPagedModel loads a first page of two revisions with a further page
// available, and focuses the Log panel.
func logPagedModel(t *testing.T) *Model {
	t.Helper()
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(logLoadedMsg{page: 1, more: true, entries: []svn.LogEntry{
		{Revision: "50", Author: "alice", Message: "fifty"},
		{Revision: "49", Author: "bob", Message: "forty-nine"},
	}})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	return next.(*Model)
}

func TestInitDefersHistoryAndSavedDiffs(t *testing.T) {
	m := sizedModel(t)
	m.Init()

	if m.logRequested || m.gens.log.gen != 0 {
		t.Errorf("startup should not fetch a page of history (requested=%v gen=%d)", m.logRequested, m.gens.log.gen)
	}
	if m.gens.saved.gen != 0 {
		t.Errorf("gens.saved = %d, want 0: the saved diffs are scanned when the Diffs view is opened", m.gens.saved.gen)
	}
	if m.gens.status.gen != 1 {
		t.Errorf("gens.status = %d, want 1: status is the load startup waits on", m.gens.status.gen)
	}
}

func TestHistoryIsFetchedOnce(t *testing.T) {
	m := sizedModel(t)
	m.Init()

	// The first status has landed, so the page can be prefetched behind it.
	m = loadItems(t, m, nil)
	if !m.logRequested || m.gens.log.gen != 1 {
		t.Fatalf("the first status should prefetch page 1 (requested=%v gen=%d)", m.logRequested, m.gens.log.gen)
	}

	// Neither another status nor a look at the panel asks for it again.
	m = loadItems(t, m, nil)
	m, cmd := pressRune(t, m, '3')
	if cmd != nil {
		t.Error("focusing the Log panel should not re-fetch a page already asked for")
	}
	if m.gens.log.gen != 1 {
		t.Errorf("gens.log = %d, want 1: history is fetched once", m.gens.log.gen)
	}
}

func TestLogPanelFocusFetchesHistoryAndReportsTheWait(t *testing.T) {
	m := sizedModel(t)
	m.Init()
	// Focus the Log panel before the status arrives, so the panel is the first
	// to ask for history.
	m, cmd := pressRune(t, m, '3')
	if cmd == nil || m.gens.log.gen != 1 {
		t.Fatalf("the first look at the Log panel should fetch page 1 (gen=%d)", m.gens.log.gen)
	}

	m = loadItems(t, m, nil)
	if m.gens.log.gen != 1 {
		t.Errorf("gens.log = %d, want 1: the status prefetch should find the page already asked for", m.gens.log.gen)
	}
	if main := stripANSI(m.main.View()); !strings.Contains(main, "Loading history") {
		t.Errorf("expected the Log panel to report the wait, got:\n%s", main)
	}

	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{{Revision: "50", Author: "alice", Message: "fifty"}}})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "r50") {
		t.Errorf("expected the revision detail once history landed, got:\n%s", main)
	}
}

func TestRevisionDetailFillsInChangedPaths(t *testing.T) {
	m := logPagedModel(t)

	// The metadata is on screen before the changed paths have been asked for.
	main := stripANSI(m.main.View())
	if !strings.Contains(main, "r50") || !strings.Contains(main, "alice") {
		t.Fatalf("expected the revision metadata immediately, got:\n%s", main)
	}
	if strings.Contains(main, "Changed paths") {
		t.Errorf("changed paths cost their own load and should not be shown yet:\n%s", main)
	}

	next, _ := m.Update(revisionDetailMsg{rev: "50", paths: []svn.ChangedPath{
		{Action: "M", Path: "/trunk/committed.txt"},
	}})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "Changed paths") || !strings.Contains(main, "M /trunk/committed.txt") {
		t.Errorf("expected the changed paths once the detail landed, got:\n%s", main)
	}

	// Revisions are immutable, so the detail is never asked for twice.
	if cmd := m.revisionDetailForSelection(); cmd != nil {
		t.Error("a revision whose detail is held should issue no command")
	}
}

func TestLogPageTurnServesCachedPage(t *testing.T) {
	m := logPagedModel(t)
	next, _ := m.Update(revisionDetailMsg{rev: "50", paths: []svn.ChangedPath{{Action: "M", Path: "/trunk/a"}}})
	m = next.(*Model)

	m, _ = pressRune(t, m, 'n')
	next, _ = m.Update(logLoadedMsg{page: 2, entries: []svn.LogEntry{{Revision: "48"}, {Revision: "47"}}})
	m = next.(*Model)

	// Page 1 was cached as it loaded, so turning back to it costs no command and
	// its rows are on screen before anything else runs.
	m, cmd := pressRune(t, m, 'p')
	if cmd != nil {
		t.Error("a page already fetched should be served from the session")
	}
	if m.logLoading {
		t.Error("a cached page turn should leave nothing in flight")
	}
	if len(m.logEntries) != 2 || m.logEntries[0].Revision != "50" {
		t.Errorf("cached page 1 = %+v, want r50 and r49", m.logEntries)
	}
	if items := m.log.Items(); len(items) != 2 || items[0].Revision != "50" {
		t.Errorf("the table should show the cached rows, got %+v", items)
	}
	if !m.logMore {
		t.Error("the cached page should restore that a further page follows it")
	}
}

func TestLogPagingWalksForwardAndBack(t *testing.T) {
	m := logPagedModel(t)

	// The page the first load ended on anchors the next one.
	m, cmd := pressRune(t, m, 'n')
	if cmd == nil {
		t.Fatal("n should load the next page")
	}
	if m.logPage != 2 {
		t.Fatalf("logPage = %d, want 2", m.logPage)
	}
	if got := m.logAnchors[0]; got != "49" {
		t.Errorf("anchor for page 2 = %q, want 49", got)
	}

	next, _ := m.Update(logLoadedMsg{page: 2, entries: []svn.LogEntry{{Revision: "48"}}})
	m = next.(*Model)
	if len(m.logEntries) != 1 || m.logEntries[0].Revision != "48" {
		t.Fatalf("page 2 entries = %+v, want just r48", m.logEntries)
	}
	if m.logMore {
		t.Error("a short page is the last page")
	}

	// n at the end warns rather than advancing past it.
	m, cmd = pressRune(t, m, 'n')
	if cmd != nil {
		t.Error("n on the last page should not load anything")
	}
	if m.logPage != 2 {
		t.Errorf("logPage = %d, want 2", m.logPage)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no older revisions") {
		t.Errorf("expected a guard toast, got:\n%s", view)
	}

	// p returns to the first page, which needs no anchor.
	m, cmd = pressRune(t, m, 'p')
	if cmd == nil {
		t.Fatal("p should load the previous page")
	}
	if m.logPage != 1 {
		t.Errorf("logPage = %d, want 1", m.logPage)
	}
	if msg, ok := cmd().(logLoadedMsg); ok && msg.page != 1 {
		t.Errorf("loaded page %d, want 1", msg.page)
	}

	// p on the first page warns too.
	next, _ = m.Update(logLoadedMsg{page: 1, more: true, entries: []svn.LogEntry{{Revision: "50"}, {Revision: "49"}}})
	m = next.(*Model)
	m, cmd = pressRune(t, m, 'p')
	if cmd != nil {
		t.Error("p on the first page should not load anything")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "already on the first page") {
		t.Errorf("expected a guard toast, got:\n%s", view)
	}
}

func TestLogPagingIgnoresSupersededLoad(t *testing.T) {
	m := logPagedModel(t)
	m, _ = pressRune(t, m, 'n')

	// A page-1 response arriving after the turn to page 2 is stale.
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{{Revision: "1"}}})
	m = next.(*Model)
	if len(m.logEntries) != 2 || m.logEntries[0].Revision != "50" {
		t.Errorf("a superseded load overwrote the page: %+v", m.logEntries)
	}
}

func TestLogPageTurnStartsAtTheTop(t *testing.T) {
	m := logPagedModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	if m.log.Index() != 1 {
		t.Fatalf("cursor = %d, want 1", m.log.Index())
	}

	m, _ = pressRune(t, m, 'n')
	next, _ = m.Update(logLoadedMsg{page: 2, entries: []svn.LogEntry{{Revision: "48"}, {Revision: "47"}}})
	m = next.(*Model)
	if m.log.Index() != 0 {
		t.Errorf("cursor = %d after a page turn, want 0", m.log.Index())
	}
}

func TestLogRefreshKeepsPageAndCursor(t *testing.T) {
	m := logPagedModel(t)
	m, _ = pressRune(t, m, 'n')
	next, _ := m.Update(logLoadedMsg{page: 2, more: true, entries: []svn.LogEntry{{Revision: "48"}, {Revision: "47"}}})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)

	m, cmd := pressRune(t, m, 'R')
	if cmd == nil {
		t.Fatal("R should reload")
	}
	if m.logPage != 2 {
		t.Errorf("logPage = %d after refresh, want 2", m.logPage)
	}
	if m.log.Index() != 1 {
		t.Errorf("cursor = %d after refresh, want 1 (kept)", m.log.Index())
	}
}

func TestUpdateToRevisionStaysOnPageButPlainUpdateResets(t *testing.T) {
	onPageTwo := func(t *testing.T) *Model {
		t.Helper()
		m := logPagedModel(t)
		m, _ = pressRune(t, m, 'n')
		next, _ := m.Update(logLoadedMsg{page: 2, more: true, entries: []svn.LogEntry{{Revision: "48"}}})
		return next.(*Model)
	}

	m := onPageTwo(t)
	next, _ := m.Update(updatedMsg{revision: "48", toRevision: true})
	if got := next.(*Model).logPage; got != 2 {
		t.Errorf("logPage = %d after updating to a revision on page 2, want 2", got)
	}

	m = onPageTwo(t)
	next, _ = m.Update(updatedMsg{revision: "50"})
	m = next.(*Model)
	if m.logPage != 1 {
		t.Errorf("logPage = %d after a plain update, want 1", m.logPage)
	}
	if m.logAnchors != nil {
		t.Errorf("anchors = %q, want cleared", m.logAnchors)
	}
}

func TestLogFooterShowsPositionAndPage(t *testing.T) {
	m := logPagedModel(t)
	if got := m.logFooter(); got != "1 of 2 · 1" {
		t.Errorf("footer = %q, want %q", got, "1 of 2 · 1")
	}

	// The filter narrows the page, and the full page size stays visible.
	m.filters[panelLog] = "user:alice"
	m.applyLogFilter()
	if got := m.logFooter(); got != "1 of 1 (2) · 1" {
		t.Errorf("filtered footer = %q, want %q", got, "1 of 1 (2) · 1")
	}

	// A page no revision matches still reports the page it is on.
	m.filters[panelLog] = "user:nobody"
	m.applyLogFilter()
	if got := m.logFooter(); got != "0 of 0 (2) · 1" {
		t.Errorf("empty-filter footer = %q, want %q", got, "0 of 0 (2) · 1")
	}
}

func TestLogFilterSurvivesPageTurn(t *testing.T) {
	m := logPagedModel(t)
	m.filters[panelLog] = "user:alice"
	m.applyLogFilter()

	m, _ = pressRune(t, m, 'n')
	next, _ := m.Update(logLoadedMsg{page: 2, entries: []svn.LogEntry{
		{Revision: "48", Author: "alice", Message: "forty-eight"},
		{Revision: "47", Author: "bob", Message: "forty-seven"},
	}})
	m = next.(*Model)
	if got := m.filters[panelLog]; got != "user:alice" {
		t.Errorf("filter = %q after a page turn, want it kept", got)
	}
	if items := m.log.Items(); len(items) != 1 || items[0].Revision != "48" {
		t.Errorf("filtered page 2 = %+v, want just r48", items)
	}
}
