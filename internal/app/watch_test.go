package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/svn"
)

// fingerprintOf takes one full-scan look at root, failing the test if the tree
// cannot be read at all.
func fingerprintOf(t *testing.T, root string) string {
	t.Helper()
	fp, err := fingerprintScope(root, time.Minute)
	if err != nil {
		t.Fatalf("fingerprintScope(%s): %v", root, err)
	}
	return fp
}

// touch writes body to a path under root and dates it in the future, so a change
// registers no matter how coarse the filesystem's timestamps are.
func touch(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

// TestFingerprintIsStableWhileTheTreeIs is the property the poller rests on:
// looking twice at a working copy nobody touched must answer the same, or every
// tick would trigger a reload.
func TestFingerprintIsStableWhileTheTreeIs(t *testing.T) {
	root := stampRoot(t, map[string]string{"a.txt": "alpha", "src/b.go": "beta"})

	first := fingerprintOf(t, root)
	if second := fingerprintOf(t, root); first != second {
		t.Errorf("fingerprint moved on an untouched tree: %s then %s", first, second)
	}
}

// TestFingerprintMovesOnEveryChange covers what a user, an editor or a build can
// do to a working copy from outside revision.
func TestFingerprintMovesOnEveryChange(t *testing.T) {
	tests := []struct {
		name  string
		apply func(t *testing.T, root string)
	}{
		{"create", func(t *testing.T, root string) { touch(t, root, "src/c.go", "gamma") }},
		{"modify", func(t *testing.T, root string) { touch(t, root, "a.txt", "alpha and then some") }},
		{"delete", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "a.txt")); err != nil {
				t.Fatal(err)
			}
		}},
		{"rename", func(t *testing.T, root string) {
			if err := os.Rename(filepath.Join(root, "a.txt"), filepath.Join(root, "z.txt")); err != nil {
				t.Fatal(err)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := stampRoot(t, map[string]string{
				"a.txt":    "alpha",
				"src/b.go": "beta",
				wcDBPath:   "wc database",
			})
			before := fingerprintOf(t, root)
			tt.apply(t, root)
			if after := fingerprintOf(t, root); after == before {
				t.Errorf("fingerprint did not move on %s", tt.name)
			}
		})
	}
}

// TestFingerprintIgnoresSvnInternals keeps svn's own churn — it rewrites .svn on
// every command — out of the full scan. What matters there is watched by the
// tracked look instead, which stamps the working copy's database directly.
func TestFingerprintIgnoresSvnInternals(t *testing.T) {
	root := stampRoot(t, map[string]string{"a.txt": "alpha", wcDBPath: "wc database"})

	before := fingerprintOf(t, root)
	touch(t, root, filepath.Join(svnDir, "tmp", "scratch"), "svn scratch space")
	if after := fingerprintOf(t, root); after != before {
		t.Error("churn inside .svn should not read as a change to the working copy")
	}
}

// TestFingerprintRefusesAScanItCannotFinish covers the working copy too large to
// read inside the budget: answering with a partial digest would report "nothing
// changed" for everything the scan never reached, so it refuses instead.
func TestFingerprintRefusesAScanItCannotFinish(t *testing.T) {
	files := map[string]string{wcDBPath: "wc database"}
	for i := 0; i < 600; i++ {
		files["f"+strconv.Itoa(i)+".txt"] = "body"
	}
	root := stampRoot(t, files)

	if _, err := fingerprintScope(root, 0); !errors.Is(err, errWatchTooSlow) {
		t.Errorf("fingerprintScope(budget 0) error = %v, want errWatchTooSlow", err)
	}
	if _, err := fingerprintScope(root, time.Minute); err != nil {
		t.Errorf("a scan inside its budget should succeed, got %v", err)
	}
}

