package app

import (
	"errors"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/component"
)

// svnDir is the metadata directory svn keeps at the root of a working copy. It
// is skipped by the scan — svn rewrites it constantly — and its database is
// stamped on its own instead.
const svnDir = ".svn"

// wcDBPath is the working copy's metadata database, relative to the root. Every
// svn operation writes it, so it is what reveals work done from another terminal
// or another copy of revision.
var wcDBPath = filepath.Join(svnDir, "wc.db")

const (
	// watchInterval is how often the working copy is looked at while live refresh
	// is on. Every look covers what svn already reports as changed, which costs
	// one stat per row on screen no matter how large the tree is.
	watchInterval = 1500 * time.Millisecond
	// watchBackoff is the interval used after a look fails, so an unreadable or
	// disappearing directory is retried without spinning on it.
	watchBackoff = 15 * time.Second
	// watchScanDuty is how many times longer revision waits between full scans
	// than the last one took, so scanning costs a fixed slice of a core rather
	// than a fixed interval.
	watchScanDuty = 20
	// watchScanMax caps how far apart that pacing may push two full scans.
	watchScanMax = 60 * time.Second
	// watchScanBudget is the longest one full scan may run. A working copy that
	// cannot be read inside it is dropped from full scanning altogether: a partial
	// digest would report "nothing changed" for everything it never reached.
	watchScanBudget = 4 * time.Second
)

// errWatchTooSlow abandons a full scan that has run past its budget.
var errWatchTooSlow = errors.New("watch: working copy too large to scan")

// workingCopyChangedMsg carries one look at the working copy, at up to two
// depths. tracked digests svn's database and the paths svn status already
// reports, and is taken every time. full digests every entry under the source
// directory, and is taken on the slower cadence a scan of that size can afford;
// took is what it cost, and tooSlow reports that it was abandoned unfinished.
//
// gen stamps the poller the look came from, so a tick from one that has since
// been stopped or restarted is dropped instead of acted on.
type workingCopyChangedMsg struct {
	tracked string
	full    string
	took    time.Duration
	scanned bool
	tooSlow bool
	err     error
	gen     uint64
}

// watchCmd schedules the next look at the working copy under root, covering the
// paths svn already reports as changed and — when full is set — every entry
// beneath it. The look happens inside the command, off the UI goroutine, and the
// model reschedules the tick when it handles the reply, so the loop lives
// entirely in the Bubble Tea command cycle. There is no bare goroutine to leak.
func watchCmd(root string, paths []string, full bool, gen uint64, every time.Duration) tea.Cmd {
	return tea.Tick(every, func(time.Time) tea.Msg {
		msg := look(root, paths, full)
		msg.gen = gen
		return msg
	})
}

// look takes one look at the working copy, at the requested depth.
func look(root string, paths []string, full bool) workingCopyChangedMsg {
	tracked, err := fingerprintTracked(root, paths)
	if err != nil || !full {
		return workingCopyChangedMsg{tracked: tracked, err: err}
	}

	msg := workingCopyChangedMsg{tracked: tracked, scanned: true}
	start := time.Now()
	fp, err := fingerprintScope(root, watchScanBudget)
	msg.took = time.Since(start)
	switch {
	case errors.Is(err, errWatchTooSlow):
		msg.tooSlow = true
	case err != nil:
		msg.err = err
	default:
		msg.full = fp
	}
	return msg
}

// fingerprintTracked digests what revision is already showing: svn's metadata
// database, and the size and modification time of every path svn status reports.
// It costs one stat per row on screen, so it is affordable on a working copy of
// any size — and it moves whenever one of those files is saved again, reverted
// or deleted, or whenever any svn command runs against the working copy.
func fingerprintTracked(root string, paths []string) (string, error) {
	if root == "" {
		return "", errors.New("watch: no directory to watch")
	}
	if _, err := os.Stat(root); err != nil {
		return "", err
	}
	h := fnv.New64a()
	stampPath(h, root, wcDBPath)
	for _, p := range paths {
		stampPath(h, root, p)
	}
	return strconv.FormatUint(h.Sum64(), 16), nil
}

