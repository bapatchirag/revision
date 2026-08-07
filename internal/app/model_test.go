package app

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/svn"
)

func TestModelRendersStatus(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded, Changelist: "revision:staged"},
		{Path: "committed.txt", State: svn.StateModified},
	})

	view := stripANSI(m.View())
	for _, want := range []string{"added.txt", "committed.txt", "/home/alice/work/wc", "Branch", "trunk", "Revision", "r42"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n---\n%s", want, view)
		}
	}
}

func TestModelEmptyState(t *testing.T) {
	m := loadItems(t, sizedModel(t), nil)
	if view := stripANSI(m.View()); !strings.Contains(view, "clean") {
		t.Errorf("expected clean message, got:\n%s", view)
	}
}

func TestModelShowsError(t *testing.T) {
	m := sizedModel(t)
	next, _ := m.Update(errMsg{err: errors.New("kaboom")})
	m = next.(*Model)

	if view := stripANSI(m.View()); !strings.Contains(view, "kaboom") {
		t.Errorf("expected error in view, got:\n%s", view)
	}
}

func TestModelQuit(t *testing.T) {
	m := sizedModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected a command from quit key")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("expected tea.QuitMsg from quit key")
	}
}

// TestModelCloseReleasesSession proves the exit path leaves nothing retained,
// and that closing twice is harmless.
func TestModelCloseReleasesSession(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "alpha.txt", State: svn.StateModified},
	})
	next, _ := m.Update(diffLoadedMsg{path: "alpha.txt", diff: "@@ -1 +1 @@\n+alpha body"})
	m = next.(*Model)
	if m.session.diffs.Len() == 0 {
		t.Fatal("expected the loaded diff to be cached")
	}

	m.Close()
	m.Close()

	if n := m.session.diffs.Len(); n != 0 {
		t.Errorf("session holds %d diffs after Close, want 0", n)
	}
	if m.fileItems != nil || m.clItems != nil || m.logEntries != nil || m.savedDiffItems != nil {
		t.Error("Close should release the retained working-copy content")
	}
	if m.diffPath != "" || m.diffText != "" || m.mainText != "" {
		t.Error("Close should release the diff and rendered content on screen")
	}
}

// fileTreeHasPath reports whether the Changes tree currently holds a file leaf at
// the given path, so tests can assert an item is shown or hidden.
func fileTreeHasPath(m *Model, path string) bool {
	for _, n := range m.files.Items() {
		if n.Item != nil && n.Item.Path == path {
			return true
		}
	}
	return false
}

// subdirModel builds a model launched inside a subdirectory of the working copy,
// so the directory the svn client is rooted at reveals the display scope in use.
func subdirModel(t *testing.T, cfg config.Config) *Model {
	t.Helper()
	info := &svn.Info{
		URL:             "https://svn.example.com/repo/trunk/src",
		WorkingCopyRoot: "/home/alice/work/wc",
		Revision:        "42",
	}
	m := New(svn.New("/home/alice/work/wc/src"), info, selfupdate.Build{}, cfg)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func TestDisplayFromCWDKeepsLaunchDirectory(t *testing.T) {
	m := subdirModel(t, config.Default())
	if got := m.client.Dir; got != "/home/alice/work/wc/src" {
		t.Errorf("client.Dir = %q, want the launch directory", got)
	}
}

func TestDisplayFromRootRootsClientAtWorkingCopy(t *testing.T) {
	cfg := config.Default()
	cfg.DisplayFrom = config.DisplayFromRoot
	m := subdirModel(t, cfg)
	if got := m.client.Dir; got != "/home/alice/work/wc" {
		t.Errorf("client.Dir = %q, want the working-copy root", got)
	}
	if m.launchDir != "/home/alice/work/wc/src" {
		t.Errorf("launchDir = %q, want the directory revision was launched in", m.launchDir)
	}
	// The Status panel reports the directory the working copy is displayed from.
	if view := stripANSI(m.View()); !strings.Contains(view, "Source") {
		t.Errorf("expected the Status panel to name the source directory, got:\n%s", view)
	}
}

func TestConfigValidatorResetsUnknownTheme(t *testing.T) {
	validate := ConfigValidator()

	// A theme that no longer exists is a conflict: it resets to the default and
	// is reported so the user can be told.
	cfg := config.Default()
	cfg.Theme = "retired-theme"
	conflicts := validate(&cfg)
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %v, want exactly one", conflicts)
	}
	if cfg.Theme != config.Default().Theme {
		t.Errorf("Theme = %q, want reset to default %q", cfg.Theme, config.Default().Theme)
	}

	// A known theme and a blank theme are both left untouched with no conflict.
	for _, name := range []string{"dracula", ""} {
		cfg := config.Default()
		cfg.Theme = name
		if conflicts := validate(&cfg); len(conflicts) != 0 {
			t.Errorf("theme %q reported conflicts %v, want none", name, conflicts)
		}
		if cfg.Theme != name {
			t.Errorf("theme %q was modified to %q", name, cfg.Theme)
		}
	}
}

func TestStartupNoticeShowsToast(t *testing.T) {
	m := sizedModel(t)
	next, _ := m.Update(startupNoticeMsg{text: "config: logLimit 0 is invalid; reset to 100"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "logLimit 0 is invalid") {
		t.Errorf("expected the startup notice toast, got:\n%s", view)
	}
}

func TestStartupNoticeCmdEmitsMessage(t *testing.T) {
	msg := startupNoticeCmd("hello")()
	sn, ok := msg.(startupNoticeMsg)
	if !ok || sn.text != "hello" {
		t.Fatalf("startupNoticeCmd() = %#v, want startupNoticeMsg{text:%q}", msg, "hello")
	}
}

func TestSetStartupNoticeTrims(t *testing.T) {
	m := sizedModel(t)
	m.SetStartupNotice("  spaced  ")
	if m.startupNotice != "spaced" {
		t.Errorf("startupNotice = %q, want %q", m.startupNotice, "spaced")
	}
	// A blank notice (no conflicts to report) must clear to empty so Init
	// schedules nothing and no toast appears.
	m.SetStartupNotice("   ")
	if m.startupNotice != "" {
		t.Errorf("blank notice should clear to empty, got %q", m.startupNotice)
	}
}
