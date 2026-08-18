package app

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
)

// TestBarHintsAreScopedToThePanel asserts each panel advertises only the keys it
// acts on: the file-oriented keys must not leak onto the Status, Main or Log
// panels, and the command log — which only scrolls — advertises nothing at all.
func TestBarHintsAreScopedToThePanel(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})

	for _, tc := range []struct {
		name  string
		panel int
		want  []string
		avoid []string
	}{
		{"files", panelFiles, []string{"space stage", "r revert", "? help"}, nil},
		{"status", panelStatus, []string{"/ search", "? help"}, []string{"space stage", "r revert"}},
		{"log", panelLog, []string{"space update to rev", "c commit"}, []string{"space stage", "d delete"}},
		{"main", panelMain, []string{"/ search", "? help"}, []string{"space stage", "d delete"}},
		{"command log", panelCmdLog, nil, []string{"space stage", "? help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.focus.Focus(tc.panel)
			m.afterFocusChange()
			got := m.barHints()
			for _, want := range tc.want {
				if !slices.Contains(got, want) {
					t.Errorf("hints %v missing %q", got, want)
				}
			}
			for _, avoid := range tc.avoid {
				if slices.Contains(got, avoid) {
					t.Errorf("hints %v should not advertise %q", got, avoid)
				}
			}
			if len(tc.want) == 0 && len(got) != 0 {
				t.Errorf("panel with no keys of its own should have no hints, got %v", got)
			}
		})
	}
}

// TestBarHintsFollowTheFilesView asserts the Files panel re-hints itself as its
// active view changes, since the keys differ per view.
func TestBarHintsFollowTheFilesView(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "feature.go", State: svn.StateModified, Changelist: "feature-x"},
	})
	if got := m.barHints(); !slices.Contains(got, "space stage") {
		t.Fatalf("Changes view hints %v missing %q", got, "space stage")
	}

	// ] switches to Changelists; the runtime delivers the view-changed message
	// the switch emits, which is what re-hints the bar.
	next, cmd := m.Update(keyRunes("]"))
	m = next.(*Model)
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if got := m.barHints(); !slices.Contains(got, "enter expand") {
		t.Errorf("Changelists view hints %v missing %q", got, "enter expand")
	}

	// Drilling into the changelist swaps stage for unstage and offers esc back.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	got := m.barHints()
	if !slices.Contains(got, "space unstage") || !slices.Contains(got, "esc back") {
		t.Errorf("drilled changelist hints %v missing the drill keys", got)
	}
}

// TestBarDropsRepoAndRevision asserts the bar no longer repeats the repository
// URL or revision numbers; those live in the Status panel.
func TestBarDropsRepoAndRevision(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	m = stepModel(t, m, logLoadedMsg{page: 1, entries: []svn.LogEntry{{Revision: "50"}}})

	bar := lastLine(stripANSI(m.View()))
	for _, unwanted := range []string{"svn.example.com", "r42", "r50", "HEAD"} {
		if strings.Contains(bar, unwanted) {
			t.Errorf("status bar should not carry %q, got:\n%s", unwanted, bar)
		}
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}
