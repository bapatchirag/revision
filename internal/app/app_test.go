package app

import (
	"os"
	"regexp"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	// Theme switches call theme.ApplyColorProfile, which would otherwise force
	// TrueColor and break the golden suite's deterministic Ascii output.
	theme.DisableColorProfile = true
	os.Exit(m.Run())
}

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

func sizedModel(t testing.TB) *Model {
	t.Helper()
	return sizedModelCfg(t, config.Default())
}

func sizedModelCfg(t testing.TB, cfg config.Config) *Model {
	t.Helper()
	info := &svn.Info{
		URL:             "https://svn.example.com/repo/trunk",
		WorkingCopyRoot: "/home/alice/work/wc",
		Revision:        "42",
	}
	m := New(svn.New("/home/alice/work/wc"), info, selfupdate.Build{}, cfg)
	m.workDir = "/home/alice/work/wc"
	m.refreshChrome()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func loadItems(t testing.TB, m *Model, items []svn.StatusItem) *Model {
	t.Helper()
	next, _ := m.Update(statusLoadedMsg{items: items})
	return next.(*Model)
}

func pressRune(t *testing.T, m *Model, r rune) (*Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return next.(*Model), cmd
}
