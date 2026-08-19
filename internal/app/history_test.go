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

// pressEsc unwinds a step. A drilled-in view is popped by the container, which
// reports it as a message the model then acts on, so both steps are taken here.
func pressEsc(t *testing.T, m *Model) *Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if cmd == nil {
		return m
	}
	next, _ = m.Update(cmd())
	return next.(*Model)
}

// drilledModel picks two revisions, opens their diff and lands the patch, so the
// Log panel is showing the tree of files the range touched.
func drilledModel(t *testing.T, diff string) *Model {
	t.Helper()
	return drillInto(t, logPagedModel(t), diff)
}

// drillInto picks r50 and r49 on a model whose Log panel is loaded and focused,
// then lands diff as their comparison.
func drillInto(t *testing.T, m *Model, diff string) *Model {
	t.Helper()
	m.logPicks = []string{"50", "49"}
	m, _ = pressEnter(t, m)
	next, _ := m.Update(revDiffLoadedMsg{rng: m.revDiff, diff: diff, gen: m.gens.revDiff.gen})
	return next.(*Model)
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
	m := drilledModel(t, "Index: a.go\n@@ -1 +1 @@\n+new")

	m = pressEsc(t, m)
	if m.inRevDrill() || m.revDiff.set() {
		t.Fatal("the first esc should come back out of the drill")
	}
	if len(m.logPicks) != 2 {
		t.Fatalf("picks = %q, want them still held for another look", m.logPicks)
	}

	m = pressEsc(t, m)
	if len(m.logPicks) != 0 {
		t.Errorf("picks = %q, want the second esc to let them go", m.logPicks)
	}
}

// The revisions are out of reach behind the tree, so the keys that act on them
// must do nothing rather than act on whichever row the hidden table left under
// its cursor.
func TestRevisionKeysAreInertWhileDrilled(t *testing.T) {
	m := drilledModel(t, "Index: a.go\n@@ -1 +1 @@\n+new")
	picks, page := len(m.logPicks), m.logPage

	for _, k := range []rune{'v', 'n', 'p', ' '} {
		m, _ = pressRune(t, m, k)
	}
	if len(m.logPicks) != picks {
		t.Errorf("picks = %q, want v inert while drilled", m.logPicks)
	}
	if m.logPage != page {
		t.Errorf("page = %d, want n/p inert while drilled", m.logPage)
	}
	if m.pending != nil {
		t.Error("space should not offer to update the working copy while drilled")
	}
	if !m.inRevDrill() {
		t.Error("none of those keys should have left the drill")
	}
}

func TestRangeDiffIsServedFromTheSession(t *testing.T) {
	m := drilledModel(t, "Index: a.go\n@@ -1 +1 @@\n+new")

	// Revisions are immutable, so looking at the same comparison again is free.
	m = pressEsc(t, m)
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
	m = pressEsc(t, m)
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
	if m.inRevDrill() {
		t.Error("the drill belongs to the tree being left too")
	}
}

// The tree lists files as a range of history left them, not as the working copy
// holds them now, so nothing that would act on a working-copy file may fire from
// it. w is the exception and is covered in the save tests.
func TestMutatingKeysAreInertInTheRevisionTree(t *testing.T) {
	m := drilledModel(t, treePatch)
	m.revFiles.SetIndex(indexOfNodePath(t, m, "src/a.go"))

	for _, k := range []rune{'c', 'r', 'd', 'm', 'n', 's', 'e', 'u', ' '} {
		m, _ = pressRune(t, m, k)
		switch {
		case m.editing:
			t.Fatalf("%q opened the commit editor", k)
		case m.naming:
			t.Fatalf("%q opened the changelist prompt", k)
		case m.savingDiff:
			t.Fatalf("%q opened the save prompt", k)
		case m.pending != nil:
			t.Fatalf("%q queued an action on the working copy", k)
		case m.splitting:
			t.Fatalf("%q opened the side-by-side overlay", k)
		case m.merging:
			t.Fatalf("%q opened the resolution overlay", k)
		case !m.inRevDrill():
			t.Fatalf("%q left the drill", k)
		}
	}
}

// treePatch touches two directories and the top level, so the tree has something
// to fold and a directory row has more than one file beneath it.
const treePatch = `Index: readme.md
===================================================================
--- readme.md	(revision 49)
+++ readme.md	(revision 50)
@@ -1 +1 @@
-old
+new
Index: src/a.go
===================================================================
--- src/a.go	(nonexistent)
+++ src/a.go	(revision 50)
@@ -0,0 +1 @@
+alpha
Index: src/b.go
===================================================================
--- src/b.go	(revision 49)
+++ src/b.go	(nonexistent)
@@ -1 +0,0 @@
-beta
`

func TestEnterDrillsIntoTheFilesTheRangeTouched(t *testing.T) {
	m := drilledModel(t, treePatch)

	if !m.inRevDrill() {
		t.Fatal("enter should drill the Log panel into the range's files")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"r49 → r50", "readme.md", "src/", "a.go", "b.go"} {
		if !strings.Contains(view, want) {
			t.Errorf("the drill should show %q, got:\n%s", want, view)
		}
	}
	// The codes come from the diff, so they say what the range did rather than
	// what the working copy holds now.
	if !strings.Contains(view, "A a.go") || !strings.Contains(view, "D b.go") {
		t.Errorf("expected the add and the delete to be marked, got:\n%s", view)
	}
}

