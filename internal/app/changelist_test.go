package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

func TestChangelistGrouping(t *testing.T) {
	groups := groupChangelists([]svn.StatusItem{
		{Path: "z.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "a.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "b.go", State: svn.StateModified},
		{Path: "c.go", State: svn.StateModified, Changelist: "alpha"},
	})
	// Named changelists first (alphabetical), then staged, then the unstaged default.
	want := []string{"alpha", "feature", "(staged)", "(unstaged)"}
	if len(groups) != len(want) {
		t.Fatalf("want %d groups, got %d: %+v", len(want), len(groups), groups)
	}
	for i, w := range want {
		if groups[i].Label() != w {
			t.Errorf("group %d = %q, want %q", i, groups[i].Label(), w)
		}
	}
	if !groups[0].Committable() {
		t.Error("a named changelist should be committable")
	}
	if groups[3].Committable() {
		t.Error("the unstaged default group should not be committable")
	}
}

func TestChangelistGroupingSkipsContainerDirectoryEntries(t *testing.T) {
	groups := groupChangelists([]svn.StatusItem{
		{Path: "usp/lib/libwmiclient/h", State: svn.StateUnversioned},
		{Path: "usp/lib/libwmiclient/h/file.c", State: svn.StateAdded, Changelist: stagedChangelist},
	})
	if len(groups) != 1 {
		t.Fatalf("expected only one group after skipping container dir entry, got %d: %+v", len(groups), groups)
	}
	if groups[0].Name != stagedChangelist || len(groups[0].Items) != 1 || groups[0].Items[0].Path != "usp/lib/libwmiclient/h/file.c" {
		t.Fatalf("unexpected groups after container-dir filtering: %+v", groups)
	}
}

func TestChangelistItemsSkipsContainerDirectoryEntries(t *testing.T) {
	m := sizedModel(t)
	m.fileItems = []svn.StatusItem{
		{Path: "src", State: svn.StateUnversioned},
		{Path: "src/a.go", State: svn.StateAdded, Changelist: stagedChangelist},
		{Path: "src/b.go", State: svn.StateModified, Changelist: "feature"},
	}
	if unstaged := m.changelistItems(""); len(unstaged) != 0 {
		t.Fatalf("container directory entry should not appear in unstaged drill, got: %+v", unstaged)
	}
	staged := m.changelistItems(stagedChangelist)
	if len(staged) != 1 || staged[0].Path != "src/a.go" {
		t.Fatalf("staged drill should include only src/a.go, got: %+v", staged)
	}
}

func TestFilesViewSwitchesToChangelists(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "b.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	// Files panel is focused by default; ] cycles to the Changelists view.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	if cmd != nil {
		next, _ = m.Update(cmd()) // deliver ViewSelectedMsg
		m = next.(*Model)
	}
	if name := m.filesViews.ActiveName(); name != "Changelists" {
		t.Fatalf("active files view = %q, want Changelists", name)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "feature") || !strings.Contains(view, "(staged)") {
		t.Errorf("the changelists view should list the groups, got:\n%s", view)
	}
}

func TestAssignChangelistPromptAndSubmit(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "mod.go", State: svn.StateModified},
	})
	// n opens the changelist-name prompt.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("n should open the changelist-name prompt")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Changelist name") {
		t.Errorf("expected the name prompt, got:\n%s", view)
	}

	// Type a name; enter submits it (single-line input).
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("feature-x")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a SubmitMsg from the name prompt")
	}
	sub, ok := cmd().(uimsg.SubmitMsg)
	if !ok || sub.ID != changelistEditorID {
		t.Fatalf("expected a changelist SubmitMsg, got %T (%+v)", cmd(), cmd())
	}
	if sub.Value != "feature-x" {
		t.Errorf("submitted name = %q, want feature-x", sub.Value)
	}

	next, cmd = m.Update(sub)
	m = next.(*Model)
	if m.naming {
		t.Error("the prompt should close after submit")
	}
	if cmd == nil {
		t.Error("expected an assign command after submit")
	}
}

func TestAssignChangelistAllowsStagedFile(t *testing.T) {
	// A file in the anonymous staged bucket can be moved into a named changelist.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "staged.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("a staged file should be assignable to a named changelist")
	}
}

func TestAssignChangelistNamesAllStagedFiles(t *testing.T) {
	// Naming a changelist while several files are staged moves the whole staged
	// set as a unit, not just the highlighted file; an unstaged file is left out.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "b.go", State: svn.StateModified, Changelist: "revision:staged"},
		{Path: "c.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("n should open the changelist-name prompt when files are staged")
	}
	got := map[string]bool{}
	for _, tgt := range m.nameTargets {
		got[tgt.path] = true
	}
	if len(m.nameTargets) != 2 || !got["a.go"] || !got["b.go"] {
		t.Errorf("nameTargets = %+v, want exactly the staged files a.go and b.go", m.nameTargets)
	}
}

func TestAssignChangelistFallsBackToSelectedFile(t *testing.T) {
	// With nothing staged, naming still targets just the selected file so the
	// single-file workflow keeps working.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "lone.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if !m.naming {
		t.Fatal("n should open the prompt for the selected file when nothing is staged")
	}
	if len(m.nameTargets) != 1 || m.nameTargets[0].path != "lone.go" {
		t.Errorf("nameTargets = %+v, want just lone.go", m.nameTargets)
	}
}

