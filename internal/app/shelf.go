package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/shelf"
	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// shelfListID identifies the shelf list on emitted selection messages.
const shelfListID = "shelf"

// shelfDir is where this working copy's shelved changes are kept. It is fixed at
// the working copy's root and deliberately not configurable: what keeps the
// store out of svn's way is its name matching Subversion's built-in ignore
// glob, which a directory somewhere else would not.
func (m *Model) shelfDir() string {
	if m.info != nil && m.info.WorkingCopyRoot != "" {
		return shelf.Dir(m.info.WorkingCopyRoot)
	}
	if m.client != nil && m.client.Dir != "" {
		return shelf.Dir(m.client.Dir)
	}
	return shelf.Dir(m.workDir)
}

// shelfLabel is what an entry is listed under: the name it was shelved with, or
// its identifier when it was shelved without one.
func shelfLabel(e shelf.Entry) string {
	if name := strings.TrimSpace(e.Name); name != "" {
		return name
	}
	return e.ID
}

// underShelfStore reports whether a path names the shelf store or something
// inside it. The store's name matches Subversion's built-in ignore glob, so it
// normally never reaches a status report at all; a path that does means the
// user's own global-ignores has replaced that built-in list, since a value there
// is a replacement rather than an addition.
func underShelfStore(path string) bool {
	return path == shelf.DirName || strings.HasPrefix(path, shelf.DirName+"/")
}

// withoutShelfStore drops the shelf store from a status report, returning what
// is left and whether anything was taken out. Whatever svn has been told to
// think of it, the store is revision's own bookkeeping and never the working
// copy's content: it is not staged, committed, reverted or shelved.
func withoutShelfStore(items []svn.StatusItem) ([]svn.StatusItem, bool) {
	kept := make([]svn.StatusItem, 0, len(items))
	for _, it := range items {
		if underShelfStore(it.Path) {
			continue
		}
		kept = append(kept, it)
	}
	return kept, len(kept) != len(items)
}

// noteShelfStoreVisible says once in a session that svn can see the shelf store,
// which it only can when global-ignores has been set and the built-in ".#*"
// rule went with it. revision keeps the store out of its own views either way;
// what the user is being told is that their other svn tools will not.
func (m *Model) noteShelfStoreVisible(leaked bool) {
	if !leaked || m.shelfStoreWarned {
		return
	}
	m.shelfStoreWarned = true
	m.showToast("svn can see "+shelf.DirName+": a global-ignores setting has replaced the built-in \".#*\" rule",
		component.LevelWarning)
}

// shelfSize is how many files an entry holds, counting the unversioned ones it
// carries as bytes alongside the ones its patch describes.
func shelfSize(e shelf.Entry) int { return len(e.Files) + len(e.Untracked) }

// renderShelfEntry is the domain adapter that turns a shelved change set into
// the row the reusable List renders: a marker, what it was shelved as, and how
// much it holds and when it was taken in muted text.
func renderShelfEntry(th theme.Theme) func(shelf.Entry) string {
	return func(e shelf.Entry) string {
		marker := lipgloss.NewStyle().Foreground(th.Success).Bold(true).Render("▣")
		name := lipgloss.NewStyle().Foreground(th.Text).Render(shelfLabel(e))
		meta := lipgloss.NewStyle().Foreground(th.Muted).
			Render(fmt.Sprintf(" (%s · %s)", fileCount(shelfSize(e)), e.Created.Format("2006-01-02 15:04")))
		return marker + " " + name + meta
	}
}

// rebuildShelves repopulates the panel from the scanned entries, narrowed by its
// filter. An entry describes changes rather than working-copy state, so the
// filter is plain free text, matched on what the entry is listed under.
func (m *Model) rebuildShelves() {
	q := strings.TrimSpace(m.filters[panelShelf])
	if q == "" {
		m.shelves.SetItems(m.shelfItems)
		return
	}
	out := make([]shelf.Entry, 0, len(m.shelfItems))
	for _, e := range m.shelfItems {
		if containsFold(shelfLabel(e), q) {
			out = append(out, e)
		}
	}
	m.shelves.SetItems(out)
}

// shelfLoadForSelection returns a command to read the highlighted entry's patch
// when it is not already the one on screen.
func (m *Model) shelfLoadForSelection() tea.Cmd {
	e, ok := m.shelves.Selected()
	if !ok || m.shelfID == e.ID {
		return nil
	}
	return m.readShelf(e.ID)
}

