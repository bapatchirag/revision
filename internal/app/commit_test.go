package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

func TestCommitRequiresStagedFiles(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified},
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if cmd != nil {
		t.Error("commit with nothing staged should not run a command")
	}
	if m.editing {
		t.Error("the editor should not open with nothing staged")
	}
}

func TestCommitEditorOpensAndSubmits(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if !m.editing {
		t.Fatal("the editor should open with a staged file")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Commit message") {
		t.Errorf("expected the commit editor to overlay the layout, got:\n%s", view)
	}

	// Type a message; ctrl+s makes the editor emit SubmitMsg.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("do it")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected a SubmitMsg command from the editor")
	}
	sub, ok := cmd().(uimsg.SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", cmd())
	}
	if sub.Value != "do it" {
		t.Errorf("submitted value = %q, want %q", sub.Value, "do it")
	}

	// Handing the SubmitMsg back closes the editor and yields a commit command.
	next, cmd = m.Update(sub)
	m = next.(*Model)
	if m.editing {
		t.Error("the editor should close after submit")
	}
	if cmd == nil {
		t.Error("expected a commit command after submit")
	}
}

// TestCommitRefusesAChangelistSvnWillNotTake pins the guard in front of the one
// operation that really is all-or-nothing: svn commits a changelist in a single
// transaction, so one conflicted or missing member sinks every other file in it.
// Catching that before the editor opens is the only place it can be caught.
func TestCommitRefusesAChangelistSvnWillNotTake(t *testing.T) {
	for _, state := range []svn.FileState{svn.StateConflicted, svn.StateMissing} {
		m := loadItems(t, sizedModel(t), []svn.StatusItem{
			{Path: "ok.go", State: svn.StateModified, Changelist: "revision:staged"},
			{Path: "bad.go", State: state, Changelist: "revision:staged"},
		})
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
		m = next.(*Model)
		if cmd != nil {
			t.Errorf("%s: a commit svn would refuse should run no command", state)
		}
		if m.editing {
			t.Errorf("%s: the editor should not open for a commit svn would refuse", state)
		}
		if view := stripANSI(m.View()); !strings.Contains(view, "bad.go") {
			t.Errorf("%s: expected the blocking file named, got:\n%s", state, view)
		}
	}
}

// TestCommitAllowsAConflictOutsideTheChangelist pins that the guard is about the
// files being committed, not the working copy at large: svn commits a changelist
// without minding what is wrong elsewhere.
func TestCommitAllowsAConflictOutsideTheChangelist(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "ok.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "elsewhere.go", State: svn.StateConflicted},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !next.(*Model).editing {
		t.Error("a conflict outside the changelist has no bearing on committing it")
	}
}

func TestCommitEditorCancels(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)

	// Esc emits DismissMsg, which the app handles to close the editor.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.editing {
		t.Error("the editor should close on cancel")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Commit message") {
		t.Error("the layout should return after cancelling the editor")
	}
}

func TestCommitResultShowsToast(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(committedMsg{revision: "128"})
	m = next.(*Model)
	if view := stripANSI(m.View()); !strings.Contains(view, "committed r128") {
		t.Errorf("expected the commit toast, got:\n%s", view)
	}
}

func TestCommitEditorGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "modified.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Fix status parsing")})
	golden.RequireEqual(t, []byte(m.View()))
}

func TestCommitResetsToFirstLogPage(t *testing.T) {
	m := logPagedModel(t)
	m, _ = pressRune(t, m, 'n')
	next, _ := m.Update(logLoadedMsg{page: 2, more: true, entries: []svn.LogEntry{{Revision: "48"}}})
	m = next.(*Model)

	next, _ = m.Update(committedMsg{revision: "51"})
	if got := next.(*Model).logPage; got != 1 {
		t.Errorf("logPage = %d after a commit, want 1", got)
	}
}

func TestCommitChangelistFromView(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if !m.editing {
		t.Fatal("c should open the commit editor for the selected changelist")
	}
	if m.commitCL != "feature" {
		t.Errorf("commit target = %q, want feature", m.commitCL)
	}
}

func TestCommitUnstagedGroupRefused(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified}, // no changelist → (unstaged)
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(*Model)
	if m.editing {
		t.Error("committing the unstaged group should be refused")
	}
	if cmd != nil {
		t.Error("no command should run for the unstaged group")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "isn't a changelist") {
		t.Errorf("expected a refusal toast, got:\n%s", view)
	}
}