// TestTrackedFingerprintWatchesWhatIsOnScreen is the look revision can always
// afford: it covers svn's database and the rows already listed, whatever the
// working copy's size, and is blind to everything else on purpose.
func TestTrackedFingerprintWatchesWhatIsOnScreen(t *testing.T) {
	root := stampRoot(t, map[string]string{
		"dirty.txt": "work in progress",
		"clean.txt": "committed",
		wcDBPath:    "wc database",
	})
	tracked := []string{"dirty.txt"}

	fp := func() string {
		t.Helper()
		got, err := fingerprintTracked(root, tracked)
		if err != nil {
			t.Fatalf("fingerprintTracked: %v", err)
		}
		return got
	}

	before := fp()
	if fp() != before {
		t.Fatal("the tracked look moved on an untouched working copy")
	}

	touch(t, root, "clean.txt", "committed, then edited outside")
	if fp() != before {
		t.Error("a file svn does not report yet is the full scan's job, not this one")
	}

	touch(t, root, "dirty.txt", "work in progress, saved again")
	if after := fp(); after == before {
		t.Fatal("saving a file already on screen must move the tracked look")
	}

	before = fp()
	touch(t, root, wcDBPath, "wc database, moved on")
	if fp() == before {
		t.Error("an svn command run elsewhere must move the tracked look")
	}

	before = fp()
	if err := os.Remove(filepath.Join(root, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	if fp() == before {
		t.Error("a tracked file that is gone must move the tracked look")
	}
}

// TestWatchPaceFollowsTheScanCost is what keeps a large working copy watched at
// all: full scans are spaced in proportion to what they cost, so scanning uses a
// fixed slice of a core rather than a fixed interval.
func TestWatchPaceFollowsTheScanCost(t *testing.T) {
	tests := []struct {
		name string
		took time.Duration
		want time.Duration
	}{
		{"small tree scans on every look", time.Millisecond, watchInterval},
		{"mid-size tree spreads out", 200 * time.Millisecond, 200 * time.Millisecond * watchScanDuty},
		{"the slowest scan still allowed is capped", watchScanBudget, watchScanMax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := watchPace(tt.took); got != tt.want {
				t.Errorf("watchPace(%s) = %s, want %s", tt.took, got, tt.want)
			}
		})
	}
}

// TestFingerprintReportsAnUnreadableRoot makes the poller's failure path
// reachable: a source directory that has gone away is an error, not silence.
func TestFingerprintReportsAnUnreadableRoot(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if _, err := fingerprintScope(gone, time.Minute); err == nil {
		t.Error("expected an error for a directory that does not exist")
	}
	if _, err := fingerprintTracked(gone, nil); err == nil {
		t.Error("expected the tracked look to report a source directory that has gone away")
	}
	if _, err := fingerprintTracked("", nil); err == nil {
		t.Error("expected an error when there is no directory to watch")
	}
}

// watching puts a model under a running poller and takes its first look, so the
// test can hand it changes the way the background watcher would. It returns the
// poller's stamp, which every tick must carry.
func watching(t *testing.T, m *Model) (*Model, uint64) {
	t.Helper()
	if cmd := m.startWatch(); cmd == nil {
		t.Fatal("expected live refresh to start a poller")
	}
	gen := m.watchGen
	next, cmd := m.Update(saw(gen, "t-0", "f-0"))
	if cmd == nil {
		t.Fatal("expected the poller to schedule its next look")
	}
	return next.(*Model), gen
}

// saw builds one look at both depths, as a poller that could afford a full scan
// reports it.
func saw(gen uint64, tracked, full string) workingCopyChangedMsg {
	return workingCopyChangedMsg{tracked: tracked, full: full, scanned: true, gen: gen}
}

// watchedAt builds a model whose svn client is rooted at a real directory and
// puts it under a running poller, so a test can drive it with real fingerprints
// instead of stand-in strings.
func watchedAt(t *testing.T, root string, items []svn.StatusItem) *Model {
	t.Helper()
	m := New(svn.New(root), &svn.Info{WorkingCopyRoot: root, Revision: "42"}, selfupdate.Build{}, config.Default())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadItems(t, next.(*Model), items)
	if cmd := m.startWatch(); cmd == nil {
		t.Fatal("expected live refresh to start a poller")
	}
	return m
}

// peek takes one real look at root on the model's behalf, exactly as the poller's
// command would.
func peek(m *Model, root string, full bool) workingCopyChangedMsg {
	msg := look(root, m.trackedPaths(), full)
	msg.gen = m.watchGen
	return msg
}

// TestLiveRefreshDoesNotChaseItsOwnReload is what keeps the poller from looping:
// the rows a reload adds are watched from then on, so the look after it must
// read them as the new baseline rather than as another change.
func TestLiveRefreshDoesNotChaseItsOwnReload(t *testing.T) {
	root := stampRoot(t, map[string]string{"alpha.txt": "alpha", wcDBPath: "wc database"})
	m := watchedAt(t, root, nil) // a clean working copy: svn status reports nothing

	next, _ := m.Update(peek(m, root, true))
	m = next.(*Model)
	loads := m.statusGen.gen

	// alpha.txt is edited outside; only the full scan can see a file svn still
	// reports as clean.
	touch(t, root, "alpha.txt", "alpha, edited outside")
	next, _ = m.Update(peek(m, root, true))
	m = next.(*Model)
	if m.statusGen.gen != loads+1 {
		t.Fatal("the full scan should have re-read the working copy")
	}

	// svn answers: the file is a row on screen now, and so is watched from here on.
	next, _ = m.Update(statusLoadedMsg{
		items: []svn.StatusItem{{Path: "alpha.txt", State: svn.StateModified}},
		gen:   m.statusGen.gen,
	})
	m = next.(*Model)

	next, _ = m.Update(peek(m, root, true))
	m = next.(*Model)
	if m.statusGen.gen != loads+1 {
		t.Error("the poller chased its own reload")
	}
}

// TestLiveRefreshReloadsOnlyWhenTheWorkingCopyMoves is the whole point of the
// fingerprint: ticking costs nothing until something actually changes, and a
// change costs exactly one status reload.
func TestLiveRefreshReloadsOnlyWhenTheWorkingCopyMoves(t *testing.T) {
	m, gen := watching(t, loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	}))
	loads := m.statusGen.gen

	next, _ := m.Update(saw(gen, "t-0", "f-0"))
	m = next.(*Model)
	if m.statusGen.gen != loads {
		t.Error("an unchanged working copy should cost no svn command")
	}

	next, _ = m.Update(saw(gen, "t-0", "f-1"))
	m = next.(*Model)
	if m.statusGen.gen != loads+1 {
		t.Fatalf("a changed working copy should have been re-read once, got %d loads", m.statusGen.gen-loads)
	}

	next, _ = m.Update(saw(gen, "t-0", "f-1"))
	m = next.(*Model)
	if m.statusGen.gen != loads+1 {
		t.Error("the same change should not be re-read on the next look")
	}
}

