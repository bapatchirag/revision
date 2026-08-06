package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/tui/component"
)

// editorLaunch is a resolved way to open a file: the command to run, the name to
// call it by in a message, and whether it needs this terminal. A terminal editor
// takes the screen over until it exits; anything else — a VS Code window, the
// desktop's file handler — is handed the file and left to run beside the TUI.
type editorLaunch struct {
	cmd      *exec.Cmd
	name     string
	terminal bool
}

// openInEditor opens the file the Files panel highlights in the configured
// editor. A terminal editor suspends the TUI and takes over the screen until it
// exits, after which the working copy is re-read so any edit shows up; every
// other editor opens the file elsewhere and leaves the TUI on screen. A
// selection that is not a single file — a directory row or the Changelists
// overview — has nothing to open.
func (m *Model) openInEditor() tea.Cmd {
	path, name, ok := m.editTarget()
	if !ok {
		m.showToast("no file to open here", component.LevelWarning)
		return nil
	}
	launch, err := resolveEditor(m.cfg.Editor, path)
	if err != nil {
		m.showToast("can't open "+name+": "+err.Error(), component.LevelWarning)
		return nil
	}
	if !launch.terminal {
		m.showToast("opening "+name+" in "+launch.name, component.LevelSuccess)
		return openDetachedCmd(launch, name)
	}
	m.dismissToast()
	return tea.ExecProcess(launch.cmd, func(err error) tea.Msg {
		return editedMsg{name: name, err: err}
	})
}

// editTarget returns the absolute path of the file the Files panel highlights,
// along with the shorter name it is known by on screen: a file leaf in the
// Changes tree or in a drilled-in changelist, a patch file in the Diffs view, or
// a reject in the Rejects view. ok is false wherever the highlight is not a
// single file.
func (m *Model) editTarget() (path, name string, ok bool) {
	if m.filesViewIsDiffs() {
		d, ok := m.savedDiffs.Selected()
		if !ok {
			return "", "", false
		}
		return d.Path, d.Name, true
	}
	if m.filesViewIsRejects() {
		r, ok := m.selectedReject()
		if !ok {
			return "", "", false
		}
		return r.Path, r.Rel, true
	}
	it, ok := m.selectedFile()
	if !ok {
		return "", "", false
	}
	return m.absPath(it.Path), it.Path, true
}

// absPath resolves a working-copy-relative path against the directory the svn
// client runs in, which is what those paths are reported relative to.
func (m *Model) absPath(rel string) string {
	if m.client == nil || filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(m.client.Dir, rel)
}

// resolveEditor turns the configured editor setting into the command that opens
// path, reporting an error when the chosen editor is not installed.
func resolveEditor(setting, path string) (editorLaunch, error) {
	switch strings.TrimSpace(setting) {
	case config.EditorVim:
		return terminalEditor(path, "vim", "vi")
	case config.EditorNvim:
		return terminalEditor(path, "nvim")
	case config.EditorNano:
		return terminalEditor(path, "nano")
	default:
		return nativeEditor(path)
	}
}

// terminalEditor resolves the first of bins found on PATH into a command that
// edits path in this terminal. Later names are alternative spellings of the same
// editor, so the first is the one reported as missing.
func terminalEditor(path string, bins ...string) (editorLaunch, error) {
	for _, bin := range bins {
		if full, err := exec.LookPath(bin); err == nil {
			return editorLaunch{cmd: exec.Command(full, path), name: bin, terminal: true}, nil
		}
	}
	return editorLaunch{}, fmt.Errorf("%s is not installed", bins[0])
}

// nativeEditor resolves the "native" setting, where the environment decides
// which editor opens the file.
//
// A VS Code integrated terminal comes first and is asked to open a tab through
// VS Code's own CLI. That is what makes a remote session work: over Remote-SSH
// the terminal — and revision with it — runs on the server, but the CLI injected
// there forwards the request to the window on the workstation, so the file opens
// in a tab beside the terminal rather than on the server's (absent) desktop.
//
// Outside VS Code the terminal editor named by $VISUAL or $EDITOR is used, and
// failing that the desktop's default handler for the file.
func nativeEditor(path string) (editorLaunch, error) {
	if bin, ok := vscodeCLI(); ok {
		return editorLaunch{cmd: exec.Command(bin, path), name: filepath.Base(bin)}, nil
	}
	if env := firstEnv("VISUAL", "EDITOR"); env != "" {
		// $EDITOR may carry flags, e.g. "emacs -nw" or "code --wait".
		fields := strings.Fields(env)
		full, err := exec.LookPath(fields[0])
		if err != nil {
			return editorLaunch{}, fmt.Errorf("$EDITOR %q is not installed", fields[0])
		}
		return editorLaunch{cmd: exec.Command(full, append(fields[1:], path)...), name: fields[0], terminal: true}, nil
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	full, err := exec.LookPath(opener)
	if err != nil {
		return editorLaunch{}, errors.New("no editor found; choose one with S")
	}
	return editorLaunch{cmd: exec.Command(full, path), name: opener}, nil
}

// vscodeCLI returns the VS Code command line for the terminal revision is
// running in, if it is one. The build is matched to the running window — Insiders
// announces itself in the version — so the file opens in the window the terminal
// belongs to rather than in a different installation that happens to be on PATH.
func vscodeCLI() (string, bool) {
	if os.Getenv("TERM_PROGRAM") != "vscode" {
		return "", false
	}
	bins := []string{"code", "code-insiders", "codium", "cursor"}
	if strings.Contains(strings.ToLower(os.Getenv("TERM_PROGRAM_VERSION")), "insider") {
		bins = []string{"code-insiders", "code"}
	}
	for _, bin := range bins {
		if full, err := exec.LookPath(bin); err == nil {
			return full, true
		}
	}
	return "", false
}

// firstEnv returns the value of the first of names that is set to a non-blank
// value.
func firstEnv(names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

// openDetachedCmd runs an editor that does not need the terminal, with its
// streams detached so nothing it prints can corrupt the TUI. These openers hand
// the file over and exit immediately; what they report on stderr is the useful
// part of a failure, so it is preferred over the bare exit status.
func openDetachedCmd(launch editorLaunch, name string) tea.Cmd {
	return func() tea.Msg {
		var stderr strings.Builder
		launch.cmd.Stderr = &stderr
		if err := launch.cmd.Run(); err != nil {
			if out := strings.TrimSpace(stderr.String()); out != "" {
				err = errors.New(out)
			}
			return editedMsg{name: name, err: err}
		}
		return editedMsg{name: name, detached: true}
	}
}
