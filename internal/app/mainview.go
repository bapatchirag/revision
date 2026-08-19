package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
)

// updateStatus fills the Status panel with the working copy's locations and
// revision state: the working-copy root, the source path revision operates on,
// the current working directory, the branch it is checked out from, and the
// checked-out and HEAD revision numbers.
// A value that is not yet known is omitted so the panel only lists facts.
func (m *Model) updateStatus() {
	lines := make([]string, 0, 6)
	bold := lipgloss.NewStyle().Bold(true)
	add := func(label, value string) {
		if value != "" {
			lines = append(lines, bold.Render(fmt.Sprintf("%-8s", label))+"  "+value)
		}
	}
	if m.info != nil {
		add("Root", m.info.WorkingCopyRoot)
	}
	if m.client != nil {
		add("Source", m.client.Dir)
	}
	add("CWD", m.workDir)
	if m.info != nil {
		add("Branch", m.info.Branch())
	}
	if m.wcRevision != "" {
		add("Revision", "r"+m.wcRevision)
	}
	if head := m.headRevision(); head != "" {
		add("HEAD", "r"+head)
	}
	m.status.SetContent(strings.Join(lines, "\n"))
}

// headRevision returns the repository's latest revision: read at startup by the
// one-entry HEAD probe, and refreshed whenever the first page of history lands.
// It is empty until that probe answers.
func (m *Model) headRevision() string { return m.headRev }

// updateMain fills the Main panel from whichever side panel currently drives it.
// The Main and Status panels are searched (not filtered): any active search
// re-highlights against the new content inside the Viewport, so nothing is set
// here beyond the content itself.
//
// Content identical to what is displayed is not written at all, and a refresh of
// the same selection keeps the reader's scroll position; only a move to another
// selection starts back at the top.
func (m *Model) updateMain() {
	// Only a unified diff carries the one-column +/-/space marker that must stay
	// pinned while the body scrolls horizontally, and only a diff has lines worth
	// pointing at; mainContent turns both on for that case, and this baseline
	// clears them for every other view.
	m.main.SetGutter(0)
	m.main.SetCursorLine(false)
	content := m.mainContent()
	key := m.mainSelectionKey()
	switch {
	case key == m.mainKey && content == m.mainText:
		return
	case key == m.mainKey:
		m.main.SetContentPreservingScroll(content)
	default:
		m.main.SetContent(content)
	}
	m.mainKey, m.mainText = key, content
}

// mainSelectionKey identifies what Main is showing — the driving panel and the
// row selected in it — so a reload of the same subject can be told from a move
// to a different one.
func (m *Model) mainSelectionKey() string {
	switch m.source {
	case sourceStatus:
		return "status"
	case sourceShelf:
		e, _ := m.shelves.Selected()
		return "shelf:" + e.ID
	case sourceLog:
		if m.revDiff.set() {
			n, _ := m.revFiles.Selected()
			return "revdiff:" + m.revDiff.from + ":" + m.revDiff.to + ":" + n.Path
		}
		e, _ := m.log.Selected()
		return "log:" + e.Revision
	}
	switch {
	case m.filesViewIsDiffs():
		d, _ := m.savedDiffs.Selected()
		return "saved:" + d.Path
	case m.filesViewIsRejects():
		n, _ := m.rejects.Selected()
		return "reject:" + n.Path
	case m.filesViewIsChangelists() && !m.inChangelistDrill():
		g, _ := m.changelists.Selected()
		return "changelist:" + g.Name
	}
	n, _, _ := m.selectedTreeNode()
	return "files:" + n.Path
}

// mainContent computes the raw Main text for the current state, setting the diff
// gutter and line cursor as a side effect when it renders a unified diff.
func (m *Model) mainContent() string {
	if m.source == sourceStatus {
		return m.statusDetail()
	}
	// The shelf store is local disk, like the Diffs and Rejects views, so it stays
	// readable while the working copy is still loading or has failed to load.
	if m.source == sourceShelf {
		if m.shelfShowsPatch() {
			m.main.SetGutter(1)
			m.main.SetCursorLine(true)
		}
		return m.shelfDetail()
	}
	// The Diffs and Rejects views browse files on local disk, so they stay
	// readable while the working copy is still loading or has failed to load.
	if m.source != sourceFiles || !m.filesViewIsStore() {
		switch {
		case m.err != nil:
			return "Error: " + m.err.Error() + "\n\nPress R to retry."
		case m.loading && len(m.fileItems) == 0:
			return "Loading working-copy status…"
		}
	}
	if m.source == sourceLog {
		if m.revDiff.set() {
			if m.revDiffShowsDiff() {
				m.main.SetGutter(1)
				m.main.SetCursorLine(true)
			}
			return m.revDiffDetail()
		}
		return m.logDetail()
	}
	if m.filesShowDiff() {
		m.main.SetGutter(1)
		m.main.SetCursorLine(true)
	}
	return m.filesMain()
}

