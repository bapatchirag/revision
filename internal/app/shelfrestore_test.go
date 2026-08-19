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

// shelvedStore writes one entry into a store beside a stand-in working copy and
// returns the working copy, the store and the entry.
func shelvedStore(t *testing.T, e shelf.Entry, patch string, payloads []shelf.Payload) (wc, store string, saved shelf.Entry) {
	t.Helper()
	wc = t.TempDir()
	store = filepath.Join(wc, shelf.DirName)
	saved, err := shelf.Save(store, e, patch, payloads)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	return wc, store, saved
}

// restoreStub returns a client whose svn binary answers `patch` with out and
// `changelist` successfully, recording every invocation.
func restoreStub(t *testing.T, dir, out string, code int) (*svn.Client, func() []string) {
	t.Helper()
	tmp := t.TempDir()
	body := filepath.Join(tmp, "patch.txt")
	if err := os.WriteFile(body, []byte(out), 0o644); err != nil {
		t.Fatalf("write stub output: %v", err)
	}
	argv := filepath.Join(tmp, "argv.txt")
	bin := filepath.Join(tmp, "svn-stub")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\ncase \"$1\" in\n  patch) cat %s; exit %d ;;\nesac\nexit 0\n",
		argv, body, code)
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

// cleanPatchOutput is what svn prints when every target went in cleanly.
const cleanPatchOutput = "U         a.txt\n"

func TestRestorePutsAPatchBackAndKeepsTheEntry(t *testing.T) {
	wc, store, e := shelvedStore(t, shelf.Entry{Name: "wip"},
		"Index: a.txt\n--- a.txt\t(revision 1)\n+++ a.txt\t(working copy)\n@@ -1 +1 @@\n+x\n", nil)
	wcFile(t, wc, "a.txt", "x\n")
	c, _ := restoreStub(t, wc, cleanPatchOutput, 0)

	msg := msgOf[shelfRestoredMsg](t, restoreShelfCmd(c, store, e, false))

	if msg.err != nil {
		t.Fatalf("restore: %v", msg.err)
	}
	if msg.dropped {
		t.Error("applying keeps the entry on the shelf")
	}
	if entries, _ := shelf.Scan(store); len(entries) != 1 {
		t.Errorf("store holds %d entries, want the applied one kept", len(entries))
	}
	if !slices.Equal(msg.res.Applied, []string{"a.txt"}) {
		t.Errorf("Applied = %v, want [a.txt]", msg.res.Applied)
	}
}

func TestPopDropsTheEntryOnACleanRestore(t *testing.T) {
	wc, store, e := shelvedStore(t, shelf.Entry{Name: "wip"},
		"Index: a.txt\n--- a.txt\t(revision 1)\n+++ a.txt\t(working copy)\n@@ -1 +1 @@\n+x\n", nil)
	wcFile(t, wc, "a.txt", "x\n")
	c, _ := restoreStub(t, wc, cleanPatchOutput, 0)

	msg := msgOf[shelfRestoredMsg](t, restoreShelfCmd(c, store, e, true))

	if msg.err != nil {
		t.Fatalf("restore: %v", msg.err)
	}
	if !msg.dropped {
		t.Error("a clean pop should take the entry off the shelf")
	}
	if entries, _ := shelf.Scan(store); len(entries) != 0 {
		t.Errorf("store holds %d entries, want it emptied by the pop", len(entries))
	}
}

func TestPopKeepsTheEntryWhenAHunkIsRejected(t *testing.T) {
	wc, store, e := shelvedStore(t, shelf.Entry{Name: "wip"},
		"Index: a.txt\n--- a.txt\t(revision 1)\n+++ a.txt\t(working copy)\n@@ -1 +1 @@\n+x\n", nil)
	wcFile(t, wc, "a.txt", "x\n")
	// One target applied, one left in conflict with its hunks in a .rej.
	c, _ := restoreStub(t, wc, "U         a.txt\nC         b.txt\n", 0)

	msg := msgOf[shelfRestoredMsg](t, restoreShelfCmd(c, store, e, true))

	if msg.err != nil {
		t.Fatalf("restore: %v", msg.err)
	}
	if msg.dropped {
		t.Error("the shelf is the only copy of what did not go back, so it must be kept")
	}
	if entries, _ := shelf.Scan(store); len(entries) != 1 {
		t.Errorf("store holds %d entries, want the entry kept", len(entries))
	}
}

func TestRestorePutsUntrackedFilesBack(t *testing.T) {
	src := t.TempDir()
	payload := wcFile(t, src, "docs/new.md", "brand new\n")
	wc, store, e := shelvedStore(t, shelf.Entry{Name: "wip"}, "",
		[]shelf.Payload{{Rel: "docs/new.md", Src: payload}})
	c, calls := restoreStub(t, wc, "", 0)

	msg := msgOf[shelfRestoredMsg](t, restoreShelfCmd(c, store, e, false))

	if msg.err != nil {
		t.Fatalf("restore: %v", msg.err)
	}
	if !slices.Equal(msg.restored, []string{"docs/new.md"}) {
		t.Errorf("restored = %v, want [docs/new.md]", msg.restored)
	}
	got, err := os.ReadFile(filepath.Join(wc, "docs", "new.md"))
	if err != nil || string(got) != "brand new\n" {
		t.Errorf("restored file = %q (%v), want its bytes back", got, err)
	}
	// An entry holding only unversioned files has no patch to run.
	for _, call := range calls() {
		if strings.HasPrefix(call, "patch") {
			t.Errorf("svn patch should not run for an empty patch: %q", call)
		}
	}
}

