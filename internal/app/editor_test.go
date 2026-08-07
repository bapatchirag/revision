package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/svn"
)

// fakeBins puts an executable stub for each name on a PATH holding nothing else,
// so editor resolution sees exactly the programs a test declares installed.
func fakeBins(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

func TestResolveEditorRunsTerminalEditorInPlace(t *testing.T) {
	dir := fakeBins(t, "nvim")

	launch, err := resolveEditor(config.EditorNvim, "/wc/main.go", 42)
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if !launch.terminal {
		t.Error("a terminal editor must take over the terminal")
	}
	if launch.cmd.Path != filepath.Join(dir, "nvim") {
		t.Errorf("cmd.Path = %q, want the nvim on PATH", launch.cmd.Path)
	}
	if got := launch.cmd.Args[1:]; len(got) != 2 || got[0] != "+42" || got[1] != "/wc/main.go" {
		t.Errorf("cmd args = %v, want the line before the file", got)
	}
}

func TestResolveEditorOmitsTheLineWhenThereIsNone(t *testing.T) {
	fakeBins(t, "nvim")

	launch, err := resolveEditor(config.EditorNvim, "/wc/main.go", 0)
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if got := launch.cmd.Args[1:]; len(got) != 1 || got[0] != "/wc/main.go" {
		t.Errorf("cmd args = %v, want the file alone", got)
	}
}

func TestResolveEditorFallsBackFromVimToVi(t *testing.T) {
	dir := fakeBins(t, "vi")

	launch, err := resolveEditor(config.EditorVim, "/wc/main.go", 0)
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if launch.cmd.Path != filepath.Join(dir, "vi") {
		t.Errorf("cmd.Path = %q, want vi when only vi is installed", launch.cmd.Path)
	}
}

func TestResolveEditorReportsMissingEditor(t *testing.T) {
	fakeBins(t)

	_, err := resolveEditor(config.EditorNano, "/wc/main.go", 0)
	if err == nil {
		t.Fatal("expected an error when the chosen editor is not installed")
	}
	if !strings.Contains(err.Error(), "nano") {
		t.Errorf("error = %q, want it to name nano", err)
	}
}

func TestResolveEditorNativePrefersVSCodeTerminal(t *testing.T) {
	dir := fakeBins(t, "code")
	t.Setenv("TERM_PROGRAM", "vscode")
	t.Setenv("TERM_PROGRAM_VERSION", "1.106.0")
	t.Setenv("EDITOR", "nano")

	launch, err := resolveEditor(config.EditorNative, "/wc/main.go", 12)
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if launch.cmd.Path != filepath.Join(dir, "code") {
		t.Errorf("cmd.Path = %q, want the VS Code CLI", launch.cmd.Path)
	}
	if launch.terminal {
		t.Error("VS Code opens a tab beside the terminal, so it must not take it over")
	}
	if got := launch.cmd.Args[1:]; len(got) != 2 || got[0] != "--goto" || got[1] != "/wc/main.go:12" {
		t.Errorf("cmd args = %v, want --goto with the position on the file", got)
	}
}

func TestResolveEditorNativeMatchesInsidersBuild(t *testing.T) {
	dir := fakeBins(t, "code", "code-insiders")
	t.Setenv("TERM_PROGRAM", "vscode")
	t.Setenv("TERM_PROGRAM_VERSION", "1.106.0-insider")

	launch, err := resolveEditor(config.EditorNative, "/wc/main.go", 0)
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if launch.cmd.Path != filepath.Join(dir, "code-insiders") {
		t.Errorf("cmd.Path = %q, want the Insiders CLI for an Insiders window", launch.cmd.Path)
	}
}

func TestResolveEditorNativeUsesEditorEnvOutsideVSCode(t *testing.T) {
	dir := fakeBins(t, "emacs")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "emacs -nw")

	launch, err := resolveEditor(config.EditorNative, "/wc/main.go", 7)
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if launch.cmd.Path != filepath.Join(dir, "emacs") {
		t.Errorf("cmd.Path = %q, want $EDITOR", launch.cmd.Path)
	}
	if !launch.terminal {
		t.Error("$EDITOR names a terminal editor, so it must take the terminal over")
	}
	if got := launch.cmd.Args[1:]; len(got) != 3 || got[0] != "-nw" || got[1] != "+7" || got[2] != "/wc/main.go" {
		t.Errorf("cmd args = %v, want the $EDITOR flags kept before the line and file", got)
	}
}