func TestDrilledTreeDrivesMain(t *testing.T) {
	m := drilledModel(t, treePatch)

	// Asserted on Main's content rather than its view: a patch longer than the
	// viewport is windowed on screen, and what is off the bottom still counts.
	if main := stripANSI(m.mainText); !strings.Contains(main, "+alpha") || !strings.Contains(main, "+new") {
		t.Errorf("the root row should show the whole range, got:\n%s", main)
	}

	// A file row shows its own section and nothing else.
	m.revFiles.SetIndex(indexOfNodePath(t, m, "readme.md"))
	m.updateMain()
	main := stripANSI(m.mainText)
	if !strings.Contains(main, "+new") || strings.Contains(main, "+alpha") {
		t.Errorf("a file row should show only its own section, got:\n%s", main)
	}
	if !strings.Contains(main, "readme.md") {
		t.Errorf("the heading should name the path being read, got:\n%s", main)
	}

	// A directory row concatenates everything beneath it.
	m.revFiles.SetIndex(indexOfNodePath(t, m, "src"))
	m.updateMain()
	main = stripANSI(m.mainText)
	if !strings.Contains(main, "+alpha") || !strings.Contains(main, "-beta") {
		t.Errorf("a directory row should span its subtree, got:\n%s", main)
	}
	if strings.Contains(main, "+new") {
		t.Errorf("a directory row should not reach outside itself, got:\n%s", main)
	}
}

func TestDrilledTreeFolds(t *testing.T) {
	m := drilledModel(t, treePatch)
	m.revFiles.SetIndex(indexOfNodePath(t, m, "src"))

	// Asserted on the rows rather than the screen: Main still shows the folded
	// directory's patch, which names the very files the tree has put away.
	m, _ = pressEnter(t, m)
	if hasNodePath(m, "src/a.go") {
		t.Errorf("enter on a directory should fold it away, got %+v", m.revFiles.Items())
	}
	if !hasNodePath(m, "readme.md") {
		t.Error("folding one directory should leave the rest of the tree alone")
	}

	m, _ = pressEnter(t, m)
	if !hasNodePath(m, "src/a.go") {
		t.Errorf("enter again should unfold it, got %+v", m.revFiles.Items())
	}
}

// hasNodePath reports whether the drilled tree currently shows a row for path.
func hasNodePath(m *Model, path string) bool {
	for _, n := range m.revFiles.Items() {
		if n.Path == path {
			return true
		}
	}
	return false
}

// The panel shows the range's files while drilled, so that is what the filter
// has to narrow — not the revisions hidden behind them.
func TestFilterNarrowsTheDrilledTree(t *testing.T) {
	m := drilledModel(t, treePatch)

	// The input names what it is about to filter, which is no longer revisions.
	m, _ = pressRune(t, m, '/')
	if got := stripANSI(m.searchBar.View()); !strings.Contains(got, "filter files") || strings.Contains(got, "user:") {
		t.Errorf("filter input reads %q, want it offering what a file row carries", got)
	}
	m = pressEsc(t, m)

	m.setFilter(panelLog, "a.go")
	if !hasNodePath(m, "src/a.go") {
		t.Errorf("the matching file should stay, got %+v", m.revFiles.Items())
	}
	if hasNodePath(m, "readme.md") || hasNodePath(m, "src/b.go") {
		t.Errorf("everything else should be narrowed away, got %+v", m.revFiles.Items())
	}
	// The revisions behind the tree are left alone.
	if len(m.log.Items()) != 2 {
		t.Errorf("log rows = %d, want the revisions untouched by a filter meant for the tree", len(m.log.Items()))
	}
	// The footer counts what is shown against everything the range touched.
	m.revFiles.SetIndex(indexOfNodePath(t, m, "src/a.go"))
	if got := m.logFooter(); got != "1 of 1 (3)" {
		t.Errorf("footer = %q, want the hidden files counted", got)
	}

	// The state the diff reported is filterable too. Spelled out rather than as
	// "D", which stateMatches also reads as a substring of "added".
	m.setFilter(panelLog, "state:deleted")
	if !hasNodePath(m, "src/b.go") {
		t.Errorf("the deleted file should stay, got %+v", m.revFiles.Items())
	}
	if hasNodePath(m, "src/a.go") || hasNodePath(m, "readme.md") {
		t.Errorf("only the deleted file should stay, got %+v", m.revFiles.Items())
	}
}

// The two filters are written in different terms — authors and dates against
// paths and states — so neither may be left standing over the other.
func TestDrillHandsTheFilterOverAndBack(t *testing.T) {
	m := logPagedModel(t)
	m.setFilter(panelLog, "user:alice")

	m = drillInto(t, m, treePatch)
	if got := m.filters[panelLog]; got != "" {
		t.Errorf("filter = %q, want the revisions' own filter set aside on the way in", got)
	}
	if !hasNodePath(m, "readme.md") {
		t.Error("the tree should open on everything the range touched")
	}

	m = pressEsc(t, m)
	if got := m.filters[panelLog]; got != "user:alice" {
		t.Errorf("filter = %q, want it given back to the revisions", got)
	}
	if items := m.log.Items(); len(items) != 1 || items[0].Revision != "50" {
		t.Errorf("log rows = %+v, want the restored filter applied", items)
	}
}

// indexOfNodePath finds the drilled tree row for a path, so a test can move the
// cursor by name rather than by a row number the tree shape decides.
func indexOfNodePath(t *testing.T, m *Model, path string) int {
	t.Helper()
	for i, n := range m.revFiles.Items() {
		if n.Path == path {
			return i
		}
	}
	t.Fatalf("no row for %q in %+v", path, m.revFiles.Items())
	return 0
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
