package app

import (
	"fmt"
	"strconv"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
)

// benchSizes are the working-copy sizes the optimistic path is measured at: a
// routine change set, a large one, and one big enough that a quadratic scan
// dominates the keypress. It stops at 2 000 deliberately — the cost of the path
// grows with the square of the set, so a larger case buys no new information for
// the wall clock it spends.
var benchSizes = []int{100, 500, 2000}

// benchPartsSize is the set the stage-by-stage breakdown runs at, large enough
// for the quadratic scans to separate from the linear work around them.
const benchPartsSize = 2000

// benchStatusItems builds a status set shaped like a real working copy: files
// spread over a nested tree, a mix of changelists so the grouping has several
// buckets to fill, some unversioned paths, and the directory entries svn reports
// alongside their own contents — those last are what the container-directory
// scan exists to skip, so a set without them would measure the cheap half of it.
func benchStatusItems(n int) []svn.StatusItem {
	const dirs = 10
	items := make([]svn.StatusItem, 0, n+dirs)
	for d := 0; d < dirs; d++ {
		items = append(items, svn.StatusItem{
			Path:  fmt.Sprintf("internal/pkg%02d", d),
			State: svn.StateUnversioned,
		})
	}
	for i := 0; i < n; i++ {
		it := svn.StatusItem{
			Path:  fmt.Sprintf("internal/pkg%02d/sub%02d/file%04d.go", i%dirs, i%7, i),
			State: svn.StateModified,
		}
		switch i % 4 {
		case 1:
			it.Changelist = stagedChangelist
		case 2:
			it.Changelist = fmt.Sprintf("feature-%d", i%3)
		case 3:
			it.State = svn.StateUnversioned
		}
		items = append(items, it)
	}
	return items
}

// benchModel loads a status set into a sized model with the Files panel already
// on its first file, which is where a stage keypress lands.
func benchModel(b *testing.B, n int) *Model {
	b.Helper()
	m := loadItems(b, sizedModel(b), benchStatusItems(n))
	if idx := firstFileIndex(m.files.Items()); idx >= 0 {
		m.files.SetIndex(idx)
	}
	return m
}

// BenchmarkOptimisticSingleStage is the headline number: what one file changing
// changelist costs the model, which is the work a stage keypress does before it
// can paint. Every iteration alternates the changelist so applyOptimistic always
// has something to change — it returns early when the status already reads the
// way the mutation would leave it — and drops the snapshot, so each iteration
// measures a keypress settled before the next rather than a run held open.
//
// Toggling one file back and forth yields two status sets, both of which stay in
// the tree cache, so this is the warm path. A stage that has not been made
// before rebuilds the tree as well; BenchmarkOptimisticParts prices that.
func BenchmarkOptimisticSingleStage(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			m := benchModel(b, n)
			it, ok := m.selectedFile()
			if !ok {
				b.Fatal("no file selected to stage")
			}
			muts := [2][]mutation{
				{{path: it.Path, changelist: stagedChangelist}},
				{{path: it.Path}},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.optimistic = nil
				m.applyOptimistic(muts[i%2])
			}
		})
	}
}

// BenchmarkOptimisticParts breaks the refresh a keypress triggers into the calls
// it is made of, so a change to any one of them can be attributed. The two
// container-directory scans are measured on their own because they are the
// quadratic pair, and SetItems because it re-measures every row's width whether
// or not the tree behind it was rebuilt.
//
// buildFileTree is priced separately from rebuildFileTree because the two differ
// by the memo cache: a stage changes a file's changelist, which changes the
// digest the tree is keyed by, so the keypress that has not been made before
// pays the build and the one repeated does not. Once the quadratic scans go, the
// build is what is left.
func BenchmarkOptimisticParts(b *testing.B) {
	m := benchModel(b, benchPartsSize)
	items := m.fileItems
	filtered := m.filteredStatusItems(items)
	visible := m.visibleChanges(filtered)
	rows := m.fileTree(visible, m.collapsedDirs)

	parts := []struct {
		name string
		run  func()
	}{
		{"groupChangelists", func() { groupChangelists(filtered) }},
		{"changelistItems", func() { m.changelistItems(stagedChangelist) }},
		{"setItems", func() { m.files.SetItems(rows) }},
		{"buildFileTree", func() { buildFileTree(visible, m.collapsedDirs) }},
		{"rebuildFileTree", m.rebuildFileTree},
		{"rebuildChangelists", m.rebuildChangelists},
		{"refreshChrome", m.refreshChrome},
		{"refreshFilesForStatus", m.refreshFilesForStatus},
	}
	for _, part := range parts {
		b.Run(part.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				part.run()
			}
		})
	}
}

// BenchmarkStageKeypress measures the whole frame a stage costs — resolving the
// action, applying it optimistically, and painting the result — so the parts
// above can be checked against what the user actually waits for. Space toggles,
// so consecutive iterations stage and unstage the same file.
func BenchmarkStageKeypress(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			m := benchModel(b, n)
			key := tea.KeyMsg{Type: tea.KeySpace}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				next, _ := m.Update(key)
				m = next.(*Model)
				_ = m.View()
			}
		})
	}
}

// BenchmarkOptimisticDirectoryStage is the large-mutation-set case: staging the
// "/" root moves every stageable file at once, so the per-mutation scan over the
// status items is paid as many times as there are files. It is the same refresh
// as a single stage, driven by a mutation set the size of the working copy.
func BenchmarkOptimisticDirectoryStage(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			m := benchModel(b, n)
			root := m.files.Items()[0]
			if root.Path != fileTreeRoot {
				b.Fatalf("first row is %q, want the %q root", root.Path, fileTreeRoot)
			}
			acts := directoryStageActions(root, m.fileItems)
			if len(acts) == 0 {
				b.Fatal("nothing under the root is stageable")
			}
			unstage := make([]stageAction, len(acts))
			for i, a := range acts {
				unstage[i] = stageAction{path: a.path}
			}
			muts := [2][]mutation{
				stageMutations(acts, stagedChangelist),
				stageMutations(unstage, stagedChangelist),
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.optimistic = nil
				m.applyOptimistic(muts[i%2])
			}
		})
	}
}
