package app

import (
	"errors"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

// pendingPaths lists, in a stable order, the rows the model is showing as
// waiting on svn.
func pendingPaths(m *Model) []string {
	paths := make([]string, 0, len(m.pendingOps))
	for p := range m.pendingOps {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// confirmPrompt accepts the confirmation modal on screen the way the user does,
// returning the model and the command the accepted action produced.
func confirmPrompt(t *testing.T, m *Model) (*Model, tea.Cmd) {
	t.Helper()
	if !m.confirming {
		t.Fatal("no confirmation modal is open")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a ConfirmMsg command from the modal")
	}
	conf, ok := cmd().(uimsg.ConfirmMsg)
	if !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
	next, cmd := m.Update(conf)
	return next.(*Model), cmd
}

// requestAndConfirm presses a destructive-action key on the current selection
// and accepts the confirmation it opens, returning the model and the command the
// accepted action produced.
func requestAndConfirm(t *testing.T, m *Model, k rune) (*Model, tea.Cmd) {
	t.Helper()
	m, _ = pressRune(t, m, k)
	return confirmPrompt(t, m)
}

var pendingItems = []svn.StatusItem{
	{Path: "src/a.go", State: svn.StateModified},
	{Path: "src/b.go", State: svn.StateModified},
	{Path: "readme.md", State: svn.StateModified, Changelist: stagedChangelist},
}

// TestPendingMarksTargetsOnDispatch is the table for the three destructive
// actions: each must mark exactly the rows it acts on, and only once the user
// has said yes.
func TestPendingMarksTargetsOnDispatch(t *testing.T) {
	tests := []struct {
		name string
		act  func(t *testing.T, m *Model) *Model
		want []string
	}{
		{
			name: "revert a file",
			act: func(t *testing.T, m *Model) *Model {
				selectFileRow(t, m, "src/a.go")
				next, _ := requestAndConfirm(t, m, 'r')
				return next
			},
			want: []string{"src/a.go"},
		},
		{
			name: "revert a directory",
			act: func(t *testing.T, m *Model) *Model {
				selectDirRow(t, m, "src")
				next, _ := requestAndConfirm(t, m, 'r')
				return next
			},
			want: []string{"src/a.go", "src/b.go"},
		},
		{
			name: "delete a file",
			act: func(t *testing.T, m *Model) *Model {
				selectFileRow(t, m, "src/b.go")
				next, _ := requestAndConfirm(t, m, 'd')
				return next
			},
			want: []string{"src/b.go"},
		},
		{
			name: "delete a directory",
			act: func(t *testing.T, m *Model) *Model {
				selectDirRow(t, m, "src")
				next, _ := requestAndConfirm(t, m, 'd')
				return next
			},
			want: []string{"src/a.go", "src/b.go"},
		},
		{
			name: "commit the staged set",
			act: func(t *testing.T, m *Model) *Model {
				next, cmd := m.Update(uimsg.SubmitMsg{ID: commitEditorID, Value: "ship it"})
				if cmd == nil {
					t.Fatal("expected a commit command")
				}
				return next.(*Model)
			},
			want: []string{"readme.md"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.act(t, loadItems(t, sizedModel(t), pendingItems))
			if got := pendingPaths(m); strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("pending rows = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPendingIsNotMarkedBeforeConfirming covers the other half of the prompt: a
// destructive action dims nothing until it has actually been dispatched, and a
// cancelled prompt dims nothing at all.
func TestPendingIsNotMarkedBeforeConfirming(t *testing.T) {
	m := loadItems(t, sizedModel(t), pendingItems)
	selectFileRow(t, m, "src/a.go")
	m, _ = pressRune(t, m, 'r')
	if !m.confirming {
		t.Fatal("expected the revert confirmation to open")
	}
	if got := pendingPaths(m); len(got) != 0 {
		t.Errorf("an unanswered prompt marked %v, want nothing", got)
	}

	// Esc emits DismissMsg, which drops the held rows without marking them.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ := m.Update(cmd())
	m = next.(*Model)
	if got := pendingPaths(m); len(got) != 0 {
		t.Errorf("a cancelled prompt marked %v, want nothing", got)
	}
	if m.pendingHold != nil {
		t.Error("a cancelled prompt should not keep rows to mark")
	}
}

// TestPendingClearsWhenSvnAnswers covers settling: success and failure alike
// release the rows, and only the rows that action started.
func TestPendingClearsWhenSvnAnswers(t *testing.T) {
	tests := []struct {
		name  string
		reply func(token uint64) tea.Msg
	}{
		{"success", func(token uint64) tea.Msg { return revertedMsg{path: "src/a.go", token: token} }},
		{"failure", func(token uint64) tea.Msg {
			return revertedMsg{path: "src/a.go", token: token, err: errors.New("svn: E155007")}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := loadItems(t, sizedModel(t), pendingItems)
			selectFileRow(t, m, "src/a.go")
			m, _ = requestAndConfirm(t, m, 'r')
			token := m.pendingTok
			if !m.isPending("src/a.go") {
				t.Fatal("expected src/a.go to be marked before the reply")
			}

			next, _ := m.Update(tc.reply(token))
			m = next.(*Model)
			if got := pendingPaths(m); len(got) != 0 {
				t.Errorf("pending rows = %v, want none once svn has answered", got)
			}
		})
	}
}

// TestPendingSurvivesAnotherActionsReply guards the token: a reply settles only
// the rows its own action marked.
func TestPendingSurvivesAnotherActionsReply(t *testing.T) {
	m := loadItems(t, sizedModel(t), pendingItems)
	selectFileRow(t, m, "src/a.go")
	m, _ = requestAndConfirm(t, m, 'r')
	first := m.pendingTok

	selectFileRow(t, m, "src/b.go")
	m, _ = requestAndConfirm(t, m, 'r')

	next, _ := m.Update(revertedMsg{path: "src/a.go", token: first})
	m = next.(*Model)
	if want := []string{"src/b.go"}; strings.Join(pendingPaths(m), ",") != strings.Join(want, ",") {
		t.Errorf("pending rows = %v, want %v still in flight", pendingPaths(m), want)
	}
}

// TestPendingRowIsInert covers the guard against acting twice: while svn is
// working on a row, the keys that would move it do nothing.
func TestPendingRowIsInert(t *testing.T) {
	for _, k := range []rune{'r', 'd', 'n'} {
		t.Run(string(k), func(t *testing.T) {
			m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.go", State: svn.StateModified}})
			m.markPending(pendingDelete, 1, []string{"a.go"})
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{k}})
			m = next.(*Model)
			if cmd != nil {
				t.Errorf("%q on a pending row produced a command", k)
			}
			if m.confirming || m.naming {
				t.Errorf("%q on a pending row opened an overlay", k)
			}
		})
	}

	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.go", State: svn.StateModified}})
	m.markPending(pendingDelete, 1, []string{"a.go"})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace}); cmd != nil {
		t.Error("space on a pending row produced a stage command")
	}
}

// TestPendingRowsAreSkippedByDirectoryActions is the directory counterpart: a
// fan-out acts on what is free to move and leaves the rest alone.
func TestPendingRowsAreSkippedByDirectoryActions(t *testing.T) {
	m := loadItems(t, sizedModel(t), pendingItems)
	m.markPending(pendingDelete, 1, []string{"src/a.go"})

	selectDirRow(t, m, "src")
	m, _ = requestAndConfirm(t, m, 'r')
	if op, ok := m.pendingOps["src/a.go"]; !ok || op.kind != pendingDelete {
		t.Errorf("src/a.go = %+v (ok=%v), want its delete untouched", op, ok)
	}
	if op, ok := m.pendingOps["src/b.go"]; !ok || op.kind != pendingRevert {
		t.Errorf("src/b.go = %+v (ok=%v), want a revert in flight", op, ok)
	}
}

// TestCommitKeyIsHeldWhileCommitRuns covers the second guard on commit: the
// staged set is already on its way, so the editor cannot be reopened for it.
func TestCommitKeyIsHeldWhileCommitRuns(t *testing.T) {
	m := loadItems(t, sizedModel(t), pendingItems)
	next, _ := m.Update(uimsg.SubmitMsg{ID: commitEditorID, Value: "ship it"})
	m = next.(*Model)

	m, _ = pressRune(t, m, 'c')
	if m.editing {
		t.Error("the commit editor should not reopen while a commit is running")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "commit is already running") {
		t.Errorf("expected a commit-in-flight toast, got:\n%s", view)
	}

	// Once svn answers, the key works again.
	next, _ = m.Update(committedMsg{revision: "51", token: m.pendingTok})
	m = next.(*Model)
	if m.commitPending() {
		t.Fatal("the commit should have settled")
	}
}

// TestPendingDisabledWaitsForSvn covers the setting: with optimistic updates off
// nothing is marked, so the screen only moves when svn answers.
func TestPendingDisabledWaitsForSvn(t *testing.T) {
	cfg := config.Default()
	cfg.OptimisticUpdates = false
	m := loadItems(t, sizedModelCfg(t, cfg), pendingItems)

	selectFileRow(t, m, "src/a.go")
	m, cmd := requestAndConfirm(t, m, 'r')
	if cmd == nil {
		t.Fatal("expected a revert command")
	}
	if got := pendingPaths(m); len(got) != 0 {
		t.Errorf("pending rows = %v, want none with optimistic updates off", got)
	}
}

func TestPendingRowsGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), pendingItems)
	m.markPending(pendingRevert, 1, []string{"src/a.go", "src/b.go"})
	golden.RequireEqual(t, []byte(m.View()))
}
