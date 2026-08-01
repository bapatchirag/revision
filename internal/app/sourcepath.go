package app

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/component"
)

// sourcePathID identifies the change-source-path prompt on emitted messages.
const sourcePathID = "source-path"

// sourcePathOptionLimit caps how many directories the prompt suggests at once,
// so a crowded directory cannot grow the overlay past the screen.
const sourcePathOptionLimit = 8

// openSourcePath shows the prompt that re-points revision at another directory
// inside the working copy. It opens on the directory revision is reading now,
// and cannot be taken above the working copy's root. The new source applies to
// the session only and is never persisted.
func (m *Model) openSourcePath() tea.Cmd {
	m.openSourcePathAt(m.defaultSourcePath())
	return nil
}

// openSourcePathAt shows the source-path prompt seeded with dir, locking the
// working copy's root at its head. It is also how a rejected path is handed back
// to the user, so the typo can be fixed in place rather than retyped.
func (m *Model) openSourcePathAt(dir string) {
	m.retargeting = true
	m.pathEditor.Reset()
	m.pathEditor.SetValue(dir)
	m.pathEditor.SetLocked(m.sourceFloor())
	m.refreshSourceOptions()
	m.pathEditor.Focus()
	m.sizeSourcePath()
}

// defaultSourcePath is where the prompt opens: the directory revision is reading
// now. The trailing separator makes the prompt list that directory's contents
// straight away.
func (m *Model) defaultSourcePath() string {
	dir := m.sourceFloor()
	if m.client != nil && m.client.Dir != "" {
		dir = filepath.Clean(m.client.Dir)
	}
	if dir == "" || dir == string(filepath.Separator) {
		return dir
	}
	return dir + string(filepath.Separator)
}

// sourceFloor is the directory the source path can never go above: the working
// copy's root, so the source can move anywhere inside the checkout but not out
// of it. It falls back to the current source while the root is unknown.
func (m *Model) sourceFloor() string {
	if m.info != nil && m.info.WorkingCopyRoot != "" {
		return filepath.Clean(m.info.WorkingCopyRoot)
	}
	if m.client != nil {
		return filepath.Clean(m.client.Dir)
	}
	return ""
}

