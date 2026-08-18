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
// answered: the paths whose diff is to be generated — one target for a file or
// directory row, every member for a changelist, none at all for the whole
// working copy. name is the file name a blank entry falls back to.
type diffSource struct {
	name  string
	paths []string
}

// saveDiff opens the prompt that names the file a diff will be written to. In
// the Changes tree — and a drilled-in changelist — it saves what Main is showing:
// a file leaf's own diff, or a directory row's combined diff of everything
// beneath it (the "/" root covering the whole working copy). On the Changelists
// overview, where Main shows a summary rather than a diff, it saves the
// highlighted changelist's diff instead. A selection with nothing to diff — a
// clean file, a directory while directory diffs are off, one whose diff is still
// loading or failed to — warns instead, as do the Diffs and Rejects views, whose
// entries are patch files on disk already.
func (m *Model) saveDiff() tea.Cmd {
	if m.filesViewIsDiffs() {
		m.showToast("this diff is already saved", component.LevelWarning)
		return nil
	}
	if m.filesViewIsRejects() {
		m.showToast("a reject is already a file on disk", component.LevelWarning)
		return nil
	}
	if m.filesViewIsChangelists() && !m.inChangelistDrill() {
		return m.saveChangelistDiff()
	}
	if !m.filesShowDiff() || m.diffErr {
		m.showToast("no diff to save for this selection", component.LevelWarning)
		return nil
	}
	// The target is snapshotted, so what is written is the diff of the selection
	// the prompt opened on, whatever happens behind the overlay meanwhile.
	m.openDiffPrompt(diffSource{name: diffFileName(m.diffPath), paths: diffTargets(m.diffPath)})
	return nil
}

// diffTargets is the path set that diffs a single Files-panel selection. The
// synthetic "/" root covers the whole working copy, which svn diffs with no path
// at all.
func diffTargets(path string) []string {
	if path == fileTreeRoot {
		return nil
	}
	return []string{path}
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
// name, into the configured output directory. The patch is generated as part of
// the write, so the `svn diff` behind it is one the user asked for and shows up
// in the command log.
func (m *Model) submitDiffName(name string) tea.Cmd {
	src := m.diffSrc
	m.closeDiffName()
	return saveDiffCmd(m.client, src.paths, m.diffDir(), diffSaveName(name, src.name))
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

// diffTargetAt maps row idx of a unified diff onto the file and line it points
// at: the path of the "Index:" block the row falls in, and the line it stands
// for in that file's modified version. A context or added row stands for a line
// of its own; a hunk header, a removal or a "\ No newline" note has none, so it
// resolves to where its hunk sits. Rows outside any hunk — the "Index:" block
// and the file markers — are skipped over, so an idx landing on one resolves to
// the start of the hunk that follows, and an idx past the last hunk falls back to
// the furthest the diff reached. It is the counterpart of colorizeDiff and
// splitDiffPages: the same unified-diff structure, read for a position instead of
// colors or columns. A combined diff is what makes the path worth reporting —
// which file a row belongs to is only in the diff itself. A diff with no hunk at
// all yields line 0, meaning "open at the top".
func diffTargetAt(diff string, idx int) (path string, line int) {
	if idx < 0 {
		idx = 0
	}
	var file, lastFile string
	var cur, last int
	for i, ln := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(ln, "Index:"):
			file, cur = strings.TrimSpace(ln[len("Index:"):]), 0
			continue
		case strings.HasPrefix(ln, "@@"):
			_, after, ok := parseHunkHeader(ln)
			if !ok {
				cur = 0
				continue
			}
			cur = after.start
		case cur == 0:
			// Before the first hunk of a file: nothing to point at. Inside one the
			// marker prefixes are ambiguous — a removed line beginning "--" reads as
			// a "---" header — so only "Index:" ends a hunk.
			continue
		}
		if i >= idx {
			return file, cur
		}
		lastFile, last = file, cur
		// A hunk header and a removed line take up no room in the modified file,
		// and "\ No newline at end of file" annotates the row above it.
		if !strings.HasPrefix(ln, "@@") && !strings.HasPrefix(ln, "-") && !strings.HasPrefix(ln, `\`) {
			cur++
		}
	}
	return lastFile, last
}

// patchFile is one file's section of a unified diff: the path its "Index:" line
// names, the change that section records, and the raw text of the section
// itself.
type patchFile struct {
	Path  string
	State svn.FileState
	Text  string
}

// splitPatchByFile cuts a unified diff into its per-file sections, in the order
// svn emitted them. It joins colorizeDiff, splitDiffPages and diffTargetAt as a
// reader of unified-diff structure, this one for the file boundaries: a diff
// between two revisions is the only description of itself there is, so the tree
// browsing it has to be built out of the patch rather than alongside it.
//
// A section's state comes from the "(nonexistent)" marker svn writes on
// whichever side the file is missing from — the left for an addition, the right
// for a deletion. The markers are only read before the section's first hunk,
// since inside one a removed line can begin "---" and be mistaken for a header.
// A replacement is indistinguishable from a modification here and reads as one.
//
// Anything ahead of the first "Index:" line belongs to no file and is dropped.
func splitPatchByFile(diff string) []patchFile {
	if strings.TrimSpace(diff) == "" {
		return nil
	}
	var (
		files  []patchFile
		cur    *patchFile
		body   []string
		inHunk bool
	)
	flush := func() {
		if cur != nil {
			cur.Text = strings.Join(body, "\n")
			files = append(files, *cur)
		}
		body = nil
	}
	for _, ln := range strings.Split(strings.TrimRight(diff, "\n"), "\n") {
		switch {
		case strings.HasPrefix(ln, "Index:"):
			flush()
			cur = &patchFile{Path: strings.TrimSpace(ln[len("Index:"):]), State: svn.StateModified}
			inHunk = false
		case cur == nil:
			continue
		case strings.HasPrefix(ln, "@@"):
			inHunk = true
		case !inHunk && strings.HasPrefix(ln, "--- ") && strings.HasSuffix(ln, "(nonexistent)"):
			cur.State = svn.StateAdded
		case !inHunk && strings.HasPrefix(ln, "+++ ") && strings.HasSuffix(ln, "(nonexistent)"):
			cur.State = svn.StateDeleted
		}
		body = append(body, ln)
	}
	flush()
	return files
}

// colorize is colorizeDiff memoized over the session. Main is rebuilt on every
// filter keystroke, focus change and reload, and re-styling a large patch line
// by line dominates that work; the theme is part of the key, so a switch of
// palette is never served the previous one's colors.
func (m *Model) colorize(diff string) string {
	if diff == "" {
		return ""
	}
	key := renderKey{digest: digestString(diff), theme: m.themeName}
	if text, ok := m.session.Rendered(key); ok {
		return text
	}
	text := colorizeDiff(m.theme, diff)
	m.session.PutRendered(key, text)
	return text
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
