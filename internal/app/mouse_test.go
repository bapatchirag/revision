package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/shelf"
	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// click is a left button press at a screen cell.
func click(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

// mouseModel is a sized model that reads the pointer, which is off by default.
func mouseModel(t testing.TB) *Model {
	t.Helper()
	cfg := config.Default()
	cfg.AllowMouse = true
	return sizedModelCfg(t, cfg)
}

func TestTheMouseIsIgnoredUntilItIsTurnedOn(t *testing.T) {
	m := sizedModel(t)
	if m.cfg.AllowMouse {
		t.Fatal("the mouse is off unless the config says otherwise")
	}
	before := m.focus.Index()

	next, _ := m.Update(click(5, 18))
	m = next.(*Model)
	if got := m.focus.Index(); got != before {
		t.Errorf("focus moved to %d with the mouse off, want it left on %d", got, before)
	}
}

// The coordinates below are read off sizedModel's 80x24 screen: the left column
// is 32 wide and stacks Status (rows 0-7), Files (8-14) and Log (15-22); the
// right column holds Main (rows 0-16) over the command log (17-22); the bar has
// the last row. Hard-coding them means the layout cannot drift unnoticed.
func TestClickFocusesThePanelUnderThePointer(t *testing.T) {
	cases := []struct {
		name string
		x, y int
		want int
	}{
		{"status", 5, 2, panelStatus},
		{"files", 5, 10, panelFiles},
		{"log", 5, 18, panelLog},
		{"main", 40, 5, panelMain},
		{"command log", 40, 20, panelCmdLog},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := mouseModel(t)
			next, _ := m.Update(click(tc.x, tc.y))
			m = next.(*Model)
			if got := m.focus.Index(); got != tc.want {
				t.Errorf("click at (%d,%d) focused panel %d, want %d", tc.x, tc.y, got, tc.want)
			}
		})
	}
}

func TestClickBelowThePanelsLeavesFocusAlone(t *testing.T) {
	m := mouseModel(t)
	before := m.focus.Index()
	// Row 23 is the status bar, which belongs to no panel.
	next, _ := m.Update(click(10, 23))
	m = next.(*Model)
	if got := m.focus.Index(); got != before {
		t.Errorf("focus moved to %d, want it left on %d", got, before)
	}
}

func TestHidingTheCommandLogGivesItsRowsToMain(t *testing.T) {
	m := mouseModel(t)
	m, _ = pressRune(t, m, 'x')
	if m.showCmdLog {
		t.Fatal("x should hide the command log")
	}
	next, _ := m.Update(click(40, 20))
	m = next.(*Model)
	if got := m.focus.Index(); got != panelMain {
		t.Errorf("focused panel %d, want Main to have taken the vacated rows", got)
	}
}

func TestOnlyALeftPressMovesFocus(t *testing.T) {
	ignored := map[string]tea.MouseMsg{
		"release":     {X: 5, Y: 18, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft},
		"drag":        {X: 5, Y: 18, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft},
		"wheel":       {X: 5, Y: 18, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown},
		"right click": {X: 5, Y: 18, Action: tea.MouseActionPress, Button: tea.MouseButtonRight},
	}
	for name, msg := range ignored {
		t.Run(name, func(t *testing.T) {
			m := mouseModel(t)
			before := m.focus.Index()
			next, _ := m.Update(msg)
			m = next.(*Model)
			if got := m.focus.Index(); got != before {
				t.Errorf("focus moved to %d, want it left on %d", got, before)
			}
		})
	}
}

func TestClicksAreIgnoredWhileAnOverlayIsOpen(t *testing.T) {
	m := mouseModel(t)
	m, _ = pressRune(t, m, '?')
	if !m.helping {
		t.Fatal("? should open the help menu")
	}
	before := m.focus.Index()
	next, _ := m.Update(click(5, 18))
	m = next.(*Model)
	if !m.helping {
		t.Error("a click should not close the help menu")
	}
	if got := m.focus.Index(); got != before {
		t.Errorf("focus moved to %d behind the overlay, want it left on %d", got, before)
	}
}