// filesMain renders the Main content for the Files panel, which depends on its
// active view: the Diffs browser shows the highlighted saved patch file, the
// Rejects browser the highlighted reject, the Changelists overview a changelist
// summary, a directory row in the Changes tree the combined diff beneath it, and
// everything else (a file in the Changes tree or a drilled-in changelist) the
// selected file.
func (m *Model) filesMain() string {
	if m.filesViewIsDiffs() {
		return m.savedDiffDetail()
	}
	if m.filesViewIsRejects() {
		return m.rejectDetail()
	}
	if m.filesViewIsChangelists() && !m.inChangelistDrill() {
		return m.changelistDetail()
	}
	if n, _, ok := m.selectedTreeNode(); ok && n.Item == nil {
		return m.directoryDetail(n)
	}
	return m.fileDetail()
}

// filesShowDiff reports whether filesMain currently renders a unified diff — the
// only Main view with a +/-/space gutter to pin. It mirrors the diff branches of
// savedDiffDetail, rejectDetail, directoryDetail and fileDetail: the Files panel
// is showing a saved patch file or a reject that has been read, or working-copy
// files (not the Changelists overview) whose selected directory row, or dirty
// file leaf, has a non-empty, freshly-loaded diff.
func (m *Model) filesShowDiff() bool {
	if m.filesViewIsDiffs() {
		d, ok := m.savedDiffs.Selected()
		return ok && !m.savedErr && m.savedPath == d.Path && strings.TrimSpace(m.savedText) != ""
	}
	if m.filesViewIsRejects() {
		r, ok := m.selectedReject()
		return ok && !m.rejectErr && m.rejectPath == r.Path && strings.TrimSpace(m.rejectText) != ""
	}
	if m.filesViewIsChangelists() && !m.inChangelistDrill() {
		return false
	}
	if n, _, ok := m.selectedTreeNode(); ok && n.Item == nil {
		return m.dirDiff && m.diffPath == n.Path && strings.TrimSpace(m.diffText) != ""
	}
	it, ok := m.selectedFile()
	if !ok || !it.State.IsDirty() {
		return false
	}
	return m.diffPath == it.Path && strings.TrimSpace(m.diffText) != ""
}

// changelistDetail summarizes the selected changelist: its label, file count and
// the paths it groups.
func (m *Model) changelistDetail() string {
	g, ok := m.changelists.Selected()
	if !ok {
		return "No changelists yet — stage files (space) or assign one (n)."
	}
	lines := []string{
		"Changelist: " + g.Label(),
		fmt.Sprintf("%d file(s)", len(g.Items)),
		"",
	}
	if g.Committable() {
		lines = append(lines, "enter expand · c commit this changelist", "")
	} else {
		lines = append(lines, "Files in no changelist (committable by default).", "")
	}
	for _, it := range g.Items {
		lines = append(lines, fmt.Sprintf("  %s %s", it.State.Code(), it.Path))
	}
	return strings.Join(lines, "\n")
}

// directoryDetail renders the combined diff of every change beneath a selected
// directory row (the "/" root covers the whole working copy). It mirrors
// fileDetail: a placeholder shows while the diff loads or when the directory has
// no textual changes. When directory diffs are toggled off it shows only a hint
// naming the key that reveals the diff.
func (m *Model) directoryDetail(n fileNode) string {
	if !m.dirDiff {
		return "(directory diff off — press " + m.keys.ToggleDirDiff.Help().Key + " to show it)"
	}
	switch {
	case m.diffPath != n.Path:
		return "Loading diff…"
	case strings.TrimSpace(m.diffText) == "":
		return "(no textual changes under this directory)"
	default:
		return m.colorize(m.diffText)
	}
}

