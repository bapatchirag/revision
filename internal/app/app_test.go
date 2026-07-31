package app

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/muesli/termenv"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/sshagent"
	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	// Theme switches call theme.ApplyColorProfile, which would otherwise force
	// TrueColor and break the golden suite's deterministic Ascii output.
	theme.DisableColorProfile = true
	os.Exit(m.Run())
}

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

func sizedModel(t *testing.T) *Model {
	t.Helper()
	return sizedModelCfg(t, config.Default())
}

func sizedModelCfg(t *testing.T, cfg config.Config) *Model {
	t.Helper()
	info := &svn.Info{
		URL:             "https://svn.example.com/repo/trunk",
		WorkingCopyRoot: "/home/alice/work/wc",
		Revision:        "42",
	}
	m := New(svn.New("/home/alice/work/wc"), info, selfupdate.Build{}, cfg)
	m.workDir = "/home/alice/work/wc"
	m.refreshChrome()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func loadItems(t *testing.T, m *Model, items []svn.StatusItem) *Model {
	t.Helper()
	next, _ := m.Update(statusLoadedMsg{items: items})
	return next.(*Model)
}

func TestModelRendersStatus(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded, Changelist: "revision:staged"},
		{Path: "committed.txt", State: svn.StateModified},
	})

	view := stripANSI(m.View())
	for _, want := range []string{"added.txt", "committed.txt", "/home/alice/work/wc", "Branch", "trunk", "Revision", "r42"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n---\n%s", want, view)
		}
	}
}

func TestModelEmptyState(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	if view := stripANSI(m.View()); !strings.Contains(view, "clean") {
		t.Errorf("expected clean message, got:\n%s", view)
	}
}

func TestModelShowsError(t *testing.T) {
	m := sizedModel(t)
	next, _ := m.Update(errMsg{err: errors.New("kaboom")})
	m = next.(*Model)

	if view := stripANSI(m.View()); !strings.Contains(view, "kaboom") {
		t.Errorf("expected error in view, got:\n%s", view)
	}
}

func TestModelQuit(t *testing.T) {
	m := sizedModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected a command from quit key")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("expected tea.QuitMsg from quit key")
	}
}

func TestSelectionUpdatesMain(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded},
		{Path: "committed.txt", State: svn.StateModified},
	})

	// The first item is selected, so its diff lands in Main.
	next, _ := m.Update(diffLoadedMsg{path: "added.txt", diff: "@@ -0,0 +1 @@\n+alpha"})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "+alpha") {
		t.Fatalf("main should start on the first item, got:\n%s", main)
	}

	// Down is forwarded to the focused Files panel, which emits SelectedMsg.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatal("expected a SelectedMsg command after moving down")
	}
	sel, ok := cmd().(uimsg.SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg, got %T", cmd())
	}
	next, _ = m.Update(sel)
	m = next.(*Model)

	// The second item's diff follows the selection into Main.
	next, _ = m.Update(diffLoadedMsg{path: "committed.txt", diff: "@@ -1 +1 @@\n+beta"})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "+beta") {
		t.Errorf("main should follow selection to the second item, got:\n%s", main)
	}
}

func TestFileDiffLoadsIntoMain(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "committed.txt", State: svn.StateModified},
	})
	// Before the diff arrives the Main panel shows a loading placeholder.
	if main := stripANSI(m.main.View()); !strings.Contains(main, "Loading diff") {
		t.Errorf("expected a loading placeholder, got:\n%s", main)
	}

	next, _ := m.Update(diffLoadedMsg{path: "committed.txt", diff: "@@ -1 +1 @@\n-old\n+new"})
	m = next.(*Model)
	main := stripANSI(m.main.View())
	if !strings.Contains(main, "+new") {
		t.Errorf("main should show the diff, got:\n%s", main)
	}
}

func TestStaleDiffIgnoredForOtherFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "committed.txt", State: svn.StateModified},
	})
	// A diff for a file that is no longer selected must not replace Main.
	next, _ := m.Update(diffLoadedMsg{path: "other.txt", diff: "+stale"})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); strings.Contains(main, "+stale") {
		t.Errorf("main should ignore a diff for an unselected file, got:\n%s", main)
	}
}

func TestDirectoryDiffLoadsIntoMain(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	// The cursor opens on the first file; move it onto the src/ directory row.
	selectDirRow(t, m, "src")

	// Highlighting a directory schedules a load of its combined diff.
	if cmd := m.diffLoadForSelection(); cmd == nil {
		t.Fatal("expected a diff-load command for the highlighted directory")
	}

	// When it arrives, Main shows the diff of every file under the directory.
	next, _ := m.Update(diffLoadedMsg{
		path: "src",
		diff: "Index: src/a.go\n@@ -1 +1 @@\n+alpha\nIndex: src/b.go\n@@ -1 +1 @@\n+beta",
	})
	m = next.(*Model)
	main := stripANSI(m.main.View())
	if !strings.Contains(main, "+alpha") || !strings.Contains(main, "+beta") {
		t.Errorf("main should show the combined directory diff, got:\n%s", main)
	}
}

func TestRootDirectoryDiffCoversWholeWorkingCopy(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "readme.md", State: svn.StateModified},
	})
	m.files.SetIndex(0) // the synthetic "/" root row covers the whole tree
	root, ok := m.files.Selected()
	if !ok || root.Path != fileTreeRoot {
		t.Fatalf("expected cursor on the / root row, got %+v (ok=%v)", root, ok)
	}
	if cmd := m.diffLoadForSelection(); cmd == nil {
		t.Fatal("expected a diff-load command for the root row")
	}

	// The root diff is keyed by the "/" sentinel so the current selection matches
	// it; it spans changes anywhere in the working copy, nested or not.
	next, _ := m.Update(diffLoadedMsg{
		path: fileTreeRoot,
		diff: "Index: readme.md\n@@ -1 +1 @@\n+top\nIndex: src/a.go\n@@ -1 +1 @@\n+nested",
	})
	m = next.(*Model)
	main := stripANSI(m.main.View())
	if !strings.Contains(main, "+top") || !strings.Contains(main, "+nested") {
		t.Errorf("root diff should cover the whole working copy, got:\n%s", main)
	}
}

func TestDirectoryDiffDisabledByConfig(t *testing.T) {
	cfg := config.Default()
	cfg.DirectoryDiff = false
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")

	// With directory diffs off globally, highlighting a directory loads nothing.
	if cmd := m.diffLoadForSelection(); cmd != nil {
		t.Fatal("expected no diff-load command while directory diffs are off")
	}
	// Main shows a hint naming the toggle key instead of a diff.
	m.updateMain()
	if main := stripANSI(m.main.View()); !strings.Contains(main, "directory diff off") {
		t.Errorf("expected the directory-diff-off hint, got:\n%s", main)
	}
}

func TestToggleDirDiffRevealsDirectoryDiff(t *testing.T) {
	cfg := config.Default()
	cfg.DirectoryDiff = false
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")

	// Pressing the toggle key schedules the directory diff load.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a diff-load command after toggling directory diffs on")
	}
	// When the diff arrives, Main shows it.
	next, _ = m.Update(diffLoadedMsg{
		path: "src",
		diff: "Index: src/a.go\n@@ -1 +1 @@\n+alpha",
	})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "+alpha") {
		t.Errorf("expected the directory diff after toggling on, got:\n%s", main)
	}
}

func TestToggleDirDiffHidesDirectoryDiff(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")
	// Directory diffs are on by default, so the loaded diff shows in Main.
	next, _ := m.Update(diffLoadedMsg{path: "src", diff: "Index: src/a.go\n@@ -1 +1 @@\n+alpha"})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "+alpha") {
		t.Fatalf("expected the directory diff to show, got:\n%s", main)
	}

	// Toggling off hides it behind the hint and drops the diff gutter.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = next.(*Model)
	if m.filesShowDiff() {
		t.Error("filesShowDiff() = true after toggling directory diffs off")
	}
	if main := stripANSI(m.main.View()); !strings.Contains(main, "directory diff off") {
		t.Errorf("expected the directory-diff-off hint, got:\n%s", main)
	}
}

// saveDiffKey presses the save-diff key and answers the file-name prompt: name
// is typed into the input when non-empty, otherwise the blank entry falls back to
// the prompt's default. It returns the message the resulting write produced, or
// nil when no prompt opened.
func saveDiffKey(t *testing.T, m *Model, name string) (*Model, tea.Msg) {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = next.(*Model)
	if !m.savingDiff {
		return m, nil
	}
	if name != "" {
		m.diffEditor.SetValue(name)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a submit command from the diff-name prompt")
	}
	next, cmd = m.Update(cmd()) // deliver the SubmitMsg
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a save command after naming the diff")
	}
	return m, cmd()
}

func TestSaveDiffWritesSelectedFileDiff(t *testing.T) {
	cfg := config.Default()
	cfg.DiffOutputDir = t.TempDir()
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
	})
	next, _ := m.Update(diffLoadedMsg{path: "src/a.go", diff: "Index: src/a.go\n@@ -1 +1 @@\n-old\n+new"})
	m = next.(*Model)

	m, msg := saveDiffKey(t, m, "")
	saved, ok := msg.(diffSavedMsg)
	if !ok {
		t.Fatalf("expected a diffSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("save failed: %v", saved.err)
	}
	// The prompt defaults to the target's path with its separators folded into
	// "-", so nested files stay distinct in one flat output directory.
	want := filepath.Join(cfg.DiffOutputDir, "src-a.go.diff")
	if saved.path != want {
		t.Errorf("saved to %q, want %q", saved.path, want)
	}
	body, err := os.ReadFile(saved.path)
	if err != nil {
		t.Fatalf("read saved diff: %v", err)
	}
	if got := string(body); got != "Index: src/a.go\n@@ -1 +1 @@\n-old\n+new\n" {
		t.Errorf("saved diff = %q, want the on-screen diff with a trailing newline", got)
	}

	next, _ = m.Update(saved)
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "diff saved") {
		t.Errorf("expected the save toast, got:\n%s", view)
	}
}

func TestSaveDiffPromptsForName(t *testing.T) {
	cfg := config.Default()
	cfg.DiffOutputDir = t.TempDir()
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
	})
	next, _ := m.Update(diffLoadedMsg{path: "src/a.go", diff: "@@ -1 +1 @@\n+new"})
	m = next.(*Model)

	// "w" floats the name prompt, empty, over the layout.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = next.(*Model)
	if !m.savingDiff {
		t.Fatal("expected the save-diff prompt to open on w")
	}
	if got := m.diffEditor.Value(); got != "" {
		t.Errorf("prompt pre-filled with %q, want an empty input", got)
	}
	if got := m.diffSrc.name; got != "src-a.go.diff" {
		t.Errorf("blank-entry fallback = %q, want the derived default", got)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Save diff as") {
		t.Errorf("expected the prompt overlay, got:\n%s", view)
	}

	// esc cancels without writing anything.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the DismissMsg
		m = next.(*Model)
	}
	if m.savingDiff {
		t.Error("the prompt should close on esc")
	}
	if entries, err := os.ReadDir(cfg.DiffOutputDir); err != nil || len(entries) != 0 {
		t.Errorf("output directory should stay empty, got %v (err %v)", entries, err)
	}
}