// fingerprintScope digests every entry under root: its path, size and
// modification time. It is the only look that catches a change svn does not know
// about yet — a file it currently reports as clean, edited in another window —
// and the only one whose cost grows with the working copy.
//
// A scan that runs past budget is abandoned with errWatchTooSlow rather than
// answered partially, since a partial digest would report "nothing changed" for
// everything it never reached.
//
// Size and modification time are the same heuristic build tools use: an edit
// that preserves both, within the filesystem's timestamp resolution, is
// indistinguishable from no edit at all.
func fingerprintScope(root string, budget time.Duration) (string, error) {
	if root == "" {
		return "", errors.New("watch: no directory to watch")
	}
	h := fnv.New64a()
	start := time.Now()
	entries := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A root that cannot be read is a real failure — the source directory
			// has gone away. Below it, a directory that cannot be read contributes
			// nothing, and a file that vanished mid-scan is already covered by its
			// parent's modification time.
			switch {
			case path == root:
				return err
			case d != nil && d.IsDir():
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() && d.Name() == svnDir {
			return fs.SkipDir
		}
		if path == root {
			return nil
		}
		// Checked in batches: reading the clock costs more than a stat.
		if entries++; entries%512 == 0 && time.Since(start) > budget {
			return errWatchTooSlow
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		writeStatStamp(h, path, fi)
		return nil
	})
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(h.Sum64(), 16), nil
}

// stampPath folds one path's on-disk state into the digest. A path that is not
// there stamps as absent, which is itself a change worth seeing.
func stampPath(h io.Writer, root, path string) {
	fi, err := os.Stat(filepath.Join(root, path))
	if err != nil {
		writeStamp(h, path)
		writeStamp(h, "absent")
		return
	}
	writeStatStamp(h, path, fi)
}

// writeStatStamp folds one entry's path and on-disk state into the digest.
func writeStatStamp(h io.Writer, path string, fi fs.FileInfo) {
	writeStamp(h, path)
	writeStamp(h, strconv.FormatInt(fi.Size(), 10))
	writeStamp(h, strconv.FormatInt(fi.ModTime().UnixNano(), 10))
}

// watchPace spreads the next full scan out in proportion to what the last one
// cost, between watchInterval and watchScanMax. A small working copy is scanned
// on every look; a large one is scanned more slowly rather than not at all.
func watchPace(took time.Duration) time.Duration {
	switch every := took * watchScanDuty; {
	case every < watchInterval:
		return watchInterval
	case every > watchScanMax:
		return watchScanMax
	default:
		return every
	}
}

// startWatch begins — or restarts — the live-refresh poller, returning the
// command that takes its first look. Any poller already running is abandoned:
// its next tick carries a stamp the model no longer holds.
func (m *Model) startWatch() tea.Cmd {
	m.stopWatch()
	if !m.liveRefresh || m.client == nil || m.client.Dir == "" {
		return nil
	}
	m.watchEvery = watchInterval
	return m.nextWatch()
}

// stopWatch abandons the poller and forgets what it had seen, so nothing left in
// flight is acted on and a later start begins from a fresh baseline.
func (m *Model) stopWatch() {
	m.watchGen++
	m.watchTrackedFP, m.watchFullFP = "", ""
	m.watchFullDue = time.Time{}
	m.watchScanOff = false
	m.watchQueued = false
	m.watchFailed = false
}

// nextWatch schedules the poller's next look, deciding there whether a full scan
// of the working copy is affordable this time round.
func (m *Model) nextWatch() tea.Cmd {
	if m.client == nil {
		return nil
	}
	full := !m.watchScanOff && !time.Now().Before(m.watchFullDue)
	return watchCmd(m.client.Dir, m.trackedPaths(), full, m.watchGen, m.watchEvery)
}

