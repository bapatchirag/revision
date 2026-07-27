package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
)

// keyRunes builds a key message typing the given text.
func keyRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// stepModel applies a message and returns the resulting *Model. For enter/esc it
// also delivers the single follow-up message the search bar emits (Submit or
// Dismiss), mirroring the Bubble Tea runtime, so the command's effect is applied
// within the step. Other follow-up commands (such as a diff load triggered while
// live-filtering) are intentionally left un-run so tests never invoke the nil
// svn client.
func stepModel(t *testing.T, m *Model, msg tea.Msg) *Model {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(*Model)
	if km, ok := msg.(tea.KeyMsg); ok && cmd != nil && (km.Type == tea.KeyEnter || km.Type == tea.KeyEsc) {
		if out := cmd(); out != nil {
			next, _ = m.Update(out)
			m = next.(*Model)
		}
	}
	return m
}

// hasFileLeaf reports whether the tree rows contain a file leaf at path.
func hasFileLeaf(nodes []fileNode, path string) bool {
	for _, n := range nodes {
		if n.Item != nil && n.Item.Path == path {
			return true
		}
	}
	return false
}

func TestFilterLogByParams(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	m = stepModel(t, m, logLoadedMsg{entries: []svn.LogEntry{
		{Revision: "42", Author: "alice", Message: "Fix crash\n\nnull pointer in parser"},
		{Revision: "41", Author: "bob", Message: "Add feature"},
		{Revision: "40", Author: "alice", Message: "Docs update"},
	}})

	// Focus the Log panel (key "3") and open the filter (key "/").
	m = stepModel(t, m, keyRunes("3"))
	m = stepModel(t, m, keyRunes("/"))
	if !m.filtering {
		t.Fatal("expected filtering to be active after /")
	}

	// A combined query: author parameter plus free text matched against the FULL
	// commit message (the "parser" term is on a later line, not the first).
	m = stepModel(t, m, keyRunes("user:alice parser"))
	if got := len(m.log.Items()); got != 1 {
		t.Fatalf("expected 1 matching entry, got %d", got)
	}
	if rev := m.log.Items()[0].Revision; rev != "42" {
		t.Errorf("expected r42, got r%s", rev)
	}

	// Enter keeps the filter and closes the input.
	m = stepModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.filtering {
		t.Error("enter should close the filter input")
	}
	if len(m.log.Items()) != 1 {
		t.Error("filter should persist after enter")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "filter:") {
		t.Errorf("status bar should advertise the active filter, got:\n%s", view)
	}
}

func TestFilterLogEscClears(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	m = stepModel(t, m, logLoadedMsg{entries: []svn.LogEntry{
		{Revision: "42", Author: "alice", Message: "one"},
		{Revision: "41", Author: "bob", Message: "two"},
		{Revision: "40", Author: "carol", Message: "three"},
	}})
	m = stepModel(t, m, keyRunes("3"))
	m = stepModel(t, m, keyRunes("/"))
	m = stepModel(t, m, keyRunes("user:bob"))
	if len(m.log.Items()) != 1 {
		t.Fatalf("expected 1 entry after filtering, got %d", len(m.log.Items()))
	}

	// esc while the input is open dismisses and clears the filter.
	m = stepModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.filtering {
		t.Error("esc should close the filter input")
	}
	if len(m.log.Items()) != 3 {
		t.Errorf("esc should restore all entries, got %d", len(m.log.Items()))
	}
}

func TestFilterFilesByPath(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
		{Path: "beta.txt", State: svn.StateAdded},
	})
	// The Files panel is focused by default.
	m = stepModel(t, m, keyRunes("/"))
	m = stepModel(t, m, keyRunes("alpha"))

	if !hasFileLeaf(m.files.Items(), "alpha.txt") {
		t.Error("alpha.txt should remain under the path filter")
	}
	if hasFileLeaf(m.files.Items(), "beta.txt") {
		t.Error("beta.txt should be filtered out")
	}
}

func TestFilterFilesByState(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
		{Path: "beta.txt", State: svn.StateAdded},
	})
	m = stepModel(t, m, keyRunes("/"))
	m = stepModel(t, m, keyRunes("state:A"))

	if hasFileLeaf(m.files.Items(), "alpha.txt") {
		t.Error("state:A should exclude the modified file")
	}
	if !hasFileLeaf(m.files.Items(), "beta.txt") {
		t.Error("state:A should keep the added file")
	}
}

func TestFilterFilesSurvivesReload(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
		{Path: "beta.txt", State: svn.StateModified},
	})
	m = stepModel(t, m, keyRunes("/"))
	m = stepModel(t, m, keyRunes("alpha"))
	m = stepModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	// A status reload re-applies the active filter to the fresh items.
	m = loadItems(t, m, []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
		{Path: "beta.txt", State: svn.StateAdded},
		{Path: "alphabet.txt", State: svn.StateModified},
	})
	if hasFileLeaf(m.files.Items(), "beta.txt") {
		t.Error("reload should keep beta.txt filtered out")
	}
	if !hasFileLeaf(m.files.Items(), "alpha.txt") || !hasFileLeaf(m.files.Items(), "alphabet.txt") {
		t.Error("reload should keep matching files (alpha, alphabet)")
	}
}