func TestSaveDiffUsesEnteredName(t *testing.T) {
	cfg := config.Default()
	cfg.DiffOutputDir = t.TempDir()
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
	})
	next, _ := m.Update(diffLoadedMsg{path: "src/a.go", diff: "@@ -1 +1 @@\n+new"})
	m = next.(*Model)

	// A name with no patch extension gains ".diff"; any directory part is dropped
	// so the file always lands in the output directory.
	_, msg := saveDiffKey(t, m, "../../review")
	saved, ok := msg.(diffSavedMsg)
	if !ok {
		t.Fatalf("expected a diffSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("save failed: %v", saved.err)
	}
	if want := filepath.Join(cfg.DiffOutputDir, "review.diff"); saved.path != want {
		t.Errorf("saved to %q, want %q", saved.path, want)
	}
}

func TestSaveDiffWritesDirectoryDiffAndCreatesOutputDir(t *testing.T) {
	cfg := config.Default()
	// A directory that does not exist yet must be created on demand.
	cfg.DiffOutputDir = filepath.Join(t.TempDir(), "patches", "nested")
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")
	next, _ := m.Update(diffLoadedMsg{path: "src", diff: "Index: src/a.go\n@@ -1 +1 @@\n+alpha\n"})
	m = next.(*Model)

	_, msg := saveDiffKey(t, m, "")
	saved, ok := msg.(diffSavedMsg)
	if !ok {
		t.Fatalf("expected a diffSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("save failed: %v", saved.err)
	}
	if want := filepath.Join(cfg.DiffOutputDir, "src.diff"); saved.path != want {
		t.Errorf("saved to %q, want %q", saved.path, want)
	}
	body, err := os.ReadFile(saved.path)
	if err != nil {
		t.Fatalf("read saved diff: %v", err)
	}
	if got := string(body); got != "Index: src/a.go\n@@ -1 +1 @@\n+alpha\n" {
		t.Errorf("saved diff = %q, want the combined directory diff", got)
	}
}

func TestSaveDiffDefaultsToWorkingCopyRoot(t *testing.T) {
	// Launched in a subdirectory: an unset diffOutputDir still writes to the
	// working copy's root, not the directory revision was started in.
	m := subdirModel(t, config.Default())
	if got, want := m.diffDir(), "/home/alice/work/wc"; got != want {
		t.Errorf("diffDir() = %q, want %q", got, want)
	}
}

func TestSaveDiffWithoutADiffWarns(t *testing.T) {
	cfg := config.Default()
	cfg.DiffOutputDir = t.TempDir()
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified},
	})
	// The diff has not arrived yet, so Main shows a placeholder, not a patch.
	m, msg := saveDiffKey(t, m, "")
	if msg != nil {
		t.Fatalf("expected no save command while no diff is on screen, got %T", msg)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no diff to save") {
		t.Errorf("expected the no-diff warning, got:\n%s", view)
	}
	if entries, err := os.ReadDir(cfg.DiffOutputDir); err != nil || len(entries) != 0 {
		t.Errorf("output directory should stay empty, got %v (err %v)", entries, err)
	}
}

func TestSaveDiffRefusesFailedDiff(t *testing.T) {
	cfg := config.Default()
	cfg.DiffOutputDir = t.TempDir()
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified},
	})
	// A load failure fills Main with a notice; it must never be written as a patch.
	next, _ := m.Update(diffLoadedMsg{path: "a.go", err: errors.New("kaboom")})
	m = next.(*Model)

	_, msg := saveDiffKey(t, m, "")
	if msg != nil {
		t.Fatalf("expected no save command for a failed diff, got %T", msg)
	}
	if entries, err := os.ReadDir(cfg.DiffOutputDir); err != nil || len(entries) != 0 {
		t.Errorf("output directory should stay empty, got %v (err %v)", entries, err)
	}
}

func TestDiffFileName(t *testing.T) {
	cases := map[string]string{
		fileTreeRoot:      "working-copy.diff",
		"a.go":            "a.go.diff",
		"src":             "src.diff",
		"src/app/main.go": "src-app-main.go.diff",
		"../escape.go":    "..-escape.go.diff",
	}
	for target, want := range cases {
		if got := diffFileName(target); got != want {
			t.Errorf("diffFileName(%q) = %q, want %q", target, got, want)
		}
	}
}

func TestDiffSaveName(t *testing.T) {
	cases := map[string]string{
		"review":              "review.diff",
		"  review  ":          "review.diff",
		"review.diff":         "review.diff",
		"review.patch":        "review.patch",
		"a.go":                "a.go.diff",
		"/etc/passwd":         "passwd.diff",
		"../../escape":        "escape.diff",
		"sub/dir/review.diff": "review.diff",
		"":                    "src-a.go.diff", // blank falls back to the default
		"..":                  "src-a.go.diff",
	}
	for entered, want := range cases {
		if got := diffSaveName(entered, "src-a.go.diff"); got != want {
			t.Errorf("diffSaveName(%q) = %q, want %q", entered, got, want)
		}
	}
}

// changelistsView switches the Files panel to the Changelists overview, where a
// changelist group is highlighted instead of a file.
func changelistsView(t *testing.T, m *Model) *Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver ViewSelectedMsg
		m = next.(*Model)
	}
	if name := m.filesViews.ActiveName(); name != "Changelists" {
		t.Fatalf("active files view = %q, want Changelists", name)
	}
	return m
}

func TestSaveDiffOfHighlightedChangelist(t *testing.T) {
	cfg := config.Default()
	cfg.DiffOutputDir = t.TempDir()
	m := changelistsView(t, loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "b.go", State: svn.StateModified, Changelist: "feature"},
	}))

	// The overview shows a summary, not a diff, so the changelist itself is saved.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = next.(*Model)
	if !m.savingDiff {
		t.Fatal("expected the save-diff prompt to open on a highlighted changelist")
	}
	if got := m.diffSrc.name; got != "feature.diff" {
		t.Errorf("default name = %q, want the changelist name", got)
	}
	if got := m.diffSrc.paths; len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Errorf("queued paths = %v, want the changelist's files", got)
	}

	// Submitting queues the write; the diff is generated as part of it.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a submit command from the diff-name prompt")
	}
	if _, cmd = m.Update(cmd()); cmd == nil {
		t.Fatal("expected a save command after naming the changelist diff")
	}
}

func TestSaveDiffOfChangelistWithoutChangesWarns(t *testing.T) {
	m := changelistsView(t, loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "new.go", State: svn.StateUnversioned, Changelist: "feature"},
	}))

	// Unversioned files have no textual diff, so there is nothing to write.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = next.(*Model)
	if m.savingDiff {
		t.Error("the prompt should not open for a changelist with no textual changes")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no textual changes in feature") {
		t.Errorf("expected the empty-changelist warning, got:\n%s", view)
	}
}

func TestChangelistDiffName(t *testing.T) {
	cases := map[string]string{
		"feature-x":      "feature-x.diff",
		stagedChangelist: "staged.diff",
		"":               "unstaged.diff",
	}
	for name, want := range cases {
		if got := changelistDiffName(changelistGroup{Name: name}); got != want {
			t.Errorf("changelistDiffName(%q) = %q, want %q", name, got, want)
		}
	}
}

// fileTreeHasPath reports whether the Changes tree currently holds a file leaf at
// the given path, so tests can assert an item is shown or hidden.
func fileTreeHasPath(m *Model, path string) bool {
	for _, n := range m.files.Items() {
		if n.Item != nil && n.Item.Path == path {
			return true
		}
	}
	return false
}

// subdirModel builds a model launched inside a subdirectory of the working copy,
// so the directory the svn client is rooted at reveals the display scope in use.
func subdirModel(t *testing.T, cfg config.Config) *Model {
	t.Helper()
	info := &svn.Info{
		URL:             "https://svn.example.com/repo/trunk/src",
		WorkingCopyRoot: "/home/alice/work/wc",
		Revision:        "42",
	}
	m := New(svn.New("/home/alice/work/wc/src"), info, selfupdate.Build{}, cfg)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func TestDisplayFromCWDKeepsLaunchDirectory(t *testing.T) {
	m := subdirModel(t, config.Default())
	if got := m.client.Dir; got != "/home/alice/work/wc/src" {
		t.Errorf("client.Dir = %q, want the launch directory", got)
	}
}

func TestDisplayFromRootRootsClientAtWorkingCopy(t *testing.T) {
	cfg := config.Default()
	cfg.DisplayFrom = config.DisplayFromRoot
	m := subdirModel(t, cfg)
	if got := m.client.Dir; got != "/home/alice/work/wc" {
		t.Errorf("client.Dir = %q, want the working-copy root", got)
	}
	if m.launchDir != "/home/alice/work/wc/src" {
		t.Errorf("launchDir = %q, want the directory revision was launched in", m.launchDir)
	}
	// The Status panel reports the directory the working copy is displayed from.
	if view := stripANSI(m.View()); !strings.Contains(view, "Source") {
		t.Errorf("expected the Status panel to name the source directory, got:\n%s", view)
	}
}

func TestSettingsSwitchesDisplayScope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := subdirModel(t, config.Default())

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	for i := 0; i < 6; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // to the Display from field
		m = next.(*Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight}) // cwd -> root
	m = next.(*Model)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}

	if m.cfg.DisplayFrom != config.DisplayFromRoot {
		t.Fatalf("cfg.DisplayFrom = %q, want %q", m.cfg.DisplayFrom, config.DisplayFromRoot)
	}
	if m.client.Dir != "/home/alice/work/wc" {
		t.Errorf("client.Dir = %q, want the working-copy root after switching scope", m.client.Dir)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.DisplayFrom != config.DisplayFromRoot {
		t.Errorf("persisted DisplayFrom = %q, want %q", got.DisplayFrom, config.DisplayFromRoot)
	}
}

func TestHideUntrackedHiddenByConfig(t *testing.T) {
	cfg := config.Default()
	cfg.HideUntracked = true
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
		{Path: "scratch.txt", State: svn.StateUnversioned},
	})
	if fileTreeHasPath(m, "scratch.txt") {
		t.Error("untracked file should be hidden from the Changes tree when hideUntracked is on")
	}
	if !fileTreeHasPath(m, "modified.go") {
		t.Error("tracked file should still show when hideUntracked is on")
	}
}

