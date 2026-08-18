package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// stubSVN re-points the model's client at a temp working copy and a stub binary
// that prints diff verbatim, so a save — which generates its patch by running
// svn — can be exercised without Subversion. The recorder is carried over so
// the command log still sees the invocation.
func stubSVN(t *testing.T, m *Model, diff string) {
	t.Helper()
	dir := t.TempDir()
	body := filepath.Join(dir, "diff.txt")
	if err := os.WriteFile(body, []byte(diff), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "svn-stub")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\ncat "+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.client = &svn.Client{Dir: dir, Bin: bin, Recorder: m.client.Recorder}
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
	stubSVN(t, m, "Index: src/a.go\n@@ -1 +1 @@\n-old\n+new")

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
	// The diffs revision loads on its own stay out of the command log, but the
	// one the user asked to be written out is an action like any other.
	logged := m.cmdLog.snapshot()
	if len(logged) != 1 || logged[0].Subcommand != "diff" || !strings.Contains(logged[0].Command, "diff src/a.go") {
		t.Errorf("expected the save's svn diff in the command log, got %+v", logged)
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
	stubSVN(t, m, "@@ -1 +1 @@\n+new")

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
	next, _ := m.Update(diffLoadedMsg{path: "src", dir: true, diff: "Index: src/a.go\n@@ -1 +1 @@\n+alpha\n"})
	m = next.(*Model)
	stubSVN(t, m, "Index: src/a.go\n@@ -1 +1 @@\n+alpha\n")

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

// drilledSaveModel drills into a range whose diff is diff, with saved patches
// going to dir.
func drilledSaveModel(t *testing.T, dir, diff string) *Model {
	t.Helper()
	cfg := config.Default()
	cfg.DiffOutputDir = dir
	m := loadItems(t, sizedModelCfg(t, cfg), nil)
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{{Revision: "50"}, {Revision: "49"}}})
	m = next.(*Model)
	m, _ = pressRune(t, m, '3')
	return drillInto(t, m, diff)
}

