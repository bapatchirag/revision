package app

import (
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
)

// openFilter opens the filter input for the focused panel, pre-filled with that
// panel's current filter and labeled with the panel name and its available
// parameters. The input captures the keyboard until the user submits (enter,
// keep) or dismisses (esc, clear).
func (m *Model) openFilter() tea.Cmd {
	p := m.focus.Index()
	m.filtering = true
	m.filterPanel = p
	m.searchBar.SetPrefix(m.filterPrefix(p))
	m.searchBar.SetValue(m.filters[p])
	m.searchBar.SetSize(m.width, 1)
	m.searchBar.Focus()
	return nil
}

// applyFilterLive re-applies the in-progress query to the panel being filtered,
// so it updates as the user types. It returns a command to refresh the Main
// panel when it follows the filtered panel, whose narrowed selection may now
// point at a different row.
func (m *Model) applyFilterLive() tea.Cmd {
	m.setFilter(m.filterPanel, m.searchBar.Value())
	return m.afterFilterChange(m.filterPanel)
}

// afterFilterChange refreshes Main when it is driven by the panel whose filter
// just changed, since narrowing the list clamps the cursor onto a different
// selection. It returns a command to load the newly-selected file's diff when
// one is needed.
func (m *Model) afterFilterChange(p int) tea.Cmd {
	switch p {
	case panelFiles:
		if m.source == sourceFiles {
			m.updateMain()
			return m.diffLoadForSelection()
		}
	case panelLog:
		if m.source == sourceLog {
			m.updateMain()
		}
	case panelShelf:
		if m.source == sourceShelf {
			m.updateMain()
			return m.shelfLoadForSelection()
		}
	}
	return nil
}

// commitFilter closes the filter input, keeping the filter that was applied live
// while typing. For a search panel with no matches it surfaces a toast so the
// user is not left wondering why nothing is highlighted.
func (m *Model) commitFilter() {
	m.filtering = false
	m.searchBar.Blur()
	if q := m.filters[m.filterPanel]; q != "" && m.isSearchPanel(m.filterPanel) && m.searchViewport(m.filterPanel).MatchCount() == 0 {
		m.showToast("no matches for "+q, component.LevelInfo)
	}
	m.updateBar()
}

// clearFilter closes the filter input and removes the filter from the panel it
// was editing (esc while the input is open), returning a command to refresh Main
// when it follows that panel.
func (m *Model) clearFilter() tea.Cmd {
	m.filtering = false
	m.searchBar.Blur()
	p := m.filterPanel
	m.setFilter(p, "")
	m.updateBar()
	return m.afterFilterChange(p)
}

// clearFocusedFilter removes the focused panel's filter when it has one (esc
// while no input is open), returning a command to refresh Main and whether a
// filter was cleared — so the caller can leave esc for the panel (e.g. to pop a
// changelist drill) when there was none.
func (m *Model) clearFocusedFilter() (tea.Cmd, bool) {
	p := m.focus.Index()
	if m.filters[p] == "" {
		return nil, false
	}
	m.setFilter(p, "")
	m.updateBar()
	return m.afterFilterChange(p), true
}

// setFilter records (or clears, when q is blank) the filter for panel p and
// re-renders that panel from its unfiltered source. The Files and Log panels
// filter (rows are removed); the Main and Status viewports search (matching lines
// are highlighted and jumped between, never removed).
func (m *Model) setFilter(p int, q string) {
	if strings.TrimSpace(q) == "" {
		delete(m.filters, p)
	} else {
		m.filters[p] = q
	}
	switch p {
	case panelFiles:
		m.rebuildFilesViews()
	case panelLog:
		// The Log panel shows the revisions or, drilled in, the files a range of
		// them touched; the filter narrows whichever of the two is on screen.
		if m.inRevDrill() {
			m.rebuildRevFiles()
		} else {
			m.applyLogFilter()
		}
	case panelStatus:
		m.status.SetSearch(m.filters[panelStatus])
	case panelMain:
		m.main.SetSearch(m.filters[panelMain])
	case panelShelf:
		m.rebuildShelves()
	}
}

// isSearchPanel reports whether panel p is a Viewport that searches (highlights +
// jumps) rather than filters (removes rows).
func (m *Model) isSearchPanel(p int) bool {
	return p == panelMain || p == panelStatus
}