func TestToggleUntrackedHidesAndShows(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
		{Path: "scratch.txt", State: svn.StateUnversioned},
	})
	// Untracked files show by default.
	if !fileTreeHasPath(m, "scratch.txt") {
		t.Fatal("untracked file should show by default")
	}

	// Pressing the toggle hides untracked files but keeps tracked ones.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = next.(*Model)
	if !m.hideUntracked {
		t.Fatal("pressing U did not enable hide-untracked")
	}
	if fileTreeHasPath(m, "scratch.txt") {
		t.Error("untracked file should be hidden after toggling")
	}
	if !fileTreeHasPath(m, "modified.go") {
		t.Error("tracked file should remain after toggling untracked off")
	}

	// Pressing it again reveals them.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = next.(*Model)
	if m.hideUntracked {
		t.Fatal("pressing U again did not disable hide-untracked")
	}
	if !fileTreeHasPath(m, "scratch.txt") {
		t.Error("untracked file should reappear after toggling back on")
	}
}

func TestSettingsSavesHideUntrackedToggle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
		{Path: "scratch.txt", State: svn.StateUnversioned},
	})
	if m.hideUntracked {
		t.Fatal("hide-untracked should start disabled by default")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	for i := 0; i < 4; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // to the Hide untracked field
		m = next.(*Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // toggle on
	m = next.(*Model)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	if !m.hideUntracked {
		t.Error("saving did not apply the hide-untracked toggle")
	}
	if fileTreeHasPath(m, "scratch.txt") {
		t.Error("untracked file should be hidden immediately after saving hide-untracked")
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !got.HideUntracked {
		t.Error("persisted HideUntracked = false, want true")
	}
}

func TestToggleUntrackedDoesNotPersistConfig(t *testing.T) {
	// Point config at a throwaway dir so any write would be observable there.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
		{Path: "scratch.txt", State: svn.StateUnversioned},
	})

	// The keybind changes only the session view, never the saved configuration.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = next.(*Model)
	if !m.hideUntracked {
		t.Fatal("pressing U did not enable the session hide-untracked toggle")
	}
	if m.cfg.HideUntracked {
		t.Error("the keybind must not change the in-memory config")
	}
	path, err := config.Path()
	if err != nil {
		t.Fatalf("config.Path: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the keybind wrote config.json; it must not persist the toggle")
	}
}

func TestToggleUntrackedNotCapturedBySettingsSave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
		{Path: "scratch.txt", State: svn.StateUnversioned},
	})

	// Hide untracked for the session via the keybind.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = next.(*Model)
	if !m.hideUntracked {
		t.Fatal("pressing U did not hide untracked files for the session")
	}

	// Open settings and save without touching the hide-untracked field. The form
	// mirrors the persisted config (untracked still shown), so the save must not
	// capture the session toggle.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.HideUntracked {
		t.Error("saving settings captured the session toggle; persisted HideUntracked should stay false")
	}
	if m.cfg.HideUntracked {
		t.Error("in-memory config HideUntracked should stay false after saving unrelated settings")
	}
	// Saving resets the session view to the persisted default, so untracked show again.
	if m.hideUntracked {
		t.Error("saving settings should reset the session toggle to the persisted default")
	}
	if !fileTreeHasPath(m, "scratch.txt") {
		t.Error("untracked file should reappear after the save reset the session toggle")
	}
}

func TestDiffWithTabsDoesNotOverflowWidth(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded},
	})
	// svn diff output is full of tabs; no rendered row may exceed the terminal
	// width, or it wraps and the whole frame overflows (panes appear to resize).
	next, _ := m.Update(diffLoadedMsg{
		path: "added.txt",
		diff: "Index: added.txt\n--- added.txt\t(nonexistent)\n+++ added.txt\t(working copy)\n@@ -0,0 +1 @@\n+new",
	})
	m = next.(*Model)

	for i, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w != 80 {
			t.Errorf("line %d width = %d, want 80: %q", i, w, stripANSI(line))
		}
	}
}

// TestDiffGutterStaysPinnedWhenScrolled proves the Main viewport keeps a unified
// diff's +/- marker column pinned to the left while the body scrolls: after
// scrolling fully right, the added and removed rows still begin with their marker.
func TestDiffGutterStaysPinnedWhenScrolled(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "wide.txt", State: svn.StateModified},
	})
	// Body lines far wider than the Main pane, so the diff scrolls horizontally
	// and an unpinned marker would otherwise slide out of view.
	long := strings.Repeat("abcdefghij", 12) // 120 columns
	next, _ := m.Update(diffLoadedMsg{
		path: "wide.txt",
		diff: "@@ -1 +1 @@\n-" + long + "\n+" + long,
	})
	m = next.(*Model)
	before := stripANSI(m.main.View())

	// Scroll the Main viewport as far right as it goes.
	m.main.Focus()
	m.main.Update(tea.KeyMsg{Type: tea.KeyEnd})
	after := stripANSI(m.main.View())

	if before == after {
		t.Fatal("diff did not scroll horizontally; the gutter cannot be observed")
	}
	var minus, plus bool
	for _, ln := range strings.Split(after, "\n") {
		switch {
		case strings.HasPrefix(ln, "-"):
			minus = true
		case strings.HasPrefix(ln, "+"):
			plus = true
		}
	}
	if !minus || !plus {
		t.Errorf("scrolled diff lost its +/- gutter:\n%s", after)
	}
}

func TestColorizeDiff(t *testing.T) {
	// Emit ANSI so the styling is observable, then restore the Ascii profile the
	// rest of the suite relies on.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	diff := "Index: a.txt\n--- a.txt\t(revision 1)\n+++ a.txt\t(working copy)\n" +
		"@@ -1,2 +1,2 @@\n context\n-old\n+new"
	got := colorizeDiff(theme.Default(), diff)

	// Coloring must only add styling, never alter the underlying text.
	if plain := stripANSI(got); plain != diff {
		t.Fatalf("colorize changed content:\n got: %q\nwant: %q", plain, diff)
	}

	// Metadata, hunk, add and delete lines are colored; context lines are not.
	wantColored := map[string]bool{
		"Index: a.txt":              true,
		"--- a.txt\t(revision 1)":   true,
		"+++ a.txt\t(working copy)": true,
		"@@ -1,2 +1,2 @@":           true,
		"-old":                      true,
		"+new":                      true,
		" context":                  false,
	}
	for _, ln := range strings.Split(got, "\n") {
		plain := stripANSI(ln)
		want, tracked := wantColored[plain]
		if !tracked {
			continue
		}
		if colored := ln != plain; colored != want {
			t.Errorf("line %q colored=%v, want %v", plain, colored, want)
		}
	}
}

func TestLogPanelSelectionUpdatesMain(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(logLoadedMsg{entries: []svn.LogEntry{
		{Revision: "42", Author: "alice", Message: "first commit"},
		{Revision: "41", Author: "bob", Message: "second commit"},
	}})
	m = next.(*Model)

	// The Log panel renders history even while unfocused.
	if view := stripANSI(m.View()); !strings.Contains(view, "r42") || !strings.Contains(view, "alice") {
		t.Errorf("view missing log history, got:\n%s", view)
	}

	// Focusing the Log panel (key "3") points Main at the log selection.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "r42") || !strings.Contains(main, "first commit") {
		t.Errorf("main should show the first revision detail, got:\n%s", main)
	}

	// Moving down updates Main to the next revision.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	sel, ok := cmd().(uimsg.SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg, got %T", cmd())
	}
	next, _ = m.Update(sel)
	m = next.(*Model)
	if main := stripANSI(m.main.View()); !strings.Contains(main, "second commit") {
		t.Errorf("main should follow the log selection, got:\n%s", main)
	}
}

func TestModelGoldenLayout(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded, Changelist: "revision:staged"},
		{Path: "modified.go", State: svn.StateModified},
		{Path: "gone.txt", State: svn.StateDeleted},
	})
	// History reveals HEAD (r50) so the Status panel shows every field.
	next, _ := m.Update(logLoadedMsg{entries: []svn.LogEntry{{Revision: "50"}, {Revision: "42"}}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestCountLabel(t *testing.T) {
	tests := []struct {
		name               string
		index, shown, full int
		want               string
	}{
		{"empty view", 0, 0, 0, ""},
		{"all hidden", 0, 0, 29, "0 of 0 (29)"},
		{"nothing hidden", 1, 3, 3, "1 of 3"},
		{"mid selection", 2, 4, 4, "2 of 4"},
		{"cursor on root", 0, 3, 3, "0 of 3"},
		{"some hidden", 1, 16, 29, "1 of 16 (29)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countLabel(tt.index, tt.shown, tt.full); got != tt.want {
				t.Errorf("countLabel(%d, %d, %d) = %q, want %q", tt.index, tt.shown, tt.full, got, tt.want)
			}
		})
	}
}

func TestFileLeafStats(t *testing.T) {
	// A tree with the synthetic root, one directory and two files under it.
	rows := buildFileTree([]svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	}, nil)

	if _, count := fileLeafStats(rows, 0); count != 2 {
		t.Fatalf("leaf count = %d, want 2 (root and dir rows must not count)", count)
	}
	// Cursor on the root and the directory rows sits above/at zero passed leaves.
	if index, _ := fileLeafStats(rows, 0); index != 0 {
		t.Errorf("root cursor index = %d, want 0", index)
	}
	// Cursor on the first and second file leaf gives their 1-based positions.
	last := len(rows) - 1
	if index, _ := fileLeafStats(rows, last); index != 2 {
		t.Errorf("last-leaf cursor index = %d, want 2", index)
	}
}

func TestFilesFooterReportsHiddenCount(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "tracked.go", State: svn.StateModified},
		{Path: "untracked1.txt", State: svn.StateUnversioned},
		{Path: "untracked2.txt", State: svn.StateUnversioned},
	})

	// With everything visible the footer counts rows without a bracketed total.
	if got := m.filesFooter(); strings.Contains(got, "(") {
		t.Errorf("footer shows a hidden count with nothing hidden: %q", got)
	}

	// Hiding untracked files drops two leaves; the footer then reports the full
	// count in brackets, and the rendered Files panel border carries it.
	m.hideUntracked = true
	m.rebuildFileTree()
	got := m.filesFooter()
	if !strings.Contains(got, "(") {
		t.Errorf("footer should report a bracketed full count when untracked files are hidden, got %q", got)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, got) {
		t.Errorf("Files panel border missing footer %q\n%s", got, view)
	}
}

func TestStatusPanelShowsAbout(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	// Widen so the full project URLs fit without horizontal scrolling.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(*Model)
	// Focusing the Status panel (1) turns Main into the about screen.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = next.(*Model)

	view := stripANSI(m.View())
	for _, want := range []string{
		"revision/issues",
		"revision/releases",
		"Chirag Bapat",
		"Press S",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("about screen missing %q\n---\n%s", want, view)
		}
	}
}

func TestStatusPanelAboutGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestStageTargetDecision(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "mod.go", State: svn.StateModified},
		{Path: "staged.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})

	// Cursor starts on the first (path-sorted) item: an unstaged change → stage.
	if act, ok := m.stageTarget(); !ok || act.path != "mod.go" || act.add || !act.stage {
		t.Errorf("mod.go: got %+v (ok=%v), want {mod.go add:false stage:true}", act, ok)
	}

	// Move to the already-staged file → unstage.
	m.files.Update(tea.KeyMsg{Type: tea.KeyDown})
	if act, ok := m.stageTarget(); !ok || act.path != "staged.go" || act.stage {
		t.Errorf("staged.go: got %+v (ok=%v), want {staged.go stage:false}", act, ok)
	}

	// Move to the unversioned file → add and stage in one step.
	m.files.Update(tea.KeyMsg{Type: tea.KeyDown})
	if act, ok := m.stageTarget(); !ok || act.path != "untracked.txt" || !act.add || !act.stage {
		t.Errorf("untracked.txt: got %+v (ok=%v), want {untracked.txt add:true stage:true}", act, ok)
	}
}

func TestSpaceStagesSelectedFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	// Space on a stageable file (Files panel is focused by default) yields a
	// command; it runs svn, so we assert only that it exists, not its result.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected a stage command for a modified file")
	}
}

func TestSpaceAddsUnversionedFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})
	// An unversioned file is now addable: space produces an add+stage command.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected an add+stage command for an unversioned file")
	}
	if act, ok := m.stageTarget(); !ok || !act.add {
		t.Errorf("unversioned stage target should svn add first, got %+v (ok=%v)", act, ok)
	}
}

func TestSpaceIgnoresIgnoredFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "build.log", State: svn.StateIgnored},
	})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd != nil {
		t.Error("an ignored file should not produce a stage command")
	}
}

// selectDirRow parks the Files-panel cursor on the tree row for the named
// directory segment, failing the test when no such directory row exists.
func selectDirRow(t *testing.T, m *Model, name string) {
	t.Helper()
	for i, n := range m.files.Items() {
		if n.Item == nil && n.Name == name {
			m.files.SetIndex(i)
			return
		}
	}
	t.Fatalf("no directory row named %q in the file tree", name)
}

func TestSpaceStagesDirectory(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
		{Path: "readme.md", State: svn.StateModified},
	})
	// The cursor opens on the first file; move it onto the src/ directory row.
	selectDirRow(t, m, "src")
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected a stage command when space is pressed on a directory row")
	}
}

func TestSpaceStagesRootDirectory(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "readme.md", State: svn.StateModified},
	})
	m.files.SetIndex(0) // the synthetic "/" root row covers the whole tree
	root, ok := m.files.Selected()
	if !ok || root.Path != fileTreeRoot {
		t.Fatalf("expected cursor on the / root row, got %+v (ok=%v)", root, ok)
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected a stage command when space is pressed on the root row")
	}
	if acts := directoryStageActions(root, m.fileItems); len(acts) != 2 {
		t.Errorf("root should stage all 2 files, got %d: %+v", len(acts), acts)
	}
}

func TestSpaceOnFullyStagedDirectoryUnstages(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified, Changelist: stagedChangelist},
		{Path: "src/b.go", State: svn.StateModified, Changelist: stagedChangelist},
	})
	selectDirRow(t, m, "src")
	// Everything under src/ is already staged, so pressing space again unstages
	// the whole subtree.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected an unstage command when a fully staged directory is toggled")
	}
	acts := directoryUnstageActions(fileNode{Name: "src", Path: "src"}, m.fileItems)
	if len(acts) != 2 {
		t.Fatalf("expected 2 unstage actions under src/, got %d: %+v", len(acts), acts)
	}
	for _, a := range acts {
		if a.stage || a.add {
			t.Errorf("an unstage action should neither stage nor add, got %+v", a)
		}
	}
}

func TestSpaceOnDirectoryInNamedChangelistsRemovesThem(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "src/b.go", State: svn.StateModified, Changelist: "bugfix"},
	})
	selectDirRow(t, m, "src")
	// Every file under src/ already belongs to a named changelist; nothing is left
	// to stage, so space removes them all from their changelists.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd == nil {
		t.Error("expected a command removing named-changelist files under the directory")
	}
	acts := directoryUnstageActions(fileNode{Name: "src", Path: "src"}, m.fileItems)
	if len(acts) != 2 {
		t.Fatalf("expected 2 removals under src/, got %d: %+v", len(acts), acts)
	}
	for _, a := range acts {
		if a.stage || a.add {
			t.Errorf("a removal should neither stage nor add, got %+v", a)
		}
	}
}

func TestSpaceOnDirectoryWithNothingToToggle(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		// An ignored file can be neither staged nor removed from a changelist, so the
		// directory toggle has nothing to do.
		{Path: "src/build.log", State: svn.StateIgnored},
	})
	selectDirRow(t, m, "src")
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd != nil {
		t.Error("a directory with only ignored files should produce no command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "nothing to stage or unstage under src/") {
		t.Errorf("expected a nothing-to-toggle toast, got:\n%s", view)
	}
}

func TestDirectoryStageActionsSelectsStageableFiles(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/new.txt", State: svn.StateUnversioned},
		{Path: "src/staged.go", State: svn.StateModified, Changelist: stagedChangelist},
		{Path: "src/named.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "src/build.log", State: svn.StateIgnored},
		{Path: "docs/readme.md", State: svn.StateModified},
	}
	acts := directoryStageActions(fileNode{Name: "src", Path: "src"}, items)

	byPath := map[string]stageAction{}
	for _, a := range acts {
		byPath[a.path] = a
	}
	if len(acts) != 2 {
		t.Fatalf("expected 2 stage actions under src/, got %d: %+v", len(acts), acts)
	}
	if a, ok := byPath["src/a.go"]; !ok || a.add || !a.stage {
		t.Errorf("src/a.go: got %+v (ok=%v), want a plain stage", a, ok)
	}
	if a, ok := byPath["src/new.txt"]; !ok || !a.add || !a.stage {
		t.Errorf("src/new.txt: got %+v (ok=%v), want add+stage", a, ok)
	}
	if _, ok := byPath["src/staged.go"]; ok {
		t.Error("an already-staged file should be skipped")
	}
	if _, ok := byPath["src/named.go"]; ok {
		t.Error("a file in a named changelist should be skipped")
	}
	if _, ok := byPath["src/build.log"]; ok {
		t.Error("an ignored file should be skipped")
	}
	if _, ok := byPath["docs/readme.md"]; ok {
		t.Error("a file outside src/ should be excluded")
	}
}

func TestDirectoryUnstageActionsSelectsChangelistFiles(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified, Changelist: stagedChangelist},
		{Path: "src/b.go", State: svn.StateModified},
		{Path: "src/named.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "docs/c.go", State: svn.StateModified, Changelist: stagedChangelist},
	}
	acts := directoryUnstageActions(fileNode{Name: "src", Path: "src"}, items)

	byPath := map[string]stageAction{}
	for _, a := range acts {
		byPath[a.path] = a
	}
	if len(acts) != 2 {
		t.Fatalf("expected 2 unstage actions under src/, got %d: %+v", len(acts), acts)
	}
	if a, ok := byPath["src/a.go"]; !ok || a.stage || a.add {
		t.Errorf("src/a.go (staged): got %+v (ok=%v), want a plain unstage", a, ok)
	}
	if a, ok := byPath["src/named.go"]; !ok || a.stage || a.add {
		t.Errorf("src/named.go (named changelist): got %+v (ok=%v), want a plain unstage", a, ok)
	}
	if _, ok := byPath["src/b.go"]; ok {
		t.Error("a file in no changelist has nothing to remove and should be skipped")
	}
	if _, ok := byPath["docs/c.go"]; ok {
		t.Error("a file outside src/ should be excluded")
	}
}

func TestStagedFileShowsMarker(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "staged.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	if view := stripANSI(m.View()); !strings.Contains(view, "●") {
		t.Errorf("expected a staged marker in the files list, got:\n%s", view)
	}
}

func TestCommitRequiresStagedFiles(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if cmd != nil {
		t.Error("commit with nothing staged should not run a command")
	}
	if m.editing {
		t.Error("the editor should not open with nothing staged")
	}
}

func TestCommitEditorOpensAndSubmits(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if !m.editing {
		t.Fatal("the editor should open with a staged file")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Commit message") {
		t.Errorf("expected the commit editor to overlay the layout, got:\n%s", view)
	}

	// Type a message; ctrl+s makes the editor emit SubmitMsg.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("do it")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected a SubmitMsg command from the editor")
	}
	sub, ok := cmd().(uimsg.SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", cmd())
	}
	if sub.Value != "do it" {
		t.Errorf("submitted value = %q, want %q", sub.Value, "do it")
	}

	// Handing the SubmitMsg back closes the editor and yields a commit command.
	next, cmd = m.Update(sub)
	m = next.(*Model)
	if m.editing {
		t.Error("the editor should close after submit")
	}
	if cmd == nil {
		t.Error("expected a commit command after submit")
	}
}

func TestCommitEditorCancels(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)

	// Esc emits DismissMsg, which the app handles to close the editor.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.editing {
		t.Error("the editor should close on cancel")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Commit message") {
		t.Error("the layout should return after cancelling the editor")
	}
}

func TestCommitResultShowsToast(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(committedMsg{revision: "128"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "committed r128") {
		t.Errorf("expected the commit toast, got:\n%s", view)
	}
}

func TestConfigValidatorResetsUnknownTheme(t *testing.T) {
	validate := ConfigValidator()

	// A theme that no longer exists is a conflict: it resets to the default and
	// is reported so the user can be told.
	cfg := config.Default()
	cfg.Theme = "retired-theme"
	conflicts := validate(&cfg)
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %v, want exactly one", conflicts)
	}
	if cfg.Theme != config.Default().Theme {
		t.Errorf("Theme = %q, want reset to default %q", cfg.Theme, config.Default().Theme)
	}

	// A known theme and a blank theme are both left untouched with no conflict.
	for _, name := range []string{"dracula", ""} {
		cfg := config.Default()
		cfg.Theme = name
		if conflicts := validate(&cfg); len(conflicts) != 0 {
			t.Errorf("theme %q reported conflicts %v, want none", name, conflicts)
		}
		if cfg.Theme != name {
			t.Errorf("theme %q was modified to %q", name, cfg.Theme)
		}
	}
}

