package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// fileDiff is a two-hunk patch with a replacement, a pure insertion and a pure
// deletion, so the pairing has every shape to line up.
const fileDiff = `Index: src/a.go
===================================================================
--- src/a.go	(revision 42)
+++ src/a.go	(working copy)
@@ -1,5 +1,6 @@
 package main
 
-import "fmt"
+import (
+	"fmt"
+)
 
 func main() {
@@ -20,3 +21,2 @@
 	done()
-	cleanup()
 }
`

// splitKey presses the side-by-side key and returns the resulting model.
func splitKey(t *testing.T, m *Model) *Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	return next.(*Model)
}

// modifiedFileModel is a model showing the diff for a single modified file.
func modifiedFileModel(t *testing.T) *Model {
	t.Helper()
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	next, _ := m.Update(diffLoadedMsg{path: "src/a.go", diff: fileDiff})
	return next.(*Model)
}

func TestSplitDiffOverlayOpensAndCloses(t *testing.T) {
	m := splitKey(t, modifiedFileModel(t))
	if !m.splitting {
		t.Fatal("expected the side-by-side overlay to open on s")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Side-by-side diff — src/a.go", "revision 42", "working copy", "esc close"} {
		if !strings.Contains(view, want) {
			t.Errorf("overlay missing %q\n---\n%s", want, view)
		}
	}

	// The overlay owns the keyboard while open, so q must not quit.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Error("q should not quit while the side-by-side overlay is open")
		}
	}
	if !m.splitting {
		t.Error("the overlay should stay open on a non-dismiss key")
	}

	// esc comes back as a DismissMsg, which closes it.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a dismiss command from esc")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.splitting {
		t.Error("the overlay should close on esc")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Side-by-side diff") {
		t.Error("the layout should return after closing the overlay")
	}
}

func TestSplitDiffTogglesClosedWithSameKey(t *testing.T) {
	m := splitKey(t, modifiedFileModel(t))
	if !m.splitting {
		t.Fatal("s should open the overlay")
	}
	if m = splitKey(t, m); m.splitting {
		t.Error("s should toggle the overlay closed")
	}
}

func TestSplitDiffShowsBothSidesOfAChange(t *testing.T) {
	view := stripANSI(splitKey(t, modifiedFileModel(t)).View())
	// The removed line and the first line that replaced it share a row, each
	// numbered in its own version of the file.
	if !strings.Contains(view, `3 -import "fmt"`) {
		t.Errorf("removed line missing from the left pane\n---\n%s", view)
	}
	if !strings.Contains(view, "3 +import (") {
		t.Errorf("added line missing from the right pane\n---\n%s", view)
	}
	if !strings.Contains(view, "1  package main") {
		t.Errorf("context line missing\n---\n%s", view)
	}
}

func TestSplitDiffCoversDirectorySubtree(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")
	next, _ := m.Update(diffLoadedMsg{path: "src", dir: true, diff: fileDiff})
	m = splitKey(t, next.(*Model))
	if !m.splitting {
		t.Fatal("expected the overlay to open for a directory row")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Side-by-side diff — src") {
		t.Errorf("overlay should be titled after the directory\n---\n%s", view)
	}
}

// dirDiff is a directory's combined diff of two files, each long enough to fill
// the overlay on its own.
func dirDiff() string {
	var b strings.Builder
	for _, name := range []string{"a", "b"} {
		fmt.Fprintf(&b, "Index: src/%s.go\n", name)
		b.WriteString("===================================================================\n")
		fmt.Fprintf(&b, "--- src/%s.go\t(revision 42)\n", name)
		fmt.Fprintf(&b, "+++ src/%s.go\t(working copy)\n", name)
		fmt.Fprintf(&b, "@@ -1,16 +1,16 @@\n-%s old\n+%s new\n", name, name)
		for i := 0; i < 15; i++ {
			fmt.Fprintf(&b, " %s line %d\n", name, i)
		}
	}
	return b.String()
}

// multiFileModel shows the combined diff of the src/ subtree.
func multiFileModel(t *testing.T) *Model {
	t.Helper()
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")
	next, _ := m.Update(diffLoadedMsg{path: "src", dir: true, diff: dirDiff()})
	return splitKey(t, next.(*Model))
}

func TestSplitDiffPagesOneFileAtATime(t *testing.T) {
	m := multiFileModel(t)
	if got := m.splitDiff.Pages(); got != 2 {
		t.Fatalf("the combined diff has %d pages, want one per file", got)
	}
	// It opens on the first file, named in the footer with its page position and
	// the keys that turn between them.
	view := stripANSI(m.View())
	if !strings.Contains(view, "src/a.go (1/2)") || !strings.Contains(view, "[ ] file") {
		t.Errorf("footer should name the open page and the page keys\n---\n%s", view)
	}
	if strings.Contains(view, "b line 0") {
		t.Errorf("the second file belongs on its own page\n---\n%s", view)
	}

	// "]" turns to the next file, which opens at its own top.
	next, _ := m.Update(keyRunes("]"))
	m = next.(*Model)
	view = stripANSI(m.View())
	if !strings.Contains(view, "src/b.go (2/2) · 1-") {
		t.Errorf("] should open the next file at its top\n---\n%s", view)
	}
	if strings.Contains(view, "-a old") {
		t.Errorf("the first file should be off the page\n---\n%s", view)
	}

	// The pages wrap, like the panel tabs.
	next, _ = m.Update(keyRunes("]"))
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "src/a.go (1/2)") {
		t.Errorf("] should wrap back to the first page\n---\n%s", view)
	}
	next, _ = m.Update(keyRunes("["))
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "src/b.go (2/2)") {
		t.Errorf("[ should wrap back to the last page\n---\n%s", view)
	}
}

