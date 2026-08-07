package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/svn"
)

// writeDiffStore fills dir with the named files, stamping each one a minute
// apart in the order given so the newest-first sort is deterministic.
func writeDiffStore(t *testing.T, dir string, names ...string) {
	t.Helper()
	base := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	for i, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("Index: "+name+"\n@@ -1 +1 @@\n-old\n+new\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		stamp := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}
}

// diffStoreModel returns a sized model whose diff output directory is a fresh
// temp dir holding the named patch files, plus that directory.
func diffStoreModel(t *testing.T, names ...string) (*Model, string) {
	t.Helper()
	dir := t.TempDir()
	writeDiffStore(t, dir, names...)
	cfg := config.Default()
	cfg.DiffOutputDir = dir
	return sizedModelCfg(t, cfg), dir
}

// showDiffsView switches the Files panel to the Diffs view and delivers the
// commands that populate it, returning the settled model.
func showDiffsView(t *testing.T, m *Model) *Model {
	t.Helper()
	next, _ := m.Update(m.reloadSavedDiffs()())
	m = next.(*Model)
	// ] cycles Changes → Changelists → Diffs.
	for range 2 {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
		m = next.(*Model)
		if cmd != nil {
			next, _ = m.Update(cmd())
			m = next.(*Model)
		}
	}
	if name := m.filesViews.ActiveName(); name != savedDiffsViewName {
		t.Fatalf("active files view = %q, want %q", name, savedDiffsViewName)
	}
	if cmd := m.savedDiffLoadForSelection(); cmd != nil {
		next, _ := m.Update(cmd())
		m = next.(*Model)
	}
	return m
}

func TestScanSavedDiffsListsPatchesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	writeDiffStore(t, dir, "first.diff", "second.patch", "third.diff")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested.diff"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := scanSavedDiffs(dir)
	if err != nil {
		t.Fatalf("scanSavedDiffs: %v", err)
	}
	want := []string{"third.diff", "second.patch", "first.diff"}
	if len(got) != len(want) {
		t.Fatalf("got %d diffs, want %d: %+v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("diff %d = %q, want %q", i, got[i].Name, name)
		}
		if got[i].Path != filepath.Join(dir, name) {
			t.Errorf("diff %d path = %q, want it under %q", i, got[i].Path, dir)
		}
	}
}

func TestScanSavedDiffsMissingDirIsEmpty(t *testing.T) {
	got, err := scanSavedDiffs(filepath.Join(t.TempDir(), "never-created"))
	if err != nil {
		t.Fatalf("a missing output directory should not be an error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d diffs, want none", len(got))
	}
}

func TestDiffsViewListsSavedDiffs(t *testing.T) {
	m, _ := diffStoreModel(t, "alpha.diff", "beta.diff")
	m = showDiffsView(t, m)

	view := stripANSI(m.View())
	if !strings.Contains(view, "alpha.diff") || !strings.Contains(view, "beta.diff") {
		t.Errorf("the Diffs view should list the saved patches, got:\n%s", view)
	}
}

func TestDiffsViewShowsHighlightedDiffInMain(t *testing.T) {
	m, _ := diffStoreModel(t, "alpha.diff", "beta.diff")
	m = showDiffsView(t, m)

	// beta.diff is newest, so it opens highlighted and drives Main.
	if !strings.Contains(stripANSI(m.main.View()), "Index: beta.diff") {
		t.Fatalf("Main should show the highlighted diff, got:\n%s", stripANSI(m.main.View()))
	}

	// Moving the cursor loads the next file's contents.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("moving the cursor should emit a selection command")
	}
	next, cmd = m.Update(cmd())
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	if !strings.Contains(stripANSI(m.main.View()), "Index: alpha.diff") {
		t.Errorf("Main should follow the highlighted diff, got:\n%s", stripANSI(m.main.View()))
	}
}

func TestDiffsViewEmptyStore(t *testing.T) {
	m, dir := diffStoreModel(t)
	m = showDiffsView(t, m)
	// The rendered viewport truncates the long temp path, so assert on the text
	// Main is filled from.
	if detail := m.savedDiffDetail(); !strings.Contains(detail, "No saved diffs in "+dir) {
		t.Errorf("an empty store should say so, got: %s", detail)
	}
}

