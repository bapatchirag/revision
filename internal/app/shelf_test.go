package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/shelf"
	"github.com/bapatchirag/revision/internal/svn"
)

// errShelfProbe stands in for a disk failure the panel has to report rather than
// let tear the app down.
var errShelfProbe = errors.New("probe")

// seedShelves puts entries into the model as a completed scan would.
func seedShelves(t *testing.T, m *Model, entries []shelf.Entry) *Model {
	t.Helper()
	next, _ := m.Update(shelvesLoadedMsg{entries: entries})
	return next.(*Model)
}

// shelfEntry builds an entry with a name and a fixed creation time, so rows
// render the same on every run.
func shelfEntry(id, name string, files int) shelf.Entry {
	recs := make([]shelf.FileRec, files)
	for i := range recs {
		recs[i] = shelf.FileRec{Path: "src/a.go", State: "modified"}
	}
	return shelf.Entry{
		ID:      id,
		Name:    name,
		Created: time.Date(2026, 8, 19, 14, 25, 0, 0, time.UTC),
		Files:   recs,
	}
}

// focusShelf moves focus onto the Shelf panel the way pressing 4 does.
func focusShelf(t *testing.T, m *Model) *Model {
	t.Helper()
	next, _ := pressRune(t, m, '4')
	return next
}

func TestShelfPanelIsFocusedByItsNumber(t *testing.T) {
	m := focusShelf(t, sizedModel(t))

	if got := m.focus.Index(); got != panelShelf {
		t.Fatalf("focused panel = %d, want panelShelf (%d)", got, panelShelf)
	}
	if m.source != sourceShelf {
		t.Errorf("source = %v, want sourceShelf", m.source)
	}
}

func TestShelfPanelIsInTheTabRing(t *testing.T) {
	m := sizedModel(t)
	seen := map[int]bool{}
	for range len(sidePanels) {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(*Model)
		seen[m.focus.Index()] = true
	}
	if !seen[panelShelf] {
		t.Errorf("Tab visited %v, want it to reach panelShelf (%d)", seen, panelShelf)
	}
}

func TestShelfPanelGrowsWhenFocusedAtTheLogPanelsExpense(t *testing.T) {
	m := sizedModel(t)
	before := m.panelRects()

	m = focusShelf(t, m)
	after := m.panelRects()

	if after[panelShelf].h <= before[panelShelf].h {
		t.Errorf("shelf height = %d focused, want more than %d", after[panelShelf].h, before[panelShelf].h)
	}
	if after[panelLog].h >= before[panelLog].h {
		t.Errorf("log height = %d, want it to give up rows from %d", after[panelLog].h, before[panelLog].h)
	}
	// The Files panel is not what the shelf takes room from, so it must not move.
	if after[panelFiles] != before[panelFiles] {
		t.Errorf("files rect = %+v, want it unchanged at %+v", after[panelFiles], before[panelFiles])
	}
	if got, want := after[panelShelf].h+after[panelLog].h, before[panelShelf].h+before[panelLog].h; got != want {
		t.Errorf("shelf+log = %d rows focused, want the same %d they shared before", got, want)
	}
}

func TestShelfPanelStaysExpandedWhileMainIsRead(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	expanded := m.panelRects()[panelShelf].h

	// Focusing Main is how a shelved patch is scrolled, so the list it was
	// picked from has to stay where it is.
	next, _ := pressRune(t, m, '0')
	m = next
	if got := m.focus.Index(); got != panelMain {
		t.Fatalf("focused panel = %d, want panelMain (%d)", got, panelMain)
	}
	if got := m.panelRects()[panelShelf].h; got != expanded {
		t.Errorf("shelf height = %d with Main focused, want it left at %d", got, expanded)
	}
}

func TestShelfPanelStaysExpandedWhileTheCommandLogIsRead(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	expanded := m.panelRects()[panelShelf].h

	// The command log is only reachable by clicking it, which is what focusing it
	// and settling the layout comes down to.
	m.focus.Focus(panelCmdLog)
	m.afterFocusChange()

	if got := m.panelRects()[panelShelf].h; got != expanded {
		t.Errorf("shelf height = %d with the command log focused, want it left at %d", got, expanded)
	}
}

