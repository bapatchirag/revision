package layout_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/tui/layout"
)

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

func TestCenterDimensions(t *testing.T) {
	got := layout.Center(10, 3, "x")
	if lipgloss.Width(got) != 10 {
		t.Errorf("Center width = %d, want 10", lipgloss.Width(got))
	}
	if h := strings.Count(got, "\n") + 1; h != 3 {
		t.Errorf("Center height = %d, want 3", h)
	}
}
