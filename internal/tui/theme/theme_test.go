package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestByNameResolvesBuiltins(t *testing.T) {
	for _, name := range []string{"auto", "everforest", "dracula", "nord", "gruvbox", "cipher"} {
		if _, ok := ByName(name); !ok {
			t.Errorf("ByName(%q) ok = false, want true", name)
		}
	}
}

func TestByNameDefaultAliasesAuto(t *testing.T) {
	got, ok := ByName("default")
	if !ok {
		t.Fatal(`ByName("default") ok = false, want true`)
	}
	if got != Auto() {
		t.Errorf(`ByName("default") = %+v, want Auto() %+v`, got, Auto())
	}
}

func TestByNameIsCaseInsensitiveAndTrims(t *testing.T) {
	got, ok := ByName("  NORD  ")
	if !ok {
		t.Fatal(`ByName("  NORD  ") ok = false, want true`)
	}
	if got != Nord() {
		t.Error(`ByName("  NORD  ") did not resolve to Nord()`)
	}
}

func TestByNameAliases(t *testing.T) {
	cases := map[string]Theme{
		"default": Auto(),
		"green":   Everforest(),
		"purple":  Dracula(),
		"blue":    Nord(),
		"gold":    Gruvbox(),
	}
	for alias, want := range cases {
		got, ok := ByName(alias)
		if !ok {
			t.Errorf("ByName(%q) ok = false, want true", alias)
		}
		if got != want {
			t.Errorf("ByName(%q) did not resolve to its canonical theme", alias)
		}
	}
}

func TestByNameUnknownFallsBackToAuto(t *testing.T) {
	got, ok := ByName("nope")
	if ok {
		t.Error(`ByName("nope") ok = true, want false`)
	}
	if got != Auto() {
		t.Error(`ByName("nope") did not fall back to Auto()`)
	}
}

func TestApplyColorProfileForcesTrueColorForNamedThemes(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	// Start from a downsampling profile so a change is observable.
	lipgloss.SetColorProfile(termenv.ANSI256)
	for _, name := range []string{"everforest", "dracula", "nord", "gruvbox", "cipher"} {
		lipgloss.SetColorProfile(termenv.ANSI256)
		ApplyColorProfile(name)
		if got := lipgloss.ColorProfile(); got != termenv.TrueColor {
			t.Errorf("ApplyColorProfile(%q) profile = %v, want TrueColor", name, got)
		}
	}
}

func TestApplyColorProfileAutoRestoresDetected(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	// Force a non-detected profile, then auto must restore the detected one.
	lipgloss.SetColorProfile(termenv.TrueColor)
	for _, name := range []string{"auto", "default", "  AUTO  "} {
		lipgloss.SetColorProfile(termenv.TrueColor)
		ApplyColorProfile(name)
		if got := lipgloss.ColorProfile(); got != detectedProfile {
			t.Errorf("ApplyColorProfile(%q) profile = %v, want detected %v", name, got, detectedProfile)
		}
	}
}

func TestApplyColorProfileDisabledIsNoOp(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() {
		DisableColorProfile = false
		lipgloss.SetColorProfile(prev)
	})

	DisableColorProfile = true
	lipgloss.SetColorProfile(termenv.ANSI256)
	ApplyColorProfile("cipher")
	if got := lipgloss.ColorProfile(); got != termenv.ANSI256 {
		t.Errorf("disabled ApplyColorProfile changed profile to %v, want ANSI256", got)
	}
}

func TestDefaultIsAuto(t *testing.T) {
	if Default() != Auto() {
		t.Error("Default() != Auto()")
	}
}

func TestAllOrderedAutoFirst(t *testing.T) {
	all := All()
	if len(all) != 6 {
		t.Fatalf("len(All()) = %d, want 6", len(all))
	}
	if all[0].Name != "auto" {
		t.Errorf("All()[0].Name = %q, want auto", all[0].Name)
	}
	for _, n := range all {
		if n.Label == "" {
			t.Errorf("theme %q has an empty Label", n.Name)
		}
	}
}

func TestNamesMatchRegistry(t *testing.T) {
	names := Names()
	all := All()
	if len(names) != len(all) {
		t.Fatalf("len(Names()) = %d, len(All()) = %d", len(names), len(all))
	}
	for i := range names {
		if names[i] != all[i].Name {
			t.Errorf("Names()[%d] = %q, want %q", i, names[i], all[i].Name)
		}
	}
}

func TestPalettesHaveNoEmptyFields(t *testing.T) {
	for _, n := range All() {
		th := n.Theme
		fields := map[string]lipgloss.Color{
			"Text":          th.Text,
			"Muted":         th.Muted,
			"Accent":        th.Accent,
			"Selection":     th.Selection,
			"SelectionBg":   th.SelectionBg,
			"Border":        th.Border,
			"BorderFocused": th.BorderFocused,
			"Success":       th.Success,
			"Warning":       th.Warning,
			"Error":         th.Error,
			"Info":          th.Info,
		}
		for field, c := range fields {
			if c == "" {
				t.Errorf("theme %q field %s is empty", n.Name, field)
			}
		}
	}
}
