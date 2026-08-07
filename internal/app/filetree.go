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

// pathRow is one visible row of a tree built from slash-separated paths: either
// a directory (Item == nil) or a leaf carrying the item it was built from. Depth
// is the row's indentation level and Path is the full relative path of the
// directory or leaf, which doubles as the stable key for collapse state.
type pathRow[T any] struct {
	Name      string
	Path      string
	Depth     int
	Collapsed bool
	Item      *T
}

// fileNode is one visible row in the Changes view's file tree, over working-copy
// status items.
type fileNode = pathRow[svn.StatusItem]

// treeDir is the mutable directory node used while assembling the tree, before
// it is flattened into display rows.
type treeDir[T any] struct {
	name  string
	path  string
	dirs  map[string]*treeDir[T]
	names []string
	files []T
}

// buildFileTree turns the flat, path-sorted status items into the flattened list
// of visible tree rows.
func buildFileTree(items []svn.StatusItem, collapsed map[string]bool) []fileNode {
	return buildPathTree(items, func(it svn.StatusItem) string { return it.Path }, collapsed)
}

// buildPathTree turns a flat list of items, each named by pathOf, into the
// flattened list of visible tree rows. Everything hangs off a single synthetic
// root row shown as "/": every path segment becomes a directory row and each
// item a leaf, indented by its depth beneath the root. Directories sort before
// files within a parent, directories alphabetically and leaves in the order they
// were given. A directory whose path is present in collapsed hides its
// descendants (its own row still shows, marked collapsed); collapsing the root
// hides the whole tree. No items yields no rows at all.
func buildPathTree[T any](items []T, pathOf func(T) string, collapsed map[string]bool) []pathRow[T] {
	if len(items) == 0 {
		return nil
	}

	root := &treeDir[T]{dirs: map[string]*treeDir[T]{}}
	for _, it := range items {
		parts := strings.Split(pathOf(it), "/")
		dir := root
		for _, seg := range parts[:len(parts)-1] {
			child, ok := dir.dirs[seg]
			if !ok {
				path := seg
				if dir.path != "" {
					path = dir.path + "/" + seg
				}
				child = &treeDir[T]{name: seg, path: path, dirs: map[string]*treeDir[T]{}}
				dir.dirs[seg] = child
				dir.names = append(dir.names, seg)
			}
			dir = child
		}
		dir.files = append(dir.files, it)
	}

	rootCollapsed := collapsed[fileTreeRoot]
	rows := []pathRow[T]{{
		Name:      "/",
		Path:      fileTreeRoot,
		Depth:     0,
		Collapsed: rootCollapsed,
	}}
	if rootCollapsed {
		return rows
	}

	var walk func(d *treeDir[T], depth int)
	walk = func(d *treeDir[T], depth int) {
		sort.Strings(d.names)
		for _, name := range d.names {
			child := d.dirs[name]
			isCollapsed := collapsed[child.path]
			rows = append(rows, pathRow[T]{
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
			path := pathOf(d.files[i])
			name := path
			if slash := strings.LastIndex(name, "/"); slash >= 0 {
				name = name[slash+1:]
			}
			// An item can name a directory that also has child entries (for example,
			// an added parent plus added files beneath it). The directory already
			// renders as a directory row, so skip the duplicate leaf.
			if _, ok := d.dirs[name]; ok {
				continue
			}
			rows = append(rows, pathRow[T]{
				Name:  name,
				Path:  path,
				Depth: depth,
				Item:  &d.files[i],
			})
		}
	}
	// Children sit one level under the "/" root row.
	walk(root, 1)
	return rows
}

// fileTree is buildFileTree memoized over the session: the same status items
// with the same directories collapsed yield the rows built for them last time.
// The tree is re-derived whenever the Files panel is rebuilt and again for the
// footer's count on every frame, so the work is otherwise repeated many times
// over a row set that has not moved — a filter keystroke that narrows nothing,
// or a redraw. The returned rows are shared, so callers must treat them (and the
// status items they point at) as read-only.
func (m *Model) fileTree(items []svn.StatusItem, collapsed map[string]bool) []fileNode {
	key := treeKey{items: itemsDigest(items), collapsed: collapsedDigest(collapsed)}
	if rows, ok := m.session.Tree(key); ok {
		return rows
	}
	rows := buildFileTree(items, collapsed)
	m.session.PutTree(key, rows)
	return rows
}

// firstFileIndex returns the index of the first leaf in rows, or -1 when the
// tree holds no leaves (empty, or only directory rows).
func firstFileIndex[T any](rows []pathRow[T]) int {
	for i := range rows {
		if rows[i].Item != nil {
			return i
		}
	}
	return -1
}

// leafCount returns the number of leaves (rows carrying an item) among rows,
// ignoring the synthetic root and directory rows.
func leafCount[T any](rows []pathRow[T]) int {
	n := 0
	for i := range rows {
		if rows[i].Item != nil {
			n++
		}
	}
	return n
}

// fileLeafStats returns a tree position indicator: the number of leaves at or
// before cursor — a 1-based position when the cursor rests on a leaf, or the
// count of leaves already passed when it rests on a directory (0 on the root) —
// together with the total leaf count.
func fileLeafStats[T any](rows []pathRow[T], cursor int) (index, count int) {
	for i := range rows {
		if rows[i].Item == nil {
			continue
		}
		count++
		if i <= cursor {
			index = count
		}
	}
	return index, count
}

// dirLabel is a directory row's display label: its segment name with a single
// trailing slash (the "/" root already carries one).
func dirLabel[T any](n pathRow[T]) string {
	if strings.HasSuffix(n.Name, "/") {
		return n.Name
	}
	return n.Name + "/"
}

// filesUnder returns the status items beneath a directory row n: the whole set
// for the "/" root, otherwise every item whose path lies under n.Path.
func filesUnder(n fileNode, items []svn.StatusItem) []svn.StatusItem {
	if n.Path == fileTreeRoot {
		return items
	}
	prefix := n.Path + "/"
	var out []svn.StatusItem
	for _, it := range items {
		if strings.HasPrefix(it.Path, prefix) {
			out = append(out, it)
		}
	}
	return out
}

// renderFileNode adapts a tree row for the reusable List: directory rows show a
// chevron (▾ expanded, ▸ collapsed) and the segment name with a trailing slash
// (the root row shows just "/"); file rows reuse the flat status rendering
// (marker + code + name) indented by depth. pending reports how many files the
// row covers that svn is still working on: a file leaf is dimmed and marked, a
// directory row carries the count.
func renderFileNode(th theme.Theme, pending func(fileNode) int) func(fileNode) string {
	return func(n fileNode) string {
		indent := strings.Repeat("  ", n.Depth)
		count := pending(n)
		if n.Item == nil {
			chevron := "▾"
			if n.Collapsed {
				chevron = "▸"
			}
			marker := lipgloss.NewStyle().Foreground(th.Muted).Render(chevron)
			name := lipgloss.NewStyle().Foreground(th.Info).Bold(true).Render(dirLabel(n))
			row := indent + marker + " " + name
			if count > 0 {
				row += lipgloss.NewStyle().Foreground(th.Muted).Render(pendingLabel(count))
			}
			return row
		}
		if count > 0 {
			return indent + pendingStatusRow(th, *n.Item, n.Name)
		}
		return indent + statusRow(th, *n.Item, n.Name)
	}
}