func TestClickingAViewNameSelectsItAndItsPanel(t *testing.T) {
	m := mouseModel(t)
	m, _ = pressRune(t, m, ']')
	if name := m.filesViews.ActiveName(); name != "Changelists" {
		t.Fatalf("active files view = %q, want ] to have moved off Changes", name)
	}
	m, _ = pressRune(t, m, '3')
	if m.focus.Index() != panelLog {
		t.Fatal("3 should focus the Log panel")
	}

	next, _ := m.Update(click(tabColumn(t, m, "Changes"), filesTop))
	m = next.(*Model)
	if name := m.filesViews.ActiveName(); name != "Changes" {
		t.Errorf("active files view = %q, want the clicked one", name)
	}
	if got := m.focus.Index(); got != panelFiles {
		t.Errorf("focused panel %d, want the clicked view's panel", got)
	}
}

// filesTop is the screen row of the Files panel's top border, where it inlays
// its view names. Its rows are the lines below it.
const filesTop = 8

// logTop and mainLeft are the Log panel's top border row and the Main panel's
// left border column on the same 80x24 screen.
const (
	logTop   = 15
	mainLeft = 32
)

// TestClickingARowSelectsIt is the pointer's half of "navigate to a row": the
// same SelectedMsg the arrow keys emit, which is what loads the diff for it.
func TestClickingARowSelectsIt(t *testing.T) {
	m := loadItems(t, mouseModel(t), []svn.StatusItem{
		{Path: "alpha.go", State: svn.StateModified},
		{Path: "beta.go", State: svn.StateModified},
		{Path: "gamma.go", State: svn.StateModified},
	})
	if m.focus.Index() != panelFiles {
		t.Fatal("the Files panel starts focused")
	}

	// The tree's first row is the working-copy root, so the three files follow it.
	next, cmd := m.Update(click(4, filesTop+4))
	m = next.(*Model)
	n, ok := m.files.Selected()
	if !ok || n.Path != "gamma.go" {
		t.Errorf("selection = %+v (ok=%v), want the clicked row", n, ok)
	}
	sel, ok := cmd().(uimsg.SelectedMsg)
	if !ok {
		t.Fatalf("expected the click to report a selection, got %T", cmd())
	}
	if sel.ID != "files" || sel.Index != 3 {
		t.Errorf("got %+v, want {files 3}", sel)
	}
}

// tabColumn returns the column that border row starts name at. Every rune in it
// is one cell wide, so a rune offset is a column.
func tabColumn(t *testing.T, m *Model, name string) int {
	t.Helper()
	rows := strings.Split(stripANSI(m.View()), "\n")
	// The right-hand column shares the row, so only the Files panel is searched.
	border := []rune(rows[filesTop])[:32]
	for i := range border {
		if strings.HasPrefix(string(border[i:]), name) {
			return i
		}
	}
	t.Fatalf("the Files panel should name %q in its border, got %q", name, string(border))
	return 0
}

// doubleClick sends two presses on one cell, close enough together to count.
func doubleClick(t *testing.T, m *Model, x, y int) (*Model, tea.Cmd) {
	t.Helper()
	next, _ := m.Update(click(x, y))
	next, cmd := next.(*Model).Update(click(x, y))
	return next.(*Model), cmd
}

// fileRow returns the screen row the Changes tree draws path on.
func fileRow(t *testing.T, m *Model, path string) int {
	t.Helper()
	for i, n := range m.files.Items() {
		if n.Path == path {
			return filesTop + 1 + i
		}
	}
	t.Fatalf("no row for %q in the Changes tree", path)
	return 0
}

func TestDoubleClickingADirectoryFoldsIt(t *testing.T) {
	m := loadItems(t, mouseModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})

	m, _ = doubleClick(t, m, 4, fileRow(t, m, "src"))
	if !m.collapsedDirs["src"] {
		t.Fatal("a double click on a directory row should fold it")
	}
	m, _ = doubleClick(t, m, 4, fileRow(t, m, "src"))
	if m.collapsedDirs["src"] {
		t.Error("a second double click should unfold it again")
	}
}

func TestDoubleClickingAFileOpensTheEditor(t *testing.T) {
	cfg := config.Default()
	cfg.AllowMouse = true
	cfg.Editor = config.EditorVim
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
	})
	// With nothing on PATH the editor cannot be resolved, so the attempt says so
	// instead of launching one — which names the file it was going to open.
	fakeBins(t)

	m, _ = doubleClick(t, m, 4, fileRow(t, m, "src/a.go"))
	if view := stripANSI(m.View()); !strings.Contains(view, "can't open src/a.go") {
		t.Errorf("a double click on a file should open it in the editor, got:\n%s", view)
	}
}

