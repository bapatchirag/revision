package focus_test

import (
	"testing"

	"github.com/bapatchirag/revision/internal/tui/focus"
)

type fake struct{ focused bool }

func (f *fake) Focus()        { f.focused = true }
func (f *fake) Blur()         { f.focused = false }
func (f *fake) Focused() bool { return f.focused }

func TestNewFocusesFirst(t *testing.T) {
	a, b, c := &fake{}, &fake{}, &fake{}
	m := focus.New(a, b, c)
	if m.Index() != 0 || !a.focused || b.focused || c.focused {
		t.Fatalf("New should focus only the first component")
	}
}

func TestNextPrevCycle(t *testing.T) {
	a, b, c := &fake{}, &fake{}, &fake{}
	m := focus.New(a, b, c)

	m.Next()
	if m.Index() != 1 || !b.focused || a.focused || c.focused {
		t.Errorf("Next should focus the second component")
	}

	m.Prev()
	m.Prev()
	if m.Index() != 2 || !c.focused {
		t.Errorf("Prev should wrap to the last component, got index %d", m.Index())
	}
}

func TestFocusWraps(t *testing.T) {
	a, b, c := &fake{}, &fake{}, &fake{}
	m := focus.New(a, b, c)

	m.Focus(5) // 5 % 3 == 2
	if m.Index() != 2 || !c.focused {
		t.Errorf("Focus(5) should wrap to index 2, got %d", m.Index())
	}
	if m.Current() != c {
		t.Errorf("Current should return the focused component")
	}
}

func TestEmptyManagerIsSafe(t *testing.T) {
	m := focus.New()
	m.Next()
	m.Focus(3)
	if m.Current() != nil || m.Len() != 0 {
		t.Errorf("empty manager should be a no-op")
	}
}
