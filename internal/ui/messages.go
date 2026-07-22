package ui

import (
	"context"
	"time"

	"github.com/bapatchirag/revision/internal/svn"
	tea "github.com/charmbracelet/bubbletea"
)

// statusLoadedMsg carries the result of a successful status refresh.
type statusLoadedMsg struct {
	items []svn.StatusItem
}

// errMsg carries an error to surface in the UI.
type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

// loadStatusCmd runs `svn status` off the UI goroutine and reports the result.
func loadStatusCmd(client *svn.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		items, err := client.Status(ctx)
		if err != nil {
			return errMsg{err}
		}
		return statusLoadedMsg{items: items}
	}
}
