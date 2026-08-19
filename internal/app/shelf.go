package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/shelf"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// shelfListID identifies the shelf list on emitted selection messages.
const shelfListID = "shelf"

// shelfDir is where this working copy's shelved changes are kept. It is fixed at
// the working copy's root and deliberately not configurable: what keeps the
// store out of svn's way is its name matching Subversion's built-in ignore
// glob, which a directory somewhere else would not.
func (m *Model) shelfDir() string {
	if m.info != nil && m.info.WorkingCopyRoot != "" {
		return shelf.Dir(m.info.WorkingCopyRoot)
	}
	if m.client != nil && m.client.Dir != "" {
		return shelf.Dir(m.client.Dir)
	}
	return shelf.Dir(m.workDir)
}

// shelfLabel is what an entry is listed under: the name it was shelved with, or
// its identifier when it was shelved without one.
func shelfLabel(e shelf.Entry) string {
	if name := strings.TrimSpace(e.Name); name != "" {
		return name
	}
	return e.ID
}

// shelfSize is how many files an entry holds, counting the unversioned ones it
// carries as bytes alongside the ones its patch describes.
func shelfSize(e shelf.Entry) int { return len(e.Files) + len(e.Untracked) }

// renderShelfEntry is the domain adapter that turns a shelved change set into
// the row the reusable List renders: a marker, what it was shelved as, and how
// much it holds and when it was taken in muted text.
func renderShelfEntry(th theme.Theme) func(shelf.Entry) string {
	return func(e shelf.Entry) string {
		marker := lipgloss.NewStyle().Foreground(th.Success).Bold(true).Render("▣")
		name := lipgloss.NewStyle().Foreground(th.Text).Render(shelfLabel(e))
		meta := lipgloss.NewStyle().Foreground(th.Muted).
			Render(fmt.Sprintf(" (%s · %s)", fileCount(shelfSize(e)), e.Created.Format("2006-01-02 15:04")))
		return marker + " " + name + meta
	}
}

// rebuildShelves repopulates the panel from the scanned entries, narrowed by its
// filter. An entry describes changes rather than working-copy state, so the
// filter is plain free text, matched on what the entry is listed under.
func (m *Model) rebuildShelves() {
	q := strings.TrimSpace(m.filters[panelShelf])
	if q == "" {
		m.shelves.SetItems(m.shelfItems)
		return
	}
	out := make([]shelf.Entry, 0, len(m.shelfItems))
	for _, e := range m.shelfItems {
		if containsFold(shelfLabel(e), q) {
			out = append(out, e)
		}
	}
	m.shelves.SetItems(out)
}

// shelfLoadForSelection returns a command to read the highlighted entry's patch
// when it is not already the one on screen.
func (m *Model) shelfLoadForSelection() tea.Cmd {
	e, ok := m.shelves.Selected()
	if !ok || m.shelfID == e.ID {
		return nil
	}
	return m.readShelf(e.ID)
}

// shelfFooter returns the position/count indicator inlaid into the Shelf panel's
// bottom border, with the unfiltered total in brackets when a filter hides some.
func (m *Model) shelfFooter() string {
	return countLabel(m.shelves.Index()+1, len(m.shelves.Items()), len(m.shelfItems))
}

// shelfDetail renders the highlighted entry in Main: the patch it holds, with a
// placeholder while it is being read, when the store is empty, or when it could
// not be listed.
func (m *Model) shelfDetail() string {
	if m.shelfErr != nil {
		return "Unable to list shelves: " + m.shelfErr.Error()
	}
	e, ok := m.shelves.Selected()
	if !ok {
		if len(m.shelfItems) > 0 {
			return "No shelves match the filter."
		}
		return "Nothing shelved yet."
	}
	switch {
	case m.shelfID != e.ID:
		return "Reading " + shelfLabel(e) + "…"
	case m.shelfReadErr:
		return m.shelfText
	case strings.TrimSpace(m.shelfText) == "":
		return "(" + shelfLabel(e) + " holds no textual changes)"
	default:
		return m.colorize(m.shelfText)
	}
}

// shelfShowsPatch reports whether shelfDetail currently renders a unified diff,
// which is the only Main view with a +/-/space gutter to pin.
func (m *Model) shelfShowsPatch() bool {
	e, ok := m.shelves.Selected()
	return ok && m.shelfErr == nil && !m.shelfReadErr &&
		m.shelfID == e.ID && strings.TrimSpace(m.shelfText) != ""
}
