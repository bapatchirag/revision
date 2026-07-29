package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

	launch, err := resolveEditor(config.EditorNvim, "/wc/main.go")
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if !launch.terminal {
		t.Error("a terminal editor must take over the terminal")
	}
	if launch.cmd.Path != filepath.Join(dir, "nvim") {
		t.Errorf("cmd.Path = %q, want the nvim on PATH", launch.cmd.Path)
	}
	if got := launch.cmd.Args[len(launch.cmd.Args)-1]; got != "/wc/main.go" {
		t.Errorf("cmd args = %v, want the file last", launch.cmd.Args)
	}
}

func TestResolveEditorFallsBackFromVimToVi(t *testing.T) {
	dir := fakeBins(t, "vi")

	launch, err := resolveEditor(config.EditorVim, "/wc/main.go")
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if launch.cmd.Path != filepath.Join(dir, "vi") {
		t.Errorf("cmd.Path = %q, want vi when only vi is installed", launch.cmd.Path)
	}
}

func TestResolveEditorReportsMissingEditor(t *testing.T) {
	fakeBins(t)

	_, err := resolveEditor(config.EditorNano, "/wc/main.go")
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

	launch, err := resolveEditor(config.EditorNative, "/wc/main.go")
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if launch.cmd.Path != filepath.Join(dir, "code") {
		t.Errorf("cmd.Path = %q, want the VS Code CLI", launch.cmd.Path)
	}
	if launch.terminal {
		t.Error("VS Code opens a tab beside the terminal, so it must not take it over")
	}
}

func TestResolveEditorNativeMatchesInsidersBuild(t *testing.T) {
	dir := fakeBins(t, "code", "code-insiders")
	t.Setenv("TERM_PROGRAM", "vscode")
	t.Setenv("TERM_PROGRAM_VERSION", "1.106.0-insider")

	launch, err := resolveEditor(config.EditorNative, "/wc/main.go")
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

	launch, err := resolveEditor(config.EditorNative, "/wc/main.go")
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if launch.cmd.Path != filepath.Join(dir, "emacs") {
		t.Errorf("cmd.Path = %q, want $EDITOR", launch.cmd.Path)
	}
	if !launch.terminal {
		t.Error("$EDITOR names a terminal editor, so it must take the terminal over")
	}
	if got := launch.cmd.Args[1:]; len(got) != 2 || got[0] != "-nw" || got[1] != "/wc/main.go" {
		t.Errorf("cmd args = %v, want the $EDITOR flags kept before the file", got)
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

	launch, err := resolveEditor(config.EditorNative, "/wc/main.go")
	if err != nil {
		t.Fatalf("resolveEditor: %v", err)
	}
	if launch.cmd.Path != filepath.Join(dir, opener) {
		t.Errorf("cmd.Path = %q, want %s", launch.cmd.Path, opener)
	}
	if launch.terminal {
		t.Error("the desktop opener returns immediately, so it must not take the terminal over")
	}
}

func TestOpenEditorResolvesHighlightedFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/modified.go", State: svn.StateModified},
	})
	// The tree opens on the first file leaf; "/" and "src" are directory rows.
	m.files.SetIndex(2)

	path, name, ok := m.editTarget()
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

func TestOpenEditorWarnsOnDirectoryRow(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/modified.go", State: svn.StateModified},
	})
	m.files.SetIndex(0) // the "/" root row

	if _, _, ok := m.editTarget(); ok {
		t.Fatal("a directory row is not a file to open")
	}
	if cmd := m.openInEditor(); cmd != nil {
		t.Error("nothing should be launched for a directory row")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no file to open here") {
		t.Errorf("expected a warning toast, got:\n%s", view)
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