// shelfFooter returns the position/count indicator inlaid into the Shelf panel's
// bottom border, with the unfiltered total in brackets when a filter hides some.
func (m *Model) shelfFooter() string {
	return countLabel(m.shelves.Index()+1, len(m.shelves.Items()), len(m.shelfItems))
}

// shelfDetail renders the highlighted entry in Main: the patch it holds, with a
// placeholder while it is being read, when the store is empty, or when it could
// not be listed.
func (m *Model) shelfDetail() string {
	if m.shelfErr != nil {
		return "Unable to list shelves: " + m.shelfErr.Error()
	}
	e, ok := m.shelves.Selected()
	if !ok {
		if len(m.shelfItems) > 0 {
			return "No shelves match the filter."
		}
		return "Nothing shelved yet."
	}
	switch {
	case m.shelfID != e.ID:
		return "Reading " + shelfLabel(e) + "…"
	case m.shelfReadErr:
		return m.shelfText
	case strings.TrimSpace(m.shelfText) == "":
		return "(" + shelfLabel(e) + " holds no textual changes)"
	default:
		return m.colorize(m.shelfText)
	}
}

// shelfShowsPatch reports whether shelfDetail currently renders a unified diff,
// which is the only Main view with a +/-/space gutter to pin.
func (m *Model) shelfShowsPatch() bool {
	e, ok := m.shelves.Selected()
	return ok && m.shelfErr == nil && !m.shelfReadErr &&
		m.shelfID == e.ID && strings.TrimSpace(m.shelfText) != ""
}

// shelveScope is the set of changes a shelve takes in. label names it in the
// prompt and is what the entry falls back to being called.
type shelveScope struct {
	label string
	items []svn.StatusItem
}

// shelvableItem reports whether a status entry is one a shelf can take and put
// back. A conflicted file is left out: it needs deciding rather than setting
// aside, and reverting one throws the merge away. Ignored and external entries
// are not this working copy's changes to move.
func shelvableItem(it svn.StatusItem) bool {
	switch it.State {
	case svn.StateConflicted, svn.StateIgnored, svn.StateExternal,
		svn.StateObstructed, svn.StateIncomplete:
		return false
	case svn.StateUnversioned:
		return true
	}
	return it.State.IsDirty() || it.PropState.IsDirty()
}

// shelvableItems narrows a set of status entries to the ones a shelf can take.
func shelvableItems(items []svn.StatusItem) []svn.StatusItem {
	out := make([]svn.StatusItem, 0, len(items))
	for _, it := range items {
		if shelvableItem(it) {
			out = append(out, it)
		}
	}
	return out
}

// itemPaths pulls the paths out of a set of status entries.
func itemPaths(items []svn.StatusItem) []string {
	paths := make([]string, 0, len(items))
	for _, it := range items {
		paths = append(paths, it.Path)
	}
	return paths
}

// isShelfPicked reports whether a path is held for the next shelve.
func (m *Model) isShelfPicked(path string) bool { return m.shelfPicks[path] }

// nodePicked reports whether a Files-panel tree row is held: a file leaf on its
// own account, a directory row when everything shelvable beneath it is.
func (m *Model) nodePicked(n fileNode) bool {
	if n.Item != nil {
		return m.isShelfPicked(n.Item.Path)
	}
	under := shelvableItems(filesUnder(n, m.fileItems))
	if len(under) == 0 {
		return false
	}
	for _, it := range under {
		if !m.isShelfPicked(it.Path) {
			return false
		}
	}
	return true
}

// groupPicked counts how many of a changelist's files are held.
func (m *Model) groupPicked(g changelistGroup) int {
	n := 0
	for _, it := range g.Items {
		if m.isShelfPicked(it.Path) {
			n++
		}
	}
	return n
}

// pickedItems resolves the held paths against the working copy as it stands, so
// a pick whose file has since been committed or reverted simply drops out.
func (m *Model) pickedItems() []svn.StatusItem {
	if len(m.shelfPicks) == 0 {
		return nil
	}
	var out []svn.StatusItem
	for _, it := range m.fileItems {
		if m.shelfPicks[it.Path] && shelvableItem(it) {
			out = append(out, it)
		}
	}
	return out
}

