package component_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/component"
)

func TestSearchBarWordNavigation(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	s.Focus()
	s.SetValue("foo bar/baz") // cursor parked at the end

	s.Update(keyAltLeft())
	s.Update(runes("|"))
	if s.Value() != "foo bar/|baz" {
		t.Fatalf("alt+left: value = %q, want foo bar/|baz", s.Value())
	}

	s.SetValue("foo bar/baz")
	s.Update(keyCtrlLeft())
	s.Update(keyCtrlLeft())
	s.Update(runes("|"))
	if s.Value() != "foo |bar/baz" {
		t.Fatalf("ctrl+left twice: value = %q, want foo |bar/baz", s.Value())
	}

	s.SetValue("foo bar/baz")
	s.Update(keyHome())
	s.Update(keyAltRight())
	s.Update(runes("|"))
	if s.Value() != "foo| bar/baz" {
		t.Fatalf("alt+right: value = %q, want foo| bar/baz", s.Value())
	}
}

// macOS Terminal sends option+←/→ as the meta-prefixed alt+b/alt+f rather than
// as a modified arrow, so those must move the cursor instead of typing "b"/"f".
func TestSearchBarMetaLettersMoveByWord(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	s.Focus()
	s.SetValue("foo bar")

	s.Update(altRunes("b"))
	s.Update(runes("|"))
	if s.Value() != "foo |bar" {
		t.Fatalf("alt+b: value = %q, want foo |bar", s.Value())
	}

	s.SetValue("foo bar")
	s.Update(keyHome())
	s.Update(altRunes("f"))
	s.Update(runes("|"))
	if s.Value() != "foo| bar" {
		t.Errorf("alt+f: value = %q, want foo| bar", s.Value())
	}
}

func TestSearchBarWordDelete(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
		want string
	}{
		{"alt+backspace", keyAltBackspace(), "foo bar/"},
		{"ctrl+w", keyCtrlW(), "foo bar/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := component.NewSearchBar("search", testTheme(), testKeys())
			s.Focus()
			s.SetValue("foo bar/baz")
			s.Update(tt.key)
			if s.Value() != tt.want {
				t.Errorf("value = %q, want %q", s.Value(), tt.want)
			}
		})
	}
}

func TestSearchBarWordDeleteForward(t *testing.T) {
	s := component.NewSearchBar("search", testTheme(), testKeys())
	s.Focus()
	s.SetValue("foo bar baz")
	s.Update(keyHome())

	s.Update(keyAltDelete())
	if s.Value() != " bar baz" {
		t.Fatalf("alt+delete: value = %q, want %q", s.Value(), " bar baz")
	}
	s.Update(altRunes("d"))
	if s.Value() != " baz" {
		t.Errorf("alt+d: value = %q, want %q", s.Value(), " baz")
	}
}

func TestPromptWordEditing(t *testing.T) {
	p := component.NewPrompt("name", "Changelist", "", testTheme(), testKeys())
	p.Focus()
	p.SetValue("feature/new-api")

	p.Update(keyAltLeft())
	p.Update(runes("|"))
	if p.Value() != "feature/new-|api" {
		t.Fatalf("alt+left: value = %q, want feature/new-|api", p.Value())
	}

	p.SetValue("feature/new-api")
	p.Update(keyAltBackspace())
	if p.Value() != "feature/new-" {
		t.Errorf("alt+backspace: value = %q, want feature/new-", p.Value())
	}
}

// A secret value is rendered as bullets, so word motion must not walk its word
// boundaries — the cursor jumps to the ends of the value instead.
func TestPromptSecretWordEditingJumpsToEnds(t *testing.T) {
	p := component.NewPrompt("pass", "Passphrase", "", testTheme(), testKeys())
	p.Focus()
	p.SetSecret(true)
	p.SetValue("hunter two")

	p.Update(keyAltLeft())
	p.Update(runes("X"))
	if p.Value() != "Xhunter two" {
		t.Fatalf("alt+left in secret mode: value = %q, want Xhunter two", p.Value())
	}

	p.SetValue("hunter two")
	p.Update(keyAltBackspace())
	if p.Value() != "" {
		t.Errorf("alt+backspace in secret mode: value = %q, want empty", p.Value())
	}
}

func TestFormWordEditing(t *testing.T) {
	f := testForm()
	f.Focus()
	f.Update(runes("src/main.go"))

	f.Update(keyAltLeft())
	f.Update(runes("|"))
	if got := f.Value(0); got != "src/main.|go" {
		t.Fatalf("alt+left: field 0 = %q, want src/main.|go", got)
	}

	f.Update(keyAltRight())
	f.Update(keyAltBackspace())
	if got := f.Value(0); got != "src/main.|" {
		t.Fatalf("alt+backspace: field 0 = %q, want src/main.|", got)
	}

	f.Update(keyCtrlLeft()) // back over "main", which alt+delete then removes
	f.Update(keyAltDelete())
	if got := f.Value(0); got != "src/.|" {
		t.Errorf("alt+delete: field 0 = %q, want src/.|", got)
	}
}

func TestTextAreaWordEditing(t *testing.T) {
	ta := component.NewTextArea("commit", "Commit", "", testTheme(), testKeys())
	ta.Focus()
	ta.SetValue("fix the parser\nrefactor lexer")

	ta.Update(keyAltLeft())
	ta.Update(runes("|"))
	if ta.Value() != "fix the parser\nrefactor |lexer" {
		t.Fatalf("alt+left: value = %q", ta.Value())
	}

	ta.SetValue("fix the parser\nrefactor lexer")
	ta.Update(keyAltBackspace())
	if ta.Value() != "fix the parser\nrefactor " {
		t.Fatalf("alt+backspace: value = %q", ta.Value())
	}

	// At the start of a row the motion steps back onto the previous one.
	ta.SetValue("fix the parser\nrefactor lexer")
	ta.Update(keyAltLeft())
	ta.Update(keyAltLeft())
	ta.Update(keyAltLeft())
	ta.Update(runes("|"))
	if ta.Value() != "fix the |parser\nrefactor lexer" {
		t.Errorf("alt+left across rows: value = %q", ta.Value())
	}
}

func TestTextAreaWordDeleteForward(t *testing.T) {
	ta := component.NewTextArea("commit", "Commit", "", testTheme(), testKeys())
	ta.Focus()
	ta.SetValue("fix parser\ndrop lexer")
	for i := 0; i < 5; i++ {
		ta.Update(keyAltLeft()) // walk back to the very start of the buffer
	}

	ta.Update(keyAltDelete())
	if ta.Value() != " parser\ndrop lexer" {
		t.Fatalf("alt+delete: value = %q", ta.Value())
	}
	ta.Update(keyCtrlRight())
	ta.Update(keyAltDelete()) // at the end of the row: pull the next row up
	if ta.Value() != " parserdrop lexer" {
		t.Errorf("alt+delete at row end: value = %q", ta.Value())
	}
}
