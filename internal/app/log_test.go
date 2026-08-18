package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

func TestLogStarsWorkingCopyRevision(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	// The working copy opens at r42 (from info); history includes it.
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{
		{Revision: "50"}, {Revision: "42"}, {Revision: "41"},
	}})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "* r42") {
		t.Errorf("expected an asterisk on the working-copy revision r42, got:\n%s", view)
	}
	if view := stripANSI(m.View()); strings.Contains(view, "* r50") {
		t.Errorf("only the working-copy revision should be starred, got:\n%s", view)
	}

	// After updating to r50 the star follows the working copy.
	next, _ = m.Update(updatedMsg{revision: "50"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "* r50") {
		t.Errorf("expected the asterisk to move to r50 after updating, got:\n%s", view)
	}
}

func TestRenderLogRowColorsWorkingCopyAsterisk(t *testing.T) {
	// Emit ANSI so the styling is observable, then restore the Ascii profile.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	th := theme.Default()

	// The working-copy row carries a coloured asterisk that strips to " * r42":
	// the pick cell ahead of it is blank but still drawn.
	star := renderLogRow(svn.LogEntry{Revision: "42"}, "42", false, false, th)[0]
	if stripANSI(star) == star {
		t.Errorf("expected the asterisk to be coloured (ANSI), got plain %q", star)
	}
	if got := stripANSI(star); got != " * r42" {
		t.Errorf("marker cell should read %q, got %q", " * r42", got)
	}

	// Other rows are a plain, unstyled prefix of the same width.
	other := renderLogRow(svn.LogEntry{Revision: "41"}, "42", false, false, th)[0]
	if other != "   r41" {
		t.Errorf("non-working-copy row should be plain %q, got %q", "   r41", other)
	}

	// A page turn in flight dims the rows of the page being left.
	stale := renderLogRow(svn.LogEntry{Revision: "41"}, "42", false, true, th)[0]
	if stripANSI(stale) == stale {
		t.Errorf("expected a stale row to be dimmed (ANSI), got plain %q", stale)
	}
	if got := stripANSI(stale); got != "   r41" {
		t.Errorf("dimmed row should still read %q, got %q", "   r41", got)
	}
}

func TestRenderLogRowMarksPickedRevisions(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	th := theme.Default()

	picked := renderLogRow(svn.LogEntry{Revision: "41"}, "42", true, false, th)[0]
	if stripANSI(picked) == picked {
		t.Errorf("expected the pick dot to be coloured (ANSI), got plain %q", picked)
	}
	if got := stripANSI(picked); got != "●  r41" {
		t.Errorf("picked row should read %q, got %q", "●  r41", got)
	}

	// A revision can be both picked and the one the working copy sits at, so the
	// two markers occupy cells of their own rather than displacing each other.
	both := renderLogRow(svn.LogEntry{Revision: "42"}, "42", true, false, th)
	if got := stripANSI(both[0]); got != "●* r42" {
		t.Errorf("picked working-copy row should read %q, got %q", "●* r42", got)
	}

	// Every combination occupies the same width, so picking cannot shift the
	// column out from under the reader.
	widths := map[string]int{}
	for _, row := range []string{
		stripANSI(renderLogRow(svn.LogEntry{Revision: "41"}, "42", false, false, th)[0]),
		stripANSI(picked),
		stripANSI(renderLogRow(svn.LogEntry{Revision: "42"}, "42", false, false, th)[0]),
		stripANSI(both[0]),
	} {
		widths[row] = len([]rune(row))
	}
	for row, w := range widths {
		if w != 6 {
			t.Errorf("row %q is %d cells wide, want every marker combination the same", row, w)
		}
	}
}
