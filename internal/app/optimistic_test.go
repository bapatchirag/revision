package app

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/svn"
)

// itemState reads a file's state and changelist out of the model, so a test can
// assert what the Files panel is showing without going through the rendering.
func itemState(t *testing.T, m *Model, path string) svn.StatusItem {
	t.Helper()
	for _, it := range m.fileItems {
		if it.Path == path {
			return it
		}
	}
	t.Fatalf("no status item for %q", path)
	return svn.StatusItem{}
}

// pressSpace stages or unstages the current Files selection, failing the test
// when the keypress produces no command to run.
func pressSpace(t *testing.T, m *Model) (*Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if cmd == nil {
		t.Fatal("expected a stage command")
	}
	return next.(*Model), cmd
}

// TestOptimisticStagingMovesTheModelFirst is the table for the three mutations
// space and n apply ahead of svn: each must be visible in the status items on
// the same frame as the keypress.
func TestOptimisticStagingMovesTheModelFirst(t *testing.T) {
	tests := []struct {
		name  string
		items []svn.StatusItem
		act   func(t *testing.T, m *Model) *Model
		path  string
		want  svn.StatusItem
	}{
		{
			name:  "stage a modified file",
			items: []svn.StatusItem{{Path: "a.go", State: svn.StateModified}},
			act:   func(t *testing.T, m *Model) *Model { next, _ := pressSpace(t, m); return next },
			path:  "a.go",
			want:  svn.StatusItem{Path: "a.go", State: svn.StateModified, Changelist: stagedChangelist},
		},
		{
			name:  "unstage a staged file",
			items: []svn.StatusItem{{Path: "a.go", State: svn.StateModified, Changelist: stagedChangelist}},
			act:   func(t *testing.T, m *Model) *Model { next, _ := pressSpace(t, m); return next },
			path:  "a.go",
			want:  svn.StatusItem{Path: "a.go", State: svn.StateModified},
		},
		{
			name:  "stage an unversioned file adds it too",
			items: []svn.StatusItem{{Path: "new.txt", State: svn.StateUnversioned}},
			act:   func(t *testing.T, m *Model) *Model { next, _ := pressSpace(t, m); return next },
			path:  "new.txt",
			want:  svn.StatusItem{Path: "new.txt", State: svn.StateAdded, Changelist: stagedChangelist},
		},
		{
			name:  "assign the staged set to a named changelist",
			items: []svn.StatusItem{{Path: "a.go", State: svn.StateModified, Changelist: stagedChangelist}},
			act: func(t *testing.T, m *Model) *Model {
				m.nameTargets = m.stagedTargets()
				if cmd := m.submitChangelist("feature-x"); cmd == nil {
					t.Fatal("expected an assign command")
				}
				return m
			},
			path: "a.go",
			want: svn.StatusItem{Path: "a.go", State: svn.StateModified, Changelist: "feature-x"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.act(t, loadItems(t, sizedModel(t), tc.items))
			if got := itemState(t, m, tc.path); got != tc.want {
				t.Errorf("status item = %+v, want %+v", got, tc.want)
			}
			if m.optimistic == nil {
				t.Error("expected the pre-change status to be kept for a rollback")
			}
		})
	}
}

// TestOptimisticStageSettlesSilentlyOnSuccess covers the confirmation: svn
// agreeing with what is already on screen must move nothing, and must ask for a
// status reload to reconcile.
func TestOptimisticStageSettlesSilentlyOnSuccess(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.go", State: svn.StateModified}})
	m, _ = pressSpace(t, m)
	token := m.optimisticTok

	next, cmd := m.Update(stagedMsg{outcome: singleOutcome("a.go", nil), token: token})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected a status reload to reconcile the optimistic change")
	}
	if got := itemState(t, m, "a.go"); got.Changelist != stagedChangelist {
		t.Errorf("a confirmed change must stay on screen, got %+v", got)
	}
	if m.optimistic != nil {
		t.Error("a settled change should no longer hold a rollback snapshot")
	}
}