func TestSearchMainHighlightsNotFilters(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	})
	// Load the selected file's diff into Main (source follows Files by default).
	m = stepModel(t, m, diffLoadedMsg{path: "alpha.txt", diff: "@@ -1 +1 @@\n+added widget line\n-removed gadget line"})

	// Focus Main (key "0") and search for "widget".
	m = stepModel(t, m, keyRunes("0"))
	m = stepModel(t, m, keyRunes("/"))
	m = stepModel(t, m, keyRunes("widget"))

	// Search HIGHLIGHTS but does not remove lines: both the matching and the
	// non-matching line stay visible.
	main := stripANSI(m.main.View())
	if !strings.Contains(main, "added widget line") || !strings.Contains(main, "gadget") {
		t.Errorf("main should keep every line (search highlights, not filters), got:\n%s", main)
	}
	if got := m.main.MatchCount(); got != 1 {
		t.Errorf("expected 1 matching line, got %d", got)
	}
}

func TestSearchMainJumpsBetweenMatches(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	})
	m = stepModel(t, m, diffLoadedMsg{path: "alpha.txt", diff: "@@ x @@\n+match one\n-other\n+match two\n context\n+match three"})
	m = stepModel(t, m, keyRunes("0"))
	m = stepModel(t, m, keyRunes("/"))
	m = stepModel(t, m, keyRunes("match"))
	m = stepModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if got := m.main.MatchCount(); got != 3 {
		t.Fatalf("expected 3 matches, got %d", got)
	}
	// The committed search lands on the first match.
	if got := m.main.CurrentMatch(); got != 1 {
		t.Fatalf("current match should be 1 after committing, got %d", got)
	}

	// n advances to the next match, N steps back (both wrap).
	m = stepModel(t, m, keyRunes("n"))
	if got := m.main.CurrentMatch(); got != 2 {
		t.Errorf("n should advance to match 2, got %d", got)
	}
	m = stepModel(t, m, keyRunes("N"))
	if got := m.main.CurrentMatch(); got != 1 {
		t.Errorf("N should return to match 1, got %d", got)
	}
	m = stepModel(t, m, keyRunes("N"))
	if got := m.main.CurrentMatch(); got != 3 {
		t.Errorf("N from the first match should wrap to the last (3), got %d", got)
	}
}

func TestSearchMainNoMatchToasts(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	})
	m = stepModel(t, m, diffLoadedMsg{path: "alpha.txt", diff: "@@ x @@\n+alpha\n-beta"})
	m = stepModel(t, m, keyRunes("0"))
	m = stepModel(t, m, keyRunes("/"))
	m = stepModel(t, m, keyRunes("zzz"))

	// Committing a search with no matches surfaces a toast.
	m = stepModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if view := stripANSI(m.View()); !strings.Contains(view, "no matches for zzz") {
		t.Errorf("expected a 'no matches' toast on commit, got:\n%s", view)
	}

	// Trying to jump with no matches toasts again.
	m = stepModel(t, m, keyRunes("n"))
	if view := stripANSI(m.View()); !strings.Contains(view, "no matches for zzz") {
		t.Errorf("expected a 'no matches' toast when jumping with no matches, got:\n%s", view)
	}
}

func TestFilterInputVisibleWhileTyping(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	m = stepModel(t, m, logLoadedMsg{entries: []svn.LogEntry{{Revision: "1", Author: "a", Message: "m"}}})
	m = stepModel(t, m, keyRunes("3"))
	m = stepModel(t, m, keyRunes("/"))
	m = stepModel(t, m, keyRunes("rev:1"))

	view := stripANSI(m.View())
	if !strings.Contains(view, "rev:1") {
		t.Errorf("the filter input should show the typed query, got:\n%s", view)
	}
	if !strings.Contains(view, "log (") {
		t.Errorf("the filter input should show the log prefix, got:\n%s", view)
	}
}

func TestClearFocusedFilterReportsWhetherCleared(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.txt", State: svn.StateModified}})

	// With no filter set, esc is left for the panel (reports false).
	if _, cleared := m.clearFocusedFilter(); cleared {
		t.Error("no filter set: clearFocusedFilter should report false")
	}

	m.setFilter(panelFiles, "a.txt")
	if _, cleared := m.clearFocusedFilter(); !cleared {
		t.Error("filter set: clearFocusedFilter should report true")
	}
	if _, ok := m.filters[panelFiles]; ok {
		t.Error("clearFocusedFilter should remove the filter")
	}
}