func TestStartupNoticeShowsToast(t *testing.T) {
	m := sizedModel(t)
	next, _ := m.Update(startupNoticeMsg{text: "config: logLimit 0 is invalid; reset to 100"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "logLimit 0 is invalid") {
		t.Errorf("expected the startup notice toast, got:\n%s", view)
	}
}

func TestStartupNoticeCmdEmitsMessage(t *testing.T) {
	msg := startupNoticeCmd("hello")()
	sn, ok := msg.(startupNoticeMsg)
	if !ok || sn.text != "hello" {
		t.Fatalf("startupNoticeCmd() = %#v, want startupNoticeMsg{text:%q}", msg, "hello")
	}
}

func TestSetStartupNoticeTrims(t *testing.T) {
	m := sizedModel(t)
	m.SetStartupNotice("  spaced  ")
	if m.startupNotice != "spaced" {
		t.Errorf("startupNotice = %q, want %q", m.startupNotice, "spaced")
	}
	// A blank notice (no conflicts to report) must clear to empty so Init
	// schedules nothing and no toast appears.
	m.SetStartupNotice("   ")
	if m.startupNotice != "" {
		t.Errorf("blank notice should clear to empty, got %q", m.startupNotice)
	}
}

func TestCommitEditorGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Fix status parsing")})
	golden.RequireEqual(t, []byte(m.View()))
}

func TestRevertRequiresConfirmation(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	// r on a dirty file opens the confirmation modal rather than acting.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("expected the confirmation modal to open")
	}
	if cmd != nil {
		t.Error("opening the modal should not run a command yet")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Revert changes?") {
		t.Errorf("expected the revert prompt, got:\n%s", view)
	}

	// Confirming emits ConfirmMsg, which the app turns into the revert command.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a ConfirmMsg command from the modal")
	}
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	next, cmd = m.Update(conf)
	m = next.(*Model)
	if m.confirming {
		t.Error("the modal should close after confirming")
	}
	if cmd == nil {
		t.Error("expected a revert command after confirming")
	}
}

func TestRevertGuardOnUnversioned(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(*Model)
	if m.confirming {
		t.Error("an unversioned file has nothing to revert; no modal should open")
	}
	if cmd != nil {
		t.Error("the revert guard should not run a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "nothing to revert") {
		t.Errorf("expected a guard toast, got:\n%s", view)
	}
}

func TestRevertDirectoryRequiresConfirmation(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	// Single-file revert returns nil on a directory row, so an opened modal here
	// proves the directory fan-out ran.
	selectDirRow(t, m, "src")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("r on a directory row should open the confirmation modal")
	}
	if cmd != nil {
		t.Error("opening the modal should not run a command yet")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Revert changes?") {
		t.Errorf("expected the revert prompt, got:\n%s", view)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	if _, cmd = m.Update(conf); cmd == nil {
		t.Error("expected a revert command after confirming a directory revert")
	}
}

func TestDirectoryRevertPathsSelectsDirtyFiles(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/gone.go", State: svn.StateDeleted},
		{Path: "src/new.txt", State: svn.StateUnversioned},
		{Path: "src/build.log", State: svn.StateIgnored},
		{Path: "docs/readme.md", State: svn.StateModified},
	}
	paths := directoryRevertPaths(fileNode{Name: "src", Path: "src"}, items)

	got := map[string]bool{}
	for _, p := range paths {
		got[p] = true
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 revertable paths under src/, got %d: %v", len(paths), paths)
	}
	if !got["src/a.go"] || !got["src/gone.go"] {
		t.Errorf("expected the modified and deleted files, got %v", paths)
	}
	if got["src/new.txt"] {
		t.Error("an unversioned file has nothing to revert and should be skipped")
	}
	if got["src/build.log"] {
		t.Error("an ignored file should be skipped")
	}
	if got["docs/readme.md"] {
		t.Error("a file outside src/ should be excluded")
	}
}

func TestRevertDirectoryNothingToRevert(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/new.txt", State: svn.StateUnversioned},
		{Path: "src/build.log", State: svn.StateIgnored},
	})
	selectDirRow(t, m, "src")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(*Model)
	if m.confirming {
		t.Error("a directory with nothing revertable should not open the modal")
	}
	if cmd != nil {
		t.Error("the guard should not run a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "nothing to revert under src/") {
		t.Errorf("expected a nothing-to-revert toast, got:\n%s", view)
	}
}

func TestDeleteConfirmationCancels(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("d should open the delete confirmation")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Delete file?") {
		t.Errorf("expected the delete prompt, got:\n%s", view)
	}

	// Esc emits DismissMsg; the app closes the modal and runs nothing.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	dis, ok := cmd().(uimsg.DismissMsg)
	if !ok {
		t.Fatalf("expected DismissMsg, got %T", cmd())
	}
	next, cmd = m.Update(dis)
	m = next.(*Model)
	if m.confirming {
		t.Error("the modal should close on cancel")
	}
	if cmd != nil {
		t.Error("cancelling delete should not run a command")
	}
}

func TestDeleteUnversionedWarnsDiskRemoval(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "untracked.txt", State: svn.StateUnversioned},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	view := stripANSI(m.View())
	if !strings.Contains(view, "untracked") || !strings.Contains(view, "disk") {
		t.Errorf("expected an unversioned-delete warning, got:\n%s", view)
	}
}

func TestDeleteDirectoryRequiresConfirmation(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateDeleted},
	})
	selectDirRow(t, m, "src")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("d on a directory row should open the delete confirmation")
	}
	if cmd != nil {
		t.Error("opening the modal should not run a command yet")
	}
	// The plural title distinguishes the directory prompt from the single-file one.
	if view := stripANSI(m.View()); !strings.Contains(view, "Delete files?") {
		t.Errorf("expected the directory delete prompt, got:\n%s", view)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	if _, cmd = m.Update(conf); cmd == nil {
		t.Error("expected a delete command after confirming a directory delete")
	}
}

func TestDirectoryDeleteActionsSkipsIgnored(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/new.txt", State: svn.StateUnversioned},
		{Path: "src/build.log", State: svn.StateIgnored},
		{Path: "docs/readme.md", State: svn.StateModified},
	}
	acts := directoryDeleteActions(fileNode{Name: "src", Path: "src"}, items)

	byPath := map[string]deleteAction{}
	for _, a := range acts {
		byPath[a.path] = a
	}
	if len(acts) != 2 {
		t.Fatalf("expected 2 delete actions under src/, got %d: %+v", len(acts), acts)
	}
	if a, ok := byPath["src/a.go"]; !ok || a.unversioned {
		t.Errorf("src/a.go: got %+v (ok=%v), want a versioned delete", a, ok)
	}
	if a, ok := byPath["src/new.txt"]; !ok || !a.unversioned {
		t.Errorf("src/new.txt: got %+v (ok=%v), want an unversioned (disk) delete", a, ok)
	}
	if _, ok := byPath["src/build.log"]; ok {
		t.Error("an ignored file should be skipped")
	}
	if _, ok := byPath["docs/readme.md"]; ok {
		t.Error("a file outside src/ should be excluded")
	}
}

func TestDeleteDirectoryMessageSeparatesDiskRemoval(t *testing.T) {
	n := fileNode{Name: "src", Path: "src"}
	mixed := deleteDirectoryMessage(n, []deleteAction{
		{path: "src/a.go"},
		{path: "src/new.txt", unversioned: true},
	})
	if !strings.Contains(mixed, "scheduled for deletion") || !strings.Contains(mixed, "permanently removed from disk") {
		t.Errorf("mixed message should mention both outcomes, got: %q", mixed)
	}

	versioned := deleteDirectoryMessage(n, []deleteAction{{path: "src/a.go"}})
	if !strings.Contains(versioned, "scheduled for deletion") || strings.Contains(versioned, "disk") {
		t.Errorf("versioned-only message should not warn about disk, got: %q", versioned)
	}

	unversioned := deleteDirectoryMessage(n, []deleteAction{{path: "src/new.txt", unversioned: true}})
	if !strings.Contains(unversioned, "permanently removed from disk") || strings.Contains(unversioned, "scheduled") {
		t.Errorf("unversioned-only message should only warn about disk, got: %q", unversioned)
	}
}

func TestDeleteDirectoryNothingToDelete(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/build.log", State: svn.StateIgnored},
	})
	selectDirRow(t, m, "src")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	if m.confirming {
		t.Error("a directory with only ignored files should not open the modal")
	}
	if cmd != nil {
		t.Error("the guard should not run a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "nothing to delete under src/") {
		t.Errorf("expected a nothing-to-delete toast, got:\n%s", view)
	}
}

func TestUpdateRunsCommand(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	// u (off the Log panel) confirms an update to the latest revision; with no
	// conflicts the single confirm runs the update.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("u should open the update confirmation")
	}
	if cmd != nil {
		t.Error("opening the modal should not run a command yet")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Update working copy?") {
		t.Errorf("expected the update confirm, got:\n%s", view)
	}
	m, cmd = confirmModal(t, m)
	if m.confirming {
		t.Error("the modal should close after confirming")
	}
	if cmd == nil {
		t.Error("confirming should run the update command")
	}
}

func TestUpdateToHeadConflictAsksAgain(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "conflicted.go", State: svn.StateConflicted},
	})
	// Default focus is the Files panel, so u targets HEAD rather than a revision.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "Update working copy?") {
		t.Fatalf("expected the default update confirm, got:\n%s", view)
	}

	// Accepting the first confirm surfaces the conflict warning, not the update.
	m, cmd := confirmModal(t, m)
	if cmd != nil {
		t.Error("the update must not run before the conflict confirm is accepted")
	}
	if !m.confirming {
		t.Fatal("a second confirmation should be open when conflicts exist")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Conflicts present") || !strings.Contains(view, "untouched") {
		t.Errorf("expected the conflict warning, got:\n%s", view)
	}

	// Accepting the conflict warning runs the update.
	m, cmd = confirmModal(t, m)
	if m.confirming {
		t.Error("both modals should be closed after the second confirm")
	}
	if cmd == nil {
		t.Error("expected the update command after the second confirm")
	}
}

func TestUpdateResultShowsToast(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updatedMsg{revision: "7"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "updated to r7") {
		t.Errorf("expected the update toast, got:\n%s", view)
	}
}