// withinSource reports whether p is floor or lies beneath it.
func withinSource(floor, p string) bool {
	if floor == "" {
		return true
	}
	rel, err := filepath.Rel(floor, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// closeSourcePath hides the source-path prompt.
func (m *Model) closeSourcePath() {
	m.retargeting = false
	m.pathEditor.Blur()
}

// sizeSourcePath sizes the source-path prompt (only its width matters; the
// height follows the input and suggestion rows). It takes nearly the whole
// screen, up to a readable maximum, because it holds absolute paths that the
// other prompts' half-width box would cut short.
func (m *Model) sizeSourcePath() {
	m.pathEditor.SetSize(clamp(m.width-6, 40, 100), 0)
}

// refreshSourceOptions re-lists the directories under the path being typed, so
// the prompt doubles as a lightweight browser: tab into the list, pick with
// ↑/↓, then tab back and type "/" to descend.
func (m *Model) refreshSourceOptions() {
	opts, more := sourcePathOptions(m.pathEditor.Value(), m.sourceFloor())
	head := "Directories:"
	if more {
		head = "Directories (type to narrow):"
	}
	m.pathEditor.SetOptions(head, opts)
}

// submitSourcePath resolves the entered path and asks svn whether it is a
// working copy; the verdict arrives on sourceChangedMsg. Nothing is switched
// until it does, so a bad path leaves the session as it was.
func (m *Model) submitSourcePath(value string) tea.Cmd {
	if m.client == nil {
		return nil
	}
	dir := strings.TrimSpace(value)
	if dir == "" {
		m.showToast("source path cannot be empty", component.LevelWarning)
		return nil
	}
	dir = m.resolveSourcePath(dir)
	if floor := m.sourceFloor(); !withinSource(floor, dir) {
		// The prompt stays open so the path can be brought back inside the checkout.
		m.showToast("source path must stay inside "+floor, component.LevelWarning)
		return nil
	}
	m.closeSourcePath()
	if dir == m.client.Dir {
		m.showToast("already reading "+dir, component.LevelInfo)
		return nil
	}
	// Copy rather than build a fresh client so the command-log recorder and the
	// configured svn binary carry over to the candidate directory.
	next := *m.client
	next.Dir = dir
	m.loading = true
	m.refreshChrome()
	return probeSourceCmd(&next)
}

// applySourceChange re-roots the session on a probed source directory: it adopts
// the client and info read from there, drops every view derived from the old
// tree, and reloads status, history and the saved diffs. A directory that is not
// a working copy changes nothing and reopens the prompt on the attempted path.
func (m *Model) applySourceChange(msg sourceChangedMsg) tea.Cmd {
	if msg.err != nil {
		m.loading = false
		m.refreshChrome()
		m.showToast(failureText("read "+msg.client.Dir, msg.err), component.LevelError)
		if !m.overlayActive() {
			m.openSourcePathAt(msg.client.Dir)
		}
		return nil
	}
	m.client = msg.client
	m.info = msg.info
	// The new source becomes the launch directory, so the display scope decides
	// which directory it is shown from exactly as it does for -path at startup.
	m.launchDir = msg.client.Dir
	m.retargetDisplay(m.cfg.DisplayFrom)
	m.resetForSource()
	m.showToast("source path: "+m.client.Dir, component.LevelSuccess)
	return tea.Batch(m.beginInitialLoad(), loadSavedDiffsCmd(m.diffDir()))
}

// resetForSource clears everything the previous source directory produced — the
// file trees and their collapse state, any changelist drill, history, the diff
// on screen, the saved-diff browser and every panel filter — so nothing from the
// old working copy survives into the new one's views.
func (m *Model) resetForSource() {
	m.fileItems = nil
	m.clItems = nil
	m.collapsedDirs = map[string]bool{}
	m.clCollapsedDirs = map[string]bool{}
	m.filesInitialized = false
	if m.inChangelistDrill() {
		m.filesViews.Pop()
	}
	m.drilledCL = ""
	m.logEntries = nil
	m.logErr = nil
	m.logMore = false
	// The Log table holds its own copy of the rows, so it has to be emptied here
	// or the old tree's revisions stay on screen until the new page arrives.
	m.applyLogFilter()
	m.log.GoTop()
	m.wcRevision = ""
	if m.info != nil {
		m.wcRevision = m.info.Revision
	}
	m.diffPath, m.diffText, m.diffErr = "", "", false
	m.savedDiffItems, m.savedDiffsErr = nil, nil
	m.savedPath, m.savedText, m.savedErr = "", "", false
	for p := range m.filters {
		m.setFilter(p, "")
	}
	m.err = nil
	m.loading = true
	m.rebuildFilesViews()
	m.refreshChrome()
}

// resolveSourcePath turns an entered path into the absolute directory to probe:
// a leading ~ expands to the home directory, and a relative path is taken from
// the current source directory, which is what the prompt starts from.
func (m *Model) resolveSourcePath(p string) string {
	p = expandHome(p)
	if !filepath.IsAbs(p) && m.client != nil {
		p = filepath.Join(m.client.Dir, p)
	}
	return filepath.Clean(p)
}

// sourcePathOptions lists the directories the path being typed could name: the
// children of its directory part whose names start with its final segment, led
// by the parent directory when no partial name has been typed, so the list can
// be walked back up as well as down. The parent is withheld at floor, which the
// source may not go above. Hidden directories are offered only once a "." is
// typed. It reports whether the list was capped, which typing narrows.
func sourcePathOptions(value, floor string) ([]string, bool) {
	value = expandHome(strings.TrimSpace(value))
	if value == "" {
		return nil, false
	}
	dir, prefix := filepath.Split(value)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	var opts []string
	if prefix == "" {
		if parent := filepath.Dir(filepath.Clean(dir)); parent != filepath.Clean(dir) && withinSource(floor, parent) {
			opts = append(opts, parent)
		}
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if len(opts) == sourcePathOptionLimit {
			return opts, true
		}
		opts = append(opts, filepath.Join(dir, e.Name()))
	}
	return opts, false
}

// expandHome replaces a leading ~ with the user's home directory, leaving the
// path as written when the home directory cannot be determined.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~"+string(filepath.Separator)) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}
