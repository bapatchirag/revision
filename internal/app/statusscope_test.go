package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bapatchirag/revision/internal/svn"
)

func TestStatusScopeCollapsesToTheSubtreesAReadHasToCover(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src", State: svn.StateAdded},
		{Path: "src/a.go", State: svn.StateAdded},
		{Path: "src/deep/b.go", State: svn.StateAdded},
		{Path: "other/c.go", State: svn.StateModified},
	}
	got := statusScope([]string{"src", "src/a.go", "src/deep/b.go", "other/c.go"}, items)
	want := []string{"other/c.go", "src"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("statusScope = %v, want %v — a status read recurses, so naming src covers what is under it", got, want)
	}
}

// TestStatusScopePullsInTheOtherHalfOfAMove pins the case a targeted read cannot
// see on its own: reverting the destination of a move leaves the source still
// scheduled for deletion but with its link gone, and a read naming only the
// destination would leave that stale row on screen.
func TestStatusScopePullsInTheOtherHalfOfAMove(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateDeleted, MovedTo: "src/renamed.go"},
		{Path: "src/renamed.go", State: svn.StateAdded, Copied: true, MovedFrom: "src/a.go"},
	}
	got := statusScope([]string{"src/renamed.go"}, items)
	want := []string{"src/a.go", "src/renamed.go"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("statusScope = %v, want %v", got, want)
	}
}

// TestStatusScopeDeclinesASetTooLargeToTarget pins the ceiling: past it the read
// stops being a bargain and the paths stop fitting on a command line, so the
// caller is sent back to a full read.
func TestStatusScopeDeclinesASetTooLargeToTarget(t *testing.T) {
	paths := make([]string, statusScopeMax+1)
	for i := range paths {
		paths[i] = "dir" + strconv.Itoa(i) + "/f.go"
	}
	if got := statusScope(paths, nil); got != nil {
		t.Errorf("statusScope over %d subtrees = %v, want nothing so a full read is taken", len(paths), got)
	}
	if got := statusScope(paths[:statusScopeMax], nil); len(got) != statusScopeMax {
		t.Errorf("statusScope at the limit returned %d paths, want %d", len(got), statusScopeMax)
	}
	if got := statusScope(nil, nil); got != nil {
		t.Errorf("statusScope(nil) = %v, want nothing", got)
	}
}

func TestScopeCovers(t *testing.T) {
	scope := []string{"src", "other/c.go"}
	cases := map[string]bool{
		"src":           true,
		"src/a.go":      true,
		"src/deep/b.go": true,
		"other/c.go":    true,
		"other":         false,
		"other/d.go":    false,
		"src-gen/e.go":  false, // a sibling whose name only starts the same way
	}
	for path, want := range cases {
		if got := scopeCovers(scope, path); got != want {
			t.Errorf("scopeCovers(%v, %q) = %v, want %v", scope, path, got, want)
		}
	}
}

// TestSpliceStatusReplacesOnlyTheScope pins the whole point of a targeted read:
// the paths it covers take their new state (or go, when it reports nothing for
// them), and everything it never looked at is left exactly as it was.
func TestSpliceStatusReplacesOnlyTheScope(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified},
		{Path: "src/one.go", State: svn.StateModified},
		{Path: "src/two.go", State: svn.StateModified},
		{Path: "z.go", State: svn.StateModified},
	}
	fresh := []svn.StatusItem{{Path: "src/two.go", State: svn.StateUnversioned}}

	got := spliceStatus(items, []string{"src"}, fresh)
	want := []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified},
		{Path: "src/two.go", State: svn.StateUnversioned},
		{Path: "z.go", State: svn.StateModified},
	}
	if len(got) != len(want) {
		t.Fatalf("spliceStatus = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestSpliceStatusKeepsThePathOrder pins the invariant every consumer of
// fileItems is built on: a status is ordered by path, and a splice that appended
// its fresh entries at the end would break it.
func TestSpliceStatusKeepsThePathOrder(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified},
		{Path: "m.go", State: svn.StateModified},
		{Path: "z.go", State: svn.StateModified},
	}
	fresh := []svn.StatusItem{
		{Path: "b.go", State: svn.StateUnversioned},
		{Path: "n.go", State: svn.StateUnversioned},
	}
	got := spliceStatus(items, []string{"b.go", "n.go"}, fresh)
	var paths []string
	for _, it := range got {
		paths = append(paths, it.Path)
	}
	if want := "a.go b.go m.go n.go z.go"; strings.Join(paths, " ") != want {
		t.Errorf("paths = %q, want %q", strings.Join(paths, " "), want)
	}
}

