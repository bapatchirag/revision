package component_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/msg"
)

func TestSearchBarEmitsSubmitAndDismiss(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	s.Focus()
	s.SetValue("rev:42 fix")

	submit := mustCmd(t, s.Update(keyEnter()))
	sub, ok := submit.(msg.SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", submit)
	}
	if sub.ID != "search" || sub.Value != "rev:42 fix" {
		t.Errorf("got %+v, want {search rev:42 fix}", sub)
	}

	dismiss := mustCmd(t, s.Update(keyEsc()))
	if d, ok := dismiss.(msg.DismissMsg); !ok || d.ID != "search" {
		t.Errorf("expected DismissMsg{search}, got %#v", dismiss)
	}
}

func TestSearchBarTypesNavLettersLiterally(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	s.Focus()
	// h/j/k/l/y/n are literal text in the filter, not navigation/confirm keys.
	s.Update(runes("hjklyn"))
	if s.Value() != "hjklyn" {
		t.Errorf("value = %q, want hjklyn (nav letters should be literal)", s.Value())
	}
}

func TestSearchBarEditing(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	s.Focus()
	s.Update(runes("abc"))
	s.Update(keyLeft()) // cursor between b and c
	s.Update(runes("X"))
	if s.Value() != "abXc" {
		t.Fatalf("insert at cursor: value = %q, want abXc", s.Value())
	}
	s.Update(keyBackspace())
	if s.Value() != "abc" {
		t.Fatalf("backspace: value = %q, want abc", s.Value())
	}
	s.Update(keyHome())
	s.Update(runes("_"))
	if s.Value() != "_abc" {
		t.Fatalf("home+insert: value = %q, want _abc", s.Value())
	}
	s.Update(keyEnd())
	s.Update(runes("!"))
	if s.Value() != "_abc!" {
		t.Fatalf("end+insert: value = %q, want _abc!", s.Value())
	}
}

func TestSearchBarIgnoresInputWhenBlurred(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	if cmd := s.Update(runes("x")); cmd != nil {
		t.Error("a blurred search bar should ignore key input")
	}
	if s.Value() != "" {
		t.Errorf("blurred search bar captured input: %q", s.Value())
	}
}

func TestSearchBarResetClearsValue(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	s.SetValue("preset")
	s.Reset()
	if s.Value() != "" {
		t.Errorf("reset should clear the value, got %q", s.Value())
	}
}

func TestSearchBarShowsPrefixAndValue(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	s.SetPrefix("log (rev: user:)")
	s.SetValue("fix bug")
	s.SetSize(50, 1)
	view := s.View()
	for _, want := range []string{"log (rev: user:)", "fix bug"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestGoldenSearchBar(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	s.SetPrefix("log (rev: user: path: date:)")
	s.SetSize(60, 1)
	s.Focus()
	s.SetValue("rev:42 fix crash")
	golden.RequireEqual(t, []byte(s.View()))
}
