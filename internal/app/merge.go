package app

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// mergeContextRows is how many unchanged lines are drawn above and below a
// region, so a side is judged in the company it keeps rather than on its own.
const mergeContextRows = 3

// The markers svn (and every other three-way merge) leaves in a conflicted file.
// The base section only appears when the merge was told to record it; its text
// is neither of the two candidates, so it is passed over.
const (
	conflictMineMarker   = "<<<<<<<"
	conflictBaseMarker   = "|||||||"
	conflictSplitMarker  = "======="
	conflictTheirsMarker = ">>>>>>>"
)

// mergeKind is what left a file needing a decision: an update that could not
// reconcile two versions of a line, or a patch whose hunks would not go in.
type mergeKind int

const (
	mergeConflict mergeKind = iota
	mergeReject
)

// mergeChoice is which of a region's two candidates the merged file takes.
// chooseNone leaves the region as it stands, which is what every region starts
// as and what keeps a file from being written half-decided.
type mergeChoice int

const (
	chooseNone mergeChoice = iota
	chooseLeft
	chooseRight
	chooseBoth
)

// mergeRegion is one decision to make: the span [start,end) of the file it
// replaces, the two candidates for it, and the one chosen so far. at is where
// the left candidate's first line sits in the file — the same as start for a
// reject, one past it for a conflict, whose span opens with a marker line.
type mergeRegion struct {
	start  int
	end    int
	at     int
	left   []string
	right  []string
	choice mergeChoice
}

// mergeDoc is a file with decisions outstanding: its lines as they stand, the
// regions to decide, and where the result goes. It is the one shape behind both
// resolution flows — a conflicted file and a reject's unplaced hunks differ only
// in where their regions come from and in what clearing them afterwards means.
type mergeDoc struct {
	kind mergeKind
	// path is the file the merged text is written to and rel the name it is
	// known by on screen. aux is the reject file to remove once its hunks have
	// been decided; a conflict has none.
	path string
	rel  string
	aux  string
	// left and right name the two candidates, as the panes are headed.
	left  string
	right string
	// lines is the file as it stands and trailing whether it ended with a
	// newline, so the merged result ends the way the original did.
	lines    []string
	trailing bool
	regions  []mergeRegion
	// unplaced counts the reject hunks whose expected text is no longer in the
	// file, which cannot be offered as a decision.
	unplaced int
}

// unresolved is how many regions are still waiting on a decision.
func (d *mergeDoc) unresolved() int {
	n := 0
	for _, r := range d.regions {
		if r.choice == chooseNone {
			n++
		}
	}
	return n
}

// merged is the file as the decisions leave it: every region replaced by the
// candidate chosen for it, everything between them untouched. A region still
// undecided keeps the text that is there now, so a partially decided document
// still yields a coherent file.
func (d *mergeDoc) merged() string {
	out := make([]string, 0, len(d.lines))
	at := 0
	for _, r := range d.regions {
		out = append(out, d.lines[at:r.start]...)
		switch r.choice {
		case chooseLeft:
			out = append(out, r.left...)
		case chooseRight:
			out = append(out, r.right...)
		case chooseBoth:
			out = append(out, r.left...)
			out = append(out, r.right...)
		default:
			out = append(out, d.lines[r.start:r.end]...)
		}
		at = r.end
	}
	out = append(out, d.lines[at:]...)
	text := strings.Join(out, "\n")
	if d.trailing {
		text += "\n"
	}
	return text
}

// conflictDoc reads a conflicted file into the decisions its markers describe.
func conflictDoc(path, rel, text string) *mergeDoc {
	lines, trailing := splitFileLines(text)
	regions, left, right := conflictRegions(lines)
	return &mergeDoc{
		kind: mergeConflict, path: path, rel: rel,
		left: left, right: right,
		lines: lines, trailing: trailing, regions: regions,
	}
}

// rejectDoc pairs a reject with the file it was written for, turning each hunk
// that still fits into a decision between the text that is there now and the
// text the patch wanted to put there.
func rejectDoc(rejPath, rel, rejText, targetPath, targetText string) *mergeDoc {
	lines, trailing := splitFileLines(targetText)
	regions, unplaced := rejectRegions(rejText, lines)
	return &mergeDoc{
		kind: mergeReject, path: targetPath, rel: rel, aux: rejPath,
		left: "working copy", right: "rejected hunk",
		lines: lines, trailing: trailing, regions: regions, unplaced: unplaced,
	}
}