func TestDiffsViewFilterNarrowsList(t *testing.T) {
	m, _ := diffStoreModel(t, "alpha.diff", "beta.diff")
	m = showDiffsView(t, m)

	m.setFilter(panelFiles, "alpha")
	if got := len(m.savedDiffs.Items()); got != 1 {
		t.Fatalf("filtered list holds %d diffs, want 1", got)
	}
	if d, _ := m.savedDiffs.Selected(); d.Name != "alpha.diff" {
		t.Errorf("selected diff = %q, want alpha.diff", d.Name)
	}
}

func TestSaveDiffRefusedInDiffsView(t *testing.T) {
	m, _ := diffStoreModel(t, "alpha.diff")
	m = showDiffsView(t, m)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = next.(*Model)
	if m.savingDiff {
		t.Error("the save-diff prompt should not open over a file that is already saved")
	}
}

func TestDeleteSavedDiffRemovesFileFromStore(t *testing.T) {
	m, dir := diffStoreModel(t, "alpha.diff", "beta.diff")
	m = showDiffsView(t, m)

	// beta.diff is newest, so it opens highlighted.
	m, cmd := requestAndConfirm(t, m, 'd')
	if cmd == nil {
		t.Fatal("confirming the prompt should emit the delete command")
	}
	next, cmd := m.Update(cmd())
	m = next.(*Model)
	if _, err := os.Stat(filepath.Join(dir, "beta.diff")); !os.IsNotExist(err) {
		t.Errorf("beta.diff should be gone from the store, stat err = %v", err)
	}
	if cmd == nil {
		t.Fatal("a successful delete should re-scan the store")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)

	if got := len(m.savedDiffs.Items()); got != 1 {
		t.Fatalf("the Diffs view lists %d patches, want 1", got)
	}
	if d, _ := m.savedDiffs.Selected(); d.Name != "alpha.diff" {
		t.Errorf("selected diff = %q, want alpha.diff", d.Name)
	}
}

func TestDeleteSavedDiffLeavesWorkingCopyAlone(t *testing.T) {
	m, _ := diffStoreModel(t, "alpha.diff")
	m = loadItems(t, m, []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	m = showDiffsView(t, m)

	m, _ = pressRune(t, m, 'd')
	if !m.confirming {
		t.Fatal("d in the Diffs view should ask before deleting")
	}
	if !strings.Contains(stripANSI(m.View()), "alpha.diff") {
		t.Errorf("the prompt should name the patch file, got:\n%s", stripANSI(m.View()))
	}
}

func TestDeleteSavedDiffOnEmptyStoreWarns(t *testing.T) {
	m, _ := diffStoreModel(t)
	m = showDiffsView(t, m)

	m, _ = pressRune(t, m, 'd')
	if m.confirming {
		t.Fatal("an empty store has nothing to confirm deleting")
	}
	if !strings.Contains(stripANSI(m.View()), "no saved diff to delete") {
		t.Errorf("an empty store should warn, got:\n%s", stripANSI(m.View()))
	}
}

func TestApplyPatchAsksForConfirmation(t *testing.T) {
	m, _ := diffStoreModel(t, "alpha.diff")
	m = showDiffsView(t, m)

	m, _ = pressRune(t, m, 'p')
	if !m.confirming {
		t.Fatal("p in the Diffs view should ask before patching the working copy")
	}
	if m.pending == nil {
		t.Error("the accepted prompt should have a patch command staged")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Apply patch?") || !strings.Contains(view, "alpha.diff") {
		t.Errorf("the prompt should name the patch file, got:\n%s", view)
	}
}

func TestApplyPatchOnlyInDiffsView(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})

	m, _ = pressRune(t, m, 'p')
	if m.confirming {
		t.Error("p patches only in the Diffs view; the Changes tree has no patch to apply")
	}
}

func TestApplyPatchOnEmptyStoreWarns(t *testing.T) {
	m, _ := diffStoreModel(t)
	m = showDiffsView(t, m)

	m, _ = pressRune(t, m, 'p')
	if m.confirming {
		t.Fatal("an empty store has nothing to confirm applying")
	}
	if !strings.Contains(stripANSI(m.View()), "no saved diff to apply") {
		t.Errorf("an empty store should warn, got:\n%s", stripANSI(m.View()))
	}
}