func TestSaveDiffWritesTheRangeOnScreen(t *testing.T) {
	dir := t.TempDir()
	m := drilledSaveModel(t, dir, treePatch)

	m, msg := saveDiffKey(t, m, "")
	saved, ok := msg.(diffSavedMsg)
	if !ok {
		t.Fatalf("expected a diffSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("save failed: %v", saved.err)
	}
	if want := filepath.Join(dir, "r49-r50.diff"); saved.path != want {
		t.Errorf("saved to %q, want %q", saved.path, want)
	}
	body, err := os.ReadFile(saved.path)
	if err != nil {
		t.Fatalf("read saved diff: %v", err)
	}
	if string(body) != treePatch {
		t.Errorf("saved diff = %q, want the whole range verbatim", body)
	}

	// Nothing was asked of svn: the patch was already in hand, and a range can be
	// expensive enough that reproducing it purely to save it is not worth it.
	if logged := m.cmdLog.snapshot(); len(logged) != 0 {
		t.Errorf("saving a range should run no svn command, got %+v", logged)
	}
}

func TestSaveDiffWritesOnePartOfTheRange(t *testing.T) {
	dir := t.TempDir()
	m := drilledSaveModel(t, dir, treePatch)
	m.revFiles.SetIndex(indexOfNodePath(t, m, "src/a.go"))
	m.updateMain()

	_, msg := saveDiffKey(t, m, "")
	saved := msg.(diffSavedMsg)
	if saved.err != nil {
		t.Fatalf("save failed: %v", saved.err)
	}
	// The default name carries the range and the path within it, separators
	// folded so nested targets stay distinct in one flat directory.
	if want := filepath.Join(dir, "r49-r50-src-a.go.diff"); saved.path != want {
		t.Errorf("saved to %q, want %q", saved.path, want)
	}
	body, err := os.ReadFile(saved.path)
	if err != nil {
		t.Fatalf("read saved diff: %v", err)
	}
	if got := string(body); !strings.Contains(got, "+alpha") || strings.Contains(got, "+new") {
		t.Errorf("saved diff = %q, want only the selected file's section", got)
	}
	if !strings.HasSuffix(string(body), "\n") {
		t.Error("a saved patch should end with a newline")
	}
}

func TestSaveDiffRefusesAFailedRange(t *testing.T) {
	m := drilledModel(t, treePatch)
	// Replace the cached patch with a failure, as a refused range would leave.
	m.session.PutRevDiff(m.revDiff, revDiffEntry{text: "Unable to load diff: E160013", failed: true})

	m, msg := saveDiffKey(t, m, "")
	if msg != nil {
		t.Fatalf("a failure notice is not a patch to write, got %v", msg)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no diff to save") {
		t.Errorf("expected a refusal, got:\n%s", view)
	}
}

// A range names files as they were, which the working copy need no longer hold,
// so there is nothing here for the editor to open. The Files panel is left
// holding a real file, which is exactly what e falls through to when the
// revision tree does not stop it.
func TestEditorIsInertInTheRevisionTree(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "live.go", State: svn.StateModified}})
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{{Revision: "50"}, {Revision: "49"}}})
	m = next.(*Model)
	m, _ = pressRune(t, m, '3')
	m = drillInto(t, m, treePatch)
	m.revFiles.SetIndex(indexOfNodePath(t, m, "src/a.go"))
	m.updateMain()

	if path, _, _, ok := m.editTarget(); ok {
		t.Errorf("the revision tree named %q to edit, want nothing", path)
	}
	m, cmd := pressRune(t, m, 'e')
	if cmd != nil {
		t.Error("e should not launch an editor from the revision tree")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no file to open here") {
		t.Errorf("expected e to say there is nothing to open, got:\n%s", view)
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

// diffFixture is a unified diff of n changed lines, big enough that colorizing
// it is measurable work.
func diffFixture(n int) string {
	lines := []string{"Index: big.txt", "===================================================================",
		"--- big.txt\t(revision 1)", "+++ big.txt\t(working copy)", "@@ -1,%d +1,%d @@"}
	for i := 0; i < n; i++ {
		lines = append(lines,
			fmt.Sprintf(" context line %d", i),
			fmt.Sprintf("-old line %d", i),
			fmt.Sprintf("+new line %d", i))
	}
	return strings.Join(lines, "\n")
}

func TestColorizedDiffIsServedFromTheSession(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := sizedModel(t)
	diff := diffFixture(3)
	want := colorizeDiff(m.theme, diff)

	// The cached path must be indistinguishable from the pure one, first time
	// (a miss, which fills the cache) and second (a hit).
	for _, pass := range []string{"miss", "hit"} {
		if got := m.colorize(diff); got != want {
			t.Errorf("%s: cached colorization differs from the direct render", pass)
		}
	}
	if n := m.session.rendered.Len(); n != 1 {
		t.Errorf("the same patch cached %d times, want 1", n)
	}
}

func TestColorizedDiffFollowsTheTheme(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := sizedModel(t)
	diff := diffFixture(3)
	before := m.colorize(diff)

	m.previewTheme("dracula")
	after := m.colorize(diff)
	if after == before {
		t.Error("a theme switch was served the previous palette's colors")
	}
	if want := colorizeDiff(m.theme, diff); after != want {
		t.Error("the switched theme's diff differs from a direct render")
	}
	// The entry for the previous theme is still there to switch back to.
	if n := m.session.rendered.Len(); n != 2 {
		t.Errorf("rendered cache holds %d entries, want one per theme", n)
	}
}

func BenchmarkColorizeDiff(b *testing.B) {
	m := sizedModel(b)
	diff := diffFixture(500)
	b.Run("uncached", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = colorizeDiff(m.theme, diff)
		}
	})
	b.Run("cached", func(b *testing.B) {
		m.colorize(diff)
		for i := 0; i < b.N; i++ {
			_ = m.colorize(diff)
		}
	})
}