// fileDetail renders the selected file's diff, prefixed by its changelist when
// it belongs to one, or a placeholder while the diff loads or when the state has
// no textual diff.
func (m *Model) fileDetail() string {
	it, ok := m.selectedFile()
	if !ok {
		return "Working copy is clean — no changes."
	}
	var head []string
	if it.Changelist != "" {
		head = append(head, "changelist: "+displayCL(it.Changelist), "")
	}
	if it.State == svn.StateConflicted {
		// The Changes hints fill the status bar at 80 columns, so the key that
		// resolves a conflict is offered here, where it is only in the way of the
		// files it applies to.
		head = append(head, "conflict — press m to resolve it side by side", "")
	}
	switch {
	case !it.State.IsDirty():
		return strings.Join(append(head, "(no textual diff for this state)"), "\n")
	case m.diffPath != it.Path:
		return strings.Join(append(head, "Loading diff…"), "\n")
	case strings.TrimSpace(m.diffText) == "":
		return strings.Join(append(head, "(no changes to display)"), "\n")
	default:
		return strings.Join(append(head, m.colorize(m.diffText)), "\n")
	}
}

// revDiffShowsDiff reports whether Main is showing a patch for the range rather
// than a placeholder — the only case with a +/-/space gutter to pin and lines
// worth pointing at.
func (m *Model) revDiffShowsDiff() bool {
	e, ok := m.session.RevDiff(m.revDiff)
	return ok && !e.failed && strings.TrimSpace(m.revPatchUnderCursor(e.text)) != ""
}

// revPatchUnderCursor is the part of the range's diff the drilled-in tree points
// at: one file's section, every section beneath a directory, or — at the "/"
// root — the whole patch, which is also what is shown before the tree has been
// moved off it.
func (m *Model) revPatchUnderCursor(whole string) string {
	n, ok := m.revFiles.Selected()
	if !ok || n.Path == fileTreeRoot {
		return whole
	}
	if n.Item != nil {
		return n.Item.Text
	}
	var parts []string
	prefix := n.Path + "/"
	for _, f := range m.revPatch {
		if strings.HasPrefix(f.Path, prefix) {
			parts = append(parts, f.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// revDiffDetail renders the diff over a range of history, headed by the range it
// covers and the path being read within it, so what is being compared stays on
// screen while the patch scrolls.
func (m *Model) revDiffDetail() string {
	e, ok := m.session.RevDiff(m.revDiff)
	switch {
	case !ok:
		return strings.Join([]string{m.revDiff.label(), "", "Loading diff…"}, "\n")
	case e.failed:
		return strings.Join([]string{m.revDiff.label(), "", e.text}, "\n")
	}
	head := m.revDiff.label()
	if n, sel := m.revFiles.Selected(); sel && n.Path != fileTreeRoot {
		head += " · " + n.Path
	}
	text := m.revPatchUnderCursor(e.text)
	if strings.TrimSpace(text) == "" {
		return strings.Join([]string{head, "", "(nothing changed between these revisions)"}, "\n")
	}
	return strings.Join([]string{head, "", m.colorize(text)}, "\n")
}

// logDetail renders the metadata, message and changed paths of the selected
// revision. The changed paths cost their own `svn log --verbose`, so they are
// filled in from the session when that load has landed and the rest of the
// detail is shown without waiting for it.
func (m *Model) logDetail() string {
	entry, ok := m.log.Selected()
	if !ok {
		switch {
		case m.logLoading:
			return "Loading history…"
		case m.logErr != nil:
			return "Unable to load history: " + m.logErr.Error()
		}
		return "No revision history."
	}
	author := entry.Author
	if author == "" {
		author = "(none)"
	}
	lines := []string{"r" + entry.Revision, "author: " + author}
	if !entry.Date.IsZero() {
		lines = append(lines, "date:   "+entry.Date.Format("2006-01-02 15:04"))
	}
	lines = append(lines, "", entry.Message)
	if paths, ok := m.session.RevDetail(entry.Revision); ok && len(paths) > 0 {
		lines = append(lines, "", "Changed paths:")
		for _, p := range paths {
			lines = append(lines, fmt.Sprintf("  %s %s", p.Action, p.Path))
		}
	}
	return strings.Join(lines, "\n")
}