func TestOpenArgsSpellThePositionEachEditorsWay(t *testing.T) {
	for _, tc := range []struct {
		bin  string
		line int
		want []string
	}{
		{"vim", 42, []string{"+42", "/wc/main.go"}},
		{"/usr/bin/nano", 42, []string{"+42", "/wc/main.go"}},
		{"code-insiders", 42, []string{"--goto", "/wc/main.go:42"}},
		{"subl", 42, []string{"/wc/main.go:42"}},
		{"hx", 42, []string{"/wc/main.go:42"}},
		{"xdg-open", 42, []string{"/wc/main.go"}},
		{"weirdedit", 42, []string{"/wc/main.go"}},
		{"vim", 0, []string{"/wc/main.go"}},
	} {
		got := openArgs(tc.bin, "/wc/main.go", tc.line)
		if len(got) != len(tc.want) {
			t.Errorf("openArgs(%q, %d) = %v, want %v", tc.bin, tc.line, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("openArgs(%q, %d) = %v, want %v", tc.bin, tc.line, got, tc.want)
				break
			}
		}
	}
}

func TestResolveEditorLeavesUnknownEditorsAlone(t *testing.T) {
	fakeBins(t, "weirdedit")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "weirdedit")

	launch, err := resolveEditor(config.EditorNative, "/wc/main.go", 7)
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	// An editor that does not know "+N" would read it as a second file to open.
	if got := launch.cmd.Args[1:]; len(got) != 1 || got[0] != "/wc/main.go" {
		t.Errorf("cmd args = %v, want the file alone for an unrecognized editor", got)
	}
}

func TestResolveEditorNativeFallsBackToDesktopOpener(t *testing.T) {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	dir := fakeBins(t, opener)
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	launch, err := resolveEditor(config.EditorNative, "/wc/main.go", 5)
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if launch.cmd.Path != filepath.Join(dir, opener) {
		t.Errorf("cmd.Path = %q, want %s", launch.cmd.Path, opener)
	}
	if launch.terminal {
		t.Error("the desktop opener returns immediately, so it must not take the terminal over")
	}
	if got := launch.cmd.Args[1:]; len(got) != 1 || got[0] != "/wc/main.go" {
		t.Errorf("cmd args = %v, want the file alone; the opener takes no position", got)
	}
}

func TestOpenEditorResolvesHighlightedFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/modified.go", State: svn.StateModified},
	})
	// The tree opens on the first file leaf; "/" and "src" are directory rows.
	m.files.SetIndex(2)

	path, name, _, ok := m.editTarget()
	if !ok {
		t.Fatal("the highlighted file leaf should be openable")
	}
	if name != "src/modified.go" {
		t.Errorf("name = %q, want the working-copy path", name)
	}
	if want := filepath.Join(m.client.Dir, "src/modified.go"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}

func TestOpenEditorWarnsOnDirectoryRowWithNoDiff(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/modified.go", State: svn.StateModified},
	})
	m.files.SetIndex(0) // the "/" root row, with no diff loaded behind it

	if _, _, _, ok := m.editTarget(); ok {
		t.Fatal("a directory row with nothing on display is not a file to open")
	}
	if cmd := m.openInEditor(); cmd != nil {
		t.Error("nothing should be launched for a directory row")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no file to open here") {
		t.Errorf("expected a warning toast, got:\n%s", view)
	}
}

func TestDiffTargetAtMapsRowsOntoTheModifiedFile(t *testing.T) {
	// fileDiff's rows: 0-3 the Index block, 4 the first hunk header (new line 1),
	// 5-12 its body, 13 the second header (new line 21), 14-16 its body.
	for _, tc := range []struct {
		name string
		row  int
		want int
	}{
		{"file header resolves to the first hunk", 0, 1},
		{"hunk header is its own start", 4, 1},
		{"context line", 5, 1},
		{"a removal points at where the text went", 7, 3},
		{"added line", 9, 4},
		{"the second hunk starts where it says", 13, 21},
		{"context after it", 16, 22},
	} {
		path, line := diffTargetAt(fileDiff, tc.row)
		if line != tc.want {
			t.Errorf("%s: diffTargetAt(row %d) line = %d, want %d", tc.name, tc.row, line, tc.want)
		}
		if path != "src/a.go" {
			t.Errorf("%s: diffTargetAt(row %d) path = %q, want src/a.go", tc.name, tc.row, path)
		}
	}
	if _, line := diffTargetAt("Index: a.go\n--- a.go\n+++ a.go\n", 0); line != 0 {
		t.Errorf("a diff with no hunk = %d, want 0 (open at the top)", line)
	}
}

