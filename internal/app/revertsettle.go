package app

import (
	"github.com/bapatchirag/revision/internal/svn"
)

// revertedStatus rewrites the status items a finished revert has settled. svn
// has already said which paths it discarded, so what became of each is known
// without asking for the working copy again — which is the difference between a
// tree that changes as the toast appears and one that waits on a second svn
// process to say what the first one just did.
//
// Almost everything a revert touches stops being reported: a file comes clean, a
// scheduled deletion is restored, a replacement is put back. The exception is a
// plain scheduled add, which svn un-schedules and leaves on disk untracked. A
// copy or move destination is not one: svn created that file and takes it away
// with the add.
//
// It reports whether anything changed, so a revert that moved nothing on screen
// costs no rebuild.
func revertedStatus(items []svn.StatusItem, done []string) ([]svn.StatusItem, bool) {
	if len(done) == 0 {
		return items, false
	}
	var untracked []string
	for i := range items {
		if scopeCovers(done, items[i].Path) && items[i].State == svn.StateAdded && !items[i].Copied {
			untracked = append(untracked, items[i].Path)
		}
	}
	// svn does not look inside an untracked directory, so a tree that comes out
	// of a revert untracked is reported as the single row at its head.
	head := make(map[string]bool, len(untracked))
	for _, cv := range svn.CoverPaths(untracked) {
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
			// Nothing else survives the add going: not the changelist it was
			// staged into, which svn only keeps for a versioned path.
			out = append(out, svn.StatusItem{
				Path:      it.Path,
				State:     svn.StateUnversioned,
				PropState: svn.StateNone,
			})
		}
		changed = true
	}

	// A move reverted a half at a time leaves the half still standing with a link
	// to a file that is no longer scheduled.
	for i := range out {
		if scopeCovers(done, out[i].MovedFrom) || scopeCovers(done, out[i].MovedTo) {
			out[i].MovedFrom, out[i].MovedTo = "", ""
			changed = true
		}
	}
	return out, changed
}

// settleRevert puts the Files views into the state svn has just reported the
// revert left the working copy in. It is not an optimistic update: every path it
// acts on is one svn has already confirmed it discarded, so there is nothing to
// undo and nothing to hold the poller for. The read that follows only confirms
// it, and no longer has the tree waiting on it.
func (m *Model) settleRevert(done []string) {
	items, changed := revertedStatus(m.fileItems, done)
	if !changed {
		return
	}
	m.fileItems = items
	m.refreshFilesForStatus()
}