// searchViewport returns the Viewport backing a search panel.
func (m *Model) searchViewport(p int) *component.Viewport {
	if p == panelStatus {
		return m.status
	}
	return m.main
}

// jumpMatch moves a search panel's viewport to its next (dir > 0) or previous
// match and refreshes the footer position; with no matches it explains why with
// a toast.
func (m *Model) jumpMatch(p, dir int) {
	vp := m.searchViewport(p)
	if vp.MatchCount() == 0 {
		m.showToast("no matches for "+m.filters[p], component.LevelInfo)
		return
	}
	if dir < 0 {
		vp.PrevMatch()
	} else {
		vp.NextMatch()
	}
	m.updateBar()
}

// filterPrefix is the muted label shown in the filter input for panel p, naming
// the panel, its behavior (filter vs. search) and its available parameters.
// Drilled into a range's files the Log panel is filtered as a file tree, so it
// offers what those rows carry rather than what a revision does.
func (m *Model) filterPrefix(p int) string {
	switch p {
	case panelFiles:
		return "filter files (state: cl:)"
	case panelLog:
		if m.inRevDrill() {
			return "filter files (state:)"
		}
		return "filter log (rev: user: path: date:)"
	case panelStatus:
		return "search status"
	default:
		return "search main"
	}
}

// filesQuery is the parsed Files-panel filter.
func (m *Model) filesQuery() filterQuery {
	return parseFilter(m.filters[panelFiles], fileFilterKeys)
}

// filteredStatusItems returns the subset of items shown in the Files views: the
// ones matching the Files-panel filter, with untracked (unversioned) files
// dropped while the hide-untracked toggle is on. Items are returned unchanged
// when no filter is set and untracked files are shown.
func (m *Model) filteredStatusItems(items []svn.StatusItem) []svn.StatusItem {
	q := m.filesQuery()
	if q.empty() && !m.hideUntracked {
		return items
	}
	out := make([]svn.StatusItem, 0, len(items))
	for _, it := range items {
		if m.hideUntracked && it.State == svn.StateUnversioned {
			continue
		}
		if matchStatusItem(it, q) {
			out = append(out, it)
		}
	}
	return out
}

// compileHideRules rebuilds the matchers from the configured rules that are in
// force. A pattern that does not compile is skipped; the configuration drops
// those on load and the rules editor refuses to save one, so a rule that reaches
// here can be relied on.
func (m *Model) compileHideRules() {
	m.hideMatchers = nil
	for _, r := range m.cfg.HideRules {
		if !r.Enabled {
			continue
		}
		if re, err := regexp.Compile(r.Pattern); err == nil {
			m.hideMatchers = append(m.hideMatchers, re)
		}
	}
}

// hideRulesActive reports whether any hide rule is in force, so the Changes view
// can say it is showing less than the working copy holds.
func (m *Model) hideRulesActive() bool { return len(m.hideMatchers) > 0 }

// visibleChanges drops the files a hide rule matches. It narrows the Changes
// tree alone: the Changelists views, a commit and every svn action still see
// every file, so a hidden file is out of sight rather than out of the way.
func (m *Model) visibleChanges(items []svn.StatusItem) []svn.StatusItem {
	if !m.hideRulesActive() {
		return items
	}
	out := make([]svn.StatusItem, 0, len(items))
	for _, it := range items {
		if !m.hiddenByRule(it.Path) {
			out = append(out, it)
		}
	}
	return out
}

// hiddenByRule reports whether any rule in force matches path. Patterns are
// unanchored, so one naming a directory hides everything beneath it.
func (m *Model) hiddenByRule(path string) bool {
	for _, re := range m.hideMatchers {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

// applyLogFilter repopulates the Log table from the raw revision history under
// the Log-panel filter.
func (m *Model) applyLogFilter() {
	m.log.SetItems(m.filteredLogEntries())
}

// filteredLogEntries returns the revisions matching the Log-panel filter, or all
// of them when no filter is set. The filter only ever sees the page on screen,
// since that is all the Log panel holds.
func (m *Model) filteredLogEntries() []svn.LogEntry {
	q := parseFilter(m.filters[panelLog], logFilterKeys)
	if q.empty() {
		return m.logEntries
	}
	out := make([]svn.LogEntry, 0, len(m.logEntries))
	for _, e := range m.logEntries {
		if matchLogEntry(e, q) {
			out = append(out, e)
		}
	}
	return out
}
