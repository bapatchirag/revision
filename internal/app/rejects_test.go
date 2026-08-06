package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/svn"
)

// writeRejects creates each named file under dir, making the directories it
// names on the way.
func writeRejects(t *testing.T, dir string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		body := "--- " + rel + "\n@@ -1,3 +1,3 @@\n-old\n+new\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

// rejectModel returns a sized model rooted at a fresh temp directory holding the
// named files, plus that directory. Rejects are found by walking the source
// path, so unlike the diff store this model's svn client has to point at real
// files on disk.
func rejectModel(t *testing.T, rels ...string) (*Model, string) {
	t.Helper()
	dir := t.TempDir()
	writeRejects(t, dir, rels...)
	info := &svn.Info{
		URL:             "https://svn.example.com/repo/trunk",
		WorkingCopyRoot: dir,
		Revision:        "42",
	}
	m := New(svn.New(dir), info, selfupdate.Build{}, config.Default())
	m.workDir = dir
	m.refreshChrome()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model), dir
}

// showRejectsView switches the Files panel to the Rejects view and delivers the
// commands that populate it, returning the settled model.
func showRejectsView(t *testing.T, m *Model) *Model {
	t.Helper()
	// ] cycles Changes → Changelists → Diffs → Rejects.
	for range 3 {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
		m = next.(*Model)
		if cmd != nil {
			next, _ = m.Update(cmd())
			m = next.(*Model)
		}
	}
	if name := m.filesViews.ActiveName(); name != rejectsViewName {
		t.Fatalf("active files view = %q, want %q", name, rejectsViewName)
	}
	next, _ := m.Update(m.reloadRejects()())
	m = next.(*Model)
	if cmd := m.rejectLoadForSelection(); cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	return m
}

// rejectLeaves returns the paths of the reject leaves the view currently shows,
// in row order, ignoring the synthetic root and directory rows.
func rejectLeaves(m *Model) []string {
	var out []string
	for _, n := range m.rejects.Items() {
		if n.Item != nil {
			out = append(out, n.Item.Rel)
		}
	}
	return out
}