func TestDiffTargetAtNamesTheFileARowBelongsTo(t *testing.T) {
	combined := fileDiff + `Index: src/b.go
===================================================================
--- src/b.go	(revision 42)
+++ src/b.go	(working copy)
@@ -7,2 +7,3 @@
 keep
+new
`
	// fileDiff runs to row 16 and src/b.go's block follows straight on: its Index
	// line is row 17, its hunk header row 21 and the line it adds row 23.
	for _, tc := range []struct {
		row  int
		path string
		line int
	}{
		{4, "src/a.go", 1},
		{13, "src/a.go", 21},
		{17, "src/b.go", 7},
		{21, "src/b.go", 7},
		{23, "src/b.go", 8},
	} {
		path, line := diffTargetAt(combined, tc.row)
		if path != tc.path || line != tc.line {
			t.Errorf("diffTargetAt(row %d) = %q:%d, want %q:%d", tc.row, path, line, tc.path, tc.line)
		}
	}
}

// scrollableDiff is a patch with a three-line hunk at file line 10 and a second
// one at 201, followed by enough context that the Main viewport can scroll well
// past both. Its rows are: 0-3 the Index block, 4 the first header, 5-7 its
// body, 8 the second header, 9 on its body.
func scrollableDiff() string {
	lines := []string{
		"Index: src/a.go",
		"===================================================================",
		"--- src/a.go\t(revision 42)",
		"+++ src/a.go\t(working copy)",
		"@@ -10,3 +10,3 @@",
		" before",
		"-was",
		"+is",
		"@@ -200,60 +201,60 @@",
	}
	for i := 0; i < 60; i++ {
		lines = append(lines, fmt.Sprintf(" tail %d", i))
	}
	return strings.Join(lines, "\n") + "\n"
}

// editLine is the line editTarget would open the current selection at.
func editLine(t *testing.T, m *Model) int {
	t.Helper()
	_, _, line, ok := m.editTarget()
	if !ok {
		t.Fatal("expected something to open")
	}
	return line
}

// pressDown sends n cursor-down keys.
func pressDown(t *testing.T, m *Model, n int) *Model {
	t.Helper()
	for i := 0; i < n; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(*Model)
	}
	return m
}

func TestOpenEditorAimsAtTheFirstHunkFromTheFilesPanel(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	next, _ := m.Update(diffLoadedMsg{path: "src/a.go", diff: scrollableDiff()})
	m = next.(*Model)

	if got := editLine(t, m); got != 10 {
		t.Errorf("line = %d, want 10 — where the first hunk begins", got)
	}
}

func TestOpenEditorFollowsTheDiffCursor(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	next, _ := m.Update(diffLoadedMsg{path: "src/a.go", diff: scrollableDiff()})
	m = next.(*Model)

	m.focus.Focus(panelMain)
	m.afterFocusChange()
	// Put the cursor on the second hunk's header, which is row 8.
	m = pressDown(t, m, 8)
	if got := m.main.Cursor(); got != 8 {
		t.Fatalf("the diff cursor is on row %d, want 8", got)
	}
	if got := editLine(t, m); got != 201 {
		t.Errorf("line = %d, want 201 — the hunk under the cursor", got)
	}

	// Reading on into that hunk's body follows the cursor rather than snapping back.
	m = pressDown(t, m, 4)
	if got := editLine(t, m); got != 204 {
		t.Errorf("line = %d, want 204", got)
	}
}

func TestDiffCursorIsMarkedWhileTheDiffIsFocused(t *testing.T) {
	// The suite renders without color, which is exactly what the highlight is made
	// of, so this one case asks for a palette.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	bar := "48;5;238" // theme.Default().SelectionBg, as a background

	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	next, _ := m.Update(diffLoadedMsg{path: "src/a.go", diff: scrollableDiff()})
	m = next.(*Model)

	// The Files panel drives Main, so there is no line of its own to mark yet.
	if strings.Contains(m.main.View(), bar) {
		t.Error("the diff carries a cursor bar while another panel drives it")
	}

	m.focus.Focus(panelMain)
	m.afterFocusChange()
	if !strings.Contains(m.main.View(), bar) {
		t.Error("the focused diff does not mark its current line")
	}
}