func TestShelfPanelCollapsesWhenAnotherSidePanelTakesOver(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	expanded := m.panelRects()[panelShelf].h

	next, _ := pressRune(t, m, '2')
	m = next
	if got := m.panelRects()[panelShelf].h; got >= expanded {
		t.Errorf("shelf height = %d with the Files panel focused, want less than %d", got, expanded)
	}
}

func TestShelfPanelStaysCollapsedWhenMainFollowsAnotherPanel(t *testing.T) {
	m := sizedModel(t)
	collapsed := m.panelRects()[panelShelf].h

	// Main is showing a file diff here, not a shelved one, so there is no list
	// behind it worth keeping open.
	next, _ := pressRune(t, m, '0')
	m = next
	if got := m.panelRects()[panelShelf].h; got != collapsed {
		t.Errorf("shelf height = %d, want it left collapsed at %d", got, collapsed)
	}
}

func TestShelfPanelLeavesTheLogPanelRoomOnAShortTerminal(t *testing.T) {
	for _, height := range []int{10, 14, 18, 24, 40} {
		m := sizedModel(t)
		next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: height})
		m = focusShelf(t, next.(*Model))

		r := m.panelRects()
		if r[panelLog].h < minPanelHeight {
			t.Errorf("height %d: log height = %d, want at least %d", height, r[panelLog].h, minPanelHeight)
		}
		total := r[panelStatus].h + r[panelFiles].h + r[panelLog].h + r[panelShelf].h
		if want := max(height-barHeight, 3); total != want {
			t.Errorf("height %d: left column = %d rows, want %d", height, total, want)
		}
	}
}

func TestShelfPanelListsScannedEntries(t *testing.T) {
	m := seedShelves(t, sizedModel(t), []shelf.Entry{
		shelfEntry("20260819-1", "wip refactor", 3),
		shelfEntry("20260819-0", "spike", 1),
	})

	view := stripANSI(m.View())
	if !strings.Contains(view, "wip refactor") {
		t.Errorf("view does not list the shelved entry:\n%s", view)
	}
	if got := len(m.shelves.Items()); got != 2 {
		t.Errorf("shelf list holds %d entries, want 2", got)
	}
}

func TestShelfPanelWithNothingShelved(t *testing.T) {
	m := focusShelf(t, sizedModel(t))

	if got := m.shelfDetail(); got != "Nothing shelved yet." {
		t.Errorf("shelfDetail = %q, want the empty-store notice", got)
	}
}

func TestShelfEntryWithoutANameIsListedByItsID(t *testing.T) {
	if got := shelfLabel(shelf.Entry{ID: "20260819-142530-ab12"}); got != "20260819-142530-ab12" {
		t.Errorf("shelfLabel = %q, want the identifier", got)
	}
	if got := shelfLabel(shelf.Entry{ID: "x", Name: "  named  "}); got != "named" {
		t.Errorf("shelfLabel = %q, want the trimmed name", got)
	}
}

func TestShelfSizeCountsTheUntrackedFilesItCarries(t *testing.T) {
	e := shelf.Entry{
		Files:     []shelf.FileRec{{Path: "a.go"}, {Path: "b.go"}},
		Untracked: []string{"docs/new.md"},
	}
	if got := shelfSize(e); got != 3 {
		t.Errorf("shelfSize = %d, want 3", got)
	}
}

func TestShelfSelectionReadsThePatchIntoMain(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	m = seedShelves(t, m, []shelf.Entry{shelfEntry("20260819-1", "wip", 1)})

	// Until the patch is read, Main says so rather than showing nothing.
	if got := m.shelfDetail(); !strings.Contains(got, "Reading wip") {
		t.Errorf("shelfDetail = %q, want it to report the read in progress", got)
	}

	next, _ := m.Update(shelfReadMsg{id: "20260819-1", text: "Index: a.go\n+new\n"})
	m = next.(*Model)
	if got := stripANSI(m.shelfDetail()); !strings.Contains(got, "Index: a.go") {
		t.Errorf("shelfDetail = %q, want the patch that was read", got)
	}
	if !m.shelfShowsPatch() {
		t.Error("a read patch should turn the diff gutter on")
	}
}

