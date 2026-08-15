package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

func BenchmarkRebuildFileTree(b *testing.B) {
	items := make([]svn.StatusItem, 0, 500)
	for i := 0; i < 500; i++ {
		items = append(items, svn.StatusItem{
			Path:  fmt.Sprintf("internal/pkg%02d/sub%02d/file%03d.go", i%10, i%7, i),
			State: svn.StateModified,
		})
	}
	m := loadItems(b, sizedModel(b), items)
	b.Run("uncached", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = buildFileTree(items, m.collapsedDirs)
		}
	})
	b.Run("cached", func(b *testing.B) {
		m.fileTree(items, m.collapsedDirs)
		for i := 0; i < b.N; i++ {
			_ = m.fileTree(items, m.collapsedDirs)
		}
	})
}

func TestCountLabel(t *testing.T) {
	tests := []struct {
		name               string
		index, shown, full int
		want               string
	}{
		{"empty view", 0, 0, 0, ""},
		{"all hidden", 0, 0, 29, "0 of 0 (29)"},
		{"nothing hidden", 1, 3, 3, "1 of 3"},
		{"mid selection", 2, 4, 4, "2 of 4"},
		{"cursor on root", 0, 3, 3, "0 of 3"},
		{"some hidden", 1, 16, 29, "1 of 16 (29)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countLabel(tt.index, tt.shown, tt.full); got != tt.want {
				t.Errorf("countLabel(%d, %d, %d) = %q, want %q", tt.index, tt.shown, tt.full, got, tt.want)
			}
		})
	}
}

func TestFilesFooterReportsHiddenCount(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "tracked.go", State: svn.StateModified},
		{Path: "untracked1.txt", State: svn.StateUnversioned},
		{Path: "untracked2.txt", State: svn.StateUnversioned},
	})

	// With everything visible the footer counts rows without a bracketed total.
	if got := m.filesFooter(); strings.Contains(got, "(") {
		t.Errorf("footer shows a hidden count with nothing hidden: %q", got)
	}

	// Hiding untracked files drops two leaves; the footer then reports the full
	// count in brackets, and the rendered Files panel border carries it.
	m.hideUntracked = true
	m.rebuildFileTree()
	got := m.filesFooter()
	if !strings.Contains(got, "(") {
		t.Errorf("footer should report a bracketed full count when untracked files are hidden, got %q", got)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, got) {
		t.Errorf("Files panel border missing footer %q\n%s", got, view)
	}
}

// ruleModel is a model whose Changes view hides anything under build/.
func ruleModel(t *testing.T, items []svn.StatusItem) *Model {
	t.Helper()
	cfg := config.Default()
	cfg.HideRules = []config.HideRule{{Pattern: "^build/", Enabled: true}}
	return loadItems(t, sizedModelCfg(t, cfg), items)
}

func TestHideRulesKeepMatchesOutOfTheChangesTree(t *testing.T) {
	m := ruleModel(t, []svn.StatusItem{
		{Path: "build/gen.go", State: svn.StateModified},
		{Path: "src/a.go", State: svn.StateModified},
	})

	if fileTreeHasPath(m, "build/gen.go") {
		t.Error("a file a rule matches should not be in the Changes tree")
	}
	if !fileTreeHasPath(m, "src/a.go") {
		t.Error("a file no rule matches should still be in the Changes tree")
	}
	// The directory row goes with its only file rather than lingering empty.
	for _, n := range m.files.Items() {
		if n.Name == "build" {
			t.Error("the directory of a hidden file should not be left behind")
		}
	}
}

