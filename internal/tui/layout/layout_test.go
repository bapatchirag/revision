package layout_test

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/bapatchirag/revision/internal/tui/layout"
)

// TestMain forces a color profile so styled backgrounds actually emit ANSI,
// letting the overlay's ANSI-awareness be exercised. Plain-string tests are
// unaffected: escapes only appear where a style is rendered.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI)
	os.Exit(m.Run())
}

func TestOverlaySplicesForeground(t *testing.T) {
	bg := "aaaa\nbbbb\ncccc"
	got := layout.Overlay(bg, "XX", 1, 1)
	want := "aaaa\nbXXb\ncccc"
	if got != want {
		t.Errorf("Overlay = %q, want %q", got, want)
	}
}

func TestOverlayPadsShortBackground(t *testing.T) {
	got := layout.Overlay("ab", "Z", 4, 0)
	if got != "ab  Z" {
		t.Errorf("Overlay = %q, want %q", got, "ab  Z")
	}
}

func TestOverlayIgnoresRowsPastBackground(t *testing.T) {
	got := layout.Overlay("row0", "a\nb\nc", 0, 2)
	if got != "row0" {
		t.Errorf("Overlay = %q, want unchanged background", got)
	}
}

func TestOverlayPreservesStyledBackground(t *testing.T) {
	// A styled background (green on both sides of where the popup lands) must
	// keep its visible text intact and stay ANSI-aware: slicing by display
	// column, not by byte, so the escape codes are not counted as width.
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	bg := green.Render("aaaaaaaa") // 8 visible cells wrapped in SGR codes
	got := layout.Overlay(bg, "XX", 3, 0)

	if stripped := ansi.Strip(got); stripped != "aaaXXaaa" {
		t.Errorf("visible overlay = %q, want %q", stripped, "aaaXXaaa")
	}
	if !strings.Contains(got, "\x1b[") {
		t.Error("expected the styled background to retain ANSI escape codes")
	}
}

func TestCenterDimensions(t *testing.T) {
	got := layout.Center(10, 3, "x")
	if lipgloss.Width(got) != 10 {
		t.Errorf("Center width = %d, want 10", lipgloss.Width(got))
	}
	if h := strings.Count(got, "\n") + 1; h != 3 {
		t.Errorf("Center height = %d, want 3", h)
	}
}

func TestDimFlattensStylingAndKeepsWidth(t *testing.T) {
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	view := red.Render("alpha") + " " + green.Render("beta") + "\nplain"

	got := layout.Dim(view, lipgloss.Color("8"))
	if plain := ansi.Strip(got); plain != "alpha beta\nplain" {
		t.Errorf("Dim changed the text to %q", plain)
	}
	for i, ln := range strings.Split(got, "\n") {
		if want := ansi.StringWidth(strings.Split(view, "\n")[i]); ansi.StringWidth(ln) != want {
			t.Errorf("line %d is %d cells wide, want %d — Overlay needs the width preserved", i, ansi.StringWidth(ln), want)
		}
	}
	// Every line ends up in the one color, so nothing keeps its original styling.
	if strings.Contains(got, red.Render("alpha")) || strings.Contains(got, green.Render("beta")) {
		t.Errorf("Dim should drop the styling it found, got %q", got)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("Dim should render in the given color, got %q", got)
	}
}
