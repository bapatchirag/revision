package app

import (
	"strings"
	"testing"

	"github.com/bapatchirag/revision/internal/svn"
)

// nodeTag renders a node as "<depth><d|f> <name>" for compact ordering asserts.
func nodeTag(n fileNode) string {
	kind := "d"
	if n.Item != nil {
		kind = "f"
	}
	return strings.Repeat(" ", n.Depth) + kind + " " + n.Name
}

func TestBuildFileTreeOrdersDirsBeforeFiles(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "README.md", State: svn.StateModified},
		{Path: "internal/app/app.go", State: svn.StateModified},
		{Path: "internal/svn/client.go", State: svn.StateModified},
	}
	got := make([]string, 0, len(items)*2)
	for _, n := range buildFileTree(items, nil) {
		got = append(got, nodeTag(n))
	}
	want := []string{
		"d /",
		" d internal",
		"  d app",
		"   f app.go",
		"  d svn",
		"   f client.go",
		" f README.md",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("tree order mismatch:\n got:\n%s\nwant:\n%s",
			strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBuildFileTreeCarriesFilePointer(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "dir/leaf.go", State: svn.StateModified, Changelist: stagedChangelist},
	}
	rows := buildFileTree(items, nil)
	// rows: [dir, leaf]; the leaf must point at the original item so file actions
	// resolve through it.
	leaf := rows[len(rows)-1]
	if leaf.Item == nil {
		t.Fatal("expected the leaf row to carry a StatusItem pointer")
	}
	if leaf.Item.Path != "dir/leaf.go" || leaf.Item.Changelist != stagedChangelist {
		t.Errorf("leaf item = %+v, want the original dir/leaf.go item", *leaf.Item)
	}
	if leaf.Name != "leaf.go" {
		t.Errorf("leaf name = %q, want basename %q", leaf.Name, "leaf.go")
	}
}

func TestBuildFileTreeCollapseHidesDescendants(t *testing.T) {
	items := []svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
		{Path: "top.txt", State: svn.StateModified},
	}
	rows := buildFileTree(items, map[string]bool{"internal": true})

	var names []string
	var internalNode fileNode
	for _, n := range rows {
		names = append(names, n.Name)
		if n.Path == "internal" {
			internalNode = n
		}
	}
	if !internalNode.Collapsed {
		t.Error("the internal/ row should be marked collapsed")
	}
	for _, hidden := range []string{"app", "app.go"} {
		for _, name := range names {
			if name == hidden {
				t.Errorf("collapsed internal/ should hide %q, rows: %v", hidden, names)
			}
		}
	}
	// Sibling content outside the collapsed directory stays visible.
	var sawTop bool
	for _, name := range names {
		if name == "top.txt" {
			sawTop = true
		}
	}
	if !sawTop {
		t.Errorf("top.txt should remain visible, rows: %v", names)
	}
}

func TestBuildFileTreeRootRow(t *testing.T) {
	rows := buildFileTree([]svn.StatusItem{
		{Path: "README.md", State: svn.StateModified},
	}, nil)
	if len(rows) != 2 {
		t.Fatalf("expected a / root row plus the file, got %d rows", len(rows))
	}
	root := rows[0]
	if root.Item != nil || root.Name != "/" || root.Path != fileTreeRoot || root.Depth != 0 {
		t.Errorf("first row should be the / root, got %+v", root)
	}
	// A top-level file nests one level under the root.
	leaf := rows[1]
	if leaf.Item == nil || leaf.Depth != 1 || leaf.Name != "README.md" {
		t.Errorf("root file should sit at depth 1 under /, got %+v", leaf)
	}
}

func TestBuildFileTreeEmptyHasNoRoot(t *testing.T) {
	if rows := buildFileTree(nil, nil); rows != nil {
		t.Errorf("an empty working copy should yield no rows, got %v", rows)
	}
}

func TestBuildFileTreeCollapseRootHidesEverything(t *testing.T) {
	rows := buildFileTree([]svn.StatusItem{
		{Path: "internal/app/app.go", State: svn.StateModified},
		{Path: "README.md", State: svn.StateModified},
	}, map[string]bool{fileTreeRoot: true})
	if len(rows) != 1 || rows[0].Path != fileTreeRoot || !rows[0].Collapsed {
		t.Fatalf("collapsing / should leave only the collapsed root row, got %+v", rows)
	}
}
