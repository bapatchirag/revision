package app

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// fileTreeRoot is the sentinel Path of the synthetic working-copy root row,
// shown as "/". A real status path is relative and never equals it, so it never
// collides with a directory or file.
const fileTreeRoot = "/"

// fileNode is one visible row in the Changes view's file tree. A node is either
// a directory (Item == nil) or a file leaf (Item != nil). Depth is the row's
// indentation level and Path is the full working-copy-relative path of the
// directory or file, which doubles as the stable key for collapse state.
type fileNode struct {
	Name      string
	Path      string
	Depth     int
	Collapsed bool
	Item      *svn.StatusItem
}

// treeDir is the mutable directory node used while assembling the tree, before
// it is flattened into display rows.
type treeDir struct {
	name  string
	path  string
	dirs  map[string]*treeDir
	names []string
	files []svn.StatusItem
}

// buildFileTree turns the flat, path-sorted status items into the flattened list
// of visible tree rows. Everything hangs off a single synthetic root row shown
// as "/" (the working-copy root): every path segment becomes a directory row and
// each file a leaf, indented by its depth beneath the root. Directories sort
// before files within a parent, both alphabetically. A directory whose path is
// present in collapsed hides its descendants (its own row still shows, marked
// collapsed); collapsing the root hides the whole tree. An empty working copy
// yields no rows at all.
func buildFileTree(items []svn.StatusItem, collapsed map[string]bool) []fileNode {
	if len(items) == 0 {
		return nil
	}

	root := &treeDir{dirs: map[string]*treeDir{}}
	for _, it := range items {
		parts := strings.Split(it.Path, "/")
		dir := root
		for _, seg := range parts[:len(parts)-1] {
			child, ok := dir.dirs[seg]
			if !ok {
				path := seg
				if dir.path != "" {
					path = dir.path + "/" + seg
				}
				child = &treeDir{name: seg, path: path, dirs: map[string]*treeDir{}}
				dir.dirs[seg] = child
				dir.names = append(dir.names, seg)
			}
			dir = child
		}
		dir.files = append(dir.files, it)
	}

	rootCollapsed := collapsed[fileTreeRoot]
	rows := []fileNode{{
		Name:      "/",
		Path:      fileTreeRoot,
		Depth:     0,
		Collapsed: rootCollapsed,
	}}
	if rootCollapsed {
		return rows
	}

	var walk func(d *treeDir, depth int)
	walk = func(d *treeDir, depth int) {
		sort.Strings(d.names)
		for _, name := range d.names {
			child := d.dirs[name]
			isCollapsed := collapsed[child.path]
			rows = append(rows, fileNode{
				Name:      name,
				Path:      child.path,
				Depth:     depth,
				Collapsed: isCollapsed,
			})
			if !isCollapsed {
				walk(child, depth+1)
			}
		}
		for i := range d.files {
			it := d.files[i]
			name := it.Path
			if slash := strings.LastIndex(name, "/"); slash >= 0 {
				name = name[slash+1:]
			}
			rows = append(rows, fileNode{
				Name:  name,
				Path:  it.Path,
				Depth: depth,
				Item:  &d.files[i],
			})
		}
	}
	// Children sit one level under the "/" root row.
	walk(root, 1)
	return rows
}

// firstFileIndex returns the index of the first file leaf in rows, or -1 when
// the tree holds no files (empty, or only directory rows).
func firstFileIndex(rows []fileNode) int {
	for i := range rows {
		if rows[i].Item != nil {
			return i
		}
	}
	return -1
}

// renderFileNode adapts a tree row for the reusable List: directory rows show a
// chevron (▾ expanded, ▸ collapsed) and the segment name with a trailing slash
// (the root row shows just "/"); file rows reuse the flat status rendering
// (marker + code + name) indented by depth.
func renderFileNode(th theme.Theme) func(fileNode) string {
	return func(n fileNode) string {
		indent := strings.Repeat("  ", n.Depth)
		if n.Item == nil {
			chevron := "▾"
			if n.Collapsed {
				chevron = "▸"
			}
			label := n.Name
			if !strings.HasSuffix(label, "/") {
				label += "/"
			}
			marker := lipgloss.NewStyle().Foreground(th.Muted).Render(chevron)
			name := lipgloss.NewStyle().Foreground(th.Info).Bold(true).Render(label)
			return indent + marker + " " + name
		}
		return indent + statusRow(th, *n.Item, n.Name)
	}
}
