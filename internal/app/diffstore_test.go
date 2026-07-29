package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
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
	next, _ := m.Update(loadSavedDiffsCmd(m.diffDir())())
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