func TestScanRejectsWalksRecursively(t *testing.T) {
	dir := t.TempDir()
	writeRejects(t, dir,
		"top.txt.svnpatch.rej",
		"src/deep/nested.go.svnpatch.rej",
		"src/other.go.rej",
	)
	// Neither a non-reject nor anything inside .svn belongs in the list.
	writeRejects(t, dir, "src/other.go")
	if err := os.MkdirAll(filepath.Join(dir, ".svn", "tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".svn", "tmp", "admin.rej"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := scanRejects(dir)
	if err != nil {
		t.Fatalf("scanRejects: %v", err)
	}
	// Path order, so the walk flattens straight into a tree.
	want := []string{"src/deep/nested.go.svnpatch.rej", "src/other.go.rej", "top.txt.svnpatch.rej"}
	if len(got) != len(want) {
		t.Fatalf("got %d rejects, want %d: %+v", len(got), len(want), got)
	}
	for i, rel := range want {
		if got[i].Rel != rel {
			t.Errorf("reject %d = %q, want %q", i, got[i].Rel, rel)
		}
		if got[i].Path != filepath.Join(dir, filepath.FromSlash(rel)) {
			t.Errorf("reject %d path = %q, want it under %q", i, got[i].Path, dir)
		}
	}
}

func TestScanRejectsMissingDirIsEmpty(t *testing.T) {
	got, err := scanRejects(filepath.Join(t.TempDir(), "never-created"))
	if err != nil {
		t.Fatalf("a missing source path should not be an error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d rejects, want none", len(got))
	}
}

func TestRejectsViewBuildsATree(t *testing.T) {
	m, _ := rejectModel(t, "a.txt.svnpatch.rej", "src/deep/b.go.svnpatch.rej")
	m = showRejectsView(t, m)

	type row struct {
		name  string
		depth int
		dir   bool
	}
	want := []row{
		{"/", 0, true},
		{"src", 1, true},
		{"deep", 2, true},
		{"b.go.svnpatch.rej", 3, false},
		{"a.txt.svnpatch.rej", 1, false},
	}
	rows := m.rejects.Items()
	if len(rows) != len(want) {
		t.Fatalf("the tree holds %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		got := row{rows[i].Name, rows[i].Depth, rows[i].Item == nil}
		if got != w {
			t.Errorf("row %d = %+v, want %+v", i, got, w)
		}
	}
	// The cursor opens on the first reject, not the synthetic root.
	if r, ok := m.selectedReject(); !ok || r.Rel != "src/deep/b.go.svnpatch.rej" {
		t.Errorf("selected reject = %q (ok=%v), want src/deep/b.go.svnpatch.rej", r.Rel, ok)
	}
}

func TestRejectsTreeCollapsesADirectory(t *testing.T) {
	m, _ := rejectModel(t, "a.txt.svnpatch.rej", "src/b.go.svnpatch.rej")
	m = showRejectsView(t, m)

	// Row 1 is the src/ directory.
	m.rejects.SetIndex(1)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	if got := rejectLeaves(m); len(got) != 1 || got[0] != "a.txt.svnpatch.rej" {
		t.Fatalf("collapsing src/ should hide the reject beneath it, leaves = %v", got)
	}
	if !m.rejectCollapsed["src"] {
		t.Error("src should be remembered as collapsed")
	}
}

func TestRejectsTreeDirectoryRowSummarizesInMain(t *testing.T) {
	m, _ := rejectModel(t, "src/a.go.svnpatch.rej", "src/b.go.svnpatch.rej")
	m = showRejectsView(t, m)

	m.rejects.SetIndex(1) // the src/ directory row
	detail := m.rejectDetail()
	if !strings.Contains(detail, "2 reject(s) under src/") {
		t.Errorf("a directory row should summarize what is beneath it, got:\n%s", detail)
	}
}

func TestRejectsViewListsRejectsRecursively(t *testing.T) {
	m, _ := rejectModel(t, "a.txt.svnpatch.rej", "src/deep/b.go.svnpatch.rej")
	m = showRejectsView(t, m)

	if got := rejectLeaves(m); len(got) != 2 {
		t.Fatalf("the Rejects view holds %d rejects, want 2: %v", len(got), got)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "b.go.svnpatch.rej") || !strings.Contains(view, "deep/") {
		t.Errorf("a nested reject should be listed under its directories, got:\n%s", view)
	}
}

func TestRejectsViewShowsHighlightedRejectInMain(t *testing.T) {
	m, _ := rejectModel(t, "a.txt.svnpatch.rej", "src/b.go.svnpatch.rej")
	m = showRejectsView(t, m)

	// Directories sort before files, so the nested reject is the first leaf.
	if !strings.Contains(stripANSI(m.main.View()), "--- src/b.go.svnpatch.rej") {
		t.Fatalf("Main should show the highlighted reject, got:\n%s", stripANSI(m.main.View()))
	}

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
	if !strings.Contains(stripANSI(m.main.View()), "--- a.txt.svnpatch.rej") {
		t.Errorf("Main should follow the highlighted reject, got:\n%s", stripANSI(m.main.View()))
	}
}

func TestRejectsViewEmpty(t *testing.T) {
	m, dir := rejectModel(t)
	m = showRejectsView(t, m)
	// The rendered viewport truncates the long temp path, so assert on the text
	// Main is filled from.
	if detail := m.rejectDetail(); !strings.Contains(detail, "No rejects under "+dir) {
		t.Errorf("an empty tree should say so, got: %s", detail)
	}
}

func TestRejectsViewIsScannedOnlyWhenShown(t *testing.T) {
	m, _ := rejectModel(t, "a.txt.svnpatch.rej")
	if cmd := m.reloadRejectsIfShown(); cmd != nil {
		t.Error("the Changes view should not pay for a recursive walk")
	}
	m = showRejectsView(t, m)
	if cmd := m.reloadRejectsIfShown(); cmd == nil {
		t.Error("the Rejects view should re-scan on refresh")
	}
}

func TestRejectsViewFilterNarrowsList(t *testing.T) {
	m, _ := rejectModel(t, "alpha.txt.svnpatch.rej", "src/beta.go.svnpatch.rej")
	m = showRejectsView(t, m)

	m.setFilter(panelFiles, "alpha")
	got := rejectLeaves(m)
	if len(got) != 1 || got[0] != "alpha.txt.svnpatch.rej" {
		t.Fatalf("filtered tree holds %v, want just alpha.txt.svnpatch.rej", got)
	}
	// The src/ directory has nothing left under it, so it is gone too.
	for _, n := range m.rejects.Items() {
		if n.Name == "src" {
			t.Error("an empty directory should not survive the filter")
		}
	}
}

func TestDeleteRejectRemovesFileFromDisk(t *testing.T) {
	m, dir := rejectModel(t, "a.txt.svnpatch.rej", "src/b.go.svnpatch.rej")
	m = showRejectsView(t, m)

	// Directories sort first, so the cursor opens on src/b.go.svnpatch.rej.
	m, cmd := requestAndConfirm(t, m, 'd')
	if cmd == nil {
		t.Fatal("confirming the prompt should emit the delete command")
	}
	next, cmd := m.Update(cmd())
	m = next.(*Model)
	if _, err := os.Stat(filepath.Join(dir, "src", "b.go.svnpatch.rej")); !os.IsNotExist(err) {
		t.Errorf("the reject should be gone, stat err = %v", err)
	}
	if cmd == nil {
		t.Fatal("a successful delete should re-scan for rejects")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)

	if got := rejectLeaves(m); len(got) != 1 || got[0] != "a.txt.svnpatch.rej" {
		t.Fatalf("the Rejects view holds %v, want just a.txt.svnpatch.rej", got)
	}
}

func TestDeleteOnARejectDirectoryRowWarns(t *testing.T) {
	m, _ := rejectModel(t, "src/a.go.svnpatch.rej")
	m = showRejectsView(t, m)

	m.rejects.SetIndex(1) // the src/ directory row
	m, _ = pressRune(t, m, 'd')
	if m.confirming {
		t.Fatal("a directory row names no single reject to delete")
	}
	if !strings.Contains(stripANSI(m.View()), "select a reject") {
		t.Errorf("a directory row should say what to do instead, got:\n%s", stripANSI(m.View()))
	}
}

func TestDeleteRejectLeavesWorkingCopyAlone(t *testing.T) {
	m, _ := rejectModel(t, "a.txt.svnpatch.rej")
	m = loadItems(t, m, []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	m = showRejectsView(t, m)

	m, _ = pressRune(t, m, 'd')
	if !m.confirming {
		t.Fatal("d in the Rejects view should ask before deleting")
	}
	if !strings.Contains(stripANSI(m.View()), "a.txt.svnpatch.rej") {
		t.Errorf("the prompt should name the reject, got:\n%s", stripANSI(m.View()))
	}
}

func TestDeleteRejectWithNoneWarns(t *testing.T) {
	m, _ := rejectModel(t)
	m = showRejectsView(t, m)

	m, _ = pressRune(t, m, 'd')
	if m.confirming {
		t.Fatal("there is nothing to confirm deleting")
	}
	if !strings.Contains(stripANSI(m.View()), "no reject to delete") {
		t.Errorf("an empty list should warn, got:\n%s", stripANSI(m.View()))
	}
}

func TestSaveDiffRefusedInRejectsView(t *testing.T) {
	m, _ := rejectModel(t, "a.txt.svnpatch.rej")
	m = showRejectsView(t, m)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = next.(*Model)
	if m.savingDiff {
		t.Error("the save-diff prompt should not open over a reject already on disk")
	}
}
