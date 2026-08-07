package main

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestMain forces the ASCII color profile so the rendered gallery carries no
// escape codes, keeping the assertions below about layout rather than styling.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// sized returns the gallery at a terminal size, which is what every demo is
// laid out against.
func sized(t *testing.T, w, h int) model {
	t.Helper()
	next, _ := newModel().Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(model)
}

func TestGalleryRendersEveryDemo(t *testing.T) {
	m := sized(t, 100, 30)
	if m.Init() != nil {
		t.Error("the gallery needs no startup command")
	}

	for i := range m.demos {
		m.idx = i
		view := m.View()
		if strings.TrimSpace(view) == "" {
			t.Errorf("demo %q rendered nothing", m.demos[i].name)
		}
		if !strings.Contains(view, m.demos[i].name) {
			t.Errorf("view for %q does not name it:\n%s", m.demos[i].name, view)
		}
	}
}

func TestGalleryRendersBeforeAnySize(t *testing.T) {
	if got := newModel().View(); got != "loading…" {
		t.Errorf("View() = %q, want the pre-size placeholder", got)
	}
}

// TestGalleryFitsATinyTerminal drives the floor in resizeDemos and View, where a
// window too small for the layout still has to produce something.
func TestGalleryFitsATinyTerminal(t *testing.T) {
	m := sized(t, 8, 4)
	if strings.TrimSpace(m.View()) == "" {
		t.Error("the gallery should still render in a tiny terminal")
	}
}

func TestGallerySwitchesDemos(t *testing.T) {
	m := sized(t, 100, 30)
	last := len(m.demos) - 1

	next, _ := m.Update(key(t, "["))
	m = next.(model)
	if m.idx != last {
		t.Errorf("idx = %d, want the switch to wrap round to %d", m.idx, last)
	}

	next, _ = m.Update(key(t, "]"))
	m = next.(model)
	if m.idx != 0 {
		t.Errorf("idx = %d, want the switch to wrap back to 0", m.idx)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	if m.idx != 1 {
		t.Errorf("idx = %d, want tab to advance", m.idx)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = next.(model)
	if m.idx != 0 {
		t.Errorf("idx = %d, want shift+tab to step back", m.idx)
	}

	// A number key jumps straight to a demo; one past the end is ignored.
	next, _ = m.Update(key(t, "3"))
	m = next.(model)
	if m.idx != 2 {
		t.Errorf("idx = %d, want the number key to jump to the third demo", m.idx)
	}
	before := m.idx
	next, _ = m.Update(key(t, "0"))
	m = next.(model)
	if m.idx != before {
		t.Errorf("idx = %d, want an out-of-range number ignored", m.idx)
	}
}

func TestGalleryForwardsOtherKeysToTheDemo(t *testing.T) {
	m := sized(t, 100, 30)
	before := m.View()

	// The first demo is a focused list, so j moves its cursor.
	next, cmd := m.Update(key(t, "j"))
	m = next.(model)
	if cmd != nil {
		t.Error("the gallery swallows the demo's command")
	}
	if m.View() == before {
		t.Error("j should have driven the focused demo")
	}
}

func TestGalleryQuits(t *testing.T) {
	m := sized(t, 100, 30)
	for _, k := range []string{"q", "ctrl+c"} {
		_, cmd := m.Update(key(t, k))
		if cmd == nil {
			t.Fatalf("%s should quit", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s produced %T, want tea.QuitMsg", k, cmd())
		}
	}
}

// TestGalleryIgnoresNonKeyMessages guards the default arm of Update.
func TestGalleryIgnoresNonKeyMessages(t *testing.T) {
	m := sized(t, 100, 30)
	next, cmd := m.Update(struct{ tea.Msg }{})
	if cmd != nil || next.(model).idx != m.idx {
		t.Error("an unrelated message should change nothing")
	}
}

// key builds the KeyMsg whose String() is s, which is what Update switches on.
func key(t *testing.T, s string) tea.KeyMsg {
	t.Helper()
	switch s {
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}
