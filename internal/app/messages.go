package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/sshagent"
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

// diffSavedMsg carries the result of writing a diff to disk, along with the
// destination it was written to (or attempted).
type diffSavedMsg struct {
	path string
	err  error
}

// savedDiffsLoadedMsg carries the patch files found in the configured diff
// output directory, for the Files panel's Diffs view.
type savedDiffsLoadedMsg struct {
	files []savedDiff
	err   error
}

// savedDiffReadMsg carries the contents of a saved patch file, keyed by the path
// it was read from so the current selection can match it.
type savedDiffReadMsg struct {
	path string
	text string
	err  error
}

// editedMsg carries the result of opening a file in the configured editor. name
// is the file as it is shown on screen. detached is true when the editor was
// handed the file and left running outside the terminal, so nothing can have
// been saved yet and the working copy need not be re-read.
type editedMsg struct {
	name     string
	detached bool
	err      error
}

// logLoadedMsg carries the result of a `svn log` load. page is the 1-based page
// it was requested for, so a response overtaken by a later page turn can be
// discarded; more reports whether a further page exists.
type logLoadedMsg struct {
	entries []svn.LogEntry
	page    int
	more    bool
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

// updatedMsg carries the result of an `svn update`. toRevision distinguishes an
// update to a revision picked in the Log — which was necessarily on the page on
// screen, so the Log stays there — from a plain update to HEAD, which lands on
// the first page.
type updatedMsg struct {
	revision   string
	toRevision bool
	err        error
}

// updateAvailableMsg is emitted when the startup check finds a newer release
// than the running binary. It is only produced on release builds.
type updateAvailableMsg struct {
	rel selfupdate.Release
}

// startupNoticeMsg carries a one-time message to display as a toast once the UI
// is up. It is used to surface configuration conflicts resolved at startup.
type startupNoticeMsg struct{ text string }

// sshCheckedMsg carries whether the configured SSH key is already loaded in the
// agent. A non-nil err means the agent could not be reached (or ssh-add is
// missing), so there is nothing to unlock.
type sshCheckedMsg struct {
	loaded bool
	err    error
}

// sshAddedMsg carries the result of adding the SSH key to the agent.
type sshAddedMsg struct{ err error }

// sourceChangedMsg carries the result of probing a candidate source directory:
// the client rooted at it and the working-copy info read from there, or the
// error that rules the directory out.
type sourceChangedMsg struct {
	client *svn.Client
	info   *svn.Info
	err    error
}

// probeSourceCmd asks svn, off the UI goroutine, whether client's directory is a
// working copy, so the session is only re-rooted on a directory known to be one.
func probeSourceCmd(client *svn.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		info, err := client.Info(ctx)
		return sourceChangedMsg{client: client, info: info, err: err}
	}
}

// startupNoticeCmd emits a startupNoticeMsg so a launch-time notice is shown
// through the normal toast path once the program is running.
func startupNoticeCmd(text string) tea.Cmd {
	return func() tea.Msg { return startupNoticeMsg{text: text} }
}

// sshCheckCmd checks, off the UI goroutine, whether the configured SSH key is
// already loaded in the running ssh-agent.
func sshCheckCmd(keyPath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		loaded, err := sshagent.KeyLoaded(ctx, keyPath)
		return sshCheckedMsg{loaded: loaded, err: err}
	}
}

// sshAddCmd adds the configured SSH key to the agent, off the UI goroutine,
// using the entered passphrase to decrypt it.
func sshAddCmd(keyPath, passphrase string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return sshAddedMsg{err: sshagent.AddKey(ctx, keyPath, passphrase)}
	}
}

// checkUpdateCmd asks GitHub, off the UI goroutine, whether a newer release
// exists. It emits updateAvailableMsg only when one does; a development build,
// an up-to-date binary, or any network/parse failure yields no message so the
// check can never disrupt startup.
func checkUpdateCmd(build selfupdate.Build) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		rel, newer, err := selfupdate.Check(ctx, build)
		if err != nil || !newer {
			return nil
		}
		return updateAvailableMsg{rel: rel}
	}
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

// loadDiffCmd runs `svn diff <path>` off the UI goroutine. A file path diffs that
// file; a directory path diffs every change beneath it. The synthetic "/" root
// (fileTreeRoot) maps to the empty path so it diffs the whole working copy, while
// the message stays keyed by the original path so the current selection can match
// it. Diff failures are carried on the message rather than promoted to a fatal
// error so a single undiffable path never tears down the UI.
func loadDiffCmd(client *svn.Client, path string) tea.Cmd {
	target := path
	if target == fileTreeRoot {
		target = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		diff, err := client.Diff(ctx, target)
		return diffLoadedMsg{path: path, diff: diff, err: err}
	}
}

// diffDirPerm and diffFilePerm are the permissions saved diffs are created with:
// the standard user-owned rwx/rw a shell redirect would produce, since a diff of
// a working copy the user can already read holds nothing more sensitive.
const (
	diffDirPerm  = 0o755
	diffFilePerm = 0o644
)

