package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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

// openInEditor opens the file the Files panel highlights — or, while the
// side-by-side overlay is up, the one it is reading — in the configured editor,
// at the line editTarget picks out. A terminal editor suspends the TUI and takes
// over the screen until it exits, after which the working copy is re-read so any
// edit shows up; every other editor opens the file elsewhere and leaves the TUI
// on screen. A selection that is not a single file — a directory row with no
// diff to place, or the Changelists overview — has nothing to open.
func (m *Model) openInEditor() tea.Cmd {
	path, name, line, ok := m.editTarget()
	if !ok {
		m.showToast("no file to open here", component.LevelWarning)
		return nil
	}
	launch, err := resolveEditor(m.cfg.Editor, path, line)
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

// editTarget returns the absolute path of the file to open, the shorter name it
// is known by on screen, and the line to open it at (0 for the top of the file):
// the file the resolution overlay is deciding, the page the side-by-side overlay
// is reading, a patch file in the Diffs or Rejects view, the file a displayed
// diff places the reader in, or failing all of those the highlighted file leaf.
// ok is false wherever none of those names a single file.
func (m *Model) editTarget() (path, name string, line int, ok bool) {
	if m.merging && m.mergeDoc != nil {
		return m.mergeDoc.path, m.mergeDoc.rel, m.mergeLine(), true
	}
	if m.splitting {
		rel, line, ok := m.splitTarget()
		if !ok {
			return "", "", 0, false
		}
		return m.absPath(rel), rel, line, true
	}
	if m.filesViewIsDiffs() {
		d, ok := m.savedDiffs.Selected()
		if !ok {
			return "", "", 0, false
		}
		return d.Path, d.Name, 0, true
	}
	if m.filesViewIsRejects() {
		r, ok := m.selectedReject()
		if !ok {
			return "", "", 0, false
		}
		return r.Path, r.Rel, 0, true
	}
	rel, line := m.diffTarget()
	if rel == "" {
		it, sel := m.selectedFile()
		if !sel {
			return "", "", 0, false
		}
		rel = it.Path
	}
	return m.absPath(rel), rel, line, true
}

// diffTarget is the file and line the diff on display places the reader in.
// Driven from the Files panel that is the first hunk of the diff, so an edit
// starts on the work in progress rather than on line one; with the diff itself
// focused it is the line under its cursor, so the editor picks up where the eye
// left off. A directory row's combined diff only names a file this way — which
// one a hunk belongs to is in the diff and nowhere else — so it is what makes a
// directory row openable at all. It reports no path when Main is showing
// something other than a working copy diff.
func (m *Model) diffTarget() (rel string, line int) {
	if m.source != sourceFiles || m.filesViewIsStore() || m.diffErr || !m.filesShowDiff() {
		return "", 0
	}
	row := 0
	if m.focus.Index() == panelMain {
		row = m.mainDiffRow()
	}
	return diffTargetAt(m.diffText, row)
}

// splitTarget is the file and line the side-by-side overlay is reading: its open
// page names the file and the row at the top of the window carries the line.
func (m *Model) splitTarget() (rel string, line int, ok bool) {
	title, row, ok := m.splitDiff.Current()
	if !ok || title == "" {
		return "", 0, false
	}
	return title, row.Line, true
}

// mainDiffRow is the row of the diff the Main cursor rests on, in the diff's own
// numbering. Main may draw a changelist header above the patch, so the rows it
// adds come off the cursor; below that the two texts run line for line,
// colorizing being a per-line pass.
func (m *Model) mainDiffRow() int {
	head := strings.Count(m.mainText, "\n") - strings.Count(m.diffText, "\n")
	return m.main.Cursor() - head
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
// path, at line when the editor can be told to, reporting an error when the
// chosen editor is not installed. A line of 0 opens the file at the top.
func resolveEditor(setting, path string, line int) (editorLaunch, error) {
	switch strings.TrimSpace(setting) {
	case config.EditorVim:
		return terminalEditor(path, line, "vim", "vi")
	case config.EditorNvim:
		return terminalEditor(path, line, "nvim")
	case config.EditorNano:
		return terminalEditor(path, line, "nano")
	default:
		return nativeEditor(path, line)
	}
}

// terminalEditor resolves the first of bins found on PATH into a command that
// edits path in this terminal. Later names are alternative spellings of the same
// editor, so the first is the one reported as missing.
func terminalEditor(path string, line int, bins ...string) (editorLaunch, error) {
	for _, bin := range bins {
		if full, err := exec.LookPath(bin); err == nil {
			return editorLaunch{cmd: exec.Command(full, openArgs(bin, path, line)...), name: bin, terminal: true}, nil
		}
	}
	return editorLaunch{}, fmt.Errorf("%s is not installed", bins[0])
}

// lineStyle is how an editor is told which line to open on. They disagree enough
// that the position has to be spelled the way each one expects.
type lineStyle int

const (
	lineNone   lineStyle = iota // the editor takes no position
	linePlus                    // "+42 path", the vi convention
	lineGoto                    // "--goto path:42", VS Code's
	lineSuffix                  // "path:42"
)

// editorLineStyle maps a program name onto the way it takes an opening line. An
// editor is only handed a position once it is recognized here: an unknown
// program would read "+42" as a second file to open, so anything unlisted — the
// desktop openers among them — is opened at the top of the file.
var editorLineStyle = map[string]lineStyle{
	"vi": linePlus, "vim": linePlus, "gvim": linePlus, "mvim": linePlus,
	"vimx": linePlus, "view": linePlus, "nvim": linePlus,
	"nano": linePlus, "pico": linePlus, "micro": linePlus, "kak": linePlus,
	"emacs": linePlus, "emacsclient": linePlus, "gedit": linePlus,

	"code": lineGoto, "code-insiders": lineGoto, "codium": lineGoto, "cursor": lineGoto,

	"subl": lineSuffix, "sublime_text": lineSuffix, "hx": lineSuffix, "helix": lineSuffix,
}

// openArgs is the argument list that opens path in bin, jumping to line when
// both the line and the editor's way of taking one are known.
func openArgs(bin, path string, line int) []string {
	if line <= 0 {
		return []string{path}
	}
	pos := path + ":" + strconv.Itoa(line)
	switch editorLineStyle[filepath.Base(bin)] {
	case linePlus:
		return []string{"+" + strconv.Itoa(line), path}
	case lineGoto:
		return []string{"--goto", pos}
	case lineSuffix:
		return []string{pos}
	default:
		return []string{path}
	}
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
func nativeEditor(path string, line int) (editorLaunch, error) {
	if bin, ok := vscodeCLI(); ok {
		return editorLaunch{cmd: exec.Command(bin, openArgs(bin, path, line)...), name: filepath.Base(bin)}, nil
	}
	if env := firstEnv("VISUAL", "EDITOR"); env != "" {
		// $EDITOR may carry flags, e.g. "emacs -nw" or "code --wait".
		fields := strings.Fields(env)
		full, err := exec.LookPath(fields[0])
		if err != nil {
			return editorLaunch{}, fmt.Errorf("$EDITOR %q is not installed", fields[0])
		}
		args := append(fields[1:], openArgs(fields[0], path, line)...)
		return editorLaunch{cmd: exec.Command(full, args...), name: fields[0], terminal: true}, nil
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
