package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// savedDiffsViewName is the Files-panel view listing the patch files already
// written to the configured diff output directory.
const savedDiffsViewName = "Diffs"

// savedDiffsListID identifies the saved-diffs list on emitted selection messages.
const savedDiffsListID = "saved-diffs"

// savedDiff is one patch file in the configured diff output directory: the store
// that `w` writes into, browsed by the Files panel's Diffs view.
type savedDiff struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

// isDiffFile reports whether a file name is one of the patch files the Diffs
// view lists — the suffixes diffSaveName produces.
func isDiffFile(name string) bool {
	return strings.HasSuffix(name, ".diff") || strings.HasSuffix(name, ".patch")
}

// scanSavedDiffs lists the patch files directly inside dir, newest first.
// Sub-directories are skipped: saved diffs are always written flat into the
// output directory. A directory that does not exist yet simply holds no diffs,
// which is not an error — nothing has been saved there.
func scanSavedDiffs(dir string) ([]savedDiff, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	diffs := make([]savedDiff, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !isDiffFile(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		diffs = append(diffs, savedDiff{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].ModTime.Equal(diffs[j].ModTime) {
			return diffs[i].Name < diffs[j].Name
		}
		return diffs[i].ModTime.After(diffs[j].ModTime)
	})
	return diffs, nil
}

// renderSavedDiff is the domain adapter that turns a saved patch file into the
// row the reusable List renders: a marker, the file name, and its size and save
// time in muted text.
func renderSavedDiff(th theme.Theme) func(savedDiff) string {
	return func(d savedDiff) string {
		marker := lipgloss.NewStyle().Foreground(th.Info).Bold(true).Render("◆")
		name := lipgloss.NewStyle().Foreground(th.Text).Render(d.Name)
		meta := lipgloss.NewStyle().Foreground(th.Muted).
			Render(fmt.Sprintf(" (%s · %s)", humanSize(d.Size), d.ModTime.Format("2006-01-02 15:04")))
		return marker + " " + name + meta
	}
}

// humanSize renders a byte count in the largest unit that keeps it under four
// digits, so a row's size annotation stays narrow.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGT"[exp])
}

// rebuildSavedDiffs repopulates the Diffs view from the scanned patch files,
// narrowed by the free text of the Files-panel filter. The panel's key:value
// parameters describe working-copy state, which a saved file has none of, so
// only the free text applies here.
func (m *Model) rebuildSavedDiffs() {
	q := m.filesQuery().text
	if q == "" {
		m.savedDiffs.SetItems(m.savedDiffItems)
		return
	}
	out := make([]savedDiff, 0, len(m.savedDiffItems))
	for _, d := range m.savedDiffItems {
		if containsFold(d.Name, q) {
			out = append(out, d)
		}
	}
	m.savedDiffs.SetItems(out)
}

// savedDiffLoadForSelection returns a command to read the highlighted saved
// patch file when its contents are not already the ones on screen.
func (m *Model) savedDiffLoadForSelection() tea.Cmd {
	d, ok := m.savedDiffs.Selected()
	if !ok || m.savedPath == d.Path {
		return nil
	}
	return m.readSavedDiff(d.Path)
}

// requestDeleteSavedDiff asks to remove the highlighted patch file from the diff
// output directory, opening a confirmation modal. This is the Diffs view's
// answer to the Changes tree's delete: the store is files on disk, so nothing is
// scheduled and nothing can put the file back.
func (m *Model) requestDeleteSavedDiff() tea.Cmd {
	d, ok := m.savedDiffs.Selected()
	if !ok {
		m.showToast("no saved diff to delete", component.LevelWarning)
		return nil
	}
	m.confirmAction(deleteSavedDiffCmd(d.Path, d.Name), nil)
	m.openConfirm("Delete saved diff?",
		"Permanently delete "+d.Name+" from "+m.diffDir()+"? This cannot be undone.")
	return nil
}

// requestApplyPatch asks to apply the highlighted patch file to the working
// copy, opening a confirmation modal. Nothing is held pending: a patch lands on
// files spread across the tree, most of which the Changes view is not even
// showing, so the status reload that follows is what says where it went.
func (m *Model) requestApplyPatch() tea.Cmd {
	d, ok := m.savedDiffs.Selected()
	if !ok {
		m.showToast("no saved diff to apply", component.LevelWarning)
		return nil
	}
	root := m.patchRoot()
	m.confirmAction(applyPatchCmd(m.client, d.Path, d.Name, root), nil)
	m.openConfirm("Apply patch?",
		"Apply "+d.Name+" to "+root+"? Its changes are merged into the working copy as local "+
			"modifications, and any hunk that does not fit is left beside its file in a .rej.")
	return nil
}

// patchRoot is the directory a patch is applied in: the one the svn client is
// rooted at, which is the source path revision is showing. Note that this need
// not be the directory saved diffs are written to (diffDir), which is the
// working copy's root.
func (m *Model) patchRoot() string {
	if m.client != nil && m.client.Dir != "" {
		return m.client.Dir
	}
	return m.workDir
}

// patchToast describes a finished patch run: the files svn changed, and the ones
// it could not place. A target left in conflict took the hunks that fit and has
// the rest written out beside it in a .svnpatch.rej file, which svn ignores and
// so never shows in the Files panel — leaving this the only announcement of it.
func patchToast(name string, res svn.PatchResult) (string, component.Level) {
	text := "applied " + name + " to " + fileCount(len(res.Applied))
	var left []string
	if n := len(res.Conflicted); n > 0 {
		left = append(left, fmt.Sprintf("%d with rejects (.rej)", n))
	}
	if n := len(res.Skipped); n > 0 {
		left = append(left, fmt.Sprintf("%d not found", n))
	}
	if len(left) == 0 {
		return text, component.LevelSuccess
	}
	return text + ", " + strings.Join(left, ", "), component.LevelWarning
}

// fileCount renders a file count for a toast, with the right plural.
func fileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

// savedDiffDetail renders the highlighted saved patch file in Main, with a
// placeholder while it is being read, when the store holds nothing, or when it
// could not be listed.
func (m *Model) savedDiffDetail() string {
	if m.savedDiffsErr != nil {
		return "Unable to list saved diffs: " + m.savedDiffsErr.Error()
	}
	d, ok := m.savedDiffs.Selected()
	if !ok {
		if len(m.savedDiffItems) > 0 {
			return "No saved diffs match the filter."
		}
		return "No saved diffs in " + m.diffDir() + " — press " +
			m.keys.SaveDiff.Help().Key + " to write one."
	}
	switch {
	case m.savedPath != d.Path:
		return "Reading " + d.Name + "…"
	case m.savedErr:
		return m.savedText
	case strings.TrimSpace(m.savedText) == "":
		return "(" + d.Name + " is empty)"
	default:
		return m.colorize(m.savedText)
	}
}
