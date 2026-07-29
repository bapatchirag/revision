package app

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// diffSource is what a queued save will write once the file-name prompt is
// answered: either the diff already on screen (text), or the paths whose diff
// must still be generated (a changelist, whose files need not share a directory).
// name is the file name a blank entry falls back to.
type diffSource struct {
	name  string
	text  string
	paths []string
}

// saveDiff opens the prompt that names the file a diff will be written to. In
// the Changes tree — and a drilled-in changelist — it saves what Main is showing:
// a file leaf's own diff, or a directory row's combined diff of everything
// beneath it (the "/" root covering the whole working copy). On the Changelists
// overview, where Main shows a summary rather than a diff, it saves the
// highlighted changelist's diff instead. A selection with nothing to diff — a
// clean file, a directory while directory diffs are off, one whose diff is still
// loading or failed to — warns instead.
func (m *Model) saveDiff() tea.Cmd {
	if m.filesViewIsChangelists() && !m.inChangelistDrill() {
		return m.saveChangelistDiff()
	}
	if !m.filesShowDiff() || m.diffErr {
		m.showToast("no diff to save for this selection", component.LevelWarning)
		return nil
	}
	// The diff is snapshotted so what is written is what was on screen when the
	// prompt opened, whatever lands behind the overlay meanwhile.
	m.openDiffPrompt(diffSource{name: diffFileName(m.diffPath), text: m.diffText})
	return nil
}

// saveChangelistDiff queues a save of the highlighted changelist's combined diff.
// The Changelists overview has no diff on screen to reuse, so the members are
// recorded here and diffed once the name is confirmed. A changelist holding no
// textual changes has nothing to write.
func (m *Model) saveChangelistDiff() tea.Cmd {
	g, ok := m.changelists.Selected()
	if !ok {
		m.showToast("no changelist to save", component.LevelWarning)
		return nil
	}
	paths := dirtyPaths(g.Items)
	if len(paths) == 0 {
		m.showToast("no textual changes in "+g.Label(), component.LevelWarning)
		return nil
	}
	m.openDiffPrompt(diffSource{name: changelistDiffName(g), paths: paths})
	return nil
}

// openDiffPrompt floats the file-name prompt for a queued save. The input opens
// empty; a blank entry takes the source's default name.
func (m *Model) openDiffPrompt(src diffSource) {
	m.savingDiff = true
	m.diffSrc = src
	m.diffEditor.Reset()
	m.diffEditor.Focus()
	m.sizeDiffEditor()
}

// submitDiffName closes the prompt and writes the queued diff under the entered
// name, into the configured output directory. A changelist's diff is generated
// as part of the write, since it was never on screen.
func (m *Model) submitDiffName(name string) tea.Cmd {
	src := m.diffSrc
	m.closeDiffName()
	file := diffSaveName(name, src.name)
	if len(src.paths) > 0 {
		return saveChangelistDiffCmd(m.client, src.paths, m.diffDir(), file)
	}
	return saveDiffCmd(m.diffDir(), file, src.text)
}

// closeDiffName hides the save-diff prompt and drops the queued source.
func (m *Model) closeDiffName() {
	m.savingDiff = false
	m.diffSrc = diffSource{}
	m.diffEditor.Blur()
}

// dirtyPaths collects the paths of the items carrying a textual change, the only
// ones svn can diff.
func dirtyPaths(items []svn.StatusItem) []string {
	var paths []string
	for _, it := range items {
		if it.State.IsDirty() {
			paths = append(paths, it.Path)
		}
	}
	return paths
}

// diffDir is the directory saved diffs are written to: the configured
// diffOutputDir, falling back to the working copy's root — so diffs collect in
// one place no matter which directory inside the working copy revision was
// started in, or which scope it displays.
func (m *Model) diffDir() string {
	root := m.workDir
	if m.info != nil && m.info.WorkingCopyRoot != "" {
		root = m.info.WorkingCopyRoot
	} else if m.client != nil {
		root = m.client.Dir
	}
	return m.cfg.DiffDir(root)
}

// diffFileName is the name a saved diff defaults to for the working-copy-relative
// target it covers: the path with its separators folded into "-", so nested
// targets stay distinct in a single flat output directory, suffixed with ".diff".
// The synthetic "/" root, which covers the whole working copy, becomes
// "working-copy.diff".
func diffFileName(target string) string {
	name := strings.Trim(target, "/")
	if name == "" {
		return "working-copy.diff"
	}
	return strings.ReplaceAll(name, "/", "-") + ".diff"
}

// changelistDiffName is the name a saved changelist diff defaults to: the
// group's label without the parentheses the staged and unstaged buckets are
// shown with.
func changelistDiffName(g changelistGroup) string {
	return diffFileName(strings.Trim(g.Label(), "()"))
}

// diffSaveName resolves the file an entered name writes to. It is reduced to its
// last path segment so a saved diff always lands in the configured output
// directory, whatever was typed, and gains a ".diff" suffix unless it already
// names a patch. A blank entry — or one that is only path syntax — falls back to
// the prompt's default.
func diffSaveName(entered, fallback string) string {
	name := filepath.Base(strings.TrimSpace(entered))
	switch name {
	case "", ".", "..", string(filepath.Separator):
		return fallback
	}
	if !strings.HasSuffix(name, ".diff") && !strings.HasSuffix(name, ".patch") {
		name += ".diff"
	}
	return name
}

// colorizeDiff is the domain adapter that maps the role of each unified-diff
// line onto a theme color, giving the Main viewport familiar diff syntax
// highlighting. It is the only place SVN diff structure is mapped onto colors,
// keeping the Viewport component diff-agnostic. Only styling is added: the
// Viewport's width math is ANSI-aware, so highlighted lines truncate and scroll
// exactly like plain ones. An empty diff is returned unchanged.
//
// Metadata lines ("Index:", the "===" rule and the "---"/"+++" file markers) are
// muted, a hunk header ("@@") takes the accent, added lines ("+") use the success
// color and removed lines ("-") the error color. The "---"/"+++" markers are
// matched before the single-character "-"/"+" cases so they read as headers
// rather than a giant delete/add.
//
// Tab conversion is disabled on the styles so this stays a pure styling pass;
// the Viewport remains the single owner of tab expansion (via tabSpaces), so
// colored lines align identically to plain ones.
func colorizeDiff(th theme.Theme, diff string) string {
	if diff == "" {
		return ""
	}
	var (
		meta = lipgloss.NewStyle().Foreground(th.Muted).TabWidth(lipgloss.NoTabConversion)
		hunk = lipgloss.NewStyle().Foreground(th.Accent).Bold(true).TabWidth(lipgloss.NoTabConversion)
		add  = lipgloss.NewStyle().Foreground(th.Success).TabWidth(lipgloss.NoTabConversion)
		del  = lipgloss.NewStyle().Foreground(th.Error).TabWidth(lipgloss.NoTabConversion)
	)
	lines := strings.Split(diff, "\n")
	for i, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "Index:"), strings.HasPrefix(ln, "==="),
			strings.HasPrefix(ln, "---"), strings.HasPrefix(ln, "+++"):
			lines[i] = meta.Render(ln)
		case strings.HasPrefix(ln, "@@"):
			lines[i] = hunk.Render(ln)
		case strings.HasPrefix(ln, "+"):
			lines[i] = add.Render(ln)
		case strings.HasPrefix(ln, "-"):
			lines[i] = del.Render(ln)
		}
	}
	return strings.Join(lines, "\n")
}