// TestOptimisticStageRollsBackOnFailure is the other half: svn refusing the
// change must put the row back exactly as it was and say so.
func TestOptimisticStageRollsBackOnFailure(t *testing.T) {
	before := []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified},
		{Path: "b.go", State: svn.StateModified, Changelist: "feature"},
	}
	m := loadItems(t, sizedModel(t), append([]svn.StatusItem(nil), before...))
	m, _ = pressSpace(t, m)
	token := m.optimisticTok

	next, cmd := m.Update(stagedMsg{outcome: singleOutcome("a.go", errors.New("locked")), token: token})
	m = next.(*Model)
	for i, want := range before {
		if got := itemState(t, m, want.Path); got != want {
			t.Errorf("item %d = %+v, want the pre-change %+v", i, got, want)
		}
	}
	if m.optimistic != nil {
		t.Error("a rolled-back change should no longer hold a snapshot")
	}
	if cmd == nil {
		t.Error("a failure should still ask svn for the real status")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "stage failed") {
		t.Errorf("expected a failure toast, got:\n%s", view)
	}
}

// TestOptimisticStageIgnoresSupersededFailure covers a reply that arrives after
// svn has reported a newer status: there is nothing left to undo, so the model
// must be left alone rather than rolled back to a stale snapshot.
func TestOptimisticStageIgnoresSupersededFailure(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "a.go", State: svn.StateModified}})
	m, _ = pressSpace(t, m)
	token := m.optimisticTok

	// A status reload lands first, making the snapshot two steps out of date.
	m = loadItems(t, m, []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: stagedChangelist},
	})
	next, _ := m.Update(stagedMsg{outcome: singleOutcome("a.go", errors.New("locked")), token: token})
	m = next.(*Model)

	if got := itemState(t, m, "a.go"); got.Changelist != stagedChangelist {
		t.Errorf("a superseded failure must not undo a newer status, got %+v", got)
	}
}

// TestOptimisticUpdatesDisabledWaitsForSvn locks the escape hatch: with the
// setting off, nothing moves until the status reload arrives.
func TestOptimisticUpdatesDisabledWaitsForSvn(t *testing.T) {
	cfg := config.Default()
	cfg.OptimisticUpdates = false
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{{Path: "a.go", State: svn.StateModified}})
	m, _ = pressSpace(t, m)

	if got := itemState(t, m, "a.go"); got.Changelist != "" {
		t.Errorf("with optimistic updates off the model must not move first, got %+v", got)
	}
	if m.optimistic != nil {
		t.Error("no snapshot should be taken when nothing was applied")
	}
}

// TestOptimisticDirectoryStageMovesEveryFileUnder covers the directory fan-out:
// one keypress restyles every stageable row beneath it.
func TestOptimisticDirectoryStageMovesEveryFileUnder(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "src/a.go", State: svn.StateModified},
		{Path: "src/b.go", State: svn.StateModified},
		{Path: "readme.md", State: svn.StateModified},
	})
	selectDirRow(t, m, "src")
	m, _ = pressSpace(t, m)

	for _, path := range []string{"src/a.go", "src/b.go"} {
		if got := itemState(t, m, path); got.Changelist != stagedChangelist {
			t.Errorf("%s should be staged on the keypress, got %+v", path, got)
		}
	}
	if got := itemState(t, m, "readme.md"); got.Changelist != "" {
		t.Errorf("a file outside src/ should be untouched, got %+v", got)
	}
}

// TestFileTreeRebuildKeepsCursorOnTheSameFile covers the select-by-path restore:
// a rebuild that reorders rows must leave the cursor on the file the user was
// on, not on the row number it happened to occupy.
func TestFileTreeRebuildKeepsCursorOnTheSameFile(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "beta.go", State: svn.StateModified},
		{Path: "alpha.go", State: svn.StateModified},
	})
	selectFileRow(t, m, "alpha.go")

	// A reload reporting the same files in the other order moves alpha.go's row.
	m = loadItems(t, m, []svn.StatusItem{
		{Path: "alpha.go", State: svn.StateModified},
		{Path: "beta.go", State: svn.StateModified},
	})
	n, ok := m.files.Selected()
	if !ok || n.Path != "alpha.go" {
		t.Errorf("cursor = %+v (ok=%v), want it still on alpha.go", n, ok)
	}
}

// selectFileRow parks the Changes-tree cursor on the row for the named file.
func selectFileRow(t *testing.T, m *Model, path string) {
	t.Helper()
	for i, n := range m.files.Items() {
		if n.Item != nil && n.Path == path {
			m.files.SetIndex(i)
			return
		}
	}
	t.Fatalf("no file row for %q in the file tree", path)
}