func TestRevisionIndicatorTracksUpdate(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	// History (newest first) reveals HEAD as r50; the working copy opens at r42.
	next, _ := m.Update(logLoadedMsg{entries: []svn.LogEntry{
		{Revision: "50"}, {Revision: "49"}, {Revision: "42"},
	}})
	m = next.(*Model)
	// The Status panel lists the checked-out revision and HEAD on their own rows.
	if view := stripANSI(m.View()); !strings.Contains(view, "Revision  r42") || !strings.Contains(view, "HEAD      r50") {
		t.Errorf("expected the status panel to show r42 and HEAD r50, got:\n%s", view)
	}

	// Updating to HEAD moves the checked-out revision to r50.
	next, _ = m.Update(updatedMsg{revision: "50"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "Revision  r50") {
		t.Errorf("expected the checked-out revision to track to r50, got:\n%s", view)
	}
}

func TestLogStarsWorkingCopyRevision(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	// The working copy opens at r42 (from info); history includes it.
	next, _ := m.Update(logLoadedMsg{entries: []svn.LogEntry{
		{Revision: "50"}, {Revision: "42"}, {Revision: "41"},
	}})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "* r42") {
		t.Errorf("expected an asterisk on the working-copy revision r42, got:\n%s", view)
	}
	if view := stripANSI(m.View()); strings.Contains(view, "* r50") {
		t.Errorf("only the working-copy revision should be starred, got:\n%s", view)
	}

	// After updating to r50 the star follows the working copy.
	next, _ = m.Update(updatedMsg{revision: "50"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "* r50") {
		t.Errorf("expected the asterisk to move to r50 after updating, got:\n%s", view)
	}
}

func TestRenderLogRowColorsWorkingCopyAsterisk(t *testing.T) {
	// Emit ANSI so the styling is observable, then restore the Ascii profile.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	th := theme.Default()

	// The working-copy row carries a coloured asterisk that strips to "* r42".
	star := renderLogRow(svn.LogEntry{Revision: "42"}, "42", th)[0]
	if stripANSI(star) == star {
		t.Errorf("expected the asterisk to be coloured (ANSI), got plain %q", star)
	}
	if got := stripANSI(star); got != "* r42" {
		t.Errorf("marker cell should read %q, got %q", "* r42", got)
	}

	// Other rows are a plain, unstyled two-space prefix of the same width.
	other := renderLogRow(svn.LogEntry{Revision: "41"}, "42", th)[0]
	if other != "  r41" {
		t.Errorf("non-working-copy row should be plain %q, got %q", "  r41", other)
	}
}

func TestUpdateShowsProgressModal(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	// History reveals HEAD r50; the working copy opens at r42.
	next, _ := m.Update(logLoadedMsg{entries: []svn.LogEntry{
		{Revision: "50"}, {Revision: "42"},
	}})
	m = next.(*Model)

	// u (off the Log panel) confirms a HEAD update; with no conflicts the single
	// confirm dispatches it and raises the progress modal. The exact target is
	// only known once svn reports it, so the box names "the latest revision".
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = next.(*Model)
	m, cmd := confirmModal(t, m)
	if !m.updatingWC {
		t.Fatal("expected the updating overlay after confirming")
	}
	if cmd == nil {
		t.Error("expected the update command to run")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Updating from r42 to the latest revision") {
		t.Errorf("expected the progress modal, got:\n%s", view)
	}

	// While the update runs, keys are swallowed so the panels stay put.
	if _, kc := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}); kc != nil {
		t.Error("keys should be ignored while the update runs")
	}

	// Completion clears the overlay.
	next, _ = m.Update(updatedMsg{revision: "50"})
	m = next.(*Model)
	if m.updatingWC {
		t.Error("the overlay should clear when the update completes")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Updating from") {
		t.Errorf("the progress modal should be gone after completion, got:\n%s", view)
	}
}

func TestUpdateToRevisionProgressShowsTarget(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(logLoadedMsg{entries: []svn.LogEntry{
		{Revision: "50"}, {Revision: "42"},
	}})
	m = next.(*Model)
	// Focus the Log panel (r50 is the selected row) and update to it with space.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)
	m, _ = confirmModal(t, m)
	// A specific revision is known up front, so the box names it exactly.
	if view := stripANSI(m.View()); !strings.Contains(view, "Updating from r42 to r50") {
		t.Errorf("expected the exact target revision in the progress modal, got:\n%s", view)
	}
}

func TestUpdateToRevisionConfirms(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(logLoadedMsg{entries: []svn.LogEntry{
		{Revision: "42", Author: "alice", Message: "first"},
		{Revision: "41", Author: "bob", Message: "second"},
	}})
	m = next.(*Model)

	// Focus the Log panel (key "3"), then space targets the selected revision.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)
	if !m.confirming {
		t.Fatal("space on the Log panel should open the confirmation modal")
	}
	if cmd != nil {
		t.Error("opening the modal should not run a command yet")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Update to revision?") || !strings.Contains(view, "r42") {
		t.Errorf("expected the update-to-revision prompt, got:\n%s", view)
	}

	// Confirming turns into the update command.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a ConfirmMsg command from the modal")
	}
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	next, cmd = m.Update(conf)
	m = next.(*Model)
	if m.confirming {
		t.Error("the modal should close after confirming")
	}
	if cmd == nil {
		t.Error("expected an update command after confirming")
	}
}

func TestUpdateToRevisionNoSelectionWarns(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	// Focus the Log panel; with no history there is nothing to select.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)
	if m.confirming {
		t.Error("no revision selected should not open the modal")
	}
	if cmd != nil {
		t.Error("no revision selected should not run a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no revision selected") {
		t.Errorf("expected a guard toast, got:\n%s", view)
	}
}

// focusLogWithRevision loads a single revision, focuses the Log panel, and
// returns the model ready for an update-to-revision keypress.
func focusLogWithRevision(t *testing.T, items []svn.StatusItem) *Model {
	t.Helper()
	m := loadItems(t, sizedModel(t), items)
	next, _ := m.Update(logLoadedMsg{entries: []svn.LogEntry{
		{Revision: "42", Author: "alice", Message: "first"},
	}})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	return next.(*Model)
}

// confirmModal presses enter on the open confirmation modal and feeds the
// resulting ConfirmMsg back into the model, returning the follow-up command.
func confirmModal(t *testing.T, m *Model) (*Model, tea.Cmd) {
	t.Helper()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("the modal should emit a command on enter")
	}
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	next, out := m.Update(conf)
	return next.(*Model), out
}

func TestUpdateToRevisionConflictAsksAgain(t *testing.T) {
	m := focusLogWithRevision(t, []svn.StatusItem{
		{Path: "conflicted.go", State: svn.StateConflicted},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "Update to revision?") {
		t.Fatalf("expected the default update confirm, got:\n%s", view)
	}

	// Accepting the first confirm surfaces the conflict warning rather than
	// running the update.
	m, cmd := confirmModal(t, m)
	if cmd != nil {
		t.Error("the update must not run before the conflict confirm is accepted")
	}
	if !m.confirming {
		t.Fatal("a second confirmation should be open when conflicts exist")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Conflicts present") || !strings.Contains(view, "untouched") {
		t.Errorf("expected the conflict warning, got:\n%s", view)
	}

	// Accepting the conflict warning finally runs the update.
	m, cmd = confirmModal(t, m)
	if m.confirming {
		t.Error("both modals should be closed after the second confirm")
	}
	if cmd == nil {
		t.Error("expected the update command after the second confirm")
	}
}

func TestUpdateToRevisionNoConflictSkipsSecondConfirm(t *testing.T) {
	m := focusLogWithRevision(t, []svn.StatusItem{
		{Path: "clean.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)

	// With no conflicts the single confirm runs the update directly.
	m, cmd := confirmModal(t, m)
	if m.confirming {
		t.Error("a clean working copy should not open a second confirmation")
	}
	if cmd == nil {
		t.Error("expected the update command straight after the only confirm")
	}
}

func TestUpdateToRevisionConflictCancelAborts(t *testing.T) {
	m := focusLogWithRevision(t, []svn.StatusItem{
		{Path: "conflicted.go", State: svn.StateConflicted},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(*Model)
	m, _ = confirmModal(t, m) // accept the first confirm to reach the warning
	if !m.confirming {
		t.Fatal("expected the conflict confirmation to be open")
	}

	// Cancelling the conflict warning drops the staged update entirely.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should emit a dismiss command")
	}
	dis, ok := cmd().(uimsg.DismissMsg)
	if !ok {
		t.Fatalf("expected DismissMsg, got %T", cmd())
	}
	next, _ = m.Update(dis)
	m = next.(*Model)
	if m.confirming {
		t.Error("cancelling should close the modal")
	}
	if m.pending != nil {
		t.Error("cancelling must clear the pending update")
	}
	if m.updateConflictPrompt != "" {
		t.Error("cancelling must clear the staged conflict prompt")
	}
}

func TestRevertResultShowsToast(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, cmd := m.Update(revertedMsg{path: "modified.go"})
	m = next.(*Model)
	if cmd == nil {
		t.Error("a revert should trigger a status reload")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "reverted modified.go") {
		t.Errorf("expected the revert toast, got:\n%s", view)
	}
}

func TestToastDismissedOnKey(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified},
		{Path: "b.go", State: svn.StateModified},
	})
	next, _ := m.Update(committedMsg{revision: "9"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "committed r9") {
		t.Fatalf("expected the commit toast, got:\n%s", view)
	}
	// Any interaction (here: navigating the Files panel) clears the toast.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	if view := stripANSI(m.View()); strings.Contains(view, "committed r9") {
		t.Errorf("the toast should clear on the next key, got:\n%s", view)
	}
}

func TestModalConfirmGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
	})
	// The cursor opens on the app.go leaf (the tree skips the / root and the
	// internal/ and app/ directory rows), so delete targets the file directly.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestChangesTreeShowsDirectoryTree(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
		{Path: "internal/svn/client.go", State: svn.StateModified},
		{Path: "README.md", State: svn.StateModified},
	})
	// Inspect the built tree rows directly, independent of the panel's visible
	// window: every path segment is its own row and files are basenames.
	var names []string
	for _, n := range m.files.Items() {
		names = append(names, n.Name)
		if n.Item != nil && strings.Contains(n.Name, "/") {
			t.Errorf("file leaf %q should be a basename, not a nested path", n.Name)
		}
	}
	for _, want := range []string{"/", "internal", "app", "svn", "app.go", "client.go", "README.md"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tree rows missing %q, got: %v", want, names)
		}
	}
}

func TestEnterCollapsesDirectory(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
		{Path: "internal/svn/client.go", State: svn.StateModified},
	})
	if view := stripANSI(m.View()); !strings.Contains(view, "app.go") {
		t.Fatalf("expected file leaves visible before collapse, got:\n%s", view)
	}

	// The cursor opens on the first file; move it onto the internal/ directory row.
	for i, n := range m.files.Items() {
		if n.Name == "internal" {
			m.files.SetIndex(i)
			break
		}
	}

	// Enter emits an ActivatedMsg the model turns into a collapse toggle.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg command from enter on a directory")
	}
	act, ok := cmd().(uimsg.ActivatedMsg)
	if !ok {
		t.Fatalf("expected ActivatedMsg, got %T", cmd())
	}
	next, _ := m.Update(act)
	m = next.(*Model)

	// Collapsing internal/ hides its descendants but keeps the directory row.
	view := stripANSI(m.View())
	if strings.Contains(view, "app.go") || strings.Contains(view, "client.go") {
		t.Errorf("collapsing internal/ should hide its descendants, got:\n%s", view)
	}
	if !strings.Contains(view, "internal/") {
		t.Errorf("the collapsed directory row should remain, got:\n%s", view)
	}
}

