package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
)

// stageSelected acts on the current Files-panel selection: on a directory row it
// toggles staging for every file beneath that directory (stage all, then unstage
// all once everything stageable is staged), and on a file leaf it toggles that
// file's staged state (the Changes view or a drilled-in changelist). It returns
// the command that performs the change (or nil when the selection is not
// stageable).
func (m *Model) stageSelected() tea.Cmd {
	if n, items, ok := m.selectedDirectory(); ok {
		return m.stageDirectory(n, m.withoutPending(items))
	}
	if m.selectionPending() {
		return nil
	}
	act, ok := m.stageTarget()
	if !ok {
		if it, sel := m.selectedFile(); sel {
			m.showToast("can't stage "+it.Path+" ("+it.State.Code()+")", component.LevelWarning)
		}
		return nil
	}
	acts := []stageAction{act}
	return stageCmd(m.client, stagedChangelist, act, m.applyOptimistic(stageMutations(acts, stagedChangelist)))
}

// stageDirectory toggles staging for the files beneath the selected directory
// row. While any file can still be staged it stages them all (adding unversioned
// files first); once nothing is left to stage, a second press removes every file
// under the directory from whatever changelist it belongs to — the staged bucket
// or a named one — mirroring how space clears a single file's changelist. A
// directory holding only clean or ignored files has nothing to do and warns
// instead of running svn.
func (m *Model) stageDirectory(n fileNode, items []svn.StatusItem) tea.Cmd {
	acts := directoryStageActions(n, items)
	if len(acts) == 0 {
		acts = directoryUnstageActions(n, items)
	}
	if len(acts) == 0 {
		m.showToast("nothing to stage or unstage under "+dirLabel(n), component.LevelWarning)
		return nil
	}
	return stageManyCmd(m.client, stagedChangelist, acts, m.applyOptimistic(stageMutations(acts, stagedChangelist)))
}

// directoryStageActions builds the stage actions that stage every stageable file
// beneath a directory row: an unversioned file is added and staged, an
// unassigned pending change is staged, and a file already staged or in a named
// changelist is left as it is. items is the tree's source set.
func directoryStageActions(n fileNode, items []svn.StatusItem) []stageAction {
	var acts []stageAction
	for _, it := range filesUnder(n, items) {
		switch {
		case it.State == svn.StateUnversioned:
			acts = append(acts, stageAction{path: it.Path, add: true, stage: true})
		case it.Changelist == "" && stageable(it.State):
			acts = append(acts, stageAction{path: it.Path, stage: true})
		}
	}
	return acts
}

// directoryUnstageActions builds the actions that remove every file beneath a
// directory row from its changelist — the staged bucket or a named changelist
// alike — so a directory-level toggle clears assignments the same way space does
// for a single file. Files in no changelist have nothing to remove. items is the
// tree's source set.
func directoryUnstageActions(n fileNode, items []svn.StatusItem) []stageAction {
	var acts []stageAction
	for _, it := range filesUnder(n, items) {
		if it.Changelist != "" {
			acts = append(acts, stageAction{path: it.Path, stage: false})
		}
	}
	return acts
}

// stageAction describes how a stage keypress should change one file.
type stageAction struct {
	path  string
	add   bool // svn add first (unversioned → versioned)
	stage bool // add to (true) or remove from (false) a changelist
}

// stageTarget resolves what a stage action would do for the current file
// selection. An unversioned file is added and staged in one step; a file already
// in any changelist (the anonymous staged bucket or a named list) is removed from
// it — space never moves a file between changelists, enforcing one-changelist-
// per-file; an unassigned pending change is added to the staged bucket. It
// returns ok=false when there is no file selected or it cannot be staged.
func (m *Model) stageTarget() (stageAction, bool) {
	it, ok := m.selectedFile()
	if !ok {
		return stageAction{}, false
	}
	switch {
	case it.State == svn.StateUnversioned:
		return stageAction{path: it.Path, add: true, stage: true}, true
	case it.Changelist != "":
		return stageAction{path: it.Path, stage: false}, true
	case stageable(it.State):
		return stageAction{path: it.Path, stage: true}, true
	default:
		return stageAction{}, false
	}
}

// stageable reports whether a working-copy state can be added to the staged
// changelist as-is. Only versioned, pending changes qualify. Unversioned files
// are handled separately by stageTarget (svn add + stage); ignored and missing
// paths are excluded (missing needs `svn rm` first).
func stageable(s svn.FileState) bool {
	switch s {
	case svn.StateModified, svn.StateAdded, svn.StateDeleted, svn.StateReplaced, svn.StateConflicted, svn.StateMerged:
		return true
	default:
		return false
	}
}
