package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
)

// statusLoadedMsg carries the result of a successful status refresh.
type statusLoadedMsg struct {
	items []svn.StatusItem
}

// errMsg carries an error to surface in the UI.
type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

// diffLoadedMsg carries the result of loading a single file's diff.
type diffLoadedMsg struct {
	path string
	diff string
	err  error
}

// logLoadedMsg carries the result of a `svn log` load.
type logLoadedMsg struct {
	entries []svn.LogEntry
	err     error
}

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

// loadDiffCmd runs `svn diff <path>` off the UI goroutine. Diff failures are
// carried on the message rather than promoted to a fatal error so a single
// undiffable file never tears down the UI.
func loadDiffCmd(client *svn.Client, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		diff, err := client.Diff(ctx, path)
		return diffLoadedMsg{path: path, diff: diff, err: err}
	}
}

// loadLogCmd runs `svn log` off the UI goroutine. Errors are carried on the
// message so history-load failures stay confined to the Log panel.
func loadLogCmd(client *svn.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		entries, err := client.Log(ctx, logLimit)
		return logLoadedMsg{entries: entries, err: err}
	}
}