func TestHelpMenuOpensAndCloses(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})

	// "?" floats the keybindings menu over the layout.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	if !m.helping {
		t.Fatal("expected the help menu to open on ?")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Keybindings", "Stage / unstage", "space", "Quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("help view missing %q\n---\n%s", want, view)
		}
	}

	// While help is open, other keys are captured by the menu — q must not quit.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Error("q should not quit while the help menu is open")
		}
	}
	if !m.helping {
		t.Error("the help menu should stay open on a non-dismiss key")
	}

	// enter must NOT close the help menu — it is a read-only reference.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the resulting ActivatedMsg
		m = next.(*Model)
	}
	if !m.helping {
		t.Error("enter should not close the help menu")
	}

	// esc closes the help menu.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if m.helping {
		t.Error("the help menu should close after esc")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Keybindings") {
		t.Error("the layout should return after closing help")
	}
}

func TestHelpMenuTogglesClosedWithQuestionMark(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	if !m.helping {
		t.Fatal("? should open the help menu")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	if m.helping {
		t.Error("? should toggle the help menu closed")
	}
}

func TestAuthFailureShowsHint(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	authErr := errors.New("svn commit: E170001: No more credentials or we tried too many times.")
	next, _ := m.Update(committedMsg{err: authErr})
	m = next.(*Model)

	view := stripANSI(m.View())
	if !strings.Contains(view, "authentication required") {
		t.Errorf("expected an auth hint toast, got:\n%s", view)
	}
	if strings.Contains(view, "E170001") {
		t.Errorf("the raw svn error should be replaced by the hint, got:\n%s", view)
	}
}

func TestHelpMenuGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestSettingsLivePreviewOnScroll(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	if m.theme != theme.Auto() {
		t.Fatal("initial theme is not Auto()")
	}

	// S opens the settings editor; the Theme field starts on the active theme.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	if !m.configuring {
		t.Fatal("pressing S did not open the settings editor")
	}

	// Navigate down to the Theme field; moving between fields must not preview.
	for i := 0; i < themeFieldIndex; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(*Model)
	}
	if m.theme != theme.Auto() {
		t.Error("navigating to the Theme field should not change the palette")
	}

	// Cycling the Theme field forward live-applies the highlighted theme
	// (auto -> everforest) without persisting the choice.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(*Model)
	if m.theme != theme.Everforest() {
		t.Error("cycling the Theme field did not live-apply Everforest()")
	}
	if m.cfg.Theme != "auto" {
		t.Errorf("preview must not persist; cfg.Theme = %q, want auto", m.cfg.Theme)
	}

	// Esc cancels, reverting the live preview to the original theme.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the DismissMsg
		m = next.(*Model)
	}
	if m.theme != theme.Auto() {
		t.Error("esc did not revert the live preview to the original theme")
	}
	if m.configuring {
		t.Error("esc did not close the settings editor")
	}
}

func TestSettingsOpensAndCancels(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})

	// S floats the settings editor over the layout.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	if !m.configuring {
		t.Fatal("pressing S did not open the settings editor")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Settings", "Log limit", "Editor", "Theme", "Directory diff", "Hide untracked", "SSH key", "Display from", "Diff output"} {
		if !strings.Contains(view, want) {
			t.Errorf("settings view missing %q\n---\n%s", want, view)
		}
	}

	// esc closes it without saving.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the DismissMsg
		m = next.(*Model)
	}
	if m.configuring {
		t.Error("esc should close the settings editor")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Directory diff") {
		t.Error("the layout should return after closing settings")
	}
}

func TestSettingsSavesThemeChange(t *testing.T) {
	// Persist to a throwaway XDG config dir so the real home is untouched.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	if m.theme != theme.Auto() {
		t.Fatal("initial theme is not Auto()")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)

	// Move to the Theme field and cycle one option forward (auto -> everforest).
	for i := 0; i < 2; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(*Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(*Model)

	// ctrl+s saves and closes.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the SubmitMsg
		m = next.(*Model)
	}
	if m.configuring {
		t.Fatal("ctrl+s should close the settings editor")
	}
	if m.theme != theme.Everforest() {
		t.Error("saving did not apply the chosen theme")
	}
	if m.cfg.Theme != "everforest" {
		t.Errorf("cfg.Theme = %q, want everforest", m.cfg.Theme)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.Theme != "everforest" {
		t.Errorf("persisted theme = %q, want everforest", got.Theme)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "settings saved") {
		t.Errorf("expected a saved toast, got:\n%s", view)
	}
}

func TestSettingsSavesDirectoryDiffToggle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	if !m.dirDiff {
		t.Fatal("directory diff should start enabled by default")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	for i := 0; i < 3; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // to the Directory diff field
		m = next.(*Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // toggle off
	m = next.(*Model)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	if m.dirDiff {
		t.Error("saving did not apply the directory-diff toggle")
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.DirectoryDiff {
		t.Error("persisted DirectoryDiff = true, want false")
	}
}

func TestSettingsEditsPersistToConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := sizedModel(t)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	// Move to the Editor field and cycle it forward to nvim (native -> vim -> nvim).
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	for i := 0; i < 2; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = next.(*Model)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(*Model)
	if cmd != nil {
		m.Update(cmd()) // deliver the SubmitMsg; persisting is a side effect
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got.Editor != "nvim" {
		t.Errorf("persisted Editor = %q, want nvim", got.Editor)
	}
}

func TestSettingsFormGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

// availableRelease is the fixture release the update-prompt tests offer.
var availableRelease = selfupdate.Release{Tag: "v1.5.0", Version: "1.5.0", URL: "https://example.test/r"}

func TestUpdatePromptOpensOnAvailable(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)

	if !m.updating {
		t.Fatal("expected the update prompt to open when a newer release is available")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Update available: v1.5.0", "Update with cURL", "Update with Go", "Don't update this time"} {
		if !strings.Contains(view, want) {
			t.Errorf("update prompt missing %q\n---\n%s", want, view)
		}
	}
}

func TestUpdatePromptCurlQuitsWithPendingMethod(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)

	// enter on the first item (Update with cURL) emits an ActivatedMsg…
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a command from selecting a menu item")
	}
	// …which, once delivered, records the method and quits.
	next, cmd = m.Update(cmd())
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a quit command after choosing an update method")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
	method, chosen := m.PendingUpdate()
	if !chosen || method != selfupdate.MethodCurl {
		t.Errorf("PendingUpdate() = (%v, %v), want (curl, true)", method, chosen)
	}
}

func TestUpdatePromptGoQuitsWithPendingMethod(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)

	// Move to the second item (Update with Go), then select it.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	next, _ = m.Update(cmd())
	m = next.(*Model)

	method, chosen := m.PendingUpdate()
	if !chosen || method != selfupdate.MethodGo {
		t.Errorf("PendingUpdate() = (%v, %v), want (go, true)", method, chosen)
	}
}

func TestUpdatePromptDeclineDismissesWithoutUpdate(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)

	// Third item is "Don't update this time": it closes the prompt and records
	// no pending update.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	if m.updating {
		t.Error("the prompt should close after declining")
	}
	if _, chosen := m.PendingUpdate(); chosen {
		t.Error("declining must not record a pending update")
	}
}

func TestUpdatePromptEscDismisses(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver the DismissMsg
		m = next.(*Model)
	}
	if m.updating {
		t.Error("esc should dismiss the update prompt")
	}
	if _, chosen := m.PendingUpdate(); chosen {
		t.Error("dismissing must not record a pending update")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Update available") {
		t.Error("the layout should return after dismissing the update prompt")
	}
}

func TestUpdatePromptSuppressedWhileOverlayActive(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	// Open the commit editor, then let the update check land underneath it.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if !m.editing {
		t.Fatal("expected the commit editor to be open")
	}
	next, _ = m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)
	if m.updating {
		t.Error("the update prompt must not steal focus from an active overlay")
	}
}

func TestUpdatePromptGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, _ := m.Update(updateAvailableMsg{rel: availableRelease})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestChangelistGrouping(t *testing.T) {
	groups := groupChangelists([]svn.StatusItem{
		{Path: "z.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "a.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "b.go", State: svn.StateModified},
		{Path: "c.go", State: svn.StateModified, Changelist: "alpha"},
	})
	// Named changelists first (alphabetical), then staged, then the unstaged default.
	want := []string{"alpha", "feature", "(staged)", "(unstaged)"}
	if len(groups) != len(want) {
		t.Fatalf("want %d groups, got %d: %+v", len(want), len(groups), groups)
	}
	for i, w := range want {
		if groups[i].Label() != w {
			t.Errorf("group %d = %q, want %q", i, groups[i].Label(), w)
		}
	}
	if !groups[0].Committable() {
		t.Error("a named changelist should be committable")
	}
	if groups[3].Committable() {
		t.Error("the unstaged default group should not be committable")
	}
}

func TestFilesViewSwitchesToChangelists(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "b.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	// Files panel is focused by default; ] cycles to the Changelists view.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver ViewSelectedMsg
		m = next.(*Model)
	}
	if name := m.filesViews.ActiveName(); name != "Changelists" {
		t.Fatalf("active files view = %q, want Changelists", name)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "feature") || !strings.Contains(view, "(staged)") {
		t.Errorf("the changelists view should list the groups, got:\n%s", view)
	}
}

func TestAssignChangelistPromptAndSubmit(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "mod.go", State: svn.StateModified},
	})
	// n opens the changelist-name prompt.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("n should open the changelist-name prompt")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Changelist name") {
		t.Errorf("expected the name prompt, got:\n%s", view)
	}

	// Type a name; enter submits it (single-line input).
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature-x")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a SubmitMsg from the name prompt")
	}
	sub, ok := cmd().(uimsg.SubmitMsg)
	if !ok || sub.ID != changelistEditorID {
		t.Fatalf("expected a changelist SubmitMsg, got %T (%+v)", cmd(), cmd())
	}
	if sub.Value != "feature-x" {
		t.Errorf("submitted name = %q, want feature-x", sub.Value)
	}

	next, cmd = m.Update(sub)
	m = next.(*Model)
	if m.naming {
		t.Error("the prompt should close after submit")
	}
	if cmd == nil {
		t.Error("expected an assign command after submit")
	}
}

func TestAssignChangelistAllowsStagedFile(t *testing.T) {
	// A file in the anonymous staged bucket can be moved into a named changelist.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "staged.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("a staged file should be assignable to a named changelist")
	}
}

func TestAssignChangelistNamesAllStagedFiles(t *testing.T) {
	// Naming a changelist while several files are staged moves the whole staged
	// set as a unit, not just the highlighted file; an unstaged file is left out.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "b.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "c.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("n should open the changelist-name prompt when files are staged")
	}
	got := map[string]bool{}
	for _, tgt := range m.nameTargets {
		got[tgt.path] = true
	}
	if len(m.nameTargets) != 2 || !got["a.go"] || !got["b.go"] {
		t.Errorf("nameTargets = %+v, want exactly the staged files a.go and b.go", m.nameTargets)
	}
}

