package app

import (
	"errors"
	"strconv"
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

func TestLogPickTogglesAndMarksTheRow(t *testing.T) {
	m := logPagedModel(t)

	m, _ = pressRune(t, m, 'v')
	if got := m.logPicks; len(got) != 1 || got[0] != "50" {
		t.Fatalf("picks = %q, want just r50", got)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "● ") {
		t.Errorf("expected the picked row to be marked, got:\n%s", view)
	}

	// The same revision again lets it go.
	m, _ = pressRune(t, m, 'v')
	if len(m.logPicks) != 0 {
		t.Errorf("picks = %q, want the second press to unpick", m.logPicks)
	}
}

func TestLogPickHoldsTwoAndDropsTheOldest(t *testing.T) {
	m := logPagedModel(t)

	m, _ = pressRune(t, m, 'v')
	m, _ = pressRune(t, m, 'j')
	m, _ = pressRune(t, m, 'v')
	if got := m.logPicks; len(got) != 2 || got[0] != "50" || got[1] != "49" {
		t.Fatalf("picks = %q, want r50 then r49 in pick order", got)
	}

	// A third pick displaces the one picked first, not the one nearest it.
	next, _ := m.Update(logLoadedMsg{page: 1, more: true, entries: []svn.LogEntry{
		{Revision: "50"}, {Revision: "49"}, {Revision: "48"},
	}})
	m = next.(*Model)
	m, _ = pressRune(t, m, 'j')
	m, _ = pressRune(t, m, 'v')
	if got := m.logPicks; len(got) != 2 || got[0] != "49" || got[1] != "48" {
		t.Errorf("picks = %q, want r50 displaced by r48", got)
	}
}

func TestLogPickSurvivesAPageTurn(t *testing.T) {
	m := logPagedModel(t)
	m, _ = pressRune(t, m, 'v')

	m, _ = pressRune(t, m, 'n')
	next, _ := m.Update(logLoadedMsg{page: 2, entries: []svn.LogEntry{{Revision: "48"}, {Revision: "47"}}})
	m = next.(*Model)

	// Picks are held by revision, so the far end of a comparison can be on
	// another page entirely.
	if got := m.logPicks; len(got) != 1 || got[0] != "50" {
		t.Fatalf("picks = %q, want r50 kept across the page turn", got)
	}
	m, _ = pressRune(t, m, 'v')
	if got := m.logPicks; len(got) != 2 || got[1] != "48" {
		t.Errorf("picks = %q, want r50 and r48 from different pages", got)
	}
}

func TestEscClearsLogPicksAfterTheFilter(t *testing.T) {
	m := logPagedModel(t)
	m, _ = pressRune(t, m, 'v')
	m.filters[panelLog] = "user:alice"
	m.applyLogFilter()

	// The filter goes first; the picks are still held.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if m.filters[panelLog] != "" {
		t.Fatalf("filter = %q, want esc to clear it first", m.filters[panelLog])
	}
	if len(m.logPicks) != 1 {
		t.Fatalf("picks = %q, want them held until the filter is gone", m.logPicks)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if len(m.logPicks) != 0 {
		t.Errorf("picks = %q, want the second esc to let them go", m.logPicks)
	}
}

func TestLogPicksAreDroppedWithTheSource(t *testing.T) {
	m := logPagedModel(t)
	m, _ = pressRune(t, m, 'v')

	// The revisions belong to the tree being left, so they cannot outlive it.
	m.resetForSource()
	if len(m.logPicks) != 0 {
		t.Errorf("picks = %q, want them dropped with the old source", m.logPicks)
	}
}

// The Log panel lists revisions that are already committed, so c has nothing to
// build a commit from there and must not open the editor — nor warn about an
// empty staged bucket the panel never referred to.
func TestCommitIsInertOnTheLogPanel(t *testing.T) {
	m := logPagedModel(t)

	m, _ = pressRune(t, m, 'c')
	if m.editing {
		t.Error("c on the Log panel should not open the commit editor")
	}
	if toast := stripANSI(m.toast.View()); strings.Contains(toast, "nothing staged") {
		t.Errorf("c on the Log panel should say nothing at all, got: %s", toast)
	}
}

// pressEnter activates the focused component. enter is not consumed by the model
// itself: the component emits an ActivatedMsg which the model then acts on, so
// both steps are taken here. The command returned is the one that reply produced.
func pressEnter(t *testing.T, m *Model) (*Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd == nil {
		return m, nil
	}
	next, cmd = m.Update(cmd())
	return next.(*Model), cmd
}

func TestPickedRangeIsOrderedLowestFirst(t *testing.T) {
	m := logPagedModel(t)

	if _, ok := m.pickedRange(); ok {
		t.Error("no picks should describe no range")
	}

	// One revision diffs against its own predecessor, which svn spells `-c`.
	m.logPicks = []string{"50"}
	if r, _ := m.pickedRange(); r.from != "" || r.to != "50" {
		t.Errorf("range = %+v, want the single revision r50", r)
	}

	// Two are ordered by number, so the patch reads forwards either way round.
	m.logPicks = []string{"50", "42"}
	r, ok := m.pickedRange()
	if !ok || r.from != "42" || r.to != "50" {
		t.Errorf("range = %+v, want r42 to r50", r)
	}
	m.logPicks = []string{"42", "50"}
	if same, _ := m.pickedRange(); same != r {
		t.Errorf("range = %+v, want the same key whichever end was picked first", same)
	}
}

func TestEnterShowsThePickedDiff(t *testing.T) {
	m := logPagedModel(t)
	m, _ = pressRune(t, m, 'v')
	m, _ = pressRune(t, m, 'j')
	m, _ = pressRune(t, m, 'v')

	m, cmd := pressEnter(t, m)
	if cmd == nil {
		t.Fatal("enter should fetch the diff for the picked revisions")
	}
	if r := m.revDiff; r.from != "49" || r.to != "50" {
		t.Fatalf("revDiff = %+v, want r49 to r50", r)
	}
	if main := stripANSI(m.main.View()); !strings.Contains(main, "Loading diff") {
		t.Errorf("Main should report the wait, got:\n%s", main)
	}

	next, _ := m.Update(revDiffLoadedMsg{rng: m.revDiff, diff: "Index: a.go\n@@ -1 +1 @@\n-old\n+new", gen: m.gens.revDiff.gen})
	m = next.(*Model)
	main := stripANSI(m.main.View())
	if !strings.Contains(main, "r49 → r50") {
		t.Errorf("Main should head the patch with the range, got:\n%s", main)
	}
	if !strings.Contains(main, "+new") {
		t.Errorf("Main should show the patch, got:\n%s", main)
	}
}

func TestEnterWithoutPicksSaysWhatToPress(t *testing.T) {
	m := logPagedModel(t)
	m, cmd := pressEnter(t, m)
	if cmd != nil || m.revDiff.set() {
		t.Error("enter with nothing picked should not fetch anything")
	}
	if toast := stripANSI(m.toast.View()); !strings.Contains(toast, "pick a revision") {
		t.Errorf("expected a nudge towards v, got: %s", toast)
	}
}

// The range is a mode, not a selection: walking the log or turning the page
// leaves it up, or the second end could never be looked at.
func TestRangeDiffSurvivesTheCursorAndThePage(t *testing.T) {
	m := logPagedModel(t)
	m.logPicks = []string{"50", "49"}
	m, _ = pressEnter(t, m)
	next, _ := m.Update(revDiffLoadedMsg{rng: m.revDiff, diff: "Index: a.go\n@@ -1 +1 @@\n+new", gen: m.gens.revDiff.gen})
	m = next.(*Model)

	m, _ = pressRune(t, m, 'j')
	if !m.revDiff.set() || !strings.Contains(stripANSI(m.main.View()), "+new") {
		t.Error("moving the cursor should leave the range diff up")
	}

	m, _ = pressRune(t, m, 'n')
	next, _ = m.Update(logLoadedMsg{page: 2, entries: []svn.LogEntry{{Revision: "48"}}})
	m = next.(*Model)
	if !m.revDiff.set() || !strings.Contains(stripANSI(m.main.View()), "+new") {
		t.Error("turning the page should leave the range diff up")
	}
}

func TestEscUnwindsTheDiffThenThePicks(t *testing.T) {
	m := logPagedModel(t)
	m.logPicks = []string{"50", "49"}
	m, _ = pressEnter(t, m)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if m.revDiff.set() {
		t.Fatal("the first esc should take the diff off Main")
	}
	if len(m.logPicks) != 2 {
		t.Fatalf("picks = %q, want them still held for another look", m.logPicks)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if len(m.logPicks) != 0 {
		t.Errorf("picks = %q, want the second esc to let them go", m.logPicks)
	}
}

// What Main shows must always answer to what is held, so changing the picks
// takes a diff computed from the old ones down.
func TestPickingAgainClosesTheDiff(t *testing.T) {
	m := logPagedModel(t)
	m.logPicks = []string{"50", "49"}
	m, _ = pressEnter(t, m)

	m, _ = pressRune(t, m, 'v')
	if m.revDiff.set() {
		t.Error("picking again should take the stale diff off Main")
	}
}

func TestRangeDiffIsServedFromTheSession(t *testing.T) {
	m := logPagedModel(t)
	m.logPicks = []string{"50", "49"}
	m, _ = pressEnter(t, m)
	next, _ := m.Update(revDiffLoadedMsg{rng: m.revDiff, diff: "Index: a.go\n@@ -1 +1 @@\n+new", gen: m.gens.revDiff.gen})
	m = next.(*Model)

	// Revisions are immutable, so looking at the same comparison again is free.
	m, _ = pressRune(t, m, 'v')
	m.logPicks = []string{"50", "49"}
	m, cmd := pressEnter(t, m)
	if cmd != nil {
		t.Error("a comparison already read should cost no command")
	}
	if !strings.Contains(stripANSI(m.main.View()), "+new") {
		t.Error("the cached patch should be on screen at once")
	}
}

func TestSupersededRangeDiffIsDropped(t *testing.T) {
	m := logPagedModel(t)
	m.logPicks = []string{"50", "49"}
	m, _ = pressEnter(t, m)
	stale := m.gens.revDiff.gen

	// A second comparison supersedes the first, whose reply must not land.
	m, _ = pressRune(t, m, 'v')
	m.logPicks = []string{"50"}
	m, _ = pressEnter(t, m)

	next, _ := m.Update(revDiffLoadedMsg{rng: revRange{from: "49", to: "50"}, diff: "Index: stale.go\n+gone", gen: stale})
	m = next.(*Model)
	if strings.Contains(stripANSI(m.main.View()), "gone") {
		t.Error("a superseded diff should never reach the screen")
	}
}

func TestRangeDiffReportsAFailure(t *testing.T) {
	m := logPagedModel(t)
	m.logPicks = []string{"50"}
	m, _ = pressEnter(t, m)

	next, _ := m.Update(revDiffLoadedMsg{rng: m.revDiff, err: errors.New("E160013: not found"), gen: m.gens.revDiff.gen})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "E160013") {
		t.Errorf("a range svn refused should say so, got:\n%s", main)
	}
	if m.revDiffShowsDiff() {
		t.Error("a failure notice is not a patch to pin a gutter to")
	}
}

func TestLargeRangeWarns(t *testing.T) {
	m := logPagedModel(t)
	m.logPicks = []string{"1", strconv.Itoa(revDiffWarnSpan + 2)}
	m, _ = pressEnter(t, m)

	if toast := stripANSI(m.toast.View()); !strings.Contains(toast, "this may take a while") {
		t.Errorf("a range spanning %d revisions should warn, got: %s", revDiffWarnSpan+1, toast)
	}
	if !m.revDiff.set() {
		t.Error("the warning must not stop the diff: it is a heads-up, not a refusal")
	}
}

func TestRangeDiffIsDroppedWithTheSource(t *testing.T) {
	m := logPagedModel(t)
	m.logPicks = []string{"50"}
	m, _ = pressEnter(t, m)

	m.resetForSource()
	if m.revDiff.set() {
		t.Error("the range belongs to the tree being left")
	}
}

func TestLogPicksAreReportedInTheStatusBar(t *testing.T) {
	m := logPagedModel(t)
	m, _ = pressRune(t, m, 'v')
	m, _ = pressRune(t, m, 'j')
	m, _ = pressRune(t, m, 'v')
	if got := m.logPickLabel(); got != "picked r50 ↔ r49" {
		t.Errorf("label = %q, want both revisions in pick order", got)
	}
	if bar := stripANSI(m.bar.View()); !strings.Contains(bar, "picked r50 ↔ r49") || !strings.Contains(bar, "esc clear") {
		t.Errorf("status bar should report the picks and how to drop them, got:\n%s", bar)
	}
}
