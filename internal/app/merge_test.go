package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// conflictedFile is a file svn left mid-merge: one block, with the ancestor
// recorded between the two candidates.
const conflictedFile = `package main

func main() {
<<<<<<< .mine
	greet("world")
||||||| .r41
	greet()
=======
	hello("world")
>>>>>>> .r42
}
`

// targetFile is the file a reject was written for. Its lines have moved since
// the patch was made, so the reject's hunk no longer names the right ones.
const targetFile = `package main

func main() {
	setup()
	run()
	teardown()
}
`

// rejectHunkFile is a reject whose one hunk still matches the target's text,
// several lines away from where it claims to belong.
const rejectHunkFile = `--- src/a.go
+++ src/a.go
@@ -10,3 +10,3 @@
 	setup()
-	run()
+	run(ctx)
 	teardown()
`

// writeAt creates a file under dir, making the directories it names on the way.
func writeAt(t *testing.T, dir, rel, body string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return path
}

// resolveKey presses the resolve key and settles whatever it dispatched, so the
// overlay is up (or the warning shown) by the time it returns.
func resolveKey(t *testing.T, m *Model) *Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	return m
}

// conflictModel returns a model whose selection is a conflicted file on disk,
// plus the directory it lives in.
func conflictModel(t *testing.T) (*Model, string) {
	t.Helper()
	m, dir := rejectModel(t)
	writeAt(t, dir, "src/a.go", conflictedFile)
	m = loadItems(t, m, []svn.StatusItem{{Path: "src/a.go", State: svn.StateConflicted}})
	selectFileRow(t, m, "src/a.go")
	return m, dir
}

func TestResolveOpensOnAConflictedFile(t *testing.T) {
	m, _ := conflictModel(t)
	m = resolveKey(t, m)
	if !m.merging {
		t.Fatal("m should open the resolution overlay on a conflicted file")
	}
	view := stripANSI(m.View())
	// The panes are headed by the markers' own labels and what the key does to
	// each; the two choices heading no pane are spelled out in the footer.
	for _, want := range []string{
		"Resolve conflict — src/a.go · 1 of 1 undecided", "1   take mine", "2   take r42",
		"3 take both", "0 undo",
		`greet("world")`, `hello("world")`, "line 5", "esc close",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("overlay missing %q\n---\n%s", want, view)
		}
	}
	// The ancestor recorded between the two candidates is neither of them.
	if strings.Contains(view, "greet()") {
		t.Errorf("the base section is not a candidate\n---\n%s", view)
	}
	// The overlay owns the keyboard while open.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Error("q should not quit while the resolution overlay is open")
		}
	}
}

func TestResolveClosesOnEscAndOnItsOwnKey(t *testing.T) {
	m, _ := conflictModel(t)
	m = resolveKey(t, m)
	if m = resolveKey(t, m); m.merging {
		t.Fatal("m should toggle the overlay closed")
	}

	m = resolveKey(t, m)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a dismiss command from esc")
	}
	next, _ = m.Update(cmd())
	if next.(*Model).merging {
		t.Error("the overlay should close on esc")
	}
}

func TestResolvePickingASideMarksIt(t *testing.T) {
	m, _ := conflictModel(t)
	m = resolveKey(t, m)

	next, _ := m.Update(keyRunes("2"))
	m = next.(*Model)
	view := stripANSI(m.View())
	if !strings.Contains(view, "2 ✓ take r42") || strings.Contains(view, "1 ✓ take mine") {
		t.Errorf("2 should tick the right pane only\n---\n%s", view)
	}
	// With nothing left undecided the footer offers the key that writes it out.
	if !strings.Contains(view, "w write") || !strings.Contains(view, "all decided") {
		t.Errorf("a decided document should offer the write key\n---\n%s", view)
	}
	if m.mergeDoc.regions[0].choice != chooseRight {
		t.Errorf("choice = %v, want chooseRight", m.mergeDoc.regions[0].choice)
	}

	// 3 takes both, 0 puts the decision back.
	next, _ = m.Update(keyRunes("3"))
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "1 ✓ take mine") || !strings.Contains(view, "2 ✓ take r42") {
		t.Errorf("3 should tick both panes\n---\n%s", view)
	}
	next, _ = m.Update(keyRunes("0"))
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "1 of 1 undecided") {
		t.Errorf("0 should clear the decision\n---\n%s", view)
	}
}

