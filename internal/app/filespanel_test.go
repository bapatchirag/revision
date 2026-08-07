package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
