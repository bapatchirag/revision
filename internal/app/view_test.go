package app

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/bapatchirag/revision/internal/svn"
)

func TestModelGoldenLayout(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{
		{Path: "added.txt", State: svn.StateAdded, Changelist: "revision:staged"},
		{Path: "modified.go", State: svn.StateModified},
		{Path: "gone.txt", State: svn.StateDeleted},
	})
	// History reveals HEAD (r50) so the Status panel shows every field.
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{{Revision: "50"}, {Revision: "42"}}})
	m = next.(*Model)
	golden.RequireEqual(t, []byte(m.View()))
}
