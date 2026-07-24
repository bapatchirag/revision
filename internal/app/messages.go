package app

import (
	"context"
	"fmt"
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

// stagedMsg carries the result of staging or unstaging a single path.
type stagedMsg struct {
	path       string
	staged     bool
	changelist string // non-empty when a named changelist was assigned
	err        error
}

// committedMsg carries the result of a commit.
type committedMsg struct {
	revision string
	err      error
}

// revertedMsg carries the result of reverting a single path.
type revertedMsg struct {
	path string
	err  error
}

// deletedMsg carries the result of deleting a single path.
type deletedMsg struct {
	path string
	err  error
}

// updatedMsg carries the result of an `svn update`.
type updatedMsg struct {
	revision string
	err      error
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

// stageCmd applies a stage action off the UI goroutine: it optionally runs
// `svn add` first (for a previously unversioned file), then adds the path to, or
// removes it from, the staged changelist.
func stageCmd(client *svn.Client, changelist string, act stageAction) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if act.add {
			if err := client.Add(ctx, act.path); err != nil {
				return stagedMsg{path: act.path, staged: act.stage, err: err}
			}
		}
		var err error
		if act.stage {
			err = client.AddToChangelist(ctx, changelist, act.path)
		} else {
			err = client.RemoveFromChangelist(ctx, act.path)
		}
		return stagedMsg{path: act.path, staged: act.stage, err: err}
	}
}

// commitCmd commits the staged changelist off the UI goroutine.
func commitCmd(client *svn.Client, message, changelist string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		rev, err := client.Commit(ctx, message, changelist)
		return committedMsg{revision: rev, err: err}
	}
}

// assignChangelistCmd moves every target into the named changelist off the UI
// goroutine, running `svn add` first for any previously unversioned file. The
// result rides on stagedMsg (carrying the changelist name so the app can confirm
// the assignment); the reported path is the sole file when one was named, or an
// "N files" count when several were named together.
func assignChangelistCmd(client *svn.Client, name string, targets []changelistTarget) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, t := range targets {
			if t.add {
				if err := client.Add(ctx, t.path); err != nil {
					return stagedMsg{path: t.path, staged: true, changelist: name, err: err}
				}
			}
			if err := client.AddToChangelist(ctx, name, t.path); err != nil {
				return stagedMsg{path: t.path, staged: true, changelist: name, err: err}
			}
		}
		return stagedMsg{path: assignedLabel(targets), staged: true, changelist: name}
	}
}

// assignedLabel summarizes which files an assign touched for the success toast:
// the sole path when one file was named, otherwise an "N files" count.
func assignedLabel(targets []changelistTarget) string {
	if len(targets) == 1 {
		return targets[0].path
	}
	return fmt.Sprintf("%d files", len(targets))
}

// revertCmd discards local modifications to path off the UI goroutine.
func revertCmd(client *svn.Client, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return revertedMsg{path: path, err: client.Revert(ctx, path)}
	}
}

// deleteCmd deletes a path off the UI goroutine: a versioned path is scheduled
// for removal (svn delete), an unversioned one is removed from disk.
func deleteCmd(client *svn.Client, act deleteAction) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var err error
		if act.unversioned {
			err = client.RemoveUnversioned(act.path)
		} else {
			err = client.Delete(ctx, act.path)
		}
		return deletedMsg{path: act.path, err: err}
	}
}

// updateCmd brings the working copy up to date off the UI goroutine.
func updateCmd(client *svn.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		rev, err := client.Update(ctx)
		return updatedMsg{revision: rev, err: err}
	}
}
