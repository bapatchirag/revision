package app

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

// TestRetryRecedesTheErrorAtOnce pins what R does to a failed status load: the
// error dims and its "Press R to retry." gives way to the retry, in Main and in
// the status bar, the moment the key lands. svn may take the whole deadline to
// answer, so a screen left as it was reads as a key that did nothing.
func TestRetryRecedesTheErrorAtOnce(t *testing.T) {
	// Emit ANSI so the dimming is observable, then restore the Ascii profile.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	const failure = "svn status --xml: timed out after 30s"
	errorLine := func(t *testing.T, m *Model) string {
		t.Helper()
		for _, ln := range strings.Split(m.mainText, "\n") {
			if strings.Contains(stripANSI(ln), failure) {
				return ln
			}
		}
		t.Fatalf("Main should keep the failure on screen, got:\n%s", stripANSI(m.mainText))
		return ""
	}
	barState := func(m *Model) string {
		return strings.TrimRight(stripANSI(m.bar.View()), " ")
	}

	for _, tc := range []struct {
		name  string
		items []svn.StatusItem
	}{
		{"first load", nil},
		{"later reload", []svn.StatusItem{{Path: "alpha.txt", State: svn.StateModified}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := sizedModel(t)
			if tc.items != nil {
				m = loadItems(t, m, tc.items)
			}
			next, _ := m.Update(errMsg{err: errors.New(failure)})
			m = next.(*Model)
			if ln := errorLine(t, m); stripANSI(ln) != ln {
				t.Errorf("a failure waiting on R should not be dimmed, got %q", ln)
			}
			if !strings.Contains(m.mainText, "Press R to retry.") || !strings.HasSuffix(barState(m), "error") {
				t.Fatalf("expected the failure to offer a retry, got main:\n%s\nbar: %q", stripANSI(m.mainText), barState(m))
			}

			m, cmd := pressRune(t, m, 'R')
			if cmd == nil {
				t.Fatal("R should re-run the load")
			}
			if ln := errorLine(t, m); stripANSI(ln) == ln {
				t.Errorf("the failure being retried should be dimmed, got plain %q", ln)
			}
			main := stripANSI(m.mainText)
			if strings.Contains(main, "Press R to retry.") || !strings.Contains(main, "Retrying…") {
				t.Errorf("Main should report the retry in place of the prompt for it, got:\n%s", main)
			}
			if bar := barState(m); !strings.HasSuffix(bar, "loading…") {
				t.Errorf("status bar should report the load in flight, got %q", bar)
			}

			// The retry failing again puts the failure and its prompt back as they were.
			next, _ = m.Update(errMsg{err: errors.New(failure)})
			m = next.(*Model)
			if ln := errorLine(t, m); stripANSI(ln) != ln {
				t.Errorf("a failed retry should restore the failure undimmed, got %q", ln)
			}
			if main := stripANSI(m.mainText); strings.Contains(main, "Retrying…") || !strings.Contains(main, "Press R to retry.") {
				t.Errorf("a failed retry should offer R again, got:\n%s", main)
			}
			if bar := barState(m); !strings.HasSuffix(bar, "error") {
				t.Errorf("status bar should report the failure again, got %q", bar)
			}

			// A retry that lands clears the failure altogether.
			m, _ = pressRune(t, m, 'R')
			m = loadItems(t, m, tc.items)
			if main := stripANSI(m.mainText); strings.Contains(main, failure) || strings.Contains(main, "Retrying…") {
				t.Errorf("a successful retry should clear the failure, got:\n%s", main)
			}
			if bar := barState(m); strings.HasSuffix(bar, "error") || strings.HasSuffix(bar, "loading…") {
				t.Errorf("status bar should be idle after a successful retry, got %q", bar)
			}
		})
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
