package component_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/bapatchirag/revision/internal/tui/component"
)

func TestViewportSearchCountsAndSelectsFirst(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("alpha\ntarget one\nbeta\ntarget two\ngamma")
	v.SetSize(30, 6)

	if got := v.SetSearch("target"); got != 2 {
		t.Fatalf("SetSearch returned %d, want 2 matches", got)
	}
	if got := v.MatchCount(); got != 2 {
		t.Errorf("MatchCount = %d, want 2", got)
	}
	// The search lands on the first match.
	if got := v.CurrentMatch(); got != 1 {
		t.Errorf("CurrentMatch = %d, want 1", got)
	}
}

func TestViewportSearchNextPrevWrap(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("m\nx\nm\nx\nm")
	v.SetSize(20, 4)
	v.SetSearch("m") // matches lines 0, 2, 4; current = 1/3

	v.NextMatch()
	if got := v.CurrentMatch(); got != 2 {
		t.Errorf("after NextMatch CurrentMatch = %d, want 2", got)
	}
	v.NextMatch()
	if got := v.CurrentMatch(); got != 3 {
		t.Errorf("after 2x NextMatch CurrentMatch = %d, want 3", got)
	}
	v.NextMatch() // wraps
	if got := v.CurrentMatch(); got != 1 {
		t.Errorf("NextMatch from the last should wrap to 1, got %d", got)
	}
	v.PrevMatch() // wraps back to last
	if got := v.CurrentMatch(); got != 3 {
		t.Errorf("PrevMatch from the first should wrap to 3, got %d", got)
	}
}

func TestViewportSearchNoMatches(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("alpha\nbeta")
	v.SetSize(20, 4)

	if got := v.SetSearch("zzz"); got != 0 {
		t.Fatalf("SetSearch returned %d, want 0", got)
	}
	if got := v.CurrentMatch(); got != 0 {
		t.Errorf("CurrentMatch = %d, want 0 when there are no matches", got)
	}
	if v.NextMatch() {
		t.Error("NextMatch should report false when there are no matches")
	}
}

func TestViewportSearchSurvivesSetContent(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("one target\ntwo")
	v.SetSize(20, 4)
	v.SetSearch("target")
	if v.MatchCount() != 1 {
		t.Fatalf("expected 1 match, got %d", v.MatchCount())
	}

	// Replacing the content re-evaluates the active search against it.
	v.SetContent("nothing here\nanother target\nand target again")
	if got := v.MatchCount(); got != 2 {
		t.Errorf("after SetContent MatchCount = %d, want 2", got)
	}
	// No match is selected until the next SetSearch or jump.
	if got := v.CurrentMatch(); got != 0 {
		t.Errorf("CurrentMatch = %d, want 0 right after content change", got)
	}
}

func TestViewportClearSearch(t *testing.T) {
	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("a target b")
	v.SetSize(20, 4)
	v.SetSearch("target")
	v.ClearSearch()
	if v.MatchCount() != 0 {
		t.Errorf("ClearSearch should drop all matches, got %d", v.MatchCount())
	}
}

func TestViewportSearchHighlightRendersDistinctStyles(t *testing.T) {
	// Verify the ANSI styling: the current match is a reverse-video bar; other
	// matches get the selection background. The component TestMain forces the
	// Ascii profile (no ANSI), so switch to a real profile for this check.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	v := component.NewViewport(testTheme(), testKeys())
	v.SetContent("alpha\ntarget one\nbeta\ntarget two")
	v.SetSize(30, 6)
	v.SetSearch("target") // matches lines 1 (current) and 3 (other)

	rows := strings.Split(v.View(), "\n")
	if len(rows) < 4 {
		t.Fatalf("expected at least 4 rows, got %d", len(rows))
	}
	if !strings.Contains(rows[1], "\x1b[7m") {
		t.Errorf("current match row should use reverse video, got %q", rows[1])
	}
	if !strings.Contains(rows[3], "48;5;238") {
		t.Errorf("other match row should use the selection background, got %q", rows[3])
	}
	if strings.Contains(rows[3], "\x1b[7m") {
		t.Errorf("non-current match should not use reverse video, got %q", rows[3])
	}
	// A non-match line carries neither highlight.
	if strings.Contains(rows[0], "\x1b[7m") || strings.Contains(rows[0], "48;5;238") {
		t.Errorf("non-match row should not be highlighted, got %q", rows[0])
	}
}