// toggleShelfPick holds or releases what the Files panel points at, for the next
// shelve. It works on a file leaf, on a directory row (everything shelvable
// beneath it, so a subtree is picked in one press), and on a changelist row in
// the Changelists overview — which is how changes already filed under a
// changelist are shelved without drilling into it.
func (m *Model) toggleShelfPick() tea.Cmd {
	if m.filesViewIsStore() {
		return nil
	}
	var targets []svn.StatusItem
	switch {
	case m.filesViewIsChangelists() && !m.inChangelistDrill():
		g, ok := m.changelists.Selected()
		if !ok {
			return nil
		}
		targets = shelvableItems(g.Items)
	default:
		n, items, ok := m.selectedTreeNode()
		if !ok {
			return nil
		}
		if n.Item != nil {
			targets = shelvableItems([]svn.StatusItem{*n.Item})
		} else {
			targets = shelvableItems(filesUnder(n, items))
		}
	}
	if len(targets) == 0 {
		m.showToast("nothing here can be shelved", component.LevelWarning)
		return nil
	}
	m.setShelfPicks(targets, !m.allPicked(targets))
	m.rebuildFilesViews()
	m.updateBar()
	return nil
}

// allPicked reports whether every one of the targets is already held, which is
// what makes a second press on the same row release them.
func (m *Model) allPicked(targets []svn.StatusItem) bool {
	for _, it := range targets {
		if !m.isShelfPicked(it.Path) {
			return false
		}
	}
	return len(targets) > 0
}

// setShelfPicks holds or releases a set of paths.
func (m *Model) setShelfPicks(targets []svn.StatusItem, hold bool) {
	if m.shelfPicks == nil {
		m.shelfPicks = map[string]bool{}
	}
	for _, it := range targets {
		if hold {
			m.shelfPicks[it.Path] = true
		} else {
			delete(m.shelfPicks, it.Path)
		}
	}
}

// clearShelfPicks releases everything held, reporting whether there was anything
// to release so esc can tell whether it was consumed here.
func (m *Model) clearShelfPicks() bool {
	if len(m.shelfPicks) == 0 {
		return false
	}
	m.shelfPicks = nil
	m.rebuildFilesViews()
	m.updateBar()
	return true
}

// shelfPickLabel describes what is held, for the status bar.
func (m *Model) shelfPickLabel() string {
	return fileCount(len(m.pickedItems())) + " picked"
}

// openShelve takes what is held out of the working copy. With nothing held it is
// the whole working copy that would go, which is a large enough thing to do by
// one keystroke that it is confirmed first.
func (m *Model) openShelve() tea.Cmd {
	if picked := m.pickedItems(); len(picked) > 0 {
		m.openShelveName(shelveScope{label: pickedScopeLabel(picked), items: picked})
		return nil
	}
	items := shelvableItems(m.fileItems)
	if len(items) == 0 {
		m.showToast("nothing to shelve", component.LevelWarning)
		return nil
	}
	m.confirmAction(func() tea.Msg { return shelveAllMsg{} }, nil)
	m.openConfirm("Shelve all changes?",
		"Nothing is picked, so all "+fileCount(len(items))+" will be taken out of the working "+
			"copy and put on the shelf. Pick files with v to shelve only some.")
	return nil
}

// pickedScopeLabel is what a shelve of the held files defaults to being called.
func pickedScopeLabel(items []svn.StatusItem) string {
	if len(items) == 1 {
		return items[0].Path
	}
	return "picked changes"
}

// openShelveName floats the prompt that names the entry. The scope is
// snapshotted here, so what is shelved is what was picked however the working
// copy moves behind the overlay.
func (m *Model) openShelveName(scope shelveScope) {
	m.shelveTarget = scope
	m.shelfNaming = true
	m.shelfEditor.Reset()
	m.shelfEditor.Focus()
	m.sizeShelfName()
}

// closeShelfName hides the name prompt and drops the queued scope.
func (m *Model) closeShelfName() {
	m.shelfNaming = false
	m.shelveTarget = shelveScope{}
	m.shelfEditor.Blur()
}

// submitShelveName takes the queued set out of the working copy under the
// entered name.
func (m *Model) submitShelveName(name string) tea.Cmd {
	scope := m.shelveTarget
	m.closeShelfName()
	if len(scope.items) == 0 {
		return nil
	}
	return shelveCmd(m.client, m.shelfDir(), shelveRequest{
		name:    shelveEntryName(name, scope.label),
		baseRev: m.wcRevision,
		items:   scope.items,
	})
}

// shelveEntryName is what an entry ends up called: what was typed, or the set it
// was taken from when nothing was.
func shelveEntryName(entered, fallback string) string {
	if name := strings.TrimSpace(entered); name != "" {
		return name
	}
	return fallback
}

// shelveToast describes a finished shelve: what was taken, and what stayed in
// the working copy because no patch could carry it.
func shelveToast(e shelf.Entry, left []string) (string, component.Level) {
	text := "shelved " + fileCount(shelfSize(e)) + " as " + shelfLabel(e)
	if len(left) == 0 {
		return text, component.LevelSuccess
	}
	return text + ", " + fileCount(len(left)) + " left behind (svn cannot put them in a patch)",
		component.LevelWarning
}