func TestShelfReadFailureIsReportedNotFatal(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	m = seedShelves(t, m, []shelf.Entry{shelfEntry("20260819-1", "wip", 1)})

	next, _ := m.Update(shelfReadMsg{id: "20260819-1", err: errShelfProbe})
	m = next.(*Model)

	if m.err != nil {
		t.Errorf("model error = %v, want an unreadable entry to stay local to the panel", m.err)
	}
	if got := m.shelfDetail(); !strings.Contains(got, "Unable to read shelf") {
		t.Errorf("shelfDetail = %q, want the read failure reported", got)
	}
	if m.shelfShowsPatch() {
		t.Error("a failure notice is not a patch and must not turn the gutter on")
	}
}

func TestShelfScanFailureIsReported(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	next, _ := m.Update(shelvesLoadedMsg{err: errShelfProbe})
	m = next.(*Model)

	if got := m.shelfDetail(); !strings.Contains(got, "Unable to list shelves") {
		t.Errorf("shelfDetail = %q, want the scan failure reported", got)
	}
}

func TestShelfPanelFilterNarrowsTheList(t *testing.T) {
	m := seedShelves(t, focusShelf(t, sizedModel(t)), []shelf.Entry{
		shelfEntry("20260819-1", "wip refactor", 1),
		shelfEntry("20260819-0", "spike", 1),
	})

	m.setFilter(panelShelf, "spike")
	if got := len(m.shelves.Items()); got != 1 {
		t.Fatalf("filtered list holds %d entries, want 1", got)
	}
	if got, _ := m.shelves.Selected(); got.Name != "spike" {
		t.Errorf("selected = %q, want spike", got.Name)
	}
	if got := m.shelfFooter(); !strings.Contains(got, "2") {
		t.Errorf("footer = %q, want it to report the unfiltered total", got)
	}

	m.setFilter(panelShelf, "")
	if got := len(m.shelves.Items()); got != 2 {
		t.Errorf("cleared filter holds %d entries, want 2", got)
	}
}

func TestShelfStoreLivesAtTheWorkingCopyRoot(t *testing.T) {
	m := sizedModel(t)
	if got, want := m.shelfDir(), "/home/alice/work/wc/"+shelf.DirName; got != want {
		t.Errorf("shelfDir = %q, want %q", got, want)
	}
}

func TestShelfPanelReadableWhileTheWorkingCopyIsLoading(t *testing.T) {
	// The store is local disk, so it is browsable before svn has answered.
	m := focusShelf(t, sizedModel(t))
	m.loading = true
	m = seedShelves(t, m, []shelf.Entry{shelfEntry("20260819-1", "wip", 1)})

	if got := m.mainContent(); strings.Contains(got, "Loading working-copy status") {
		t.Errorf("mainContent = %q, want the shelf shown rather than the loading notice", got)
	}
}

func TestShelfPanelGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "loose.txt", State: svn.StateModified}})
	m = seedShelves(t, m, []shelf.Entry{
		shelfEntry("20260819-142530-a1b2", "wip refactor", 3),
		shelfEntry("20260819-091500-c3d4", "spike", 1),
	})
	m = focusShelf(t, m)
	next, _ := m.Update(shelfReadMsg{
		id:   "20260819-142530-a1b2",
		text: "Index: cmd/main.go\n@@ -1,3 +1,4 @@\n func main() {\n+\tsetup()\n \trun()\n }\n",
	})
	golden.RequireEqual(t, []byte(next.(*Model).View()))
}

func TestShelvePicksGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "cmd/main.go", State: svn.StateModified},
		{Path: "internal/server/handler.go", State: svn.StateAdded},
		{Path: "internal/server/server.go", State: svn.StateModified},
		{Path: "notes.txt", State: svn.StateUnversioned},
	})
	selectDirRow(t, m, "internal")
	m, _ = pressRune(t, m, 'v')
	golden.RequireEqual(t, []byte(m.View()))
}