// conflictSide is which section of a conflict block a scan is inside.
type conflictSide int

const (
	sideMine conflictSide = iota
	sideBase
	sideTheirs
)

// conflictRegions reads the conflict markers out of a file: everything between
// "<<<<<<<" and "=======" is one candidate, everything between "=======" and
// ">>>>>>>" the other, and the whole block — markers and all — is the span they
// replace. The optional "|||||||" section holds the common ancestor, which is
// neither candidate and is dropped along with the markers. The markers' own
// labels name the two sides. An unterminated block ends the scan: without its
// closing marker there is no telling where the second candidate stops.
func conflictRegions(lines []string) (regions []mergeRegion, left, right string) {
	left, right = "mine", "theirs"
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], conflictMineMarker) {
			continue
		}
		var mine, theirs []string
		side, end := sideMine, -1
		for j := i + 1; j < len(lines) && end < 0; j++ {
			switch ln := lines[j]; {
			case strings.HasPrefix(ln, conflictBaseMarker):
				side = sideBase
			case strings.HasPrefix(ln, conflictSplitMarker):
				side = sideTheirs
			case strings.HasPrefix(ln, conflictTheirsMarker):
				end = j
			case side == sideMine:
				mine = append(mine, ln)
			case side == sideTheirs:
				theirs = append(theirs, ln)
			}
		}
		if end < 0 {
			break
		}
		if len(regions) == 0 {
			left = markerLabel(lines[i], conflictMineMarker, left)
			right = markerLabel(lines[end], conflictTheirsMarker, right)
		}
		regions = append(regions, mergeRegion{start: i, end: end + 1, at: i + 1, left: mine, right: theirs})
		i = end
	}
	return regions, left, right
}

// markerLabel names a side from its conflict marker — "<<<<<<< .mine" is "mine",
// ">>>>>>> .r42" is "r42" — falling back to fallback for an unlabelled one.
func markerLabel(line, marker, fallback string) string {
	label := strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, marker)), ".")
	if label == "" {
		return fallback
	}
	return label
}

// rejectHunk is one hunk out of a reject file: the line it claims to belong at,
// the text it expects to find there, and what it would put in its place.
type rejectHunk struct {
	at  int
	old []string
	new []string
}

// rejectRegions turns a reject's hunks into decisions against the file they were
// meant for. A hunk is only offered once its expected text has been found: the
// file has moved on since the patch was written — that is why the hunk was
// rejected — so where it belongs is decided by matching its text rather than by
// the line number it names. Hunks that no longer match anything, and those that
// would land on top of one already placed, are counted as unplaced instead.
func rejectRegions(rej string, lines []string) (regions []mergeRegion, unplaced int) {
	var placed []mergeRegion
	for _, h := range parseRejectHunks(rej) {
		at, ok := locateHunk(lines, h.old, max(h.at-1, 0))
		if !ok {
			unplaced++
			continue
		}
		placed = append(placed, mergeRegion{
			start: at, end: at + len(h.old), at: at,
			left:  append([]string(nil), lines[at:at+len(h.old)]...),
			right: h.new,
		})
	}
	sort.SliceStable(placed, func(i, j int) bool { return placed[i].start < placed[j].start })
	end := 0
	for _, r := range placed {
		if r.start < end {
			unplaced++
			continue
		}
		regions = append(regions, r)
		end = r.end
	}
	return regions, unplaced
}

