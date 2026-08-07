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

	// The working-copy row carries a coloured asterisk that strips to "* r42".
	star := renderLogRow(svn.LogEntry{Revision: "42"}, "42", false, th)[0]
	if stripANSI(star) == star {
		t.Errorf("expected the asterisk to be coloured (ANSI), got plain %q", star)
	}
	if got := stripANSI(star); got != "* r42" {
		t.Errorf("marker cell should read %q, got %q", "* r42", got)
	}

	// Other rows are a plain, unstyled two-space prefix of the same width.
	other := renderLogRow(svn.LogEntry{Revision: "41"}, "42", false, th)[0]
	if other != "  r41" {
		t.Errorf("non-working-copy row should be plain %q, got %q", "  r41", other)
	}

	// A page turn in flight dims the rows of the page being left.
	stale := renderLogRow(svn.LogEntry{Revision: "41"}, "42", true, th)[0]
	if stripANSI(stale) == stale {
		t.Errorf("expected a stale row to be dimmed (ANSI), got plain %q", stale)
	}
	if got := stripANSI(stale); got != "  r41" {
		t.Errorf("dimmed row should still read %q, got %q", "  r41", got)
	}
}