// requestRestoreShelf asks to put the highlighted entry back, opening a
// confirmation modal. pop drops the entry afterwards, so what it says differs.
// Nothing is held pending: a shelf lands on files spread across the tree, most
// of which the Changes view is not even showing, so the status reload that
// follows is what says where it went.
func (m *Model) requestRestoreShelf(pop bool) tea.Cmd {
	e, ok := m.shelves.Selected()
	if !ok {
		m.showToast("no shelf to restore", component.LevelWarning)
		return nil
	}
	m.confirmAction(restoreShelfCmd(m.client, m.shelfDir(), e, pop), nil)
	title, body := "Apply shelf?", "Merge "+shelfLabel(e)+" back into the working copy, keeping it on the shelf."
	if pop {
		title = "Pop shelf?"
		body = "Merge " + shelfLabel(e) + " back into the working copy and take it off the shelf. " +
			"It is kept if any of it does not go back cleanly."
	}
	m.openConfirm(title, body+m.shelfDriftNote(e))
	return nil
}

// shelfDriftNote warns that an entry was shelved at a different revision from
// the one the working copy is at now, which is when hunks stop fitting.
func (m *Model) shelfDriftNote(e shelf.Entry) string {
	if e.BaseRevision == "" || m.wcRevision == "" || e.BaseRevision == m.wcRevision {
		return ""
	}
	return " It was shelved at r" + e.BaseRevision + " and the working copy is at r" + m.wcRevision +
		"; whatever no longer fits is left beside its file in a .rej."
}

// requestDropShelf asks to remove the highlighted entry, opening a confirmation
// modal. Unlike a revert there is nothing behind it: the shelf is the only copy
// of what it holds.
func (m *Model) requestDropShelf() tea.Cmd {
	e, ok := m.shelves.Selected()
	if !ok {
		m.showToast("no shelf to drop", component.LevelWarning)
		return nil
	}
	m.confirmAction(dropShelfCmd(m.shelfDir(), e.ID, shelfLabel(e)), nil)
	m.openConfirm("Drop shelf?",
		"Permanently delete "+shelfLabel(e)+" and the "+fileCount(shelfSize(e))+
			" it holds. Nothing else has a copy of them.")
	return nil
}

// openShelfRename floats the prompt that relabels the highlighted entry,
// prefilled with what it is called now.
func (m *Model) openShelfRename() tea.Cmd {
	e, ok := m.shelves.Selected()
	if !ok {
		m.showToast("no shelf to rename", component.LevelWarning)
		return nil
	}
	m.renameTarget = e.ID
	m.shelfRenaming = true
	m.renameEditor.Reset()
	m.renameEditor.SetValue(shelfLabel(e))
	m.renameEditor.Focus()
	m.sizeShelfRename()
	return nil
}

// closeShelfRename hides the rename prompt and drops the entry it was editing.
func (m *Model) closeShelfRename() {
	m.shelfRenaming = false
	m.renameTarget = ""
	m.renameEditor.Blur()
}

// submitShelfRename relabels the entry the prompt was opened on. A blank name is
// left alone: an entry with no name of its own is listed by its identifier, and
// blanking one is more likely a slip than an intent.
func (m *Model) submitShelfRename(name string) tea.Cmd {
	id := m.renameTarget
	m.closeShelfRename()
	name = strings.TrimSpace(name)
	if id == "" || name == "" {
		return nil
	}
	return renameShelfCmd(m.shelfDir(), id, name)
}

// shelfRestoreToast describes a finished restore: the files svn put back, and
// whatever did not come with them.
func shelfRestoreToast(msg shelfRestoredMsg) (string, component.Level) {
	verb := "applied"
	if msg.dropped {
		verb = "popped"
	}
	text := verb + " " + msg.name + " to " + fileCount(len(msg.res.Applied)+len(msg.restored))
	var left []string
	if n := len(msg.res.Conflicted); n > 0 {
		left = append(left, fmt.Sprintf("%d with rejects (.rej)", n))
	}
	if n := len(msg.res.Skipped); n > 0 {
		left = append(left, fmt.Sprintf("%d not found", n))
	}
	if n := len(msg.blocked); n > 0 {
		left = append(left, fmt.Sprintf("%d already in the way", n))
	}
	if len(left) == 0 {
		return text, component.LevelSuccess
	}
	return text + ", " + strings.Join(left, ", ") + " — kept on the shelf", component.LevelWarning
}
