package shelf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// seedFile writes a file under dir, creating its parents, and returns its path.
func seedFile(t *testing.T, dir, rel, body string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), perm); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return path
}

// savedEntry stores one entry and fails the test if it could not be written.
func savedEntry(t *testing.T, dir string, e Entry, patch string, untracked []Payload) Entry {
	t.Helper()
	got, err := Save(dir, e, patch, untracked)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	return got
}

func TestSaveAndScanRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Entry{
		Name:         "wip refactor",
		BaseRevision: "1234",
		Files: []FileRec{
			{Path: "src/a.go", State: "modified", Changelist: "revision:staged"},
			{Path: "src/b.go", State: "added"},
		},
		SkippedBinary: []string{"assets/logo.png"},
	}
	saved := savedEntry(t, dir, want, "Index: src/a.go\n", nil)

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Scan returned %d entries, want 1", len(got))
	}
	if got[0].ID != saved.ID {
		t.Errorf("ID = %q, want %q", got[0].ID, saved.ID)
	}
	if got[0].Name != want.Name || got[0].BaseRevision != want.BaseRevision {
		t.Errorf("Scan = %+v, want name %q and base %q", got[0], want.Name, want.BaseRevision)
	}
	if !reflect.DeepEqual(got[0].Files, want.Files) {
		t.Errorf("Files = %+v, want %+v", got[0].Files, want.Files)
	}
	if !reflect.DeepEqual(got[0].SkippedBinary, want.SkippedBinary) {
		t.Errorf("SkippedBinary = %+v, want %+v", got[0].SkippedBinary, want.SkippedBinary)
	}

	patch, err := ReadPatch(dir, saved.ID)
	if err != nil {
		t.Fatalf("ReadPatch: %v", err)
	}
	if want := "Index: src/a.go\n"; patch != want {
		t.Errorf("ReadPatch = %q, want %q", patch, want)
	}
}

func TestSaveAssignsAnIDAndCreationTime(t *testing.T) {
	dir := t.TempDir()
	before := time.Now()
	got := savedEntry(t, dir, Entry{Name: "wip"}, "", nil)

	if got.ID == "" {
		t.Error("Save left the ID empty")
	}
	if got.Created.Before(before.Truncate(time.Second)) {
		t.Errorf("Created = %v, want a time at or after %v", got.Created, before)
	}
	if got.Version != formatVersion {
		t.Errorf("Version = %d, want %d", got.Version, formatVersion)
	}
}

func TestSaveKeepsAnIDAndCreationTimeItWasGiven(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 8, 19, 14, 25, 30, 0, time.UTC)
	got := savedEntry(t, dir, Entry{ID: "fixed-id", Created: at}, "", nil)

	if got.ID != "fixed-id" {
		t.Errorf("ID = %q, want %q", got.ID, "fixed-id")
	}
	if !got.Created.Equal(at) {
		t.Errorf("Created = %v, want %v", got.Created, at)
	}
}

func TestSaveRefusesToOverwriteAnExistingEntry(t *testing.T) {
	dir := t.TempDir()
	savedEntry(t, dir, Entry{ID: "dup"}, "first", nil)

	if _, err := Save(dir, Entry{ID: "dup"}, "second", nil); err == nil {
		t.Fatal("Save over an existing entry = nil, want an error")
	}
	patch, err := ReadPatch(dir, "dup")
	if err != nil {
		t.Fatalf("ReadPatch: %v", err)
	}
	if patch != "first" {
		t.Errorf("patch = %q, want the original %q left intact", patch, "first")
	}
}

func TestSaveRejectsAnUnusableID(t *testing.T) {
	dir := t.TempDir()
	if _, err := Save(dir, Entry{ID: "../escape"}, "", nil); err == nil {
		t.Fatal("Save with a traversing id = nil, want an error")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape")); err == nil {
		t.Error("Save wrote outside the store")
	}
}

