package keymap

import (
	"strings"
	"testing"
)

// TestDefaultBindingsAreComplete guards against a binding being added to the
// KeyMap struct and left without keys, which would silently do nothing.
func TestDefaultBindingsAreComplete(t *testing.T) {
	km := Default()
	bindings := map[string][]string{
		"Up":                km.Up.Keys(),
		"Down":              km.Down.Keys(),
		"Left":              km.Left.Keys(),
		"Right":             km.Right.Keys(),
		"PageUp":            km.PageUp.Keys(),
		"PageDown":          km.PageDown.Keys(),
		"Top":               km.Top.Keys(),
		"Bottom":            km.Bottom.Keys(),
		"LineStart":         km.LineStart.Keys(),
		"LineEnd":           km.LineEnd.Keys(),
		"WordLeft":          km.WordLeft.Keys(),
		"WordRight":         km.WordRight.Keys(),
		"DeleteWordLeft":    km.DeleteWordLeft.Keys(),
		"DeleteWordRight":   km.DeleteWordRight.Keys(),
		"Enter":             km.Enter.Keys(),
		"Confirm":           km.Confirm.Keys(),
		"Cancel":            km.Cancel.Keys(),
		"Back":              km.Back.Keys(),
		"Submit":            km.Submit.Keys(),
		"FocusNext":         km.FocusNext.Keys(),
		"FocusPrev":         km.FocusPrev.Keys(),
		"PrevView":          km.PrevView.Keys(),
		"NextView":          km.NextView.Keys(),
		"ToggleDirDiff":     km.ToggleDirDiff.Keys(),
		"ToggleUntracked":   km.ToggleUntracked.Keys(),
		"ToggleCmdLog":      km.ToggleCmdLog.Keys(),
		"ToggleLiveRefresh": km.ToggleLiveRefresh.Keys(),
		"SaveDiff":          km.SaveDiff.Keys(),
		"SplitDiff":         km.SplitDiff.Keys(),
		"OpenEditor":        km.OpenEditor.Keys(),
		"Filter":            km.Filter.Keys(),
		"Refresh":           km.Refresh.Keys(),
		"Settings":          km.Settings.Keys(),
		"ChangeDir":         km.ChangeDir.Keys(),
		"Help":              km.Help.Keys(),
		"Quit":              km.Quit.Keys(),
	}
	for name, keys := range bindings {
		if len(keys) == 0 {
			t.Errorf("%s has no keys bound", name)
		}
	}
}

// TestHelpSectionsAreWellFormed pins the shape of the reference table shared by
// the in-app "?" overlay and the website's keybindings page: every row needs the
// four fields both consumers render, and no two rows may name the same action
// twice within a section.
func TestHelpSectionsAreWellFormed(t *testing.T) {
	sections := HelpSections()
	if len(sections) == 0 {
		t.Fatal("HelpSections() is empty")
	}
	for _, s := range sections {
		if s.Title == "" {
			t.Error("a section has no title")
		}
		if len(s.Bindings) == 0 {
			t.Errorf("section %q has no bindings", s.Title)
		}
		seen := make(map[string]bool, len(s.Bindings))
		for _, b := range s.Bindings {
			switch {
			case b.Action == "":
				t.Errorf("section %q has a binding with no action", s.Title)
			case len(b.Keys) == 0:
				t.Errorf("%s / %s lists no keys", s.Title, b.Action)
			case b.Context == "":
				t.Errorf("%s / %s has no context", s.Title, b.Action)
			case b.Description == "":
				t.Errorf("%s / %s has no description", s.Title, b.Action)
			}
			if seen[b.Action] {
				t.Errorf("section %q lists %q twice", s.Title, b.Action)
			}
			seen[b.Action] = true
		}
	}
}

func TestKeyHint(t *testing.T) {
	cases := map[string]struct {
		binding Binding
		want    string
	}{
		"single key":     {Binding{Keys: []string{"q"}}, "q"},
		"default joiner": {Binding{Keys: []string{"r", "d"}}, "r / d"},
		"custom joiner":  {Binding{Keys: []string{"1", "2", "3"}, sep: " "}, "1 2 3"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.binding.KeyHint(); got != tc.want {
				t.Errorf("KeyHint() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestHelpSectionsRenderEveryKeyHint exercises KeyHint over the real table, so a
// row whose keys join into something empty is caught.
func TestHelpSectionsRenderEveryKeyHint(t *testing.T) {
	for _, s := range HelpSections() {
		for _, b := range s.Bindings {
			if strings.TrimSpace(b.KeyHint()) == "" {
				t.Errorf("%s / %s renders an empty key hint", s.Title, b.Action)
			}
		}
	}
}