func TestAssignChangelistFallsBackToSelectedFile(t *testing.T) {
	// With nothing staged, naming still targets just the selected file so the
	// single-file workflow keeps working.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "lone.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("n should open the prompt for the selected file when nothing is staged")
	}
	if len(m.nameTargets) != 1 || m.nameTargets[0].path != "lone.go" {
		t.Errorf("nameTargets = %+v, want just lone.go", m.nameTargets)
	}
}

func TestAssignChangelistOffersExistingNames(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "loose.go", State: svn.StateModified},
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "b.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	view := stripANSI(m.View())
	if !strings.Contains(view, "Existing changelists:") || !strings.Contains(view, "feature") {
		t.Errorf("the prompt should list existing named changelists, got:\n%s", view)
	}
	// The anonymous buckets are not offered as pickable names.
	if strings.Contains(view, "(staged)") || strings.Contains(view, "(unstaged)") {
		t.Errorf("anonymous buckets should not appear as options, got:\n%s", view)
	}
}

func TestAssignChangelistGuardsNamedChangelist(t *testing.T) {
	// A file already in a *named* changelist cannot be reassigned (unstage first).
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if m.naming {
		t.Error("a file already in a named changelist should not open the prompt")
	}
	if cmd != nil {
		t.Error("the guard should not produce a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "already in") {
		t.Errorf("expected an already-assigned guard toast, got:\n%s", view)
	}
}

func TestAssignChangelistCancels(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "mod.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	// Esc emits DismissMsg, which the app handles to close the prompt.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.naming {
		t.Error("the prompt should close on cancel")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Changelist name") {
		t.Error("the layout should return after cancelling the prompt")
	}
}

func TestChangelistDrillExpandsAndCollapses(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)

	// enter drills into the selected changelist.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from the changelists list")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() == 0 {
		t.Fatal("enter should drill into the changelist")
	}
	if m.drilledCL != "feature" {
		t.Errorf("drilled changelist = %q, want feature", m.drilledCL)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "a.go") {
		t.Errorf("the drill should list the changelist's files, got:\n%s", view)
	}

	// esc collapses back out.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a SubViewPoppedMsg on esc")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() != 0 {
		t.Error("esc should collapse the drill")
	}
	if m.drilledCL != "" {
		t.Error("the drilled changelist should be cleared on collapse")
	}
}

func TestChangelistDrillShowsTree(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "internal/svn/client.go", State: svn.StateModified, Changelist: "feature"},
	})
	// Switch to Changelists and drill into "feature".
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from the changelists list")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() == 0 {
		t.Fatal("expected to be drilled into the changelist")
	}

	// The drill renders the same "/"-rooted tree: a root row, directory rows, and
	// basename leaves (never a full nested path on one row).
	var names []string
	for _, n := range m.clFiles.Items() {
		names = append(names, n.Name)
		if n.Item != nil && strings.Contains(n.Name, "/") {
			t.Errorf("drill file leaf %q should be a basename", n.Name)
		}
	}
	for _, want := range []string{"/", "internal", "app", "svn", "app.go", "client.go"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("drill tree missing %q, got: %v", want, names)
		}
	}

	// Enter on the internal/ directory row collapses it, hiding its descendants.
	for i, n := range m.clFiles.Items() {
		if n.Name == "internal" {
			m.clFiles.SetIndex(i)
			break
		}
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from enter on a drill directory")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	for _, n := range m.clFiles.Items() {
		if n.Name == "app.go" || n.Name == "client.go" {
			t.Errorf("collapsing internal/ in the drill should hide its files, got: %v", m.clFiles.Items())
		}
	}
}

func TestCommitChangelistFromView(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if !m.editing {
		t.Fatal("c should open the commit editor for the selected changelist")
	}
	if m.commitCL != "feature" {
		t.Errorf("commit target = %q, want feature", m.commitCL)
	}
}

func TestCommitUnstagedGroupRefused(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified}, // no changelist → (unstaged)
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if m.editing {
		t.Error("committing the unstaged group should be refused")
	}
	if cmd != nil {
		t.Error("no command should run for the unstaged group")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "isn't a changelist") {
		t.Errorf("expected a refusal toast, got:\n%s", view)
	}
}

func TestNamedChangelistFileShowsAccentMarker(t *testing.T) {
	// A named-changelist file is marked in the Changes view (distinct from the
	// staged bucket's marker), so both render the dot.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "feature.go", State: svn.StateModified, Changelist: "feature"},
	})
	if view := stripANSI(m.View()); !strings.Contains(view, "●") {
		t.Errorf("expected a changelist marker in the files list, got:\n%s", view)
	}
}

func sshModel(t *testing.T) *Model {
	t.Helper()
	m := New(nil, &svn.Info{URL: "svn+ssh://host/repo/trunk", Revision: "7"}, selfupdate.Build{}, config.Default())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func TestNeedsSSHKeyFromURL(t *testing.T) {
	https := New(nil, &svn.Info{URL: "https://h/r"}, selfupdate.Build{}, config.Default())
	if https.needsSSHKey {
		t.Error("an https working copy should not need an SSH key")
	}
	ssh := New(nil, &svn.Info{URL: "svn+ssh://h/r"}, selfupdate.Build{}, config.Default())
	if !ssh.needsSSHKey {
		t.Error("an svn+ssh working copy should need an SSH key")
	}
}

func TestSSHKeyLoadedProceedsWithoutPrompt(t *testing.T) {
	m := sshModel(t)
	next, cmd := m.Update(sshCheckedMsg{loaded: true})
	m = next.(*Model)
	if m.unlocking {
		t.Error("a key already in the agent should not open the passphrase prompt")
	}
	if cmd == nil {
		t.Error("expected the initial load to start once the key is ready")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "SSH key passphrase") {
		t.Errorf("passphrase overlay should not be shown, got:\n%s", view)
	}
}

func TestSSHKeyMissingOpensPrompt(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	if !m.unlocking {
		t.Fatal("a key missing from the agent should open the passphrase prompt")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "SSH key passphrase") {
		t.Errorf("passphrase overlay missing, got:\n%s", view)
	}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// assertAbortThenQuit checks that m is showing the centered fatal-error toast
// with the quit hint, has recorded an abort reason, and quits on the next key.
func assertAbortThenQuit(t *testing.T, m *Model) {
	t.Helper()
	if !m.aborting {
		t.Fatal("expected the model to enter the abort state")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Press any key to quit and try again") {
		t.Errorf("expected the quit hint in a centered toast, got:\n%s", view)
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}); !isQuit(cmd) {
		t.Error("any key should quit after an abort")
	}
}

func TestSSHCheckErrorAborts(t *testing.T) {
	m := sshModel(t)
	next, cmd := m.Update(sshCheckedMsg{err: errors.New("agent unreachable")})
	m = next.(*Model)
	if m.unlocking {
		t.Error("an agent error should not open the prompt")
	}
	if cmd != nil {
		t.Error("an abort should wait for a keypress, not quit immediately")
	}
	assertAbortThenQuit(t, m)
}

func TestSSHPassphraseMaskedInOverlay(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	// Type a passphrase through the model; it must never appear on screen.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hunter2")})
	m = next.(*Model)
	view := stripANSI(m.View())
	if strings.Contains(view, "hunter2") {
		t.Errorf("passphrase leaked into the view:\n%s", view)
	}
	if !strings.Contains(view, "\u2022") {
		t.Errorf("expected masked bullets in the overlay, got:\n%s", view)
	}
}

func TestSSHAddWrongPassphraseAllowsRetry(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	next, _ = m.Update(sshAddedMsg{err: errors.New("bad passphrase")})
	m = next.(*Model)
	if !m.unlocking {
		t.Error("a wrong passphrase should keep the prompt open for another try")
	}
	if m.adding {
		t.Error("the input should be unlocked again after a failed attempt")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "wrong passphrase") {
		t.Errorf("expected a wrong-passphrase toast, got:\n%s", view)
	}
}

func TestSSHTooManyBadPassphrasesAborts(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)

	for i := 0; i < maxPassphraseAttempts; i++ {
		next, _ = m.Update(sshAddedMsg{err: errors.New("bad passphrase")})
		m = next.(*Model)
	}
	assertAbortThenQuit(t, m)
}

func TestSSHAddAgentErrorAborts(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	next, _ = m.Update(sshAddedMsg{err: sshagent.ErrAgentUnreachable})
	m = next.(*Model)
	assertAbortThenQuit(t, m)
}

func TestSSHAddSuccessClosesPromptAndLoads(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	next, cmd := m.Update(sshAddedMsg{err: nil})
	m = next.(*Model)
	if m.unlocking {
		t.Error("a successful add should close the passphrase prompt")
	}
	if cmd == nil {
		t.Error("expected the initial load to start after the key is added")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "SSH key added") {
		t.Errorf("expected a success toast, got:\n%s", view)
	}
}

func TestSSHPromptDismissAborts(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	next, _ = m.Update(uimsg.DismissMsg{ID: passphraseEditorID})
	m = next.(*Model)
	assertAbortThenQuit(t, m)
}

func TestSSHSubmitLocksInputAndIndicates(t *testing.T) {
	m := sshModel(t)
	next, _ := m.Update(sshCheckedMsg{loaded: false})
	m = next.(*Model)
	next, cmd := m.Update(uimsg.SubmitMsg{ID: passphraseEditorID, Value: "pw"})
	m = next.(*Model)
	if !m.adding {
		t.Error("submitting should mark the add as in progress")
	}
	if m.passEditor.Focused() {
		t.Error("submitting should lock (blur) the passphrase input")
	}
	if cmd == nil {
		t.Error("submitting should start the ssh-add command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Adding SSH key") {
		t.Errorf("expected a processing indicator, got:\n%s", view)
	}
	// While the add is in flight, further key input is ignored.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(*Model)
	if m.passEditor.Value() != "" {
		t.Errorf("input should be locked while adding, got %q", m.passEditor.Value())
	}
}

func TestChangelistsViewGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded, Changelist: "revision:staged"},
		{Path: "feature.go", State: svn.StateModified, Changelist: "feature-x"},
		{Path: "loose.txt", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestChangelistDrillLocksViewSwitch(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	// Switch to Changelists, then drill in.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() == 0 {
		t.Fatal("expected to be drilled into the changelist")
	}

	// While drilled, [ and ] must not switch the Files view.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if name := m.filesViews.ActiveName(); name != "Changelists" {
		t.Errorf("view switched while drilled (now %q); it should stay locked", name)
	}
	if m.filesViews.Depth() == 0 {
		t.Error("the drill should remain open while view switching is locked")
	}
}

func TestChangelistDrillHeaderGolden(t *testing.T) {
	// Expanding a changelist labels the panel header with just the changelist
	// name (no tabs, no chevron).
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "feature.go", State: svn.StateModified, Changelist: "feature-x"},
		{Path: "other.go", State: svn.StateAdded, Changelist: "feature-x"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}