func TestDoubleClickingARevisionAsksToUpdateToIt(t *testing.T) {
	m := loadItems(t, mouseModel(t), nil)
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{
		{Revision: "42", Author: "alice", Message: "first"},
		{Revision: "41", Author: "bob", Message: "second"},
	}})
	m = next.(*Model)

	// The Log panel's first row is its header, so r41 is the second row under it.
	m, _ = doubleClick(t, m, 4, logTop+2)
	if !m.confirming {
		t.Fatal("a double click on a revision should ask before moving the working copy")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Update to revision?") || !strings.Contains(view, "r41") {
		t.Errorf("expected the prompt to name the double-clicked revision, got:\n%s", view)
	}
}

func TestDoubleClickingAChangelistOpensIt(t *testing.T) {
	m := loadItems(t, mouseModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, _ := m.Update(click(tabColumn(t, m, "Changelist"), filesTop))
	m = next.(*Model)
	if !m.filesViewIsChangelists() {
		t.Fatal("the Changelists view should be showing")
	}

	m, _ = doubleClick(t, m, 4, filesTop+1)
	if m.filesViews.Depth() != 1 {
		t.Errorf("drill depth = %d, want the double-clicked changelist opened", m.filesViews.Depth())
	}
}

func TestDoubleClickingADiffLineAimsTheEditorAtIt(t *testing.T) {
	m := loadItems(t, mouseModel(t), []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	next, _ := m.Update(diffLoadedMsg{path: "src/a.go", diff: scrollableDiff()})
	m = next.(*Model)

	// Row 8 of the diff is the second hunk's header, which starts at file line 201.
	m, _ = doubleClick(t, m, mainLeft+2, 1+8)
	if got := m.main.Cursor(); got != 8 {
		t.Fatalf("the diff cursor is on row %d, want the double-clicked one", got)
	}
	if got := editLine(t, m); got != 201 {
		t.Errorf("line = %d, want 201 — the hunk the click landed in", got)
	}
}

// shelfRow returns the screen row the Shelf panel draws its i'th listed entry
// on. The panel moves as it grows and shrinks with focus, so it is read off the
// layout rather than hard-coded like the panels above it.
func shelfRow(m *Model, i int) int { return m.panelRects()[panelShelf].y + 1 + i }

// shelfModel is a mouse-reading model with entries on the shelf and the panel
// focused, which is what opens it past the single row it shows collapsed.
func shelfModel(t *testing.T) *Model {
	t.Helper()
	m := focusShelf(t, mouseModel(t))
	return seedShelves(t, m, []shelf.Entry{
		shelfEntry("a1", "alpha", 1),
		shelfEntry("b2", "beta", 2),
		shelfEntry("c3", "gamma", 3),
		shelfEntry("d4", "delta", 4),
	})
}

func TestClickingAShelfHighlightsIt(t *testing.T) {
	m := shelfModel(t)

	next, cmd := m.Update(click(4, shelfRow(m, 2)))
	m = next.(*Model)
	e, ok := m.shelves.Selected()
	if !ok || e.ID != "c3" {
		t.Errorf("selection = %+v (ok=%v), want the clicked entry", e, ok)
	}
	sel, ok := cmd().(uimsg.SelectedMsg)
	if !ok {
		t.Fatalf("expected the click to report a selection, got %T", cmd())
	}
	if sel.ID != shelfListID || sel.Index != 2 {
		t.Errorf("got %+v, want {shelf 2}", sel)
	}
}

func TestClickingTheShelfPanelFocusesItAndDrivesMain(t *testing.T) {
	m := mouseModel(t)
	m = seedShelves(t, m, []shelf.Entry{shelfEntry("a1", "alpha", 1)})
	if m.focus.Index() == panelShelf {
		t.Fatal("the Files panel starts focused")
	}

	next, _ := m.Update(click(4, shelfRow(m, 0)))
	m = next.(*Model)
	if got := m.focus.Index(); got != panelShelf {
		t.Errorf("focused panel %d, want the clicked one (%d)", got, panelShelf)
	}
	if m.source != sourceShelf {
		t.Errorf("source = %v, want the Shelf panel driving Main", m.source)
	}
}

func TestDoubleClickingAShelfAsksToApplyIt(t *testing.T) {
	m := shelfModel(t)

	m, _ = doubleClick(t, m, 4, shelfRow(m, 1))
	if !m.confirming {
		t.Fatal("a double click on a shelf should ask before merging it back")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Apply shelf?") || !strings.Contains(view, "beta") {
		t.Errorf("expected the apply prompt to name the double-clicked shelf, got:\n%s", view)
	}
	// Applying keeps the entry; popping is p's, and is not what a stray click does.
	if !strings.Contains(view, "keeping it on the shelf") {
		t.Errorf("expected the prompt to apply rather than pop, got:\n%s", view)
	}
}

// The Shelf panel grows into the Log panel's rows as the first click focuses it,
// so the second click of a pair lands on a different entry than the cell it
// repeats. Judging the pair by the row rather than the screen cell is what keeps
// a double click from applying a shelf nobody pointed at.
func TestDoubleClickingTheCollapsedShelfActsOnTheRowUnderThePointer(t *testing.T) {
	m := mouseModel(t)
	m = seedShelves(t, m, []shelf.Entry{
		shelfEntry("a1", "alpha", 1),
		shelfEntry("b2", "beta", 2),
		shelfEntry("c3", "gamma", 3),
		shelfEntry("d4", "delta", 4),
	})
	cell := shelfRow(m, 0)

	m, _ = doubleClick(t, m, 4, cell)
	if m.confirming {
		t.Fatal("the panel moved between the two clicks, so they are not a pair")
	}
	e, _ := m.shelves.Selected()
	moved := e.ID

	next, _ := m.Update(click(4, cell))
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("a second click on the settled row should complete the pair")
	}
	e, _ = m.shelves.Selected()
	if e.ID != moved {
		t.Errorf("applied %q, want the entry under the pointer (%q)", e.ID, moved)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, shelfLabel(e)) {
		t.Errorf("expected the prompt to name %q, got:\n%s", shelfLabel(e), view)
	}
}

func TestDoubleClickingAShelvedDiffOpensNoFile(t *testing.T) {
	m := loadItems(t, mouseModel(t), []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	m = seedShelves(t, focusShelf(t, m), []shelf.Entry{shelfEntry("20260819-1", "wip", 1)})
	next, _ := m.Update(shelfReadMsg{id: "20260819-1", text: scrollableDiff()})
	m = next.(*Model)
	// Main is focused to scroll the patch, which is where a diff line is clicked.
	m, _ = pressRune(t, m, '0')
	if !m.shelfShowsPatch() {
		t.Fatal("the shelved patch should be what Main is showing")
	}

	m, _ = doubleClick(t, m, mainLeft+2, 1+8)
	if got := m.main.Cursor(); got != 8 {
		t.Fatalf("the diff cursor is on row %d, want the double-clicked one", got)
	}
	if path, _, _, ok := m.editTarget(); ok {
		t.Errorf("a shelved patch named %q to open, want nothing — it describes files the working copy does not hold", path)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no file to open here") {
		t.Errorf("expected the open to be refused, got:\n%s", view)
	}
}

// settingsModel is a mouse-reading model with the settings editor open.
func settingsModel(t *testing.T) *Model {
	t.Helper()
	m, _ := pressRune(t, mouseModel(t), 'S')
	if !m.configuring {
		t.Fatal("S should open the settings editor")
	}
	return m
}

// settingIndex is the position of the row labeled label in the settings editor.
func settingIndex(t *testing.T, m *Model, label string) int {
	t.Helper()
	for i, f := range m.form.Fields() {
		if f.Label == label {
			return i
		}
	}
	t.Fatalf("the settings editor has no %q row", label)
	return 0
}

// settingCell is a screen cell inside the row the settings editor draws field i
// on. The editor is centered on whatever the terminal is, so it is read off the
// same placement the view uses rather than hard-coded.
func settingCell(m *Model, i int) (x, y int) {
	r := m.overlayRect(m.form.View())
	return r.x + 2, r.y + 1 + i
}

func TestClickingASettingHighlightsIt(t *testing.T) {
	m := settingsModel(t)

	x, y := settingCell(m, themeFieldIndex)
	next, _ := m.Update(click(x, y))
	m = next.(*Model)

	if view := stripANSI(m.View()); !strings.Contains(view, "> Theme") {
		t.Errorf("expected the clicked row to be the active one, got:\n%s", view)
	}
	// Moving between fields is not picking one, so the palette must sit still.
	if m.theme != theme.Auto() {
		t.Error("clicking a row should not preview a theme")
	}
}

func TestClickingOutsideTheSettingsEditorIsIgnored(t *testing.T) {
	m := settingsModel(t)
	before := m.focus.Index()

	next, _ := m.Update(click(0, 0))
	m = next.(*Model)

	if !m.configuring {
		t.Error("a click beside the editor should not close it")
	}
	if got := m.focus.Index(); got != before {
		t.Errorf("focus moved to %d, want the layout under the editor left alone", got)
	}
}

func TestDoubleClickingAChoiceSettingCyclesIt(t *testing.T) {
	m := settingsModel(t)
	before := m.form.Value(themeFieldIndex)

	x, y := settingCell(m, themeFieldIndex)
	m, _ = doubleClick(t, m, x, y)

	if got := m.form.Value(themeFieldIndex); got == before {
		t.Fatalf("the Theme row still reads %q, want the next option", got)
	}
	// The palette follows the row as it is cycled, exactly as →/space leaves it.
	if m.theme != theme.Everforest() {
		t.Error("the cycled theme should be previewed live")
	}
	if m.cfg.Theme != before {
		t.Errorf("cfg.Theme = %q, want the choice unsaved until ctrl+s", m.cfg.Theme)
	}
}

func TestDoubleClickingAToggleSettingFlipsIt(t *testing.T) {
	m := settingsModel(t)
	i := settingIndex(t, m, "Directory diff")
	before := m.form.Value(i)

	x, y := settingCell(m, i)
	m, _ = doubleClick(t, m, x, y)

	if got := m.form.Value(i); got == before {
		t.Errorf("the Directory diff row still reads %q, want it flipped", got)
	}
}

func TestDoubleClickingATextSettingOnlyMovesToIt(t *testing.T) {
	m := settingsModel(t)
	i := settingIndex(t, m, "SSH key")
	before := m.form.Value(i)

	x, y := settingCell(m, i)
	m, _ = doubleClick(t, m, x, y)

	if got := m.form.Value(i); got != before {
		t.Errorf("the SSH key row reads %q, want a text field typed into rather than acted on (%q)", got, before)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "> SSH key") {
		t.Errorf("expected the row to still be the active one, got:\n%s", view)
	}
}

func TestDoubleClickingTheHideRulesRowOpensItsEditor(t *testing.T) {
	m := settingsModel(t)
	i := settingIndex(t, m, "Hide rules")

	x, y := settingCell(m, i)
	m, cmd := doubleClick(t, m, x, y)
	if cmd == nil {
		t.Fatal("a double click on the rules row should report it activated")
	}
	next, _ := m.Update(cmd())
	m = next.(*Model)

	if !m.editingRules {
		t.Error("the rules editor should have opened over the settings editor")
	}
}

// wheel sends one notch of the given wheel button over a cell.
func wheel(t *testing.T, m *Model, x, y int, button tea.MouseButton) *Model {
	t.Helper()
	next, _ := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: button})
	return next.(*Model)
}