func TestRestoreWillNotOverwriteAFileInTheWay(t *testing.T) {
	src := t.TempDir()
	payload := wcFile(t, src, "docs/new.md", "shelved\n")
	wc, store, e := shelvedStore(t, shelf.Entry{Name: "wip"}, "",
		[]shelf.Payload{{Rel: "docs/new.md", Src: payload}})
	wcFile(t, wc, "docs/new.md", "current work\n")
	c, _ := restoreStub(t, wc, "", 0)

	msg := msgOf[shelfRestoredMsg](t, restoreShelfCmd(c, store, e, true))

	if msg.err != nil {
		t.Fatalf("restore: %v", msg.err)
	}
	if !slices.Equal(msg.blocked, []string{"docs/new.md"}) {
		t.Errorf("blocked = %v, want the occupied path reported", msg.blocked)
	}
	if got, _ := os.ReadFile(filepath.Join(wc, "docs", "new.md")); string(got) != "current work\n" {
		t.Errorf("file = %q, want the working copy's own version untouched", got)
	}
	if msg.dropped {
		t.Error("a payload that could not be put back must keep the entry")
	}
}

func TestRestoreReplaysChangelistMembership(t *testing.T) {
	wc, store, e := shelvedStore(t, shelf.Entry{
		Name: "wip",
		Files: []shelf.FileRec{
			{Path: "a.txt", State: "modified", Changelist: "feature-x"},
			{Path: "b.txt", State: "modified"},
			{Path: "gone.txt", State: "deleted", Changelist: "feature-x"},
		},
	}, "Index: a.txt\n--- a.txt\t(revision 1)\n+++ a.txt\t(working copy)\n@@ -1 +1 @@\n+x\n", nil)
	wcFile(t, wc, "a.txt", "x\n")
	wcFile(t, wc, "b.txt", "y\n")
	c, calls := restoreStub(t, wc, cleanPatchOutput, 0)

	msg := msgOf[shelfRestoredMsg](t, restoreShelfCmd(c, store, e, false))

	if msg.err != nil {
		t.Fatalf("restore: %v", msg.err)
	}
	var changelisted []string
	for _, call := range calls() {
		if strings.HasPrefix(call, "changelist") {
			changelisted = append(changelisted, call)
		}
	}
	if len(changelisted) != 1 || !strings.Contains(changelisted[0], "feature-x a.txt") {
		t.Errorf("changelist calls = %v, want only a.txt put back in feature-x", changelisted)
	}
}

func TestRestoreRefusesAShelfFromAnotherDirectory(t *testing.T) {
	wc, store, e := shelvedStore(t, shelf.Entry{Name: "wip"},
		"Index: a.txt\n--- a.txt\t(revision 1)\n+++ a.txt\t(working copy)\n@@ -1 +1 @@\n+x\n", nil)
	// a.txt is not in this working copy, so the patch was taken somewhere else.
	c, calls := restoreStub(t, wc, cleanPatchOutput, 0)

	msg := msgOf[shelfRestoredMsg](t, restoreShelfCmd(c, store, e, false))

	if msg.err == nil {
		t.Fatal("a shelf whose files are not here must be refused")
	}
	if got := calls(); len(got) != 0 {
		t.Errorf("svn ran %v, want nothing attempted", got)
	}
	if entries, _ := shelf.Scan(store); len(entries) != 1 {
		t.Error("a refused restore must leave the entry alone")
	}
}

func TestDropRemovesTheEntryFromTheStore(t *testing.T) {
	_, store, e := shelvedStore(t, shelf.Entry{Name: "wip"}, "patch", nil)

	msg := msgOf[shelfDroppedMsg](t, dropShelfCmd(store, e.ID, "wip"))

	if msg.err != nil {
		t.Fatalf("drop: %v", msg.err)
	}
	if entries, _ := shelf.Scan(store); len(entries) != 0 {
		t.Errorf("store holds %d entries, want it emptied", len(entries))
	}
}

func TestRenameRelabelsTheEntry(t *testing.T) {
	_, store, e := shelvedStore(t, shelf.Entry{Name: "before"}, "patch", nil)

	msg := msgOf[shelfRenamedMsg](t, renameShelfCmd(store, e.ID, "after"))

	if msg.err != nil {
		t.Fatalf("rename: %v", msg.err)
	}
	entries, _ := shelf.Scan(store)
	if len(entries) != 1 || entries[0].Name != "after" {
		t.Errorf("store = %+v, want the entry relabelled", entries)
	}
}