// trackedPaths is what svn status currently reports, copied out for the poller
// to stat off the UI goroutine.
func (m *Model) trackedPaths() []string {
	paths := make([]string, 0, len(m.fileItems))
	for _, it := range m.fileItems {
		paths = append(paths, it.Path)
	}
	return paths
}

// rebaseWatch re-reads the cheap look from the status that has just landed. The
// baseline is by definition the state the rows on screen were read from, so the
// rows this status added are not a change waiting to be re-read — and any churn
// svn's own database saw while it ran is already accounted for.
func (m *Model) rebaseWatch() {
	if m.watchTrackedFP == "" || m.client == nil {
		return
	}
	if fp, err := fingerprintTracked(m.client.Dir, m.trackedPaths()); err == nil {
		m.watchTrackedFP = fp
	}
}

// observeWorkingCopy takes in one look at the working copy and schedules the
// next. A fingerprint that has not moved costs nothing. One that has calls for a
// quiet re-read of the status — unless the screen is busy, in which case the
// refresh is held and taken at the first look after it frees up.
func (m *Model) observeWorkingCopy(msg workingCopyChangedMsg) tea.Cmd {
	if msg.gen != m.watchGen {
		return nil
	}
	if msg.err != nil {
		// Report the first failure only: a working copy that has gone away would
		// otherwise toast on every tick.
		if !m.watchFailed {
			m.watchFailed = true
			m.showToast(failureText("watch working copy", msg.err), component.LevelWarning)
		}
		m.watchEvery = watchBackoff
		return m.nextWatch()
	}
	m.watchFailed = false
	m.watchEvery = watchInterval
	m.settleScan(msg)

	// The first look at each depth is a baseline: the status on screen was read
	// from it, so there is nothing to compare it to and nothing to refresh.
	moved := m.watchTrackedFP != "" && msg.tracked != m.watchTrackedFP
	moved = moved || (msg.full != "" && m.watchFullFP != "" && msg.full != m.watchFullFP)
	m.watchTrackedFP = msg.tracked
	if msg.full != "" {
		m.watchFullFP = msg.full
	}
	if moved {
		m.watchQueued = true
	}
	if !m.watchQueued || m.refreshHeld() {
		return m.nextWatch()
	}
	m.watchQueued = false
	// A plain status reload is already quiet: it raises no loading flag and no
	// toast, keeps the cursor on the same file and Main at the same scroll
	// position, and serves every diff the working copy has not moved under from
	// the session.
	return tea.Batch(m.nextWatch(), m.reloadStatus())
}

// settleScan records what a full scan cost, pacing the next one by it. A working
// copy that cannot be read inside the budget is dropped from full scanning for
// the rest of the session: revision keeps watching the changes it already knows
// about, which costs nothing, and says so once so the limit is not a mystery.
func (m *Model) settleScan(msg workingCopyChangedMsg) {
	switch {
	case !msg.scanned:
	case msg.tooSlow:
		m.watchScanOff = true
		m.showToast("live refresh: too large to scan — watching current changes only",
			component.LevelWarning)
	default:
		m.watchFullDue = time.Now().Add(watchPace(msg.took))
	}
}

// refreshHeld reports whether now is the wrong moment to re-read the working
// copy on the poller's behalf: an overlay owns the screen, an svn update is
// running, rows are waiting on an action of the user's own, or the first status
// has not landed yet. The refresh is held rather than dropped, and taken at the
// next look once the screen is free.
func (m *Model) refreshHeld() bool {
	return m.overlayActive() || m.updatingWC || m.loading || len(m.pendingOps) > 0
}

// toggleLiveRefresh flips whether the working copy is watched in the background.
// Like the untracked and directory-diff keys it is a session-only view toggle:
// the saved liveRefresh setting keeps deciding the default. It reports the new
// state with a toast.
func (m *Model) toggleLiveRefresh() tea.Cmd {
	m.liveRefresh = !m.liveRefresh
	if !m.liveRefresh {
		m.stopWatch()
		m.showToast("live refresh off", component.LevelInfo)
		return nil
	}
	m.showToast("live refresh on", component.LevelInfo)
	return m.startWatch()
}
