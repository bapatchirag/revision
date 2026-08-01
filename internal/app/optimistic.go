package app

import (
	"github.com/bapatchirag/revision/internal/svn"
)

// mutation is one file's share of a change revision has asked svn for but has
// not yet had confirmed: the changelist the file should end up in ("" removes it
// from whichever one it was in), and whether it becomes versioned at the same
// time.
type mutation struct {
	path       string
	changelist string
	add        bool
}

// optimisticState is the working-copy status as svn last reported it, kept while
// a change revision has already shown is still in flight so a failure can put it
// back. One state covers every such change at once: they are applied on top of
// one another, so the only status certainly true is the one before the first.
type optimisticState struct {
	token uint64
	items []svn.StatusItem
}

// stageMutations turns the actions a stage keypress resolved to into the change
// they make to the status on screen: staging moves a file into changelist,
// unstaging removes it from whichever one it is in, and an unversioned file
// becomes added as it is staged.
func stageMutations(acts []stageAction, changelist string) []mutation {
	muts := make([]mutation, 0, len(acts))
	for _, a := range acts {
		cl := ""
		if a.stage {
			cl = changelist
		}
		muts = append(muts, mutation{path: a.path, changelist: cl, add: a.add})
	}
	return muts
}

// changelistMutations turns an assign-to-changelist action into the change it
// makes to the status on screen: every target moves into the named changelist,
// an unversioned one becoming added with it.
func changelistMutations(targets []changelistTarget, name string) []mutation {
	muts := make([]mutation, 0, len(targets))
	for _, t := range targets {
		muts = append(muts, mutation{path: t.path, changelist: name, add: t.add})
	}
	return muts
}

// applyOptimistic shows muts as though svn had already applied them, so the
// Files panel restyles on the keypress instead of a round trip later. It returns
// the token the command that follows must settle, or 0 when nothing was changed
// — optimistic updates are off, or the status already reads that way — in which
// case there is nothing to settle or undo.
func (m *Model) applyOptimistic(muts []mutation) uint64 {
	if !m.cfg.OptimisticUpdates || len(muts) == 0 {
		return 0
	}
	before := append([]svn.StatusItem(nil), m.fileItems...)
	if !applyMutations(m.fileItems, muts) {
		return 0
	}
	m.optimisticTok++
	// A change applied on top of one still in flight leaves the older snapshot
	// standing: that is the last status svn confirmed.
	if m.optimistic == nil {
		m.optimistic = &optimisticState{items: before}
	}
	m.optimistic.token = m.optimisticTok
	m.refreshFilesForStatus()
	return m.optimisticTok
}

// settleOptimistic closes out the change token stands for. A failure restores
// the status svn last reported; a success leaves the optimistic view standing
// for the reload that follows to confirm. A token the model no longer holds — a
// later status has since replaced the snapshot — settles as a no-op.
func (m *Model) settleOptimistic(token uint64, err error) {
	st := m.optimistic
	if token == 0 || st == nil || st.token != token {
		return
	}
	m.optimistic = nil
	if err == nil {
		return
	}
	m.fileItems = st.items
	m.refreshFilesForStatus()
}

// dropOptimistic forgets the snapshot, for when svn has just reported the real
// status: it now describes a working copy two steps back, and the change it
// covered either landed or will be settled by a reload.
func (m *Model) dropOptimistic() { m.optimistic = nil }

// applyMutations rewrites the status items the mutations name, reporting whether
// any of them changed anything. A path svn status no longer reports is skipped:
// there is no row on screen to move.
func applyMutations(items []svn.StatusItem, muts []mutation) bool {
	changed := false
	for _, mut := range muts {
		for i := range items {
			if items[i].Path != mut.path {
				continue
			}
			if mut.add && items[i].State == svn.StateUnversioned {
				items[i].State = svn.StateAdded
				changed = true
			}
			if items[i].Changelist != mut.changelist {
				items[i].Changelist = mut.changelist
				changed = true
			}
			break
		}
	}
	return changed
}

// refreshFilesForStatus re-renders the Files views from the status items after a
// change that did not come from a status reload. Main keeps the diff it is
// showing: staging a file changes where it is listed, not what it contains.
func (m *Model) refreshFilesForStatus() {
	m.rebuildFileTree()
	m.rebuildChangelists()
	m.syncDrill()
	m.refreshChrome()
}