// TestLiveRefreshCatchesATrackedFileWithoutAScan is the case a large working
// copy depends on: saving a file already on screen is seen by the cheap look, so
// it lands at full speed even when a full scan is not affordable.
func TestLiveRefreshCatchesATrackedFileWithoutAScan(t *testing.T) {
	m, gen := watching(t, loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	}))
	loads := m.statusGen.gen

	next, _ := m.Update(workingCopyChangedMsg{tracked: "t-1", gen: gen})
	m = next.(*Model)
	if m.statusGen.gen != loads+1 {
		t.Error("a saved file already on screen should be re-read without a full scan")
	}
}

// TestLiveRefreshWaitsForTheScreenToBeFree covers the overlay rule: nothing is
// re-read behind a dialog, and the change seen while it was open is not lost.
func TestLiveRefreshWaitsForTheScreenToBeFree(t *testing.T) {
	m, gen := watching(t, loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: stagedChangelist},
	}))

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if !m.editing {
		t.Fatal("expected the commit editor to open")
	}
	loads := m.statusGen.gen

	next, _ = m.Update(saw(gen, "t-0", "f-1"))
	m = next.(*Model)
	if m.statusGen.gen != loads {
		t.Fatal("the working copy must not be re-read behind an overlay")
	}

	// esc emits the dismissal the app closes the editor on.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.editing {
		t.Fatal("expected the commit editor to close")
	}

	next, _ = m.Update(saw(gen, "t-0", "f-1"))
	m = next.(*Model)
	if m.statusGen.gen != loads+1 {
		t.Error("the held refresh should be taken once the screen is free")
	}
	next, _ = m.Update(saw(gen, "t-0", "f-1"))
	m = next.(*Model)
	if m.statusGen.gen != loads+1 {
		t.Error("the held refresh should be taken once, not on every look after")
	}
}

