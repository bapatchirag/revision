package app

import (
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
)

func TestCommandLogShownByDefault(t *testing.T) {
	m := sizedModel(t)
	if !m.showCmdLog {
		t.Fatal("command log should be shown by default")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Command Log") {
		t.Errorf("default view should include the Command Log panel:\n%s", view)
	}
}

func TestNewWiresRecorderToClient(t *testing.T) {
	m := sizedModel(t)
	if m.client.Recorder == nil {
		t.Fatal("New should install a command-log recorder on the client")
	}
}

func TestCommandLogShowsCommandAndSuccessNotOutput(t *testing.T) {
	m := sizedModel(t)
	m.cmdLog.record(svn.CommandRecord{
		Command:    "svn commit --file /tmp/msg --non-interactive",
		Subcommand: "commit",
		Output:     "Committed revision 51.",
	})
	// A subsequent message drives the refresh, just as a real command completion
	// would; syncCommandLog runs at the top of Update.
	next, _ := m.Update(statusLoadedMsg{})
	m = next.(*Model)

	view := stripANSI(m.cmdLogView.View())
	if !strings.Contains(view, "svn commit --file /tmp/msg --non-interactive") {
		t.Errorf("command log missing the command:\n%s", view)
	}
	if strings.Contains(view, "Committed revision") {
		t.Errorf("command log should not show command output:\n%s", view)
	}
	if !strings.Contains(view, "✓") {
		t.Errorf("command log should mark a successful command:\n%s", view)
	}
}

func TestCommandLogMarksFailureWithoutErrorText(t *testing.T) {
	m := sizedModel(t)
	m.cmdLog.record(svn.CommandRecord{
		Command:    "svn commit --non-interactive",
		Subcommand: "commit",
		Err:        "svn: E155011: is out of date",
	})
	m.syncCommandLog()

	view := stripANSI(m.cmdLogView.View())
	if !strings.Contains(view, "✗") {
		t.Errorf("command log should mark a failed command:\n%s", view)
	}
	if strings.Contains(view, "out of date") {
		t.Errorf("command log should not show the error text, only success/failure:\n%s", view)
	}
}

func TestCommandLogOmitsReadOnlyCommands(t *testing.T) {
	m := sizedModel(t)
	// The client recorder is the filter: read-only queries are dropped, actions
	// are kept.
	m.client.Recorder(svn.CommandRecord{Command: "svn diff a.go --non-interactive", Subcommand: "diff"})
	m.client.Recorder(svn.CommandRecord{Command: "svn status --xml --non-interactive", Subcommand: "status"})
	m.client.Recorder(svn.CommandRecord{Command: "svn revert a.go --non-interactive", Subcommand: "revert"})
	m.syncCommandLog()

	view := stripANSI(m.cmdLogView.View())
	if strings.Contains(view, "diff") || strings.Contains(view, "status") {
		t.Errorf("command log should omit diff/status read-only queries:\n%s", view)
	}
	if !strings.Contains(view, "svn revert a.go --non-interactive") {
		t.Errorf("command log should keep the revert action:\n%s", view)
	}
}

func TestCommandLogKeepsUserRequestedReadOnlyCommands(t *testing.T) {
	m := sizedModel(t)
	// Writing a patch out runs the same subcommand the Files panel loads on its
	// own; only the one the user asked for is reported.
	m.client.Recorder(svn.CommandRecord{Command: "svn diff a.go --non-interactive", Subcommand: "diff"})
	m.client.Recorder(svn.CommandRecord{Command: "svn diff b.go --non-interactive", Subcommand: "diff", UserAction: true})
	m.syncCommandLog()

	view := stripANSI(m.cmdLogView.View())
	if strings.Contains(view, "diff a.go") {
		t.Errorf("an automatic diff should stay out of the command log:\n%s", view)
	}
	if !strings.Contains(view, "svn diff b.go --non-interactive") {
		t.Errorf("a user-requested diff should be reported:\n%s", view)
	}
}

func TestFocusCommandLogPanel(t *testing.T) {
	m := sizedModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	m = next.(*Model)
	if m.focus.Index() != panelCmdLog {
		t.Fatalf("pressing 4 should focus the command log, got index %d", m.focus.Index())
	}
}

func TestFocusFourRevealsHiddenCommandLog(t *testing.T) {
	m := sizedModel(t)
	// Hide it, then focus by number: it should reappear and take focus.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	m = next.(*Model)
	if !m.showCmdLog {
		t.Error("focusing the command log should reveal it when hidden")
	}
	if m.focus.Index() != panelCmdLog {
		t.Errorf("pressing 4 should focus the command log, got index %d", m.focus.Index())
	}
}

func TestHidingFocusedCommandLogMovesFocusToMain(t *testing.T) {
	m := sizedModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}}) // focus it
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}) // hide it
	m = next.(*Model)
	if m.focus.Index() != panelMain {
		t.Errorf("hiding the focused command log should move focus to Main, got index %d", m.focus.Index())
	}
}

func TestTabSkipsHiddenCommandLog(t *testing.T) {
	m := sizedModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}) // hide it
	m = next.(*Model)
	m.focus.Focus(panelMain)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // cycle forward
	m = next.(*Model)
	if m.focus.Index() == panelCmdLog {
		t.Error("Tab should skip the command log while it is hidden")
	}
}

func TestToggleCommandLogHidesAndShows(t *testing.T) {
	m := sizedModel(t)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(*Model)
	if m.showCmdLog {
		t.Fatal("x should hide the command log")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Command Log") {
		t.Errorf("a hidden command log should not render its title:\n%s", view)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(*Model)
	if !m.showCmdLog {
		t.Fatal("x should show the command log again")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Command Log") {
		t.Errorf("the command log should render again after a second toggle:\n%s", view)
	}
}

func TestCommandLogRecordIsConcurrencySafe(t *testing.T) {
	l := newCommandLog(50)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.record(svn.CommandRecord{Command: "svn status"})
		}()
	}
	wg.Wait()

	if l.seq() != 100 {
		t.Errorf("seq = %d, want 100", l.seq())
	}
	if got := len(l.snapshot()); got != 50 {
		t.Errorf("snapshot length = %d, want 50 (ring capped at the limit)", got)
	}
}