func TestTheWheelScrollsThePanelUnderThePointer(t *testing.T) {
	m := loadItems(t, mouseModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified},
		{Path: "b.go", State: svn.StateModified},
	})
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{
		{Revision: "42"}, {Revision: "41"},
	}})
	m = next.(*Model)

	before := m.files.Index()
	m = wheel(t, m, 4, filesTop+1, tea.MouseButtonWheelDown)
	if got := m.files.Index(); got != before+1 {
		t.Errorf("the Changes cursor = %d, want the wheel to have moved it from %d", got, before)
	}

	// The Log panel is not focused, and scrolling it is not a reason to be.
	m = wheel(t, m, 4, logTop+1, tea.MouseButtonWheelDown)
	if got := m.log.Index(); got != 1 {
		t.Errorf("the Log cursor = %d, want the wheel to have moved it", got)
	}
	if got := m.focus.Index(); got != panelFiles {
		t.Errorf("focused panel %d, want scrolling to leave focus alone", got)
	}
}

func TestTwoSlowClicksAreNotADoubleClick(t *testing.T) {
	m := loadItems(t, mouseModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
	})
	row := fileRow(t, m, "src")

	next, _ := m.Update(click(4, row))
	m = next.(*Model)
	// The pause a reader takes between looking at two rows.
	m.lastClick.at = time.Now().Add(-2 * doubleClickWindow)
	next, _ = m.Update(click(4, row))
	m = next.(*Model)

	if m.collapsedDirs["src"] {
		t.Error("two clicks a pause apart are two clicks, not a double click")
	}
}
