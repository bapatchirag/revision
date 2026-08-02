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