func TestSaveCopiesUntrackedPayloads(t *testing.T) {
	wc, dir := t.TempDir(), t.TempDir()
	src := seedFile(t, wc, "docs/new.md", "brand new\n", 0o644)

	saved := savedEntry(t, dir, Entry{}, "", []Payload{{Rel: "docs/new.md", Src: src}})

	if want := []string{"docs/new.md"}; !reflect.DeepEqual(saved.Untracked, want) {
		t.Fatalf("Untracked = %+v, want %+v", saved.Untracked, want)
	}
	payloads, err := UntrackedDir(dir, saved.ID)
	if err != nil {
		t.Fatalf("UntrackedDir: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(payloads, "docs", "new.md"))
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if got := string(body); got != "brand new\n" {
		t.Errorf("payload = %q, want %q", got, "brand new\n")
	}
}

func TestSaveKeepsAPayloadsPermissions(t *testing.T) {
	wc, dir := t.TempDir(), t.TempDir()
	src := seedFile(t, wc, "run.sh", "#!/bin/sh\n", 0o755)

	saved := savedEntry(t, dir, Entry{}, "", []Payload{{Rel: "run.sh", Src: src}})

	payloads, err := UntrackedDir(dir, saved.ID)
	if err != nil {
		t.Fatalf("UntrackedDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(payloads, "run.sh"))
	if err != nil {
		t.Fatalf("stat payload: %v", err)
	}
	// A shelved script has to come back executable, or restoring it breaks it.
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("payload mode = %v, want %v", got, os.FileMode(0o755))
	}
}

func TestSaveRejectsAPayloadEscapingTheEntry(t *testing.T) {
	wc, dir := t.TempDir(), t.TempDir()
	src := seedFile(t, wc, "secret.txt", "x", 0o644)

	if _, err := Save(dir, Entry{}, "", []Payload{{Rel: "../../escaped.txt", Src: src}}); err == nil {
		t.Fatal("Save with a traversing payload = nil, want an error")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escaped.txt")); err == nil {
		t.Error("Save wrote a payload outside the store")
	}
}

func TestSaveRejectsAPayloadThatIsNotARegularFile(t *testing.T) {
	wc, dir := t.TempDir(), t.TempDir()
	target := seedFile(t, wc, "target.txt", "x", 0o644)
	link := filepath.Join(wc, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := Save(dir, Entry{}, "", []Payload{{Rel: "link.txt", Src: link}}); err == nil {
		t.Fatal("Save of a symlink payload = nil, want an error")
	}
}

func TestSaveLeavesNoEntryBehindWhenAPayloadFails(t *testing.T) {
	dir := t.TempDir()

	if _, err := Save(dir, Entry{ID: "doomed"}, "patch", []Payload{{Rel: "gone.txt", Src: filepath.Join(dir, "nope")}}); err == nil {
		t.Fatal("Save with a missing payload source = nil, want an error")
	}
	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Scan returned %d entries, want none after a failed Save", len(got))
	}
	if _, err := os.Stat(filepath.Join(dir, "doomed")); err == nil {
		t.Error("a half-written entry was left in the store")
	}
}

// TestRestorePutsBackWhatItCanPastAFailure pins that one payload the shelf
// cannot write does not strand the ones listed after it. The blocked path is
// named, the rest are put back, and the entry keeps its copy of both.
func TestRestorePutsBackWhatItCanPastAFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory permission this test relies on")
	}
	wc, dir := t.TempDir(), t.TempDir()
	for _, rel := range []string{"a.txt", "sub/wedged.txt", "c.txt"} {
		seedFile(t, wc, rel, rel, 0o644)
	}
	e := savedEntry(t, dir, Entry{ID: "entry"}, "", []Payload{
		{Rel: "a.txt", Src: filepath.Join(wc, "a.txt")},
		{Rel: "sub/wedged.txt", Src: filepath.Join(wc, "sub", "wedged.txt")},
		{Rel: "c.txt", Src: filepath.Join(wc, "c.txt")},
	})

	// A destination whose parent cannot be written to: nothing is in the way, so
	// the payload is attempted and the write is what fails.
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o700) })

	restored, blocked, err := Restore(dir, e.ID, root)
	if err == nil {
		t.Fatal("a payload that could not be written has to be reported")
	}
	if len(restored) != 2 {
		t.Errorf("restored = %v, want the two payloads either side of it", restored)
	}
	if len(blocked) != 1 || blocked[0] != "sub/wedged.txt" {
		t.Errorf("blocked = %v, want only the payload that would not go", blocked)
	}
	if _, statErr := os.Stat(filepath.Join(root, "c.txt")); statErr != nil {
		t.Errorf("c.txt was never written — the earlier failure blocked it: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, e.ID, untrackedDir, "sub", "wedged.txt")); statErr != nil {
		t.Errorf("the entry must keep its copy of a blocked payload: %v", statErr)
	}
}