// TestLiveReloadKeepsTheCursorAndTheScroll is what makes a background refresh
// bearable: it happens under the user rather than to them.
func TestLiveReloadKeepsTheCursorAndTheScroll(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
		{Path: "beta.txt", State: svn.StateModified},
	}
	m := loadItems(t, sizedModel(t), items)

	// Move onto beta and give it a diff long enough to scroll.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(*Model)
	}
	body := make([]string, 40)
	for i := range body {
		body[i] = fmt.Sprintf("+line%02d", i)
	}
	next, _ = m.Update(diffLoadedMsg{path: "beta.txt", diff: "@@ -1 +1 @@\n" + strings.Join(body, "\n")})
	m = next.(*Model)

	m.focus.Focus(panelMain)
	m.afterFocusChange()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = next.(*Model)
	scrolled := stripANSI(m.main.View())
	if strings.Contains(scrolled, "+line00") {
		t.Fatalf("expected Main to have scrolled past the top, got:\n%s", scrolled)
	}

	// The poller's reload lands: same files, same diff.
	next, _ = m.Update(statusLoadedMsg{items: items})
	m = next.(*Model)

	if path := selectedNodePath(m.files); path != "beta.txt" {
		t.Errorf("cursor moved to %q, want it left on beta.txt", path)
	}
	if got := stripANSI(m.main.View()); got != scrolled {
		t.Errorf("a background reload moved Main\n--- before ---\n%s\n--- after ---\n%s", scrolled, got)
	}
}

// TestLiveRefreshToggleStopsThePoller covers the L key: turning live refresh off
// abandons the poller, and the tick it left in flight is dropped rather than
// rescheduled.
func TestLiveRefreshToggleStopsThePoller(t *testing.T) {
	m, gen := watching(t, loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	}))
	loads := m.statusGen.gen

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	m = next.(*Model)
	if m.liveRefresh {
		t.Fatal("L should have turned live refresh off")
	}

	next, cmd := m.Update(saw(gen, "t-0", "f-1"))
	m = next.(*Model)
	if cmd != nil {
		t.Error("a stopped poller must not schedule another look")
	}
	if m.statusGen.gen != loads {
		t.Error("a stopped poller must not re-read the working copy")
	}

	// Pressing it again starts a fresh poller, on a new stamp.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	m = next.(*Model)
	if !m.liveRefresh || cmd == nil {
		t.Fatal("L should have turned live refresh back on")
	}
	if m.watchGen == gen {
		t.Error("a restarted poller should carry a stamp of its own")
	}
}

// TestLiveRefreshOffNeverWatches proves the setting is a real off switch: with
// it disabled nothing is scheduled, so the working copy is never scanned.
func TestLiveRefreshOffNeverWatches(t *testing.T) {
	cfg := config.Default()
	cfg.LiveRefresh = false
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	})
	if cmd := m.startWatch(); cmd != nil {
		t.Error("live refresh is off; nothing should be scheduled")
	}
}

// TestLiveRefreshBacksOffAfterAFailure keeps a working copy that has gone away
// from toasting on every tick, while still retrying.
func TestLiveRefreshBacksOffAfterAFailure(t *testing.T) {
	m, gen := watching(t, loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	}))

	next, cmd := m.Update(workingCopyChangedMsg{err: errors.New("no such directory"), gen: gen})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("a failed look should still schedule the next one")
	}
	if m.watchEvery != watchBackoff {
		t.Errorf("watch interval = %s, want the backoff %s", m.watchEvery, watchBackoff)
	}
	if !m.showingToast {
		t.Error("expected the failure to be reported once")
	}

	m.dismissToast()
	next, _ = m.Update(workingCopyChangedMsg{err: errors.New("no such directory"), gen: gen})
	m = next.(*Model)
	if m.showingToast {
		t.Error("a failure already reported should not toast again")
	}

	next, _ = m.Update(saw(gen, "t-0", "f-0"))
	m = next.(*Model)
	if m.watchEvery != watchInterval {
		t.Errorf("watch interval = %s, want it recovered to %s", m.watchEvery, watchInterval)
	}
}

// TestLiveRefreshDropsScanningItCannotAfford is the large working copy's
// outcome: full scanning stops and says so, but the cheap look carries on, so
// the files already on screen are still watched.
func TestLiveRefreshDropsScanningItCannotAfford(t *testing.T) {
	m, gen := watching(t, loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	}))
	loads := m.statusGen.gen

	next, cmd := m.Update(workingCopyChangedMsg{tracked: "t-0", scanned: true, tooSlow: true, gen: gen})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("the cheap look should carry on when scanning is dropped")
	}
	if !m.watchScanOff {
		t.Error("expected full scanning to be dropped")
	}
	if !m.showingToast {
		t.Error("dropping full scanning should be reported, not silent")
	}
	if !m.liveRefresh {
		t.Error("live refresh itself should stay on")
	}

	next, _ = m.Update(workingCopyChangedMsg{tracked: "t-1", gen: gen})
	m = next.(*Model)
	if m.statusGen.gen != loads+1 {
		t.Error("a file already on screen should still be watched after scanning is dropped")
	}
}