// TestApplyPatchRefusesAPatchFromAnotherDirectory covers the guard that runs
// before svn is asked anything: a patch whose paths resolve to nothing in the
// directory it would be applied to is refused outright, since svn would create
// each missing target and reject the patch's hunks into it.
func TestApplyPatchRefusesAPatchFromAnotherDirectory(t *testing.T) {
	elsewhere := t.TempDir()
	store := t.TempDir()
	patch := filepath.Join(store, "alpha.diff")
	body := "Index: a.txt\n" +
		"===================================================================\n" +
		"--- a.txt\t(revision 1)\n+++ a.txt\t(working copy)\n@@ -1 +1 @@\n-one\n+ONE\n"
	if err := os.WriteFile(patch, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	msg, ok := applyPatchCmd(svn.New(elsewhere), patch, "alpha.diff", elsewhere)().(patchAppliedMsg)
	if !ok {
		t.Fatalf("applyPatchCmd should report a patchAppliedMsg, got %T", msg)
	}
	if msg.err == nil {
		t.Fatal("a patch none of whose files are in the directory must be refused")
	}
	if !strings.Contains(msg.err.Error(), "taken from another directory") {
		t.Errorf("error = %q, want it to say the patch came from somewhere else", msg.err)
	}
}

// TestPatchTrialErrRefusesOnlyAPatchThatLandsNothing pins the gate down to what
// it is for. A patch that partly fits is worth applying — svn takes the hunks it
// can and leaves the rest as rejects — so only one with nothing at all to give
// is turned away.
func TestPatchTrialErrRefusesOnlyAPatchThatLandsNothing(t *testing.T) {
	refused := []struct {
		name string
		res  svn.PatchResult
		want string
	}{
		{"unreadable patch", svn.PatchResult{}, "nothing in it to apply"},
		{
			"every target missing",
			svn.PatchResult{Skipped: []string{"a.txt", "b.txt"}},
			"svn cannot find 2 files",
		},
		{
			"every target conflicted",
			svn.PatchResult{Conflicted: []string{"a.txt"}},
			"not one of its changes applies here",
		},
	}
	for _, tt := range refused {
		t.Run(tt.name, func(t *testing.T) {
			err := patchTrialErr(tt.res)
			if err == nil {
				t.Fatalf("%+v lands nothing and should be refused", tt.res)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to mention %q", err, tt.want)
			}
		})
	}

	applied := []struct {
		name string
		res  svn.PatchResult
	}{
		{"clean", svn.PatchResult{Applied: []string{"a.txt"}}},
		{
			"some hunks rejected",
			svn.PatchResult{Applied: []string{"a.txt"}, Conflicted: []string{"b.txt"}},
		},
		{
			"some targets missing",
			svn.PatchResult{Applied: []string{"a.txt"}, Skipped: []string{"b.txt"}},
		},
	}
	for _, tt := range applied {
		t.Run(tt.name, func(t *testing.T) {
			if err := patchTrialErr(tt.res); err != nil {
				t.Errorf("%+v puts something in, so it should be applied, got %v", tt.res, err)
			}
		})
	}
}

func TestPatchAppliedReportsResult(t *testing.T) {
	m, _ := diffStoreModel(t, "alpha.diff")
	m = showDiffsView(t, m)

	next, cmd := m.Update(patchAppliedMsg{
		name: "alpha.diff",
		res:  svn.PatchResult{Applied: []string{"a.txt", "sub/b.txt"}},
	})
	m = next.(*Model)
	if got := m.toast.Message(); !strings.Contains(got, "applied alpha.diff to 2 files") {
		t.Errorf("a finished patch should report what it changed, got %q", got)
	}
	if cmd == nil {
		t.Error("a patch changes the working copy, so the status behind it must be re-read")
	}

	// A partly applied patch is a result, not a failure — but the rejects it left
	// behind are svn-ignored, so the toast is where they are announced.
	next, _ = m.Update(patchAppliedMsg{
		name: "alpha.diff",
		res: svn.PatchResult{
			Applied:    []string{"a.txt"},
			Conflicted: []string{"sub/b.txt"},
			Skipped:    []string{"gone.txt"},
		},
	})
	m = next.(*Model)
	if want := "applied alpha.diff to 1 file, 1 with rejects (.rej), 1 not found"; m.toast.Message() != want {
		t.Errorf("toast = %q, want %q", m.toast.Message(), want)
	}

	next, _ = m.Update(patchAppliedMsg{name: "alpha.diff", err: errors.New("boom")})
	m = next.(*Model)
	if got := m.toast.Message(); !strings.Contains(got, "apply alpha.diff failed: boom") {
		t.Errorf("a refused patch should say why, got %q", got)
	}
}
