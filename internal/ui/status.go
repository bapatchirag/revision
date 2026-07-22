package ui

import (
	"io"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// statusRow adapts an svn.StatusItem to the list.Item interface.
type statusRow struct {
	item svn.StatusItem
}

func (r statusRow) FilterValue() string { return r.item.Path }

// statusDelegate renders each row on a single line: "<code>  <path>".
type statusDelegate struct{}

func (statusDelegate) Height() int                         { return 1 }
func (statusDelegate) Spacing() int                        { return 0 }
func (statusDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (statusDelegate) Render(w io.Writer, m list.Model, index int, it list.Item) {
	row, ok := it.(statusRow)
	if !ok {
		return
	}
	code := row.item.State.Code()
	codeStyled := lipgloss.NewStyle().Foreground(stateColor(code)).Bold(true).Render(code)

	var line string
	if index == m.Index() {
		line = selectedStyle.Render("> ") + codeStyled + "  " + selectedStyle.Render(row.item.Path)
	} else {
		line = "  " + codeStyled + "  " + row.item.Path
	}
	_, _ = io.WriteString(w, line)
}

// newStatusList returns an empty, chrome-free status list.
func newStatusList() list.Model {
	l := list.New(nil, statusDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(false)
	return l
}

// rowsFromItems converts svn status items into list items.
func rowsFromItems(items []svn.StatusItem) []list.Item {
	rows := make([]list.Item, len(items))
	for i, it := range items {
		rows[i] = statusRow{item: it}
	}
	return rows
}