// TestRestoreLeavesAnOccupiedPathAlone pins that a payload whose destination is
// already taken is reported rather than written over: the file there now is
// somebody's current work.
func TestRestoreLeavesAnOccupiedPathAlone(t *testing.T) {
	wc, dir := t.TempDir(), t.TempDir()
	seedFile(t, wc, "a.txt", "shelved", 0o644)
	e := savedEntry(t, dir, Entry{ID: "entry"}, "", []Payload{{Rel: "a.txt", Src: filepath.Join(wc, "a.txt")}})

	root := t.TempDir()
	seedFile(t, root, "a.txt", "current work", 0o644)

	restored, blocked, err := Restore(dir, e.ID, root)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(restored) != 0 || len(blocked) != 1 || blocked[0] != "a.txt" {
		t.Errorf("restored = %v, blocked = %v, want the occupied path blocked", restored, blocked)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(got) != "current work" {
		t.Errorf("a.txt = %q, want the file already there left alone", got)
	}
}

func TestScanOnAMissingStoreIsEmpty(t *testing.T) {
	got, err := Scan(filepath.Join(t.TempDir(), "never-shelved"))
	if err != nil {
		t.Fatalf("Scan on a missing store = %v, want no error", err)
	}
	if len(got) != 0 {
		t.Errorf("Scan returned %d entries, want none", len(got))
	}
}

func TestScanSkipsUnreadableEntries(t *testing.T) {
	dir := t.TempDir()
	good := savedEntry(t, dir, Entry{Name: "keep"}, "", nil)
	// A directory with no metadata at all, and one whose metadata is not JSON.
	if err := os.MkdirAll(filepath.Join(dir, "no-meta"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seedFile(t, dir, "bad-meta/"+metaFile, "{not json", 0o644)

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 || got[0].ID != good.ID {
		t.Errorf("Scan = %+v, want only the readable entry %q", got, good.ID)
	}
}

func TestScanSkipsAnEntryFromANewerFormat(t *testing.T) {
	dir := t.TempDir()
	data, err := json.Marshal(Entry{Version: formatVersion + 1, ID: "future", Name: "from tomorrow"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	seedFile(t, dir, "future/"+metaFile, string(data), 0o644)

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Scan = %+v, want the newer-format entry skipped", got)
	}
}

func TestScanSkipsAnEntryStillBeingWritten(t *testing.T) {
	dir := t.TempDir()
	seedFile(t, dir, tmpPrefix+"123/"+metaFile, `{"id":"half","name":"half written"}`, 0o644)

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Scan = %+v, want the in-progress directory skipped", got)
	}
}

func TestScanReturnsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	savedEntry(t, dir, Entry{ID: "b", Name: "middle", Created: base.Add(time.Hour)}, "", nil)
	savedEntry(t, dir, Entry{ID: "a", Name: "oldest", Created: base}, "", nil)
	savedEntry(t, dir, Entry{ID: "c", Name: "newest", Created: base.Add(2 * time.Hour)}, "", nil)

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	var names []string
	for _, e := range got {
		names = append(names, e.Name)
	}
	if want := []string{"newest", "middle", "oldest"}; !reflect.DeepEqual(names, want) {
		t.Errorf("Scan order = %v, want %v", names, want)
	}
}

func TestScanTrustsTheDirectoryNameOverTheMetadata(t *testing.T) {
	dir := t.TempDir()
	seedFile(t, dir, "real-id/"+metaFile, `{"version":1,"id":"lying-id","name":"x"}`, 0o644)

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 || got[0].ID != "real-id" {
		t.Errorf("Scan = %+v, want the id taken from the directory name", got)
	}
}

func TestReadPatchRejectsATraversingID(t *testing.T) {
	if _, err := ReadPatch(t.TempDir(), "../../etc/passwd"); err == nil {
		t.Fatal("ReadPatch with a traversing id = nil, want an error")
	}
}

func TestDropRemovesTheEntry(t *testing.T) {
	dir := t.TempDir()
	saved := savedEntry(t, dir, Entry{Name: "throwaway"}, "patch", nil)

	if err := Drop(dir, saved.ID); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Scan = %+v, want the store empty after Drop", got)
	}
}

func TestDropRejectsATraversingID(t *testing.T) {
	dir := t.TempDir()
	victim := seedFile(t, filepath.Dir(dir), "bystander.txt", "x", 0o644)

	if err := Drop(dir, ".."); err == nil {
		t.Fatal("Drop with a traversing id = nil, want an error")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("Drop reached outside the store: %v", err)
	}
}

func TestRenameChangesOnlyTheName(t *testing.T) {
	dir := t.TempDir()
	saved := savedEntry(t, dir, Entry{
		Name:  "before",
		Files: []FileRec{{Path: "src/a.go", State: "modified"}},
	}, "Index: src/a.go\n", nil)

	if err := Rename(dir, saved.ID, "  after  "); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Scan returned %d entries, want 1", len(got))
	}
	if got[0].Name != "after" {
		t.Errorf("Name = %q, want %q (trimmed)", got[0].Name, "after")
	}
	if !got[0].Created.Equal(saved.Created) {
		t.Errorf("Created = %v, want it untouched at %v", got[0].Created, saved.Created)
	}
	if !reflect.DeepEqual(got[0].Files, saved.Files) {
		t.Errorf("Files = %+v, want them untouched at %+v", got[0].Files, saved.Files)
	}
	patch, err := ReadPatch(dir, saved.ID)
	if err != nil {
		t.Fatalf("ReadPatch: %v", err)
	}
	if want := "Index: src/a.go\n"; patch != want {
		t.Errorf("patch = %q, want it untouched at %q", patch, want)
	}
}

func TestRenameOnAMissingEntryFails(t *testing.T) {
	if err := Rename(t.TempDir(), "not-there", "x"); err == nil {
		t.Fatal("Rename of a missing entry = nil, want an error")
	}
}