func TestAssignChangelistOffersExistingNames(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "loose.go", State: svn.StateModified},
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "b.go", State: svn.StateModified, Changelist: "revision:staged"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	view := stripANSI(m.View())
	if !strings.Contains(view, "Existing changelists:") || !strings.Contains(view, "feature") {
		t.Errorf("the prompt should list existing named changelists, got:\n%s", view)
	}
	// The anonymous buckets are not offered as pickable names.
	if strings.Contains(view, "(staged)") || strings.Contains(view, "(unstaged)") {
		t.Errorf("anonymous buckets should not appear as options, got:\n%s", view)
	}
}

func TestAssignChangelistGuardsNamedChangelist(t *testing.T) {
	// A file already in a *named* changelist cannot be reassigned (unstage first).
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	if m.naming {
		t.Error("a file already in a named changelist should not open the prompt")
	}
	if cmd != nil {
		t.Error("the guard should not produce a command")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "already in") {
		t.Errorf("expected an already-assigned guard toast, got:\n%s", view)
	}
}

func TestAssignChangelistCancels(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "mod.go", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*Model)
	// Esc emits DismissMsg, which the app handles to close the prompt.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.naming {
		t.Error("the prompt should close on cancel")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "Changelist name") {
		t.Error("the layout should return after cancelling the prompt")
	}
}

func TestChangelistDrillExpandsAndCollapses(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)

	// enter drills into the selected changelist.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from the changelists list")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() == 0 {
		t.Fatal("enter should drill into the changelist")
	}
	if m.drilledCL != "feature" {
		t.Errorf("drilled changelist = %q, want feature", m.drilledCL)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "a.go") {
		t.Errorf("the drill should list the changelist's files, got:\n%s", view)
	}

	// esc collapses back out.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a SubViewPoppedMsg on esc")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() != 0 {
		t.Error("esc should collapse the drill")
	}
	if m.drilledCL != "" {
		t.Error("the drilled changelist should be cleared on collapse")
	}
}

func TestChangelistDrillShowsTree(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "internal/svn/client.go", State: svn.StateModified, Changelist: "feature"},
	})
	// Switch to Changelists and drill into "feature".
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from the changelists list")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() == 0 {
		t.Fatal("expected to be drilled into the changelist")
	}

	// The drill renders the same "/"-rooted tree: a root row, directory rows, and
	// basename leaves (never a full nested path on one row).
	var names []string
	for _, n := range m.clFiles.Items() {
		names = append(names, n.Name)
		if n.Item != nil && strings.Contains(n.Name, "/") {
			t.Errorf("drill file leaf %q should be a basename", n.Name)
		}
	}
	for _, want := range []string{"/", "internal", "app", "svn", "app.go", "client.go"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("drill tree missing %q, got: %v", want, names)
		}
	}

	// Enter on the internal/ directory row collapses it, hiding its descendants.
	for i, n := range m.clFiles.Items() {
		if n.Name == "internal" {
			m.clFiles.SetIndex(i)
			break
		}
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg from enter on a drill directory")
	}
	next, _ = m.Update(cmd())
	m = next.(*Model)
	for _, n := range m.clFiles.Items() {
		if n.Name == "app.go" || n.Name == "client.go" {
			t.Errorf("collapsing internal/ in the drill should hide its files, got: %v", m.clFiles.Items())
		}
	}
}

func TestNamedChangelistFileShowsAccentMarker(t *testing.T) {
	// A named-changelist file is marked in the Changes view (distinct from the
	// staged bucket's marker), so both render the dot.
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "feature.go", State: svn.StateModified, Changelist: "feature"},
	})
	if view := stripANSI(m.View()); !strings.Contains(view, "●") {
		t.Errorf("expected a changelist marker in the files list, got:\n%s", view)
	}
}

func TestChangelistsViewGolden(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded, Changelist: "revision:staged"},
		{Path: "feature.go", State: svn.StateModified, Changelist: "feature-x"},
		{Path: "loose.txt", State: svn.StateModified},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}

func TestChangelistDrillLocksViewSwitch(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "a.go", State: svn.StateModified, Changelist: "feature"},
	})
	// Switch to Changelists, then drill in.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	if m.filesViews.Depth() == 0 {
		t.Fatal("expected to be drilled into the changelist")
	}

	// While drilled, [ and ] must not switch the Files view.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if name := m.filesViews.ActiveName(); name != "Changelists" {
		t.Errorf("view switched while drilled (now %q); it should stay locked", name)
	}
	if m.filesViews.Depth() == 0 {
		t.Error("the drill should remain open while view switching is locked")
	}
}

func TestChangelistDrillHeaderGolden(t *testing.T) {
	// Expanding a changelist labels the panel header with just the changelist
	// name (no tabs, no chevron).
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "feature.go", State: svn.StateModified, Changelist: "feature-x"},
		{Path: "other.go", State: svn.StateAdded, Changelist: "feature-x"},
	})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = next.(*Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}
