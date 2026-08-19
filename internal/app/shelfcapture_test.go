package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/shelf"
	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
)

// shelveStub returns a client whose svn binary answers `diff` with patch and
// exits with revertCode for `revert`, together with a reader for the argument
// lists it was called with. It lets a capture run for real against a working
// copy that reverts cleanly and one that does not.
func shelveStub(t *testing.T, dir, patch string, revertCode int) (*svn.Client, func() []string) {
	t.Helper()
	tmp := t.TempDir()
	body := filepath.Join(tmp, "diff.txt")
	if err := os.WriteFile(body, []byte(patch), 0o644); err != nil {
		t.Fatalf("write stub diff: %v", err)
	}
	argv := filepath.Join(tmp, "argv.txt")
	bin := filepath.Join(tmp, "svn-stub")
	script := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %s
case "$1" in
  diff) cat %s ;;
  revert) exit %d ;;
esac
exit 0
`, argv, body, revertCode)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub binary: %v", err)
	}
	calls := func() []string {
		b, err := os.ReadFile(argv)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	}
	return &svn.Client{Dir: dir, Bin: bin}, calls
}

// wcFile writes a file into a stand-in working copy and returns its path.
func wcFile(t *testing.T, wc, rel, body string) string {
	t.Helper()
	path := filepath.Join(wc, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return path
}

// capture runs a shelve against a stub and returns its message.
func capture(t *testing.T, c *svn.Client, store string, req shelveRequest) shelvedMsg {
	t.Helper()
	return msgOf[shelvedMsg](t, shelveCmd(c, store, req))
}

func TestShelveWritesTheEntryBeforeTouchingTheWorkingCopy(t *testing.T) {
	wc := t.TempDir()
	store := filepath.Join(wc, shelf.DirName)
	wcFile(t, wc, "a.txt", "changed\n")
	// A revert that fails is the moment the ordering has to hold: the entry is
	// already on disk, so nothing has been lost.
	c, _ := shelveStub(t, wc, "Index: a.txt\n+changed\n", 1)

	msg := capture(t, c, store, shelveRequest{
		name:  "wip",
		items: []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}},
	})

	if msg.err == nil {
		t.Fatal("a failing revert must be reported")
	}
	if msg.entry.ID == "" {
		t.Fatal("the entry must be reported so the caller knows the changes were saved")
	}
	entries, err := shelf.Scan(store)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "wip" {
		t.Errorf("store holds %+v, want the entry kept despite the failed revert", entries)
	}
	if _, err := os.Stat(filepath.Join(wc, "a.txt")); err != nil {
		t.Errorf("a.txt should be untouched after a failed revert: %v", err)
	}
}

func TestShelveRecordsStateAndChangelistPerFile(t *testing.T) {
	wc := t.TempDir()
	store := filepath.Join(wc, shelf.DirName)
	c, _ := shelveStub(t, wc, "Index: a.txt\n+x\n", 0)

	msg := capture(t, c, store, shelveRequest{
		name:    "wip",
		baseRev: "1234",
		items: []svn.StatusItem{
			{Path: "a.txt", State: svn.StateModified, Changelist: stagedChangelist},
			{Path: "b.txt", State: svn.StateDeleted},
		},
	})

	if msg.err != nil {
		t.Fatalf("shelve: %v", msg.err)
	}
	if msg.entry.BaseRevision != "1234" {
		t.Errorf("BaseRevision = %q, want 1234", msg.entry.BaseRevision)
	}
	want := []shelf.FileRec{
		{Path: "a.txt", State: "modified", Changelist: stagedChangelist},
		{Path: "b.txt", State: "deleted"},
	}
	if !slices.Equal(msg.entry.Files, want) {
		t.Errorf("Files = %+v, want %+v — svn's patch format keeps neither", msg.entry.Files, want)
	}
}

func TestShelveLeavesBinariesInTheWorkingCopy(t *testing.T) {
	wc := t.TempDir()
	store := filepath.Join(wc, shelf.DirName)
	patch := "Index: a.txt\n===\n@@ -1 +1 @@\n+x\n" +
		"Index: logo.bin\n===\nCannot display: file marked as a binary type.\n"
	c, calls := shelveStub(t, wc, patch, 0)

	msg := capture(t, c, store, shelveRequest{
		name: "wip",
		items: []svn.StatusItem{
			{Path: "a.txt", State: svn.StateModified},
			{Path: "logo.bin", State: svn.StateModified},
		},
	})

	if msg.err != nil {
		t.Fatalf("shelve: %v", msg.err)
	}
	if want := []string{"logo.bin"}; !slices.Equal(msg.entry.SkippedBinary, want) {
		t.Errorf("SkippedBinary = %v, want %v", msg.entry.SkippedBinary, want)
	}
	if !slices.Contains(msg.left, "logo.bin") {
		t.Errorf("left = %v, want the binary reported as left behind", msg.left)
	}
	if len(msg.entry.Files) != 1 || msg.entry.Files[0].Path != "a.txt" {
		t.Errorf("Files = %+v, want only the file the patch actually carries", msg.entry.Files)
	}
	// Reverting a file whose content the patch does not hold would destroy it.
	for _, call := range calls() {
		if strings.HasPrefix(call, "revert") && strings.Contains(call, "logo.bin") {
			t.Errorf("revert called with the binary: %q", call)
		}
	}
}

func TestShelveCarriesUntrackedFilesAsBytesAndClearsThem(t *testing.T) {
	wc := t.TempDir()
	store := filepath.Join(wc, shelf.DirName)
	wcFile(t, wc, "docs/new.md", "brand new\n")
	c, _ := shelveStub(t, wc, "", 0)

	msg := capture(t, c, store, shelveRequest{
		name:  "wip",
		items: []svn.StatusItem{{Path: "docs/new.md", State: svn.StateUnversioned}},
	})

	if msg.err != nil {
		t.Fatalf("shelve: %v", msg.err)
	}
	if want := []string{"docs/new.md"}; !slices.Equal(msg.entry.Untracked, want) {
		t.Fatalf("Untracked = %v, want %v — svn diff cannot see an unversioned file", msg.entry.Untracked, want)
	}
	payloads, err := shelf.UntrackedDir(store, msg.entry.ID)
	if err != nil {
		t.Fatalf("UntrackedDir: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(payloads, "docs", "new.md")); err != nil || string(got) != "brand new\n" {
		t.Errorf("payload = %q (%v), want the file's bytes", got, err)
	}
	if _, err := os.Stat(filepath.Join(wc, "docs", "new.md")); !os.IsNotExist(err) {
		t.Errorf("the untracked file should have been cleared from the working copy (%v)", err)
	}
}

func TestShelveClearsAScheduledAddLeftBehindByRevert(t *testing.T) {
	wc := t.TempDir()
	store := filepath.Join(wc, shelf.DirName)
	// svn un-schedules the add but leaves the file, so the capture removes it.
	wcFile(t, wc, "added.txt", "new\n")
	c, _ := shelveStub(t, wc, "Index: added.txt\n+new\n", 0)

	msg := capture(t, c, store, shelveRequest{
		name:  "wip",
		items: []svn.StatusItem{{Path: "added.txt", State: svn.StateAdded}},
	})

	if msg.err != nil {
		t.Fatalf("shelve: %v", msg.err)
	}
	if _, err := os.Stat(filepath.Join(wc, "added.txt")); !os.IsNotExist(err) {
		t.Errorf("added.txt should have been removed after the revert un-scheduled it (%v)", err)
	}
}

func TestShelveLeavesAnUntrackedDirectoryAlone(t *testing.T) {
	wc := t.TempDir()
	store := filepath.Join(wc, shelf.DirName)
	if err := os.MkdirAll(filepath.Join(wc, "scratch"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	wcFile(t, wc, "a.txt", "x\n")
	c, _ := shelveStub(t, wc, "Index: a.txt\n+x\n", 0)

	msg := capture(t, c, store, shelveRequest{
		name: "wip",
		items: []svn.StatusItem{
			{Path: "a.txt", State: svn.StateModified},
			{Path: "scratch", State: svn.StateUnversioned},
		},
	})

	if msg.err != nil {
		t.Fatalf("shelve: %v", msg.err)
	}
	if !slices.Contains(msg.left, "scratch") {
		t.Errorf("left = %v, want the directory reported as left behind", msg.left)
	}
	if _, err := os.Stat(filepath.Join(wc, "scratch")); err != nil {
		t.Errorf("an untracked directory has no bytes to carry, so it must stay: %v", err)
	}
}

func TestShelveRefusesWhenNothingCanBeCarried(t *testing.T) {
	wc := t.TempDir()
	store := filepath.Join(wc, shelf.DirName)
	patch := "Index: logo.bin\n===\nCannot display: file marked as a binary type.\n"
	c, _ := shelveStub(t, wc, patch, 0)

	msg := capture(t, c, store, shelveRequest{
		name:  "wip",
		items: []svn.StatusItem{{Path: "logo.bin", State: svn.StateModified}},
	})

	if msg.err == nil {
		t.Fatal("a shelve that would hold nothing must be refused")
	}
	if entries, _ := shelf.Scan(store); len(entries) != 0 {
		t.Errorf("store holds %+v, want no empty entry written", entries)
	}
}

func TestShelveReportsAFailingDiffWithoutWriting(t *testing.T) {
	wc := t.TempDir()
	store := filepath.Join(wc, shelf.DirName)
	c, _ := shelveStub(t, wc, "", 0)
	c.Bin = filepath.Join(t.TempDir(), "missing-svn")

	msg := capture(t, c, store, shelveRequest{
		name:  "wip",
		items: []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}},
	})

	if msg.err == nil {
		t.Fatal("a diff svn refused must be reported")
	}
	if entries, _ := shelf.Scan(store); len(entries) != 0 {
		t.Errorf("store holds %+v, want nothing written when the diff failed", entries)
	}
}

func TestShelvableItemSkipsWhatCannotBeSetAside(t *testing.T) {
	cases := map[string]struct {
		item svn.StatusItem
		want bool
	}{
		"modified":      {svn.StatusItem{State: svn.StateModified}, true},
		"added":         {svn.StatusItem{State: svn.StateAdded}, true},
		"deleted":       {svn.StatusItem{State: svn.StateDeleted}, true},
		"unversioned":   {svn.StatusItem{State: svn.StateUnversioned}, true},
		"property only": {svn.StatusItem{State: svn.StateNormal, PropState: svn.StateModified}, true},
		"clean":         {svn.StatusItem{State: svn.StateNormal}, false},
		"conflicted":    {svn.StatusItem{State: svn.StateConflicted}, false},
		"ignored":       {svn.StatusItem{State: svn.StateIgnored}, false},
		"external":      {svn.StatusItem{State: svn.StateExternal}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := shelvableItem(tc.item); got != tc.want {
				t.Errorf("shelvableItem = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPickHoldsTheHighlightedFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified},
		{Path: "b.txt", State: svn.StateModified},
	})
	selectFileRow(t, m, "a.txt")

	m, _ = pressRune(t, m, 'v')

	if !m.isShelfPicked("a.txt") {
		t.Error("v should hold the highlighted file")
	}
	if m.isShelfPicked("b.txt") {
		t.Error("v should hold only what it was pressed on")
	}
	// A second press lets it go again.
	m, _ = pressRune(t, m, 'v')
	if m.isShelfPicked("a.txt") {
		t.Error("v pressed again should release the file")
	}
}

func TestPickOnADirectoryHoldsEverythingUnderIt(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.txt", State: svn.StateModified},
		{Path: "src/b.txt", State: svn.StateModified},
		{Path: "docs/c.txt", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")

	m, _ = pressRune(t, m, 'v')

	if !m.isShelfPicked("src/a.txt") || !m.isShelfPicked("src/b.txt") {
		t.Error("v on a directory should hold everything shelvable beneath it")
	}
	if m.isShelfPicked("docs/c.txt") {
		t.Error("v on a directory should not reach outside it")
	}
}

func TestPickOnAChangelistHoldsItsFiles(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified, Changelist: "feature-x"},
		{Path: "b.txt", State: svn.StateModified, Changelist: "feature-x"},
		{Path: "c.txt", State: svn.StateModified},
	})
	// Changes files already filed under a changelist without drilling into it.
	m, _ = pressRune(t, m, ']')
	if !m.filesViewIsChangelists() {
		t.Fatal("expected the Changelists view")
	}

	m, _ = pressRune(t, m, 'v')

	g, ok := m.changelists.Selected()
	if !ok {
		t.Fatal("no changelist selected")
	}
	for _, it := range g.Items {
		if !m.isShelfPicked(it.Path) {
			t.Errorf("%s should be held after picking its changelist", it.Path)
		}
	}
	if got := m.groupPicked(g); got != len(g.Items) {
		t.Errorf("groupPicked = %d, want all %d", got, len(g.Items))
	}
}

func TestPickOnAChangelistReleasesItOnASecondPress(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified, Changelist: "feature-x"},
	})
	m, _ = pressRune(t, m, ']')

	m, _ = pressRune(t, m, 'v')
	m, _ = pressRune(t, m, 'v')

	if len(m.pickedItems()) != 0 {
		t.Error("a second press on the changelist should release its files")
	}
}

func TestPickInsideADrilledChangelist(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified, Changelist: "feature-x"},
		{Path: "b.txt", State: svn.StateModified, Changelist: "feature-x"},
	})
	m, _ = pressRune(t, m, ']')
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from the changelists list")
	}
	next, _ := m.Update(cmd())
	m = next.(*Model)
	if !m.inChangelistDrill() {
		t.Fatal("expected to be drilled into the changelist")
	}
	for i, n := range m.clFiles.Items() {
		if n.Item != nil && n.Item.Path == "a.txt" {
			m.clFiles.SetIndex(i)
		}
	}

	m, _ = pressRune(t, m, 'v')

	if !m.isShelfPicked("a.txt") {
		t.Error("v should hold a file inside a drilled-in changelist")
	}
	if m.isShelfPicked("b.txt") {
		t.Error("v should hold only the highlighted file")
	}
}

func TestPickedFilesSurviveAStatusReload(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified},
		{Path: "b.txt", State: svn.StateModified},
	}
	m := loadItems(t, sizedModel(t), items)
	selectFileRow(t, m, "a.txt")
	m, _ = pressRune(t, m, 'v')

	m = loadItems(t, m, items)

	if !m.isShelfPicked("a.txt") {
		t.Error("picks are held by path, so a reload should not drop them")
	}
}

func TestPickedFileThatIsGoneDropsOutOfTheSet(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified},
		{Path: "b.txt", State: svn.StateModified},
	})
	selectFileRow(t, m, "a.txt")
	m, _ = pressRune(t, m, 'v')

	// a.txt has been committed away behind the pick.
	m = loadItems(t, m, []svn.StatusItem{{Path: "b.txt", State: svn.StateModified}})

	if got := len(m.pickedItems()); got != 0 {
		t.Errorf("pickedItems = %d, want the stale pick resolved away", got)
	}
}

func TestEscReleasesEveryPick(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}})
	selectFileRow(t, m, "a.txt")
	m, _ = pressRune(t, m, 'v')

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)

	if len(m.pickedItems()) != 0 {
		t.Error("esc should release the picks")
	}
}

func TestEscLeavesADrillWithoutReleasingItsPicks(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified, Changelist: "feature-x"},
		{Path: "b.txt", State: svn.StateModified, Changelist: "feature-x"},
	})
	m, _ = pressRune(t, m, ']')
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from the changelists list")
	}
	next, _ := m.Update(cmd())
	m = next.(*Model)
	m, _ = pressRune(t, m, 'v')
	if len(m.pickedItems()) == 0 {
		t.Fatal("expected a pick inside the drill")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)

	// Picking inside a changelist and stepping back out is how a shelve of part
	// of one is built, so esc has to be a step out and not a release.
	if m.inChangelistDrill() {
		t.Error("esc should leave the drill")
	}
	if len(m.pickedItems()) == 0 {
		t.Fatal("esc out of a drill must keep what was picked in it")
	}

	m, _ = pressRune(t, m, 'z')
	if m.confirming {
		t.Error("the picks carried out of the drill are what z should act on")
	}
	if !m.shelfNaming {
		t.Error("z should open the name prompt for the picks kept from the drill")
	}
}

func TestEscReleasesPicksOnceTheDrillIsBehind(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified, Changelist: "feature-x"},
	})
	m, _ = pressRune(t, m, ']')
	m, _ = pressRune(t, m, 'v')

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)

	if len(m.pickedItems()) != 0 {
		t.Error("esc with no drill to leave should release the picks")
	}
}

func TestShelveWithPicksGoesStraightToTheNamePrompt(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified},
		{Path: "b.txt", State: svn.StateModified},
	})
	selectFileRow(t, m, "a.txt")
	m, _ = pressRune(t, m, 'v')

	m, _ = pressRune(t, m, 'z')

	if m.confirming {
		t.Error("an explicit pick has already said what to shelve; nothing to confirm")
	}
	if !m.shelfNaming {
		t.Fatal("z with files picked should open the name prompt")
	}
	if got := len(m.shelveTarget.items); got != 1 {
		t.Errorf("held scope covers %d items, want just the picked one", got)
	}
	if got := m.shelveTarget.label; got != "a.txt" {
		t.Errorf("label = %q, want the single picked file's path", got)
	}
}

func TestShelveWithNothingPickedConfirmsFirst(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.txt", State: svn.StateModified},
		{Path: "b.txt", State: svn.StateModified},
	})

	m, _ = pressRune(t, m, 'z')

	if !m.confirming {
		t.Fatal("taking the whole working copy should be confirmed first")
	}
	if m.shelfNaming {
		t.Error("the name prompt must wait for the confirmation")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	// The confirmation's command is what opens the prompt.
	next, _ = m.Update(shelveAllMsg{})
	m = next.(*Model)

	if !m.shelfNaming {
		t.Fatal("accepting the confirmation should open the name prompt")
	}
	if got := len(m.shelveTarget.items); got != 2 {
		t.Errorf("held scope covers %d items, want the whole working copy", got)
	}
	if got := m.shelveTarget.label; got != "all changes" {
		t.Errorf("label = %q, want %q", got, "all changes")
	}
}

func TestShelveWithNothingToTakeSaysSo(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.txt", State: svn.StateNormal}})

	m, _ = pressRune(t, m, 'z')

	if m.confirming || m.shelfNaming {
		t.Error("there is nothing to shelve, so nothing should open")
	}
	if got := m.toast.Message(); !strings.Contains(got, "nothing to shelve") {
		t.Errorf("toast = %q, want it to say there is nothing to shelve", got)
	}
}

func TestShelveIsInertOnTheLogPanel(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}})
	m, _ = pressRune(t, m, '3')

	m, _ = pressRune(t, m, 'z')

	if m.confirming || m.shelfNaming {
		t.Error("the Log panel shows history, which holds no changes to shelve")
	}
}

func TestPickIsInertInTheDiffsView(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}})
	for range 2 {
		m, _ = pressRune(t, m, ']')
	}
	if !m.filesViewIsDiffs() {
		t.Fatal("expected the Diffs view")
	}

	m, _ = pressRune(t, m, 'v')

	if len(m.shelfPicks) != 0 {
		t.Error("the Diffs view lists files on disk, which no shelve can take")
	}
}

func TestShelveNameFallsBackToTheSetItWasTakenFrom(t *testing.T) {
	if got := shelveEntryName("  ", "all changes"); got != "all changes" {
		t.Errorf("shelveEntryName = %q, want the fallback", got)
	}
	if got := shelveEntryName("  wip  ", "all changes"); got != "wip" {
		t.Errorf("shelveEntryName = %q, want the trimmed entry", got)
	}
}

func TestPickedScopeLabelNamesASingleFile(t *testing.T) {
	one := []svn.StatusItem{{Path: "src/a.txt"}}
	if got := pickedScopeLabel(one); got != "src/a.txt" {
		t.Errorf("pickedScopeLabel = %q, want the file's own path", got)
	}
	two := []svn.StatusItem{{Path: "a.txt"}, {Path: "b.txt"}}
	if got := pickedScopeLabel(two); got != "picked changes" {
		t.Errorf("pickedScopeLabel = %q, want %q", got, "picked changes")
	}
}

func TestDismissingTheNamePromptDropsTheShelve(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}})
	selectFileRow(t, m, "a.txt")
	m, _ = pressRune(t, m, 'v')
	m, _ = pressRune(t, m, 'z')

	m.closeShelfName()

	if m.shelfNaming {
		t.Error("the prompt should be closed")
	}
	if len(m.shelveTarget.items) != 0 {
		t.Error("the queued scope should be dropped with the prompt")
	}
}

func TestShelveToastNamesWhatStayedBehind(t *testing.T) {
	e := shelf.Entry{Name: "wip", Files: []shelf.FileRec{{Path: "a.txt"}, {Path: "b.txt"}}}

	text, level := shelveToast(e, nil)
	if !strings.Contains(text, "2 files") || !strings.Contains(text, "wip") {
		t.Errorf("toast = %q, want the count and the name", text)
	}
	if level != component.LevelSuccess {
		t.Errorf("level = %v, want success when nothing was left", level)
	}

	text, level = shelveToast(e, []string{"logo.bin"})
	if !strings.Contains(text, "1 file left behind") {
		t.Errorf("toast = %q, want what stayed behind reported", text)
	}
	if level != component.LevelWarning {
		t.Error("leaving files behind should not read as an unqualified success")
	}
}