// recordingClient returns a client whose svn stub answers every invocation with
// an empty status, together with a reader for the command lines it was asked to
// run — which is where a targeted read can be told from a full one.
func recordingClient(t *testing.T) (*svn.Client, func() []string) {
	t.Helper()
	const emptyStatus = `<?xml version="1.0"?><status><target path="."></target></status>`
	client := cmdClient(t, emptyStatus, 0)
	var cmds []string
	client.Recorder = func(r svn.CommandRecord) { cmds = append(cmds, r.Command) }
	return client, func() []string { return cmds }
}

// TestRevertReReadsOnlyWhatItTouched is the point of the whole exercise: the
// reload a revert triggers used to crawl the entire working copy before a single
// row could change, however little was reverted.
func TestRevertReReadsOnlyWhatItTouched(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "other/c.go", State: svn.StateModified},
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	client, calls := recordingClient(t)
	m.client = client

	_, cmd := m.Update(revertedMsg{outcome: batchOutcome{done: []string{"src/a.go", "src/b.go"}}})
	run(t, cmd)

	got := calls()
	if len(got) != 1 {
		t.Fatalf("ran %d commands, want one status read: %v", len(got), got)
	}
	for _, want := range []string{"src/a.go", "src/b.go"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("%q does not name %s", got[0], want)
		}
	}
	if strings.Contains(got[0], "other/c.go") {
		t.Errorf("%q re-reads a path the revert never touched", got[0])
	}
}

// TestRevertReReadsAPathItWasRefused pins that a refusal is not a reason to skip
// the path: svn judges each on its own, and one it refused can still have moved.
func TestRevertReReadsAPathItWasRefused(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	client, calls := recordingClient(t)
	m.client = client

	out := batchOutcome{done: []string{"src/a.go"}}
	out.fail("src/b.go", errSkipped)
	_, cmd := m.Update(revertedMsg{outcome: out})
	run(t, cmd)

	if got := calls(); len(got) != 1 || !strings.Contains(got[0], "src/b.go") {
		t.Errorf("commands = %v, want the refused path re-read too", got)
	}
}

// TestReloadStatusForFallsBackToAFullRead pins the two cases a targeted read is
// wrong for: nothing worth targeting, and a change shown ahead of svn that is
// still in flight — the snapshot behind it describes the whole status, and a
// read covering part of it has nothing to say about the rest.
func TestReloadStatusForFallsBackToAFullRead(t *testing.T) {
	full := func(t *testing.T, m *Model, paths []string, why string) {
		t.Helper()
		client, calls := recordingClient(t)
		m.client = client
		run(t, m.reloadStatusFor(paths))
		got := calls()
		if len(got) != 1 || !strings.HasSuffix(got[0], "status --xml --non-interactive") {
			t.Errorf("commands = %v, want one whole-working-copy read (%s)", got, why)
		}
	}

	items := []svn.StatusItem{{Path: "src/a.go", State: svn.StateModified}}
	m := loadItems(t, sizedModel(t), items)
	m.optimistic = &optimisticState{token: 1, items: items}
	full(t, m, []string{"src/a.go"}, "a change is still in flight")

	full(t, loadItems(t, sizedModel(t), items), nil, "no paths to target")
}

// TestAScopedReadLeavesTheRestOfTheTreeAlone drives the reply itself: the paths
// it covers settle on what it reports, and the ones it never looked at stand.
func TestAScopedReadLeavesTheRestOfTheTreeAlone(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "other/c.go", State: svn.StateModified},
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	})
	next, _ := m.Update(statusLoadedMsg{
		scope: []string{"src/a.go", "src/b.go"},
		items: []svn.StatusItem{{Path: "src/b.go", State: svn.StateUnversioned}},
	})
	m = next.(*Model)

	var got []string
	for _, it := range m.fileItems {
		got = append(got, it.Path+":"+string(it.State))
	}
	want := "other/c.go:modified src/b.go:unversioned"
	if strings.Join(got, " ") != want {
		t.Errorf("fileItems = %q, want %q", strings.Join(got, " "), want)
	}
}

// TestRevertKeepsADiffItDidNotTouch pins what the reload no longer costs: Main
// used to be blanked on every revert, including one of a file it was not
// showing.
func TestRevertKeepsADiffItDidNotTouch(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
	}
	m := loadItems(t, sizedModel(t), items)
	client, _ := recordingClient(t)
	m.client = client
	m.applyDiff(diffKey{path: "src/b.go"}, diffEntry{text: "@@ -1 +1 @@\n-old\n+new\n"})

	next, _ := m.Update(revertedMsg{outcome: singleOutcome("src/a.go", nil)})
	m = next.(*Model)
	if m.diffPath != "src/b.go" || m.diffText == "" {
		t.Errorf("diff = %q/%q, want the untouched file's diff still on screen", m.diffPath, m.diffText)
	}

	next, _ = m.Update(revertedMsg{outcome: singleOutcome("src/b.go", nil)})
	m = next.(*Model)
	if m.diffPath != "" || m.diffText != "" {
		t.Errorf("diff = %q/%q, want the reverted file's diff dropped", m.diffPath, m.diffText)
	}
}

