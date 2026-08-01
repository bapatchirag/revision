package app

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
)

// pendingMarker tails a row whose action svn has been asked for but has not yet
// answered.
const pendingMarker = "…"

// pendingKind names the destructive action a row is waiting on. Unlike a stage,
// which revision shows as done and puts back if svn disagrees, none of these is
// ever shown as having succeeded before it has: the row is marked as in flight
// and settles on the real result.
type pendingKind int

const (
	pendingRevert pendingKind = iota + 1
	pendingDelete
	pendingCommit
)

// pendingOp records that svn has been asked to revert, delete or commit a path
// and has not answered yet. token ties the mark to the reply that settles it, so
// one action's result never clears another's rows.
type pendingOp struct {
	kind  pendingKind
	token uint64
}

// pendingHold is what an open confirmation prompt will mark once it is accepted,
// kept aside while the modal asks so a cancelled prompt marks nothing.
type pendingHold struct {
	kind  pendingKind
	token uint64
	paths []string
}

// holdPending describes the rows a confirmed prompt should mark, or nil when
// there is nothing to mark: optimistic updates are off (token 0) or the action
// touches no rows.
func holdPending(kind pendingKind, token uint64, paths ...string) *pendingHold {
	if token == 0 || len(paths) == 0 {
		return nil
	}
	return &pendingHold{kind: kind, token: token, paths: paths}
}

// nextPendingToken mints the token a destructive action's reply carries back, so
// its rows can be found again. It returns 0 when optimistic updates are off, in
// which case nothing is marked and the screen waits for svn as it always did.
func (m *Model) nextPendingToken() uint64 {
	if !m.cfg.OptimisticUpdates {
		return 0
	}
	m.pendingTok++
	return m.pendingTok
}

// confirmAction stages the command a confirmation prompt runs when accepted,
// together with the rows that acceptance puts in flight.
func (m *Model) confirmAction(cmd tea.Cmd, hold *pendingHold) {
	m.pending = cmd
	m.pendingHold = hold
}

// markHeldPending marks the rows the accepted prompt was holding.
func (m *Model) markHeldPending() {
	h := m.pendingHold
	m.pendingHold = nil
	if h != nil {
		m.markPending(h.kind, h.token, h.paths)
	}
}

// markPending shows paths as waiting on svn, so the keypress that started the
// action changes the screen without claiming the action has finished.
func (m *Model) markPending(kind pendingKind, token uint64, paths []string) {
	if token == 0 || len(paths) == 0 {
		return
	}
	if m.pendingOps == nil {
		m.pendingOps = map[string]pendingOp{}
	}
	for _, p := range paths {
		m.pendingOps[p] = pendingOp{kind: kind, token: token}
	}
	m.refreshFilesForStatus()
}

// clearPending drops the marks token stands for, whether its action succeeded or
// failed: either way those rows are no longer waiting on svn.
func (m *Model) clearPending(token uint64) {
	if token == 0 {
		return
	}
	cleared := false
	for p, op := range m.pendingOps {
		if op.token == token {
			delete(m.pendingOps, p)
			cleared = true
		}
	}
	if cleared {
		m.refreshFilesForStatus()
	}
}

// isPending reports whether path is waiting on an svn action.
func (m *Model) isPending(path string) bool {
	_, ok := m.pendingOps[path]
	return ok
}

// pendingCount reports how many files a tree row covers that are waiting on svn:
// one or none for a file leaf, the number beneath a directory row.
func (m *Model) pendingCount(n fileNode) int {
	if len(m.pendingOps) == 0 {
		return 0
	}
	if n.Item != nil {
		if m.isPending(n.Path) {
			return 1
		}
		return 0
	}
	if n.Path == fileTreeRoot {
		return len(m.pendingOps)
	}
	prefix := n.Path + "/"
	count := 0
	for p := range m.pendingOps {
		if strings.HasPrefix(p, prefix) {
			count++
		}
	}
	return count
}

// pendingLabel is the suffix a directory row carries while files beneath it are
// in flight.
func pendingLabel(count int) string { return " " + strconv.Itoa(count) + pendingMarker }

// commitPending reports whether a commit is still in flight, which holds the
// commit key until it settles.
func (m *Model) commitPending() bool {
	for _, op := range m.pendingOps {
		if op.kind == pendingCommit {
			return true
		}
	}
	return false
}

// selectionPending reports whether the highlighted file row is already waiting
// on svn, in which case the keys that would act on it do nothing.
func (m *Model) selectionPending() bool {
	it, ok := m.selectedFile()
	return ok && m.isPending(it.Path)
}

// withoutPending drops the items already waiting on svn, so a directory-level
// action fans out only over the rows that are free to move.
func (m *Model) withoutPending(items []svn.StatusItem) []svn.StatusItem {
	if len(m.pendingOps) == 0 {
		return items
	}
	out := make([]svn.StatusItem, 0, len(items))
	for _, it := range items {
		if !m.isPending(it.Path) {
			out = append(out, it)
		}
	}
	return out
}