func TestSplitDiffPageKeepsItsOwnScroll(t *testing.T) {
	m := multiFileModel(t)
	// Scroll the first page, then turn over: the next page opens at its own top
	// rather than inheriting the offset.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "(1/2) · 2-") {
		t.Fatalf("the first page should have scrolled\n---\n%s", view)
	}
	next, _ = m.Update(keyRunes("]"))
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "(2/2) · 1-") {
		t.Errorf("a turned-to page should open at its top\n---\n%s", view)
	}
}

func TestSplitDiffOfOneFileHidesThePageKeys(t *testing.T) {
	m := splitKey(t, modifiedFileModel(t))
	if got := m.splitDiff.Pages(); got != 1 {
		t.Fatalf("a single-file diff has %d pages, want 1", got)
	}
	view := stripANSI(m.View())
	if strings.Contains(view, "[ ] file") {
		t.Errorf("the page keys should not be advertised with one page\n---\n%s", view)
	}
	// A lone page is named but carries no position, since there is nowhere else.
	if !strings.Contains(view, "src/a.go · 1-") {
		t.Errorf("footer should name the only page without a position\n---\n%s", view)
	}
}

func TestSplitDiffWithoutADiffWarns(t *testing.T) {
	// The diff has not arrived yet, so Main shows a placeholder, not a patch.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.go", State: svn.StateModified}})
	m = splitKey(t, m)
	if m.splitting {
		t.Error("the overlay should not open without a diff on screen")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "no diff to compare") {
		t.Errorf("expected the no-diff warning, got:\n%s", view)
	}
}

func TestSplitDiffRefusesFailedDiff(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.go", State: svn.StateModified}})
	// A load failure fills Main with a notice; it is not a patch to lay out.
	next, _ := m.Update(diffLoadedMsg{path: "a.go", err: errors.New("kaboom")})
	m = splitKey(t, next.(*Model))
	if m.splitting {
		t.Error("the overlay should not open for a failed diff")
	}
}

func TestSplitDiffIsInertWhereMainIsNotShowingTheFilesDiff(t *testing.T) {
	m := modifiedFileModel(t)
	next, _ := m.Update(keyRunes("3")) // Log now drives Main
	if m = splitKey(t, next.(*Model)); m.splitting {
		t.Error("s should be inert while a panel other than Files is focused")
	}
	// Focusing Main leaves the Log panel driving it, so there is still no
	// file diff on screen to lay out.
	next, _ = m.Update(keyRunes("0"))
	if m = splitKey(t, next.(*Model)); m.splitting {
		t.Error("s should not lay out a stale file diff Main is no longer showing")
	}
}

func TestSplitDiffOpensFromTheFocusedDiff(t *testing.T) {
	m := modifiedFileModel(t)
	next, _ := m.Update(keyRunes("0")) // focus Main, still showing the file's diff
	m = splitKey(t, next.(*Model))
	if !m.splitting {
		t.Fatal("s should open the overlay with the diff itself focused")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Side-by-side diff — src/a.go") {
		t.Errorf("overlay should lay out the diff on screen\n---\n%s", view)
	}
}

func TestSplitDiffGolden(t *testing.T) {
	golden.RequireEqual(t, []byte(splitKey(t, modifiedFileModel(t)).View()))
}

func TestSplitDiffRowsPairChanges(t *testing.T) {
	pages := splitDiffPages(theme.Default(), fileDiff)
	if len(pages) != 1 || pages[0].Title != "src/a.go" {
		t.Fatalf("a one-file diff should yield one page titled after it, got %d", len(pages))
	}
	var got []string
	for _, r := range pages[0].Rows {
		if r.Span {
			got = append(got, "| "+strings.TrimSpace(r.Left))
			continue
		}
		got = append(got, strings.TrimSpace(strings.TrimSpace(r.Left)+" ~ "+strings.TrimSpace(r.Right)))
	}
	want := []string{
		"| @@ -1,5 +1,6 @@",
		"1  package main ~ 1  package main",
		"2 ~ 2",
		`3 -import "fmt" ~ 3 +import (`,
		"~ 4 +    \"fmt\"",
		"~ 5 +)",
		"4 ~ 6",
		"5  func main() { ~ 7  func main() {",
		"| @@ -20,3 +21,2 @@",
		"20      done() ~ 21      done()",
		"21 -    cleanup() ~",
		"22  } ~ 22  }",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitDiffLabels(t *testing.T) {
	left, right := splitDiffLabels(fileDiff)
	if left != "revision 42" || right != "working copy" {
		t.Errorf("labels = %q / %q, want the versions from the file markers", left, right)
	}
	// A patch with no file markers still names its panes.
	if left, right = splitDiffLabels("@@ -1 +1 @@\n-a\n+b\n"); left != "original" || right != "modified" {
		t.Errorf("fallback labels = %q / %q, want generic names", left, right)
	}
}

func TestSplitDiffRowsOfEmptyDiff(t *testing.T) {
	if pages := splitDiffPages(theme.Default(), "   \n\n"); pages != nil {
		t.Errorf("a blank diff should yield no pages, got %d", len(pages))
	}
}

func TestParseHunkHeader(t *testing.T) {
	before, after, ok := parseHunkHeader("@@ -12,7 +15 @@ func main()")
	if !ok {
		t.Fatal("expected a well-formed hunk header to parse")
	}
	// A missing count means the span covers a single line.
	if before != (hunkRange{start: 12, count: 7}) || after != (hunkRange{start: 15, count: 1}) {
		t.Errorf("parsed %+v / %+v", before, after)
	}
	for _, ln := range []string{"@@ malformed", "context", "@@ -a,b +c,d @@"} {
		if _, _, ok := parseHunkHeader(ln); ok {
			t.Errorf("%q should not parse as a hunk header", ln)
		}
	}
}