// parseRejectHunks reads the hunks out of a reject file, which is a unified diff
// of just the changes that would not go in. Context and removed lines make up
// the text a hunk expects to find; context and added lines what it would leave
// behind. Anything that is neither ends the hunk, so the headers between a
// multi-file reject's sections do not run into it.
func parseRejectHunks(text string) []rejectHunk {
	var (
		out  []rejectHunk
		cur  rejectHunk
		open bool
	)
	flush := func() {
		if open {
			out = append(out, cur)
		}
		cur, open = rejectHunk{}, false
	}
	// The newline a reject ends with closes its last line rather than opening
	// another, so it is dropped before the body is read as one.
	body, _ := splitFileLines(text)
	for _, ln := range body {
		if before, _, ok := parseHunkHeader(ln); ok {
			flush()
			cur, open = rejectHunk{at: before.start}, true
			continue
		}
		if !open {
			continue
		}
		switch {
		case strings.HasPrefix(ln, `\`):
			// "\ No newline at end of file" annotates the line above, not one of its own.
		case strings.HasPrefix(ln, "-"):
			cur.old = append(cur.old, ln[1:])
		case strings.HasPrefix(ln, "+"):
			cur.new = append(cur.new, ln[1:])
		case ln == "" || strings.HasPrefix(ln, " "):
			// A context line stands on both sides. Trailing whitespace is often
			// stripped in transit, which leaves a blank context line truly empty.
			body := strings.TrimPrefix(ln, " ")
			cur.old = append(cur.old, body)
			cur.new = append(cur.new, body)
		default:
			flush()
		}
	}
	flush()
	return out
}

// locateHunk finds where a hunk's expected text sits in the file now, searching
// outwards from the line the hunk names so the nearest of several identical
// blocks is the one taken. A hunk that expects nothing — a pure insertion — goes
// where it says, if the file is still long enough to have that line.
func locateHunk(lines, old []string, hint int) (int, bool) {
	if len(old) == 0 {
		if hint > len(lines) {
			return 0, false
		}
		return hint, true
	}
	for d := 0; d <= len(lines); d++ {
		if matchesAt(lines, old, hint+d) {
			return hint + d, true
		}
		if d > 0 && matchesAt(lines, old, hint-d) {
			return hint - d, true
		}
	}
	return 0, false
}

// matchesAt reports whether block appears in lines starting at at.
func matchesAt(lines, block []string, at int) bool {
	if at < 0 || at+len(block) > len(lines) {
		return false
	}
	for i, ln := range block {
		if lines[at+i] != ln {
			return false
		}
	}
	return true
}

// splitFileLines breaks a file into its lines, remembering whether it ended with
// a newline so the merged result can end the way the original did. The newline a
// file ends with closes its last line rather than opening another, so it does
// not count as a line of its own; an empty file has no lines at all.
func splitFileLines(text string) (lines []string, trailing bool) {
	if text == "" {
		return nil, false
	}
	trailing = strings.HasSuffix(text, "\n")
	if trailing {
		text = text[:len(text)-1]
	}
	return strings.Split(text, "\n"), trailing
}

// rejectTarget names the file a reject was written for. Both svn and GNU patch
// name a reject after its target, svn interposing its own ".svnpatch" first.
func rejectTarget(path string) string {
	return strings.TrimSuffix(strings.TrimSuffix(path, ".rej"), ".svnpatch")
}

// mergePages lays each region out as a page of its own: a few unchanged lines
// for bearings, then the two candidates side by side, the left one against the
// right. It is splitDiffPages' counterpart for a decision rather than a reading
// — the same two-pane layout, with the side that has been chosen standing out
// and the one passed over receding.
func mergePages(th theme.Theme, d *mergeDoc) []component.SplitPage {
	width := max(len(strconv.Itoa(len(d.lines))), minDiffNumWidth)
	meta := lipgloss.NewStyle().Foreground(th.Muted)
	text := lipgloss.NewStyle().Foreground(th.Text)

	pages := make([]component.SplitPage, 0, len(d.regions))
	for _, r := range d.regions {
		left, right := mergeSideStyles(th, r.choice)
		var rows []component.SplitRow
		context := func(from, to int) {
			for i := max(from, 0); i < min(to, len(d.lines)); i++ {
				cell := diffCell(meta, text, width, i+1, " ", d.lines[i])
				rows = append(rows, component.SplitRow{Left: cell, Right: cell, Line: i + 1})
			}
		}
		context(r.start-mergeContextRows, r.start)
		for i := 0; i < len(r.left) || i < len(r.right); i++ {
			// Every row of a region points at where the region opens, so editing by
			// hand from any of them lands on the decision rather than inside it.
			row := component.SplitRow{Line: r.at + 1}
			if i < len(r.left) {
				row.Left = diffCell(meta, left, width, r.at+1+i, " ", r.left[i])
			}
			if i < len(r.right) {
				row.Right = diffCell(meta, right, width, r.at+1+i, " ", r.right[i])
			}
			rows = append(rows, row)
		}
		context(r.end, r.end+mergeContextRows)
		pages = append(pages, component.SplitPage{Title: "line " + strconv.Itoa(r.at+1), Rows: rows})
	}
	return pages
}

// mergeSideStyles colors the two candidates: the left as a diff colors what is
// being taken away and the right what is coming in, until a decision is made, at
// which point the side passed over recedes into the margin's color.
func mergeSideStyles(th theme.Theme, c mergeChoice) (left, right lipgloss.Style) {
	left = lipgloss.NewStyle().Foreground(th.Error)
	right = lipgloss.NewStyle().Foreground(th.Success)
	dim := lipgloss.NewStyle().Foreground(th.Muted)
	switch c {
	case chooseLeft:
		right = dim
	case chooseRight:
		left = dim
	}
	return left, right
}

// mergeTitle heads the overlay with what is being resolved and how much of it is
// still outstanding. The count goes here rather than in the footer, which has
// only the room the pane labels and the scroll position leave it.
func mergeTitle(d *mergeDoc) string {
	subject := "Resolve conflict — " + d.rel
	if d.kind == mergeReject {
		subject = "Resolve rejects — " + d.rel
	}
	if n := d.unresolved(); n > 0 {
		return fmt.Sprintf("%s · %d of %d undecided", subject, n, len(d.regions))
	}
	return subject + " · all decided"
}

// mergeHint is the footer legend: the keys that decide the region on screen, and
// — once every region has been decided — the key that writes the result out,
// which is offered only when it would do anything. It spells out only the two
// choices that head no pane; 1 and 2 are named where they act, above the side
// each one takes.
func mergeHint(d *mergeDoc) string {
	if d.unresolved() > 0 {
		return "3 take both · 0 undo · [ ] next · esc close"
	}
	return "3 take both · 0 undo · w write · esc close"
}

// mergePaneLabel heads a pane with what its key does to it — "1 take mine"
// rather than a bare digit, so the keys need no legend — and a tick once it has
// been taken.
func mergePaneLabel(key, name string, taken bool) string {
	mark := "  "
	if taken {
		mark = "✓ "
	}
	return key + " " + mark + "take " + name
}

// mergeNothingToDo explains a file that turned out to need no decisions, which
// is a different situation for each kind: a conflicted file with no markers has
// been dealt with by hand already, while a reject none of whose hunks fit has
// nothing to offer but its own text.
func mergeNothingToDo(d *mergeDoc) string {
	if d.kind == mergeReject {
		return "none of " + filepath.Base(d.aux) + "'s hunks still fit " + d.rel + " — apply them by hand (e)"
	}
	return "no conflict markers left in " + d.rel + " — resolve it with svn, or edit it (e)"
}

// openMerge floats the resolution overlay for the active Files-panel view. The
// two things that need resolving are deliberately kept apart, because they are
// different decisions about different text: the Changes tree resolves a
// conflicted file's own markers, and the Rejects view resolves a reject's hunks
// against the file it was written for. A file left both conflicted and with
// rejects beside it is therefore dealt with once in each view, rather than in
// one pass that mixes the two. Whichever it is, the file is read off disk when
// the key is pressed, so what is decided is the file as it actually stands.
func (m *Model) openMerge() tea.Cmd {
	switch {
	case m.filesViewIsRejects():
		return m.openRejectMerge()
	case m.filesViewIsDiffs():
		m.showToast("a saved patch has nothing to resolve — apply it with p", component.LevelWarning)
		return nil
	case m.filesViewIsChangelists() && !m.inChangelistDrill():
		m.showToast("select a file to resolve, not a changelist", component.LevelWarning)
		return nil
	}
	return m.openConflictMerge()
}

// openRejectMerge reads the highlighted reject against the file it was written
// for. A directory row names no reject of its own.
func (m *Model) openRejectMerge() tea.Cmd {
	r, ok := m.selectedReject()
	if !ok {
		m.showToast("no reject selected to resolve", component.LevelWarning)
		return nil
	}
	return loadRejectMergeCmd(r.Path, r.Rel)
}

// openConflictMerge reads the conflicted file under the working-copy cursor —
// the Changes tree, or a drilled-in changelist. Only a conflicted file has
// markers to decide; a reject sitting beside a file that is otherwise fine
// belongs to the Rejects view, which is where the warning points.
func (m *Model) openConflictMerge() tea.Cmd {
	it, ok := m.selectedFile()
	if !ok {
		m.showToast("select a file to resolve", component.LevelWarning)
		return nil
	}
	if it.State != svn.StateConflicted {
		m.showToast(it.Path+" is not in conflict — rejects are resolved in the Rejects view",
			component.LevelWarning)
		return nil
	}
	return loadConflictCmd(m.absPath(it.Path), it.Path)
}

// showMerge puts a loaded document on screen, opening on its first decision.
func (m *Model) showMerge(d *mergeDoc) {
	m.mergeDoc = d
	m.merging = true
	m.mergeView.SetPages(mergePages(m.theme, d))
	m.syncMerge()
	m.mergeView.Focus()
	m.sizeMerge()
}

// closeMerge hides the resolution overlay, dropping the decisions with it: they
// describe a file as it was read, which anything else may since have changed.
func (m *Model) closeMerge() {
	m.merging = false
	m.mergeDoc = nil
	m.mergeView.Blur()
}

// sizeMerge sizes the overlay like the side-by-side diff it borrows: two panes
// need every column they can get.
func (m *Model) sizeMerge() {
	w := clamp(m.width-4, 40, max(m.width-2, 40))
	h := clamp(m.height-4, 8, max(m.height-2, 8))
	m.mergeView.SetSize(w, h)
}

// mergeKey drives the overlay while it is up: the digits decide the region on
// screen, w writes the result out, and everything else is the SplitView's own —
// scrolling, and the view keys that move between regions.
func (m *Model) mergeKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "1":
		return m.chooseMerge(chooseLeft)
	case "2":
		return m.chooseMerge(chooseRight)
	case "3":
		return m.chooseMerge(chooseBoth)
	case "0":
		return m.chooseMerge(chooseNone)
	case "w":
		return m.writeMerge()
	case "m":
		m.closeMerge()
		return nil
	}
	if key.Matches(k, m.keys.OpenEditor) {
		// Merging by hand is always available: the overlay holds a snapshot, so it
		// steps aside rather than sit over a file being changed.
		if cmd := m.openInEditor(); cmd != nil {
			m.closeMerge()
			return cmd
		}
		return nil
	}
	cmd := m.mergeView.Update(k)
	// A view key may have moved to another region, whose decision the panes and
	// the footer describe.
	m.syncMerge()
	return cmd
}

// chooseMerge records a decision for the region on screen and re-renders in
// place, so the choice shows without losing the reader's position.
func (m *Model) chooseMerge(c mergeChoice) tea.Cmd {
	d := m.mergeDoc
	i := m.mergeView.Page()
	if d == nil || i < 0 || i >= len(d.regions) {
		return nil
	}
	d.regions[i].choice = c
	m.mergeView.RefreshPages(mergePages(m.theme, d))
	m.syncMerge()
	return nil
}

// syncMerge re-heads the overlay from the state of the decisions: how much is
// still outstanding, which key takes which side of the region on screen, and
// which of them has been taken.
func (m *Model) syncMerge() {
	d := m.mergeDoc
	if d == nil {
		return
	}
	c := chooseNone
	if i := m.mergeView.Page(); i >= 0 && i < len(d.regions) {
		c = d.regions[i].choice
	}
	m.mergeView.SetTitle(mergeTitle(d))
	m.mergeView.SetLabels(
		mergePaneLabel("1", d.left, c == chooseLeft || c == chooseBoth),
		mergePaneLabel("2", d.right, c == chooseRight || c == chooseBoth),
	)
	m.mergeView.SetHint(mergeHint(d))
}

// writeMerge writes the decisions out, once every one of them has been made. A
// file written half-decided would still hold the markers (or the hunks) it was
// opened for, so the remaining ones are asked for instead.
func (m *Model) writeMerge() tea.Cmd {
	d := m.mergeDoc
	if d == nil {
		return nil
	}
	if n := d.unresolved(); n > 0 {
		m.showToast(fmt.Sprintf("%d of %d still undecided — pick a side for each (1/2/3)",
			n, len(d.regions)), component.LevelWarning)
		return nil
	}
	m.closeMerge()
	return writeMergeCmd(m.client, d)
}

// mergeLine is where in the file the overlay is reading, so editing by hand
// picks up on the decision on screen.
func (m *Model) mergeLine() int {
	_, row, ok := m.mergeView.Current()
	if !ok {
		return 0
	}
	return row.Line
}

// mergeDoneText reports what a written resolution amounted to.
func mergeDoneText(msg mergeWrittenMsg) string {
	if msg.kind == mergeReject {
		return fmt.Sprintf("applied %s to %s — %s cleared",
			blockCount(msg.count, "hunk"), msg.rel, filepath.Base(msg.aux))
	}
	return fmt.Sprintf("resolved %s in %s", blockCount(msg.count, "conflict"), msg.rel)
}

// blockCount renders a count of decided blocks for a toast, with the right plural.
func blockCount(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