func TestResolveRefusesToWriteWhileUndecided(t *testing.T) {
	m, dir := conflictModel(t)
	m = resolveKey(t, m)
	next, cmd := m.Update(keyRunes("w"))
	m = next.(*Model)
	if cmd != nil {
		t.Error("nothing should be written while a region is undecided")
	}
	if !m.merging {
		t.Error("the overlay should stay open when the write is refused")
	}
	if !strings.Contains(m.toast.Message(), "1 of 1 still undecided") {
		t.Errorf("toast = %q, want the undecided count", m.toast.Message())
	}
	body, err := os.ReadFile(filepath.Join(dir, "src", "a.go"))
	if err != nil || string(body) != conflictedFile {
		t.Error("the file must be left exactly as it was")
	}
}

func TestResolveNothingToDoWarns(t *testing.T) {
	m, dir := rejectModel(t)
	writeAt(t, dir, "src/a.go", "package main\n")
	m = loadItems(t, m, []svn.StatusItem{{Path: "src/a.go", State: svn.StateConflicted}})
	selectFileRow(t, m, "src/a.go")
	m = resolveKey(t, m)
	if m.merging {
		t.Error("a file with no markers has nothing to resolve")
	}
	if !strings.Contains(m.toast.Message(), "no conflict markers left") {
		t.Errorf("toast = %q, want the no-markers warning", m.toast.Message())
	}
}

func TestResolveRefusesAFileNotInConflict(t *testing.T) {
	m, _ := rejectModel(t)
	m = loadItems(t, m, []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}})
	selectFileRow(t, m, "src/a.go")
	m = resolveKey(t, m)
	if m.merging {
		t.Error("only a conflicted file can be resolved")
	}
	if !strings.Contains(m.toast.Message(), "is not in conflict") {
		t.Errorf("toast = %q, want the not-in-conflict warning", m.toast.Message())
	}
}

func TestResolveIsFilesPanelOnly(t *testing.T) {
	m, _ := conflictModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}) // focus Log
	if resolveKey(t, next.(*Model)).merging {
		t.Error("m should be inert while a panel other than Files is focused")
	}
}

func TestResolveKeepsConflictsAndRejectsApart(t *testing.T) {
	// A file svn left both conflicted and with a reject beside it: two separate
	// decisions, each belonging to the view that can see it.
	both := strings.TrimSuffix(conflictedFile, "}\n") + "\tsetup()\n\trun()\n\tteardown()\n}\n"
	m, dir := rejectModel(t, "src/a.go.svnpatch.rej")
	writeAt(t, dir, "src/a.go", both)
	writeAt(t, dir, "src/a.go.svnpatch.rej", rejectHunkFile)
	m = loadItems(t, m, []svn.StatusItem{{Path: "src/a.go", State: svn.StateConflicted}})
	selectFileRow(t, m, "src/a.go")

	// The Changes tree resolves the file's own markers.
	m = resolveKey(t, m)
	if !m.merging || m.mergeDoc.kind != mergeConflict {
		t.Fatalf("the Changes view should open the conflict, got merging=%v", m.merging)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Resolve conflict — src/a.go") {
		t.Errorf("expected the conflict overlay\n---\n%s", view)
	}
	if got := len(m.mergeDoc.regions); got != 1 {
		t.Errorf("got %d regions, want only the marker block", got)
	}
	m = resolveKey(t, m) // close

	// The Rejects view resolves the reject's hunks, against the same file.
	m = showRejectsView(t, m)
	m = resolveKey(t, m)
	if !m.merging || m.mergeDoc.kind != mergeReject {
		t.Fatalf("the Rejects view should open the reject, got merging=%v", m.merging)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Resolve rejects — src/a.go") {
		t.Errorf("expected the reject overlay\n---\n%s", view)
	}
	// A document is one kind or the other — never a mix of both.
	if got := len(m.mergeDoc.regions); got != 1 {
		t.Fatalf("got %d regions, want only the rejected hunk", got)
	}
	if strings.Contains(strings.Join(m.mergeDoc.regions[0].left, "\n"), conflictMineMarker) {
		t.Error("a reject's regions must not carry conflict markers")
	}
}

func TestResolveIsInertWhereThereIsNothingToResolve(t *testing.T) {
	// The Diffs view browses saved patches, which are applied rather than resolved.
	m, _ := rejectModel(t)
	for range 2 {
		next, cmd := m.Update(keyRunes("]"))
		m = next.(*Model)
		if cmd != nil {
			next, _ = m.Update(cmd())
			m = next.(*Model)
		}
	}
	if m = resolveKey(t, m); m.merging {
		t.Error("m should be inert in the Diffs view")
	}
	if !strings.Contains(m.toast.Message(), "apply it with p") {
		t.Errorf("toast = %q, want the Diffs-view warning", m.toast.Message())
	}
}