// rangePatch is `svn diff -r 2:4` verbatim over a repository where r2 added
// b.txt, r3 modified a.txt and r4 deleted b.txt.
const rangePatch = `Index: b.txt
===================================================================
--- b.txt	(revision 2)
+++ b.txt	(nonexistent)
@@ -1 +0,0 @@
-b1
Index: a.txt
===================================================================
--- a.txt	(revision 2)
+++ a.txt	(revision 4)
@@ -1 +1,2 @@
 a1
+a2
`

func TestSplitPatchByFile(t *testing.T) {
	files := splitPatchByFile(rangePatch)
	if len(files) != 2 {
		t.Fatalf("split %d sections, want one per file: %+v", len(files), files)
	}

	// svn's order is kept: the tree decides how to arrange them, not this.
	if files[0].Path != "b.txt" || files[1].Path != "a.txt" {
		t.Errorf("paths = %q, %q, want b.txt then a.txt", files[0].Path, files[1].Path)
	}
	if files[0].State != svn.StateDeleted {
		t.Errorf("b.txt state = %s, want deleted", files[0].State)
	}
	if files[1].State != svn.StateModified {
		t.Errorf("a.txt state = %s, want modified", files[1].State)
	}

	// Each section must carry its own "Index:" line and stop before the next.
	if !strings.HasPrefix(files[0].Text, "Index: b.txt\n") || strings.Contains(files[0].Text, "a.txt") {
		t.Errorf("b.txt section leaked into the next file:\n%s", files[0].Text)
	}

	// Nothing may be lost or duplicated: the sections must tile the patch.
	var texts []string
	for _, f := range files {
		texts = append(texts, f.Text)
	}
	if got := strings.Join(texts, "\n"); got != strings.TrimRight(rangePatch, "\n") {
		t.Errorf("sections do not reassemble into the patch:\n%s", got)
	}
}

func TestSplitPatchByFileReadsAnAddition(t *testing.T) {
	const added = `Index: b.txt
===================================================================
--- b.txt	(nonexistent)
+++ b.txt	(revision 2)
@@ -0,0 +1 @@
+b1
`
	files := splitPatchByFile(added)
	if len(files) != 1 || files[0].State != svn.StateAdded {
		t.Errorf("split = %+v, want a single added file", files)
	}
}

// A removed line can begin "---" and end in the same marker svn writes on a file
// header, so the markers are only trusted before a section's first hunk.
func TestSplitPatchByFileIgnoresMarkersInsideHunks(t *testing.T) {
	const tricky = `Index: notes.txt
===================================================================
--- notes.txt	(revision 1)
+++ notes.txt	(revision 2)
@@ -1,2 +1,2 @@
---- old.txt	(nonexistent)
++++ new.txt	(nonexistent)
`
	files := splitPatchByFile(tricky)
	if len(files) != 1 {
		t.Fatalf("split %d sections, want 1: %+v", len(files), files)
	}
	if files[0].State != svn.StateModified {
		t.Errorf("state = %s, want modified: a hunk's own lines are not file headers", files[0].State)
	}
}

func TestSplitPatchByFileEdgeCases(t *testing.T) {
	const binary = `Index: logo.png
===================================================================
Cannot display: file marked as a binary type.
svn:mime-type = application/octet-stream
`
	t.Run("binary still yields a row", func(t *testing.T) {
		files := splitPatchByFile(binary)
		if len(files) != 1 || files[0].Path != "logo.png" {
			t.Fatalf("split = %+v, want the binary file listed", files)
		}
		if !strings.Contains(files[0].Text, "Cannot display") {
			t.Errorf("svn's notice should be kept as the section body:\n%s", files[0].Text)
		}
	})

	for name, diff := range map[string]string{
		"empty":       "",
		"whitespace":  "  \n\n",
		"no Index":    "@@ -1 +1 @@\n-old\n+new",
		"only a hunk": "--- a.txt\t(nonexistent)\n@@ -0,0 +1 @@\n+a1",
	} {
		t.Run(name, func(t *testing.T) {
			if files := splitPatchByFile(diff); files != nil {
				t.Errorf("split = %+v, want nothing: no section to attach the text to", files)
			}
		})
	}
}