func TestEnterAsksBeforeApplyingAShelf(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	m = seedShelves(t, m, []shelf.Entry{shelfEntry("20260819-1", "wip", 2)})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from the shelf list")
	}
	next, _ := m.Update(cmd())
	m = next.(*Model)

	if !m.confirming {
		t.Fatal("applying a shelf should be confirmed first")
	}
	if got := stripANSI(m.View()); !strings.Contains(got, "Apply shelf?") {
		t.Errorf("expected the apply confirmation, got:\n%s", got)
	}
}

func TestPopAndDropAskWithTheirOwnWording(t *testing.T) {
	base := func() *Model {
		m := focusShelf(t, sizedModel(t))
		return seedShelves(t, m, []shelf.Entry{shelfEntry("20260819-1", "wip", 2)})
	}
	cases := map[string]struct{ key, want string }{
		"pop":  {"p", "Pop shelf?"},
		"drop": {"d", "Drop shelf?"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m, _ := pressRune(t, base(), rune(tc.key[0]))
			if !m.confirming {
				t.Fatalf("%s should be confirmed first", name)
			}
			if got := stripANSI(m.View()); !strings.Contains(got, tc.want) {
				t.Errorf("expected %q, got:\n%s", tc.want, got)
			}
		})
	}
}

func TestRestoreConfirmationWarnsWhenTheBaseHasMoved(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	e := shelfEntry("20260819-1", "wip", 1)
	e.BaseRevision = "40"
	m = seedShelves(t, m, []shelf.Entry{e})
	// sizedModel's working copy is at r42.

	m, _ = pressRune(t, m, 'p')

	got := stripANSI(m.View())
	if !strings.Contains(got, "r40") || !strings.Contains(got, "r42") {
		t.Errorf("expected both revisions named in the warning, got:\n%s", got)
	}
}

func TestRestoreConfirmationIsQuietWhenTheBaseMatches(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	e := shelfEntry("20260819-1", "wip", 1)
	e.BaseRevision = "42"
	m = seedShelves(t, m, []shelf.Entry{e})

	m, _ = pressRune(t, m, 'p')

	if got := stripANSI(m.View()); strings.Contains(got, ".rej") {
		t.Errorf("no drift, so no warning was called for:\n%s", got)
	}
}

func TestShelfActionsSayWhenTheStoreIsEmpty(t *testing.T) {
	for _, key := range []rune{'p', 'd', 'n'} {
		m := focusShelf(t, sizedModel(t))
		m, _ = pressRune(t, m, key)
		if m.confirming || m.shelfRenaming {
			t.Errorf("%c should do nothing with an empty store", key)
		}
		if got := m.toast.Message(); !strings.Contains(got, "no shelf to") {
			t.Errorf("%c toast = %q, want it to say there is no shelf", key, got)
		}
	}
}

func TestRenamePromptOpensPrefilled(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	m = seedShelves(t, m, []shelf.Entry{shelfEntry("20260819-1", "wip refactor", 1)})

	m, _ = pressRune(t, m, 'n')

	if !m.shelfRenaming {
		t.Fatal("n should open the rename prompt")
	}
	if got := m.renameEditor.Value(); got != "wip refactor" {
		t.Errorf("prompt value = %q, want it prefilled with the current name", got)
	}
	if m.renameTarget != "20260819-1" {
		t.Errorf("renameTarget = %q, want the highlighted entry", m.renameTarget)
	}
}

func TestRenameToBlankLeavesTheEntryAlone(t *testing.T) {
	m := focusShelf(t, sizedModel(t))
	m = seedShelves(t, m, []shelf.Entry{shelfEntry("20260819-1", "wip", 1)})
	m, _ = pressRune(t, m, 'n')

	if cmd := m.submitShelfRename("   "); cmd != nil {
		t.Error("a blank name is more likely a slip than an intent, so nothing should run")
	}
	if m.shelfRenaming {
		t.Error("the prompt should close either way")
	}
}

func TestShelfRestoreToastReportsWhatDidNotComeBack(t *testing.T) {
	clean := shelfRestoredMsg{name: "wip", res: svn.PatchResult{Applied: []string{"a.txt", "b.txt"}}}
	text, level := shelfRestoreToast(clean)
	if !strings.Contains(text, "applied wip to 2 files") {
		t.Errorf("toast = %q, want the count of files put back", text)
	}
	if level != component.LevelSuccess {
		t.Errorf("level = %v, want success", level)
	}

	popped := clean
	popped.dropped = true
	if text, _ := shelfRestoreToast(popped); !strings.HasPrefix(text, "popped") {
		t.Errorf("toast = %q, want it to say the entry was popped", text)
	}

	messy := shelfRestoredMsg{
		name:    "wip",
		res:     svn.PatchResult{Applied: []string{"a.txt"}, Conflicted: []string{"b.txt"}},
		blocked: []string{"docs/new.md"},
	}
	text, level = shelfRestoreToast(messy)
	if !strings.Contains(text, "rejects") || !strings.Contains(text, "in the way") {
		t.Errorf("toast = %q, want both leftovers named", text)
	}
	if !strings.Contains(text, "kept on the shelf") {
		t.Errorf("toast = %q, want it to say the entry was kept", text)
	}
	if level == component.LevelSuccess {
		t.Error("leftovers should not read as an unqualified success")
	}
}