// TestDiffTouchedByCoversADirectoryRow pins the other shape Main can be showing:
// a directory diff spans every change beneath it, so a revert of any one of them
// discards content it is displaying.
func TestDiffTouchedByCoversADirectoryRow(t *testing.T) {
	m := &Model{diffPath: "src", diffOfDir: true}
	if !m.diffTouchedBy([]string{"src/a.go"}) {
		t.Error("a directory diff is touched by a revert beneath it")
	}
	if m.diffTouchedBy([]string{"other/c.go"}) {
		t.Error("a directory diff is not touched by a revert outside it")
	}
	if (&Model{}).diffTouchedBy([]string{"src/a.go"}) {
		t.Error("no diff on screen can be touched by nothing")
	}
}

// svnRun runs a command in dir, failing the test if it does not come back clean.
func svnRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
}

// TestRevertRefreshMatchesAFullStatusRead is the claim both halves of the revert
// refresh rest on, checked against a real svn rather than a stub. Neither the
// state settled from svn's own per-path verdict nor the one spliced in from a
// targeted read may differ from re-reading the working copy whole.
//
// The fixture holds every shape a revert leaves behind: a file that comes clean,
// a plain add that un-schedules to untracked, an added directory that collapses
// to the one row at its head, a copy destination svn takes away with the add, a
// restored deletion, and a move whose halves are reverted apart — beside changes
// the revert never touches, which must survive it.
func TestRevertRefreshMatchesAFullStatusRead(t *testing.T) {
	for _, bin := range []string{"svn", "svnadmin"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found on PATH; skipping integration test", bin)
		}
	}
	root := t.TempDir()
	repo, wc := filepath.Join(root, "repo"), filepath.Join(root, "wc")
	svnRun(t, "", "svnadmin", "create", repo)
	svnRun(t, "", "svn", "checkout", "file://"+repo, wc)

	write := func(rel, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(wc, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(wc, rel), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{"src/edited.txt", "src/moved.txt", "src/gone.txt", "src/origin.txt", "other/kept.txt"} {
		write(rel, "one\n")
	}
	svnRun(t, wc, "svn", "add", "src", "other")
	svnRun(t, wc, "svn", "commit", "-m", "initial")
	svnRun(t, wc, "svn", "update")

	write("src/edited.txt", "one\ntwo\n")
	write("src/fresh.txt", "new\n")
	write("src/tree/a.txt", "a\n")
	write("src/tree/b.txt", "b\n")
	write("other/kept.txt", "one\ntwo\n") // untouched by the revert below
	svnRun(t, wc, "svn", "add", "src/fresh.txt", "src/tree")
	svnRun(t, wc, "svn", "delete", "src/gone.txt")
	svnRun(t, wc, "svn", "move", "src/moved.txt", "src/renamed.txt")
	svnRun(t, wc, "svn", "copy", "src/origin.txt", "src/copied.txt")
	// A staged add: the changelist goes with the add, svn keeping one only for a
	// path it still versions.
	svnRun(t, wc, "svn", "changelist", "revision:staged", "src/fresh.txt")

	ctx := context.Background()
	c := svn.New(wc)
	before, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	attempted := []string{
		"src/edited.txt", "src/fresh.txt", "src/gone.txt",
		"src/renamed.txt", "src/copied.txt",
		"src/tree", "src/tree/a.txt", "src/tree/b.txt",
	}
	res := c.RevertPaths(ctx, attempted)
	if res.Err() != nil {
		t.Fatalf("RevertPaths: %v", res.Err())
	}

	want, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status after revert: %v", err)
	}
	same := func(label string, got []svn.StatusItem) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s has %d rows, a full read has %d:\n got %+v\nwant %+v",
				label, len(got), len(want), got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s row %d = %+v, want %+v", label, i, got[i], want[i])
			}
		}
	}

	settled, changed := revertedStatus(before, res.Reverted)
	if !changed {
		t.Error("a revert that discarded eight paths must have changed the status")
	}
	same("settled", settled)

	scope := statusScope(attempted, before)
	if !scopeCovers(scope, "src/moved.txt") {
		t.Fatalf("scope %v leaves out the other half of the move", scope)
	}
	fresh, err := c.StatusPaths(ctx, scope)
	if err != nil {
		t.Fatalf("StatusPaths: %v", err)
	}
	same("spliced", spliceStatus(before, scope, fresh))
	// The confirming read must land on the settled state too, or the tree would
	// flicker between the two.
	same("settled then spliced", spliceStatus(settled, scope, fresh))
}