func TestHideRulesLeaveTheRestOfTheAppAlone(t *testing.T) {
	m := ruleModel(t, []svn.StatusItem{
		{Path: "build/gen.go", State: svn.StateModified, Changelist: "feature"},
		{Path: "src/a.go", State: svn.StateModified, Changelist: "feature"},
	})

	// svn's own view of the working copy is untouched, so a hidden file is still
	// committed, staged and reverted with everything else.
	if len(m.fileItems) != 2 {
		t.Errorf("fileItems = %d, want both files (rules only narrow the view)", len(m.fileItems))
	}
	if got := len(m.changelistItems("feature")); got != 2 {
		t.Errorf("changelist drill holds %d files, want both", got)
	}
	groups := groupChangelists(m.filteredStatusItems(m.fileItems))
	if len(groups) != 1 || len(groups[0].Items) != 2 {
		t.Errorf("changelists view = %+v, want one group of both files", groups)
	}
}

func TestDisabledHideRuleHidesNothing(t *testing.T) {
	cfg := config.Default()
	cfg.HideRules = []config.HideRule{{Pattern: "^build/", Enabled: false}}
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "build/gen.go", State: svn.StateModified},
	})

	if !fileTreeHasPath(m, "build/gen.go") {
		t.Error("a rule that is turned off should hide nothing")
	}
	if m.hideRulesActive() {
		t.Error("hideRulesActive should be false with every rule off")
	}
}

func TestHideRulesMatchAnywhereInThePath(t *testing.T) {
	cfg := config.Default()
	cfg.HideRules = []config.HideRule{{Pattern: `\.class$`, Enabled: true}}
	m := loadItems(t, sizedModelCfg(t, cfg), []svn.StatusItem{
		{Path: "src/deep/Main.class", State: svn.StateUnversioned},
		{Path: "src/deep/Main.java", State: svn.StateModified},
	})

	if fileTreeHasPath(m, "src/deep/Main.class") {
		t.Error("an unanchored pattern should match anywhere in the path")
	}
	if !fileTreeHasPath(m, "src/deep/Main.java") {
		t.Error("the sibling that does not match should stay")
	}
}

func TestFilesFooterCountsFilesHiddenByRule(t *testing.T) {
	m := ruleModel(t, []svn.StatusItem{
		{Path: "build/gen.go", State: svn.StateModified},
		{Path: "src/a.go", State: svn.StateModified},
	})

	if got, want := m.filesFooter(), "1 of 1 (2)"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
}

func TestChangesTreeShowsDirectoryTree(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
		{Path: "internal/svn/client.go", State: svn.StateModified},
		{Path: "README.md", State: svn.StateModified},
	})
	// Inspect the built tree rows directly, independent of the panel's visible
	// window: every path segment is its own row and files are basenames.
	var names []string
	for _, n := range m.files.Items() {
		names = append(names, n.Name)
		if n.Item != nil && strings.Contains(n.Name, "/") {
			t.Errorf("file leaf %q should be a basename, not a nested path", n.Name)
		}
	}
	for _, want := range []string{"/", "internal", "app", "svn", "app.go", "client.go", "README.md"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tree rows missing %q, got: %v", want, names)
		}
	}
}

func TestEnterCollapsesDirectory(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
		{Path: "internal/svn/client.go", State: svn.StateModified},
	})
	if view := stripANSI(m.View()); !strings.Contains(view, "app.go") {
		t.Fatalf("expected file leaves visible before collapse, got:\n%s", view)
	}

	// The cursor opens on the first file; move it onto the internal/ directory row.
	for i, n := range m.files.Items() {
		if n.Name == "internal" {
			m.files.SetIndex(i)
			break
		}
	}

	// Enter emits an ActivatedMsg the model turns into a collapse toggle.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an ActivatedMsg command from enter on a directory")
	}
	act, ok := cmd().(uimsg.ActivatedMsg)
	if !ok {
		t.Fatalf("expected ActivatedMsg, got %T", cmd())
	}
	next, _ := m.Update(act)
	m = next.(*Model)

	// Collapsing internal/ hides its descendants but keeps the directory row.
	view := stripANSI(m.View())
	if strings.Contains(view, "app.go") || strings.Contains(view, "client.go") {
		t.Errorf("collapsing internal/ should hide its descendants, got:\n%s", view)
	}
	if !strings.Contains(view, "internal/") {
		t.Errorf("the collapsed directory row should remain, got:\n%s", view)
	}
}
