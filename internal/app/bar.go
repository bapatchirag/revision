package app

import (
	"fmt"
)

// updateBar sets the contextual key hints and the right-aligned load state.
func (m *Model) updateBar() {
	m.bar.SetHints(m.barHints())

	switch {
	case m.err != nil:
		m.bar.SetRight("error")
	case m.loading:
		m.bar.SetRight("loading…")
	default:
		m.bar.SetRight("")
	}
}

// barHints returns the key hints for the focused panel — and, for the Files
// panel, its active view — in the order the bar should drop them when they do
// not all fit. Only keys that act on that panel are listed; the global ones live
// in the help menu, so a panel that only scrolls (the command log) gets no hints
// and leaves the bar empty. When the focused panel has an active filter or
// search (and the input is closed) it is described first, so the user can see
// it, jump between search matches, and clear it.
func (m *Model) barHints() []string {
	p := m.focus.Index()
	hints := m.panelHints(p)
	if len(hints) == 0 {
		return nil
	}
	hints = append(hints, "? help")
	if q := m.filters[p]; q != "" && !m.filtering {
		if m.isSearchPanel(p) {
			return append(m.searchHints(p, q), hints...)
		}
		return append([]string{"filter: " + q, "esc clear"}, hints...)
	}
	return hints
}

// panelHints are the keys panel p responds to, most useful first.
func (m *Model) panelHints(p int) []string {
	switch p {
	case panelStatus:
		// The Status panel is where the source path is shown, so it is also where
		// the keys that move it are worth advertising.
		return []string{"/ search", "P source path", "W repository"}
	case panelMain:
		return []string{"/ search"}
	case panelFiles:
		return m.filesHints()
	case panelShelf:
		if len(m.shelfItems) == 0 {
			return []string{"z shelve"}
		}
		return []string{"enter apply", "p pop", "d drop", "n rename", "z shelve", "/ filter"}
	case panelLog:
		if m.inRevDrill() {
			return []string{"diff " + m.revDiff.label(), "enter expand", "w save", "/ filter", "esc back"}
		}
		hints := []string{"v pick", "space update to rev", "n/p page", "/ filter"}
		if len(m.logPicks) > 0 {
			return append([]string{m.logPickLabel(), "enter diff", "esc clear"}, hints...)
		}
		return hints
	}
	// The command log only scrolls; it has no actions of its own.
	return nil
}

// filesHints are the Files panel's keys for its active view: the saved-diff
// browser, the reject browser, a drilled-in changelist, the Changelists
// overview, or the Changes tree. While files are picked for a shelve, what is
// held leads, since that is what z will act on from any of them.
func (m *Model) filesHints() []string {
	hints := m.filesViewHints()
	if n := len(m.pickedItems()); n > 0 {
		return append([]string{m.shelfPickLabel(), "z shelve", "esc clear"}, hints...)
	}
	return hints
}

func (m *Model) filesViewHints() []string {
	switch {
	case m.filesViewIsDiffs():
		return []string{"e open", "p apply", "d delete", "/ filter", "[ ] view"}
	case m.filesViewIsRejects():
		return []string{"enter expand", "m resolve", "e open", "d delete", "/ filter", "[ ] view"}
	case m.inChangelistDrill():
		return []string{"space unstage", "v pick", "c commit", "esc back", "[ ] view"}
	case m.filesViewIsChangelists():
		return []string{"enter expand", "v pick", "n name", "c commit", "[ ] view"}
	}
	return []string{"space stage", "v pick", "n changelist", "c commit", "r revert", "d delete", "[ ] view"}
}

// searchHints describe the active search on a Viewport panel: the query, the
// current match position (or that there are none), and the jump/clear keys.
func (m *Model) searchHints(p int, q string) []string {
	vp := m.searchViewport(p)
	if vp.MatchCount() == 0 {
		return []string{"search: " + q, "no matches", "esc clear"}
	}
	where := fmt.Sprintf("%d matches", vp.MatchCount())
	if pos := vp.CurrentMatch(); pos > 0 {
		where = fmt.Sprintf("%d/%d", pos, vp.MatchCount())
	}
	return []string{"search: " + q, where, "n next", "N prev", "esc clear"}
}