func TestResolveRejectAppliesItsHunkAndClearsIt(t *testing.T) {
	m, dir := rejectModel(t, "src/a.go.svnpatch.rej")
	target := writeAt(t, dir, "src/a.go", targetFile)
	rej := writeAt(t, dir, "src/a.go.svnpatch.rej", rejectHunkFile)
	m = showRejectsView(t, m)
	m = resolveKey(t, m)
	if !m.merging {
		t.Fatal("m should open the resolution overlay on a reject")
	}
	view := stripANSI(m.View())
	// It is the target file that is being resolved, not the reject itself.
	for _, want := range []string{"Resolve rejects — src/a.go", "1   take working copy", "2   take rejected hunk", "run(ctx)"} {
		if !strings.Contains(view, want) {
			t.Errorf("overlay missing %q\n---\n%s", want, view)
		}
	}

	// Take the rejected hunk, then write: the target picks up the change and the
	// reject goes away.
	next, _ := m.Update(keyRunes("2"))
	m = next.(*Model)
	next, cmd := m.Update(keyRunes("w"))
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("w should write a fully decided document")
	}
	if m.merging {
		t.Error("the overlay should close once the write is under way")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)

	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !strings.Contains(string(body), "\trun(ctx)\n") || strings.Contains(string(body), "\trun()\n") {
		t.Errorf("the hunk was not applied to the target:\n%s", body)
	}
	if _, err := os.Stat(rej); !os.IsNotExist(err) {
		t.Error("the reject should be removed once its hunks have been dealt with")
	}
	if !strings.Contains(m.toast.Message(), "applied 1 hunk to src/a.go") {
		t.Errorf("toast = %q, want the cleared-reject notice", m.toast.Message())
	}
}

func TestResolveRejectWhoseHunksNoLongerFit(t *testing.T) {
	m, dir := rejectModel(t, "src/a.go.svnpatch.rej")
	writeAt(t, dir, "src/a.go", "package main\n")
	writeAt(t, dir, "src/a.go.svnpatch.rej", rejectHunkFile)
	m = showRejectsView(t, m)
	m = resolveKey(t, m)
	if m.merging {
		t.Error("a reject with nowhere to go has nothing to decide")
	}
	if !strings.Contains(m.toast.Message(), "hunks still fit") {
		t.Errorf("toast = %q, want the no-longer-fits warning", m.toast.Message())
	}
}

func TestResolveGolden(t *testing.T) {
	// Driven through the load message rather than off disk, so the paths the
	// layout shows are the suite's fixed ones.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "src/a.go", State: svn.StateConflicted}})
	next, _ := m.Update(mergeLoadedMsg{
		doc: conflictDoc("/home/alice/work/wc/src/a.go", "src/a.go", conflictedFile),
		rel: "src/a.go",
	})
	golden.RequireEqual(t, []byte(next.(*Model).View()))
}

func TestConflictRegions(t *testing.T) {
	lines, trailing := splitFileLines(conflictedFile)
	if !trailing {
		t.Error("the file ends with a newline")
	}
	regions, left, right := conflictRegions(lines)
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	if left != "mine" || right != "r42" {
		t.Errorf("labels = %q / %q, want the markers' own", left, right)
	}
	r := regions[0]
	if r.start != 3 || r.end != 10 || r.at != 4 {
		t.Errorf("span = [%d,%d) at %d, want the whole block including its markers", r.start, r.end, r.at)
	}
	if len(r.left) != 1 || !strings.Contains(r.left[0], `greet("world")`) {
		t.Errorf("left candidate = %q", r.left)
	}
	if len(r.right) != 1 || !strings.Contains(r.right[0], `hello("world")`) {
		t.Errorf("right candidate = %q", r.right)
	}
}

func TestConflictRegionsIgnoresAnUnterminatedBlock(t *testing.T) {
	lines, _ := splitFileLines("a\n<<<<<<< .mine\nb\n=======\nc\n")
	if regions, _, _ := conflictRegions(lines); len(regions) != 0 {
		t.Errorf("got %d regions, want none — there is no telling where it ends", len(regions))
	}
	// Unlabelled markers still name the two sides.
	lines, _ = splitFileLines("<<<<<<<\nb\n=======\nc\n>>>>>>>\n")
	regions, left, right := conflictRegions(lines)
	if len(regions) != 1 || left != "mine" || right != "theirs" {
		t.Errorf("got %d regions labelled %q / %q, want generic names", len(regions), left, right)
	}
}

func TestMergedTakesTheChosenSide(t *testing.T) {
	d := conflictDoc("/wc/src/a.go", "src/a.go", conflictedFile)
	if got := d.merged(); got != conflictedFile {
		t.Errorf("an undecided document must be left as it was, got:\n%s", got)
	}
	for _, tc := range []struct {
		choice mergeChoice
		want   string
	}{
		{chooseLeft, "package main\n\nfunc main() {\n\tgreet(\"world\")\n}\n"},
		{chooseRight, "package main\n\nfunc main() {\n\thello(\"world\")\n}\n"},
		{chooseBoth, "package main\n\nfunc main() {\n\tgreet(\"world\")\n\thello(\"world\")\n}\n"},
	} {
		d.regions[0].choice = tc.choice
		if got := d.merged(); got != tc.want {
			t.Errorf("choice %v produced:\n%s\nwant:\n%s", tc.choice, got, tc.want)
		}
	}
}

