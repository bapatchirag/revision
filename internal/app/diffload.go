package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/component"
)

// reloadStatus, loadDiff, reloadSavedDiffs and readSavedDiff issue the loads
// whose replies can be overtaken by a later request. Each abandons the request
// still in flight and stamps the new one, so only the reply the model is still
// waiting for is applied.
func (m *Model) reloadStatus() tea.Cmd {
	ctx, gen := m.gens.status.begin(loadTimeout)
	return loadStatusCmd(ctx, m.client, gen)
}

// loadDiff puts the diff for k on screen. One the session already holds for the
// working copy's current state is applied on the spot and costs no command; a
// miss is debounced, so passing over a row does not spawn an svn process for it.
func (m *Model) loadDiff(k diffKey) tea.Cmd {
	if e, ok := m.session.Diff(k, m.diffStamp(k)); ok {
		// Whatever is in flight was for the row just left; its reply must not
		// land on top of this one.
		m.gens.diff.next()
		m.applyDiff(k, e)
		if m.source == sourceFiles {
			m.updateMain()
		}
		return nil
	}
	gen := m.gens.diff.next()
	return tea.Tick(diffDebounce, func(time.Time) tea.Msg {
		return diffPendingMsg{key: k, gen: gen}
	})
}

// diffStamp fingerprints the working-copy state a diff of k would be produced
// from, as the session store's key to whether a cached diff is still good.
func (m *Model) diffStamp(k diffKey) string {
	root := ""
	if m.client != nil {
		root = m.client.Dir
	}
	return diffStampFor(root, m.wcRevision, m.fileItems, k)
}

// applyDiff puts a loaded (or cached) diff on screen, recording what it was
// produced for so a later refresh can be told from a move to another selection.
func (m *Model) applyDiff(k diffKey, e diffEntry) {
	m.diffPath, m.diffOfDir = k.path, k.dir
	m.diffText, m.diffErr = e.text, e.failed
}

// clearDiff drops the diff on screen, so Main falls back to its placeholder
// until a fresh one lands.
func (m *Model) clearDiff() {
	m.diffPath, m.diffText, m.diffErr, m.diffOfDir = "", "", false, false
}

// rederiveDiff re-reads the diff on screen from the session once the status
// behind it has moved: one the session still stands behind stays put, and one it
// dropped clears, so the load that follows fetches it afresh.
func (m *Model) rederiveDiff() {
	if m.diffPath == "" {
		return
	}
	k := diffKey{path: m.diffPath, dir: m.diffOfDir}
	if e, ok := m.session.Diff(k, m.diffStamp(k)); ok {
		m.applyDiff(k, e)
		return
	}
	m.clearDiff()
}

func (m *Model) reloadSavedDiffs() tea.Cmd {
	return loadSavedDiffsCmd(m.diffDir(), m.gens.saved.next())
}

// reloadSavedDiffsIfShown re-scans the patch files only while the Diffs view is
// the one on screen. Anywhere else the scan is deferred: that view re-scans
// whenever it is opened, so the disk is never read for a list nobody is looking
// at.
func (m *Model) reloadSavedDiffsIfShown() tea.Cmd {
	if !m.filesViewIsDiffs() {
		return nil
	}
	return m.reloadSavedDiffs()
}

func (m *Model) readSavedDiff(path string) tea.Cmd {
	return readSavedDiffCmd(path, m.gens.diff.next())
}

// reloadRejects re-walks the source path for the .rej files a patch left behind.
// They are ignored by svn and so are invisible to the Changes view; the disk is
// the only place they can be found.
func (m *Model) reloadRejects() tea.Cmd {
	return loadRejectsCmd(m.patchRoot(), m.gens.reject.next())
}

// reloadRejectsIfShown re-walks for rejects only while the Rejects view is the
// one on screen. Anywhere else the walk is deferred: that view re-scans whenever
// it is opened, so a whole working copy is never read for a list nobody is
// looking at.
func (m *Model) reloadRejectsIfShown() tea.Cmd {
	if !m.filesViewIsRejects() {
		return nil
	}
	return m.reloadRejects()
}

func (m *Model) readReject(path string) tea.Cmd {
	return readRejectCmd(path, m.gens.diff.next())
}

// diffLoadForSelection returns a command to load the diff that Main should show
// for the current Files selection when it is not already loaded. In the Diffs
// view that is the highlighted saved patch file and in the Rejects view the
// highlighted reject; otherwise a directory row loads the combined diff of every
// change beneath it (the "/" root covers the whole working copy) and a file leaf
// loads its own diff when it is dirty.
func (m *Model) diffLoadForSelection() tea.Cmd {
	if m.filesViewIsDiffs() {
		return m.savedDiffLoadForSelection()
	}
	if m.filesViewIsRejects() {
		return m.rejectLoadForSelection()
	}
	k, ok := m.diffSelection()
	if !ok || m.diffPath == k.path {
		// There is nothing to fetch, so a load still in flight is for a row that
		// has been left; its reply must not land on this one.
		m.gens.diff.next()
		return nil
	}
	return m.loadDiff(k)
}

// diffSelection returns the diff the Files selection calls for, or ok=false when
// it calls for none: no selection, a file with no textual diff, or a directory
// row while directory diffs are off.
func (m *Model) diffSelection() (diffKey, bool) {
	if n, _, ok := m.selectedTreeNode(); ok && n.Item == nil {
		if !m.dirDiff {
			return diffKey{}, false
		}
		return diffKey{path: n.Path, dir: true}, true
	}
	it, ok := m.selectedFile()
	if !ok || !it.State.IsDirty() {
		return diffKey{}, false
	}
	return diffKey{path: it.Path}, true
}

// toggleDirDiff flips whether directory rows show their combined diff. It lets a
// working copy that disables directory diffs globally (config) reveal one on
// demand, and hide it again. It reports the new state with a toast and, when Main
// follows the Files panel, refreshes it — loading the diff if it now needs one.
func (m *Model) toggleDirDiff() tea.Cmd {
	m.dirDiff = !m.dirDiff
	if m.dirDiff {
		m.showToast("directory diff on", component.LevelInfo)
	} else {
		m.showToast("directory diff off", component.LevelInfo)
	}
	if m.source != sourceFiles {
		return nil
	}
	m.updateMain()
	return m.diffLoadForSelection()
}

// toggleUntracked flips whether untracked (unversioned) files are hidden from the
// Changes and diff views. It lets a working copy that shows untracked files
// globally hide the noise on demand, and reveal it again. It rebuilds the Files
// views so the change takes effect immediately, reports the new state with a
// toast and, when Main follows the Files panel, refreshes it — the cursor may
// have moved onto a different file as rows appeared or disappeared.
func (m *Model) toggleUntracked() tea.Cmd {
	m.hideUntracked = !m.hideUntracked
	if m.hideUntracked {
		m.showToast("untracked files hidden", component.LevelInfo)
	} else {
		m.showToast("untracked files shown", component.LevelInfo)
	}
	m.rebuildFilesViews()
	if m.source != sourceFiles {
		return nil
	}
	m.updateMain()
	return m.diffLoadForSelection()
}
