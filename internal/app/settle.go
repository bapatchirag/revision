package app

import (
	"github.com/bapatchirag/revision/internal/svn"
)

// settledStatus rewrites the status items a finished action has settled. svn has
// already said which paths it acted on, so what became of each is known without
// asking for the working copy again — which is the difference between a tree
// that changes as the toast appears and one that waits on a second svn process
// to say what the first one just did.
//
// settle reports the row a path the action touched is left with, or false when
// svn stops reporting that path at all. Rows outside done are left alone.
//
// It reports whether anything changed, so an action that moved nothing on screen
// costs no rebuild.
func settledStatus(items []svn.StatusItem, done []string, settle func(svn.StatusItem) (svn.StatusItem, bool)) ([]svn.StatusItem, bool) {
	if len(done) == 0 {
		return items, false
	}
	rows := make(map[string]svn.StatusItem, len(items))
	var kept []string
	for _, it := range items {
		if !scopeCovers(done, it.Path) {
			continue
		}
		if row, ok := settle(it); ok {
			rows[it.Path] = row
			kept = append(kept, it.Path)
		}
	}
	// svn does not look inside a directory it has stopped tracking or scheduled to
	// go, so a tree an action settles is reported as the single row at its head.
	head := make(map[string]bool, len(kept))
	for _, cv := range svn.CoverPaths(kept) {
		head[cv.Lead] = true
	}

	out := make([]svn.StatusItem, 0, len(items))
	changed := false
	for _, it := range items {
		switch {
		case !scopeCovers(done, it.Path):
			out = append(out, it)
			continue
		case head[it.Path]:
			row := rows[it.Path]
			out = append(out, row)
			if row == it {
				continue
			}
		}
		changed = true
	}

	// A move settled a half at a time leaves the half still standing with a link
	// to a file that is no longer there. A named half svn left where it was still
	// is the other end of the move, so its partner's link stands.
	still := make(map[string]bool, len(out))
	for i := range out {
		still[out[i].Path] = true
	}
	gone := func(partner string) bool { return scopeCovers(done, partner) && !still[partner] }
	for i := range out {
		if gone(out[i].MovedFrom) || gone(out[i].MovedTo) {
			out[i].MovedFrom, out[i].MovedTo = "", ""
			changed = true
		}
	}
	return out, changed
}

// revertedStatus derives the status a finished revert left behind.
//
// Almost everything a revert touches stops being reported: a file comes clean, a
// scheduled deletion is restored, a replacement is put back. The exception is a
// plain scheduled add, which svn un-schedules and leaves on disk untracked. A
// copy or move destination is not one: svn created that file and takes it away
// with the add.
func revertedStatus(items []svn.StatusItem, done []string) ([]svn.StatusItem, bool) {
	return settledStatus(items, done, func(it svn.StatusItem) (svn.StatusItem, bool) {
		if it.State != svn.StateAdded || it.Copied {
			return svn.StatusItem{}, false
		}
		// Nothing else survives the add going: not the changelist it was staged
		// into, which svn only keeps for a versioned path.
		return svn.StatusItem{Path: it.Path, State: svn.StateUnversioned, PropState: svn.StateNone}, true
	})
}

// deletedStatus derives the status a finished delete left behind.
//
// A path svn still versions does not stop being reported: the row stays and
// turns deleted, keeping the revision and the changelist it was staged into,
// since svn goes on versioning a path it has only scheduled to remove. Property
// changes go with the content, so the row comes back with none.
//
// A path svn was not versioning is simply gone: an untracked file is removed
// from disk, and a scheduled add — plain, or the destination of a copy or move —
// is un-scheduled and taken away with it.
//
// A path already scheduled for deletion is left exactly as it was, down to a
// move link svn keeps: deleting it again is a silent no-op, not a second delete
// that would break the pairing.
func deletedStatus(items []svn.StatusItem, done []string) ([]svn.StatusItem, bool) {
	return settledStatus(items, done, func(it svn.StatusItem) (svn.StatusItem, bool) {
		switch it.State {
		case svn.StateAdded, svn.StateUnversioned, svn.StateIgnored:
			return svn.StatusItem{}, false
		case svn.StateDeleted:
			return it, true
		}
		return svn.StatusItem{
			Path:       it.Path,
			State:      svn.StateDeleted,
			PropState:  svn.StateNone,
			Revision:   it.Revision,
			Changelist: it.Changelist,
		}, true
	})
}

// settleRevert puts the Files views into the state svn has just reported the
// revert left the working copy in. It is not an optimistic update: every path it
// acts on is one svn has already confirmed it discarded, so there is nothing to
// undo and nothing to hold the poller for. The read that follows only confirms
// it, and no longer has the tree waiting on it.
func (m *Model) settleRevert(done []string) {
	m.settleStatus(revertedStatus(m.fileItems, done))
}

// settleDelete puts the Files views into the state svn has just reported the
// delete left the working copy in, on the same terms as settleRevert.
func (m *Model) settleDelete(done []string) {
	m.settleStatus(deletedStatus(m.fileItems, done))
}

// settleStatus adopts a derived status, rebuilding the Files views only when it
// differs from the one on screen.
func (m *Model) settleStatus(items []svn.StatusItem, changed bool) {
	if !changed {
		return
	}
	m.fileItems = items
	m.refreshFilesForStatus()
}