func TestDiffCursorSurvivesARefresh(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	next, _ := m.Update(diffLoadedMsg{path: "src/a.go", diff: scrollableDiff()})
	m = next.(*Model)
	m.focus.Focus(panelMain)
	m.afterFocusChange()
	m = pressDown(t, m, 8)

	next, _ = m.Update(diffLoadedMsg{path: "src/a.go", diff: scrollableDiff()})
	m = next.(*Model)
	if got := m.main.Cursor(); got != 8 {
		t.Errorf("the cursor moved to row %d over a refresh of the same diff, want 8", got)
	}
}

func TestOpenEditorPicksAFileOutOfADirectoryDiff(t *testing.T) {
	combined := fileDiff + `Index: src/b.go
===================================================================
--- src/b.go	(revision 42)
+++ src/b.go	(working copy)
@@ -7,2 +7,3 @@
 keep
+new
`
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	m.files.SetIndex(1) // the "src" directory row
	next, _ := m.Update(diffLoadedMsg{path: "src", diff: combined, dir: true})
	m = next.(*Model)

	// From the Files panel a directory opens at the first change under it.
	path, name, line, ok := m.editTarget()
	if !ok {
		t.Fatal("a directory row showing a diff should be openable")
	}
	if want := filepath.Join(m.client.Dir, "src/a.go"); path != want || name != "src/a.go" || line != 1 {
		t.Errorf("editTarget() = %q (%q) at %d, want %q (src/a.go) at 1", path, name, line, want)
	}

	// Reading down into the second file's hunk opens that file instead.
	m.focus.Focus(panelMain)
	m.afterFocusChange()
	m = pressDown(t, m, 22)
	_, name, line, ok = m.editTarget()
	if !ok || name != "src/b.go" || line != 7 {
		t.Errorf("editTarget() = %q at %d, want src/b.go at 7", name, line)
	}
}

func TestOpenEditorFromTheSideBySideView(t *testing.T) {
	m := splitKey(t, modifiedFileModel(t))
	if !m.splitting {
		t.Fatal("expected the side-by-side overlay to be open")
	}

	_, name, line, ok := m.editTarget()
	if !ok || name != "src/a.go" || line != 1 {
		t.Errorf("editTarget() = %q at %d, want src/a.go at 1", name, line)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "e edit") {
		t.Errorf("the overlay does not advertise the edit key:\n%s", view)
	}
}

func TestOpenEditorClosesTheSideBySideView(t *testing.T) {
	fakeBins(t, "nvim")
	m := modifiedFileModel(t)
	m.cfg.Editor = config.EditorNvim
	m = splitKey(t, m)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("`e` did not launch an editor from the overlay")
	}
	if m.splitting {
		t.Error("the overlay holds a snapshot, so it must step aside for an edit")
	}
}

func TestOpenEditorHasNoLineForAFileWithoutADiff(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "new.txt", State: svn.StateUnversioned}})
	m.files.SetIndex(1)

	if got := editLine(t, m); got != 0 {
		t.Errorf("line = %d, want 0 — there is no hunk to aim at", got)
	}
}

func TestEditedMsgRefreshesWorkingCopy(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})

	// A terminal editor has exited: the file may have changed, so status reloads.
	next, cmd := m.Update(editedMsg{name: "modified.go"})
	m = next.(*Model)
	if cmd == nil {
		t.Error("expected a reload after a terminal editor exited")
	}

	// A detached editor is still open, so there is nothing to re-read yet.
	if _, cmd = m.Update(editedMsg{name: "modified.go", detached: true}); cmd != nil {
		t.Error("a detached editor should not trigger a reload")
	}
}

func TestEditedMsgReportsFailure(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)

	next, _ := m.Update(editedMsg{name: "modified.go", err: os.ErrNotExist})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "open modified.go failed") {
		t.Errorf("expected a failure toast, got:\n%s", view)
	}
}

func TestOpenEditorKeyIsBoundToE(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)

	// With no file highlighted, `e` is still consumed by the editor action rather
	// than reaching the focused panel.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "no file to open here") {
		t.Errorf("`e` did not run the open-in-editor action, got:\n%s", view)
	}
}