// saveDiffCmd writes diff into dir as name off the UI goroutine.
func saveDiffCmd(dir, name, diff string) tea.Cmd {
	return func() tea.Msg { return writeDiff(dir, name, diff) }
}

// loadSavedDiffsCmd lists the patch files already saved in dir, off the UI
// goroutine, for the Diffs view to browse.
func loadSavedDiffsCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		files, err := scanSavedDiffs(dir)
		return savedDiffsLoadedMsg{files: files, err: err}
	}
}

// readSavedDiffCmd reads a saved patch file off the UI goroutine so it can be
// shown in Main. A read failure is carried on the message rather than promoted
// to a fatal error, so an unreadable file never tears down the UI.
func readSavedDiffCmd(path string) tea.Cmd {
	return func() tea.Msg {
		b, err := os.ReadFile(path)
		return savedDiffReadMsg{path: path, text: string(b), err: err}
	}
}

// saveChangelistDiffCmd generates the combined diff of the given paths and
// writes it, off the UI goroutine. It backs saving a changelist, whose diff is
// not on screen and so has to be produced first.
func saveChangelistDiffCmd(client *svn.Client, paths []string, dir, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		diff, err := client.DiffPaths(ctx, paths)
		if err != nil {
			return diffSavedMsg{path: filepath.Join(dir, name), err: err}
		}
		return writeDiff(dir, name, diff)
	}
}

// writeDiff writes diff into dir as name, creating dir (and any missing parent)
// when it does not exist yet. A trailing newline is ensured so the result is a
// well-formed patch. The destination rides on the message either way, so success
// can name it and failure can say which path it was.
func writeDiff(dir, name, diff string) diffSavedMsg {
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(dir, diffDirPerm); err != nil {
		return diffSavedMsg{path: path, err: err}
	}
	if !strings.HasSuffix(diff, "\n") {
		diff += "\n"
	}
	if err := os.WriteFile(path, []byte(diff), diffFilePerm); err != nil {
		return diffSavedMsg{path: path, err: err}
	}
	return diffSavedMsg{path: path}
}

// loadLogCmd runs `svn log` off the UI goroutine for one page of history: anchor
// is the revision the previous page ended on (empty for the first page) and page
// is the 1-based number the result belongs to. Errors are carried on the message
// so history-load failures stay confined to the Log panel.
func loadLogCmd(client *svn.Client, anchor string, page, limit int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		entries, more, err := client.LogPage(ctx, anchor, limit)
		return logLoadedMsg{entries: entries, page: page, more: more, err: err}
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

// stageManyCmd applies several stage actions in one pass off the UI goroutine:
// for each action it optionally runs `svn add` first (for a previously
// unversioned file), then adds the path to (stage) or removes it from (unstage)
// the staged changelist. It stops on the first error. Success rides on a single
// stagedMsg with no changelist name, so — like acting on a single file — it shows
// no toast; the follow-up status reload makes the change visible.
func stageManyCmd(client *svn.Client, changelist string, acts []stageAction) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		for _, act := range acts {
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
			if err != nil {
				return stagedMsg{path: act.path, staged: act.stage, err: err}
			}
		}
		return stagedMsg{staged: true}
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

// batchLabel summarizes a fan-out for a success toast: the sole path when one
// file was touched, otherwise an "N files" count.
func batchLabel(n int, first string) string {
	if n == 1 {
		return first
	}
	return fmt.Sprintf("%d files", n)
}

// revertCmd discards local modifications to path off the UI goroutine.
func revertCmd(client *svn.Client, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return revertedMsg{path: path, err: client.Revert(ctx, path)}
	}
}

// revertManyCmd discards local modifications to several paths in one pass off the
// UI goroutine, stopping on the first error. Success rides on a single
// revertedMsg carrying an "N files" summary, mirroring how acting on one file
// reports a single path.
func revertManyCmd(client *svn.Client, paths []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		for _, p := range paths {
			if err := client.Revert(ctx, p); err != nil {
				return revertedMsg{path: p, err: err}
			}
		}
		return revertedMsg{path: batchLabel(len(paths), paths[0])}
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

// deleteManyCmd deletes several paths in one pass off the UI goroutine: each
// versioned path is scheduled for removal (svn delete) and each unversioned one
// is removed from disk. It stops on the first error; success rides on a single
// deletedMsg carrying an "N files" summary.
func deleteManyCmd(client *svn.Client, acts []deleteAction) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		for _, act := range acts {
			var err error
			if act.unversioned {
				err = client.RemoveUnversioned(act.path)
			} else {
				err = client.Delete(ctx, act.path)
			}
			if err != nil {
				return deletedMsg{path: act.path, err: err}
			}
		}
		return deletedMsg{path: batchLabel(len(acts), acts[0].path)}
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

// updateToRevisionCmd moves the working copy to a specific revision off the UI
// goroutine.
func updateToRevisionCmd(client *svn.Client, rev string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		r, err := client.UpdateToRevision(ctx, rev)
		return updatedMsg{revision: r, toRevision: true, err: err}
	}
}
