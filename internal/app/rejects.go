package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/shelf"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// rejectsViewName is the Files-panel view listing the rejects a patch left
// behind: the hunks svn could not place, written out beside the file they were
// meant for.
const rejectsViewName = "Rejects"

// rejectsListID identifies the rejects tree on emitted selection messages.
const rejectsListID = "rejects"

// rejectFile is one reject left in the working copy by an applied patch. svn
// writes these as <target>.svnpatch.rej and marks them ignored, so they never
// appear in the Changes view; Rel is the path below the scan root, which is what
// the tree is built from.
type rejectFile struct {
	Name string
	Rel  string
	Path string
}

// rejectNode is one visible row in the Rejects view's tree: a directory (Item ==
// nil) or a reject leaf.
type rejectNode = pathRow[rejectFile]

// isRejectFile reports whether a file name is a reject — the suffix both svn's
// own patch code and GNU patch give the hunks they could not apply.
func isRejectFile(name string) bool { return strings.HasSuffix(name, ".rej") }

// scanRejects walks dir for reject files, in path order so they flatten into a
// tree the way svn's own path-sorted status does. Unlike the saved-diff store
// the search is recursive: a reject lands beside the file it failed to patch, so
// it can be anywhere beneath the root. The administrative .svn directories are
// skipped, and an entry that cannot be read is passed over rather than failing
// the scan — the rejects elsewhere are still worth listing. A root that does not
// exist simply holds no rejects, which is not an error.
func scanRejects(dir string) ([]rejectFile, error) {
	if dir == "" {
		return nil, nil
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []rejectFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".svn" || d.Name() == shelf.DirName {
				return fs.SkipDir
			}
			return nil
		}
		if !isRejectFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = path
		}
		out = append(out, rejectFile{
			Name: d.Name(),
			Rel:  filepath.ToSlash(rel),
			Path: path,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

// renderRejectNode adapts a tree row for the reusable List, matching the Changes
// tree row for row: directory rows show a chevron and the segment name with a
// trailing slash, and leaves keep the same blank changelist slot, status code
// and file name. The code is `!` — the hunks in a reject never landed.
func renderRejectNode(th theme.Theme) func(rejectNode) string {
	return func(n rejectNode) string {
		indent := strings.Repeat("  ", n.Depth)
		if n.Item == nil {
			chevron := "▾"
			if n.Collapsed {
				chevron = "▸"
			}
			marker := lipgloss.NewStyle().Foreground(th.Muted).Render(chevron)
			name := lipgloss.NewStyle().Foreground(th.Info).Bold(true).Render(dirLabel(n))
			return indent + marker + " " + name
		}
		code := lipgloss.NewStyle().Foreground(th.Warning).Bold(true).Render("!")
		return indent + "  " + code + " " + n.Name
	}
}

// rejectPath is the key a reject is placed in the tree by.
func rejectPath(r rejectFile) string { return r.Rel }

// rebuildRejects re-flattens the scanned rejects into the view's tree, narrowed
// by the free text of the Files-panel filter and honoring the remembered
// per-directory collapse state. The panel's key:value parameters describe
// working-copy state, which an ignored reject has none of, so only the free text
// applies here — matched on the path below the root, since where a reject sits
// is what identifies it. The cursor is put back on the row it was on by path;
// the first scan, which had no cursor to keep, opens on the first reject.
func (m *Model) rebuildRejects() {
	path := selectedNodePath(m.rejects)
	m.rejects.SetItems(buildPathTree(m.filteredRejects(), rejectPath, m.rejectCollapsed))
	if path == "" {
		if idx := firstFileIndex(m.rejects.Items()); idx >= 0 {
			m.rejects.SetIndex(idx)
		}
		return
	}
	selectNodePath(m.rejects, path)
}

// filteredRejects returns the rejects the view should show: the ones matching
// the free text of the Files-panel filter.
func (m *Model) filteredRejects() []rejectFile {
	q := m.filesQuery().text
	if q == "" {
		return m.rejectItems
	}
	out := make([]rejectFile, 0, len(m.rejectItems))
	for _, r := range m.rejectItems {
		if containsFold(r.Rel, q) {
			out = append(out, r)
		}
	}
	return out
}

// selectedReject returns the reject under the Rejects-view cursor, or ok=false
// when the cursor rests on a directory row (or the view holds nothing).
func (m *Model) selectedReject() (rejectFile, bool) {
	if n, ok := m.rejects.Selected(); ok && n.Item != nil {
		return *n.Item, true
	}
	return rejectFile{}, false
}

// toggleRejectCollapse expands or collapses the directory under the Rejects-view
// cursor and rebuilds the tree. It is inert on a reject leaf.
func (m *Model) toggleRejectCollapse() tea.Cmd {
	n, ok := m.rejects.Selected()
	if !ok || n.Item != nil {
		return nil
	}
	if m.rejectCollapsed[n.Path] {
		delete(m.rejectCollapsed, n.Path)
	} else {
		m.rejectCollapsed[n.Path] = true
	}
	m.rebuildRejects()
	m.updateMain()
	return nil
}

// rejectLoadForSelection returns a command to read the highlighted reject when
// its contents are not already the ones on screen. A directory row has no file
// to read — Main summarizes what is beneath it instead.
func (m *Model) rejectLoadForSelection() tea.Cmd {
	r, ok := m.selectedReject()
	if !ok || m.rejectPath == r.Path {
		return nil
	}
	return m.readReject(r.Path)
}

// clearRejects empties the tree and the contents it put in Main, for when the
// directory they were found in is no longer the one being shown.
func (m *Model) clearRejects() {
	m.rejectItems, m.rejectsErr = nil, nil
	m.rejectPath, m.rejectText, m.rejectErr = "", "", false
	m.rejectCollapsed = map[string]bool{}
	m.rebuildRejects()
}

// requestDeleteReject asks to remove the highlighted reject from disk, opening a
// confirmation modal. Like the Diffs view's delete this is a plain file removal:
// svn ignores rejects, so there is nothing scheduled and nothing that can put
// the file back. A directory row names no single file, so it warns instead.
func (m *Model) requestDeleteReject() tea.Cmd {
	r, ok := m.selectedReject()
	if !ok {
		if n, sel := m.rejects.Selected(); sel {
			m.showToast("select a reject under "+dirLabel(n)+" to delete", component.LevelWarning)
			return nil
		}
		m.showToast("no reject to delete", component.LevelWarning)
		return nil
	}
	m.confirmAction(deleteRejectCmd(r.Path, r.Rel), nil)
	m.openConfirm("Delete reject?",
		"Permanently delete "+r.Rel+"? The hunks it holds have not been applied, "+
			"and this cannot be undone.")
	return nil
}

// rejectDetail renders the highlighted reject in Main, with a placeholder while
// it is being read, when nothing was rejected, or when the tree could not be
// walked. A directory row lists what sits beneath it instead.
func (m *Model) rejectDetail() string {
	if m.rejectsErr != nil {
		return "Unable to list rejects: " + m.rejectsErr.Error()
	}
	n, ok := m.rejects.Selected()
	if !ok {
		if len(m.rejectItems) > 0 {
			return "No rejects match the filter."
		}
		return "No rejects under " + m.patchRoot()
	}
	if n.Item == nil {
		return rejectDirDetail(n, m.rejects.Items())
	}
	switch {
	case m.rejectPath != n.Item.Path:
		return "Reading " + n.Item.Rel + "…"
	case m.rejectErr:
		return m.rejectText
	case strings.TrimSpace(m.rejectText) == "":
		return "(" + n.Item.Rel + " is empty)"
	default:
		return m.colorize(m.rejectText)
	}
}

// rejectDirDetail summarizes a directory row: how many rejects sit beneath it
// and where, since a directory has no contents of its own to show. A collapsed
// directory hides its rows, so the count follows what is on screen.
func rejectDirDetail(n rejectNode, rows []rejectNode) string {
	var paths []string
	for _, row := range rows {
		if row.Item == nil {
			continue
		}
		if n.Path == fileTreeRoot || strings.HasPrefix(row.Item.Rel, n.Path+"/") {
			paths = append(paths, row.Item.Rel)
		}
	}
	label := dirLabel(n)
	if len(paths) == 0 {
		return "No rejects under " + label
	}
	lines := []string{fmt.Sprintf("%d reject(s) under %s", len(paths), label), ""}
	for _, p := range paths {
		lines = append(lines, "  "+p)
	}
	return strings.Join(lines, "\n")
}