func TestRejectRegionsFindHunksThatHaveMoved(t *testing.T) {
	lines, _ := splitFileLines(targetFile)
	regions, unplaced := rejectRegions(rejectHunkFile, lines)
	if unplaced != 0 {
		t.Errorf("unplaced = %d, want 0 — the hunk's text is still there", unplaced)
	}
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	// The hunk claims line 10; its text actually starts on line 4.
	r := regions[0]
	if r.start != 3 || r.end != 6 {
		t.Errorf("span = [%d,%d), want the three lines the hunk expects", r.start, r.end)
	}
	if len(r.right) != 3 || !strings.Contains(r.right[1], "run(ctx)") {
		t.Errorf("right candidate = %q", r.right)
	}
}

func TestRejectRegionsCountWhatCannotBePlaced(t *testing.T) {
	lines, _ := splitFileLines("package main\n")
	regions, unplaced := rejectRegions(rejectHunkFile, lines)
	if len(regions) != 0 || unplaced != 1 {
		t.Errorf("got %d regions and %d unplaced, want 0 and 1", len(regions), unplaced)
	}
	// Two hunks over the same lines cannot both apply; the second is left out.
	twice := rejectHunkFile + strings.TrimPrefix(rejectHunkFile, "--- src/a.go\n+++ src/a.go\n")
	lines, _ = splitFileLines(targetFile)
	regions, unplaced = rejectRegions(twice, lines)
	if len(regions) != 1 || unplaced != 1 {
		t.Errorf("got %d regions and %d unplaced, want 1 and 1", len(regions), unplaced)
	}
}

func TestLocateHunkTakesTheNearestMatch(t *testing.T) {
	lines := []string{"x", "dup", "y", "dup", "z"}
	if at, ok := locateHunk(lines, []string{"dup"}, 3); !ok || at != 3 {
		t.Errorf("at = %d (ok=%v), want the match nearest the hint", at, ok)
	}
	if at, ok := locateHunk(lines, []string{"dup"}, 0); !ok || at != 1 {
		t.Errorf("at = %d (ok=%v), want the match nearest the hint", at, ok)
	}
	// A pure insertion goes where it says, as long as the file reaches that far.
	if at, ok := locateHunk(lines, nil, 2); !ok || at != 2 {
		t.Errorf("at = %d (ok=%v), want the named line", at, ok)
	}
	if _, ok := locateHunk(lines, nil, 99); ok {
		t.Error("an insertion past the end of the file has nowhere to go")
	}
}

func TestRejectTargetNamesTheFileItWasWrittenFor(t *testing.T) {
	for in, want := range map[string]string{
		"src/a.go.svnpatch.rej": "src/a.go",
		"src/a.go.rej":          "src/a.go",
		"src/a.go":              "src/a.go",
	} {
		if got := rejectTarget(in); got != want {
			t.Errorf("rejectTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitFileLinesKeepsTheEnding(t *testing.T) {
	for _, text := range []string{"a\nb\n", "a\nb", "", "\n"} {
		lines, trailing := splitFileLines(text)
		d := &mergeDoc{lines: lines, trailing: trailing}
		if got := d.merged(); got != text {
			t.Errorf("round trip of %q gave %q", text, got)
		}
	}
}

func TestMergePagesDimTheSidePassedOver(t *testing.T) {
	d := conflictDoc("/wc/src/a.go", "src/a.go", conflictedFile)
	d.regions[0].choice = chooseLeft
	pages := mergePages(theme.Default(), d)
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want one per region", len(pages))
	}
	if pages[0].Title != "line 5" {
		t.Errorf("title = %q, want the line the region opens on", pages[0].Title)
	}
	// Three lines of context above (the file only has three), the candidates, and
	// the one line below.
	var rows []string
	for _, r := range pages[0].Rows {
		rows = append(rows, strings.TrimSpace(stripANSI(r.Left))+" ~ "+strings.TrimSpace(stripANSI(r.Right)))
	}
	want := []string{
		"1  package main ~ 1  package main",
		"2 ~ 2",
		"3  func main() { ~ 3  func main() {",
		`5      greet("world") ~ 5      hello("world")`,
		"11  } ~ 11  }",
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d:\n%s", len(rows), len(want), strings.Join(rows, "\n"))
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, rows[i], want[i])
		}
	}
}
