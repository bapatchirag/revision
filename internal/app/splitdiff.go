package app

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// minDiffNumWidth is the narrowest the line-number column is drawn, so short
// files still line their numbers up under a common margin.
const minDiffNumWidth = 3

// hunkRange is one side of a hunk header's "@@ -start,count +start,count @@"
// span: where the hunk begins in that version of the file and how many lines of
// it the hunk covers.
type hunkRange struct {
	start int
	count int
}

// openSplitDiff floats the side-by-side view of whatever diff Main is showing
// for the Files panel — a file's, a directory subtree's combined one, or a saved
// patch being browsed. A selection Main has no diff for warns instead, as does
// one whose diff is still loading or failed to load. The rows are laid out once,
// from the diff as it stands, so a reload landing behind the overlay cannot
// re-shuffle what is being read.
func (m *Model) openSplitDiff() tea.Cmd {
	diff, target, ok := m.mainDiff()
	if !ok {
		m.showToast("no diff to compare for this selection", component.LevelWarning)
		return nil
	}
	pages := splitDiffPages(m.theme, diff)
	if len(pages) == 0 {
		m.showToast("no diff to compare for this selection", component.LevelWarning)
		return nil
	}
	left, right := splitDiffLabels(diff)
	m.splitDiff.SetTitle("Side-by-side diff — " + target)
	m.splitDiff.SetLabels(left, right)
	m.splitDiff.SetPages(pages)
	m.splitDiff.SetHint(splitDiffHint(len(pages)))
	m.splitting = true
	m.splitDiff.Focus()
	m.sizeSplitDiff()
	return nil
}

// splitDiffHint advertises the page keys only for a diff that spans more than
// one file, since there is no page to turn to otherwise.
func splitDiffHint(files int) string {
	if files > 1 {
		return "[ ] file · esc close"
	}
	return "esc close"
}

// closeSplitDiff hides the side-by-side overlay.
func (m *Model) closeSplitDiff() {
	m.splitting = false
	m.splitDiff.Blur()
}

// sizeSplitDiff sizes the overlay to just inside the screen: two panes side by
// side need every column they can get, so it is the one overlay that does not
// leave much of the layout showing around it.
func (m *Model) sizeSplitDiff() {
	w := clamp(m.width-4, 40, max(m.width-2, 40))
	h := clamp(m.height-4, 8, max(m.height-2, 8))
	m.splitDiff.SetSize(w, h)
}

// mainDiff returns the unified diff Main is currently showing for the Files
// panel, named by the target it covers. It reports ok=false whenever Main holds
// anything else — a changelist summary, a placeholder, or a load failure — so
// the side-by-side view is only offered when there is a patch to lay out.
func (m *Model) mainDiff() (diff, target string, ok bool) {
	if !m.filesShowDiff() {
		return "", "", false
	}
	if m.filesViewIsDiffs() {
		d, sel := m.savedDiffs.Selected()
		if !sel {
			return "", "", false
		}
		return m.savedText, d.Name, true
	}
	if m.filesViewIsRejects() {
		r, sel := m.selectedReject()
		if !sel {
			return "", "", false
		}
		return m.rejectText, r.Rel, true
	}
	if m.diffErr {
		return "", "", false
	}
	if n, _, sel := m.selectedTreeNode(); sel && n.Item == nil {
		if n.Path == fileTreeRoot {
			return m.diffText, "working copy", true
		}
		return m.diffText, n.Path, true
	}
	it, sel := m.selectedFile()
	if !sel {
		return "", "", false
	}
	return m.diffText, it.Path, true
}

// splitDiffPages re-lays a unified diff as aligned side-by-side rows: the text
// before the change on the left, the text after it on the right. It is the
// counterpart of colorizeDiff — the same unified-diff structure mapped onto a
// two-pane layout instead of a single colored column — and, like it, is the only
// place that knows what a diff line means, keeping the SplitView pairing-agnostic.
//
// Each file the diff covers becomes its own page, titled with its path, so a
// directory's combined diff is read one file at a time rather than as one long
// scroll. Within a page, each run of removals is paired against the additions
// that follow it, so a rewritten line sits opposite its replacement and the odd
// ones out get a blank cell on the other side. Context lines appear on both
// sides, hunk headings span the full width, and every line carries the number it
// has in its own version of the file. An empty diff yields no pages.
func splitDiffPages(th theme.Theme, diff string) []component.SplitPage {
	if strings.TrimSpace(diff) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	width := diffNumWidth(lines)

	var (
		head = lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
		meta = lipgloss.NewStyle().Foreground(th.Muted)
		add  = lipgloss.NewStyle().Foreground(th.Success)
		del  = lipgloss.NewStyle().Foreground(th.Error)
		text = lipgloss.NewStyle().Foreground(th.Text)
	)

	var (
		pages          []component.SplitPage
		page           component.SplitPage
		removed, added []string
		oldNo, newNo   int
		inHunk         bool
	)
	// flush pairs the removals and additions collected since the last context
	// line into rows, longest run first, and numbers each side as it goes.
	flush := func() {
		for i := 0; i < len(removed) || i < len(added); i++ {
			var row component.SplitRow
			if i < len(removed) {
				row.Left = diffCell(meta, del, width, oldNo, "-", removed[i])
				oldNo++
			}
			if i < len(added) {
				row.Right = diffCell(meta, add, width, newNo, "+", added[i])
				newNo++
			}
			page.Rows = append(page.Rows, row)
		}
		removed, added = nil, nil
	}
	// turn files the page built so far and opens the next one. A diff that opens
	// with content of its own — a patch with no "Index:" line at all — keeps it
	// on a leading untitled page rather than dropping it.
	turn := func(title string) {
		flush()
		if len(page.Rows) > 0 {
			pages = append(pages, page)
		}
		page = component.SplitPage{Title: title}
		inHunk = false
	}

	for _, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "@@"):
			flush()
			before, after, ok := parseHunkHeader(ln)
			oldNo, newNo, inHunk = before.start, after.start, ok
			page.Rows = append(page.Rows, component.SplitRow{Left: head.Render(ln), Span: true})
		case strings.HasPrefix(ln, "Index:"):
			turn(strings.TrimSpace(ln[len("Index:"):]))
		case !inHunk:
			// Between files: the "===" rule and the "---"/"+++" markers repeat what
			// the page title already says, so only anything else svn reports here —
			// property blocks, binary notices — is kept.
			if ln == "" || strings.HasPrefix(ln, "===") || strings.HasPrefix(ln, "---") || strings.HasPrefix(ln, "+++") {
				continue
			}
			page.Rows = append(page.Rows, component.SplitRow{Left: meta.Render(ln), Span: true})
		case strings.HasPrefix(ln, "-"):
			removed = append(removed, ln[1:])
		case strings.HasPrefix(ln, "+"):
			added = append(added, ln[1:])
		case strings.HasPrefix(ln, `\`):
			// "\ No newline at end of file" annotates the line above, on both sides.
			page.Rows = append(page.Rows, component.SplitRow{Left: meta.Render(ln), Span: true})
		default:
			flush()
			body := strings.TrimPrefix(ln, " ")
			page.Rows = append(page.Rows, component.SplitRow{
				Left:  diffCell(meta, text, width, oldNo, " ", body),
				Right: diffCell(meta, text, width, newNo, " ", body),
			})
			oldNo++
			newNo++
		}
	}
	turn("")
	return pages
}

// diffCell renders one pane's cell: the line's number in its own version of the
// file, right-aligned to width and muted so it reads as a margin, then the
// change marker and the line, colored by what the marker means. The marker is
// kept against the text, as the diff had it, so indentation still lines up.
func diffCell(num, body lipgloss.Style, width, no int, marker, line string) string {
	n := ""
	if no > 0 {
		n = strconv.Itoa(no)
	}
	return num.Render(fmt.Sprintf("%*s", width, n)) + " " + body.Render(marker+line)
}

// diffNumWidth is the width the line-number margin needs: enough digits for the
// highest line any hunk reaches, on either side, never narrower than
// minDiffNumWidth.
func diffNumWidth(lines []string) int {
	highest := 0
	for _, ln := range lines {
		before, after, ok := parseHunkHeader(ln)
		if !ok {
			continue
		}
		highest = max(highest, before.last(), after.last())
	}
	return max(len(strconv.Itoa(highest)), minDiffNumWidth)
}

// last is the final line number the range covers.
func (h hunkRange) last() int { return h.start + max(h.count-1, 0) }

// parseHunkHeader reads the line spans out of a unified-diff hunk header,
// "@@ -oldStart,oldCount +newStart,newCount @@ [section]", where a missing count
// means a single line. It reports ok=false for anything that is not one.
func parseHunkHeader(ln string) (before, after hunkRange, ok bool) {
	if !strings.HasPrefix(ln, "@@") {
		return before, after, false
	}
	rest := ln[len("@@"):]
	end := strings.Index(rest, "@@")
	if end < 0 {
		return before, after, false
	}
	spans := strings.Fields(rest[:end])
	if len(spans) < 2 {
		return before, after, false
	}
	before, okBefore := parseHunkRange(spans[0], "-")
	after, okAfter := parseHunkRange(spans[1], "+")
	return before, after, okBefore && okAfter
}

// parseHunkRange reads one signed "start,count" span from a hunk header.
func parseHunkRange(span, sign string) (hunkRange, bool) {
	if !strings.HasPrefix(span, sign) {
		return hunkRange{}, false
	}
	span = span[len(sign):]
	start, count := span, "1"
	if i := strings.IndexByte(span, ','); i >= 0 {
		start, count = span[:i], span[i+1:]
	}
	s, err := strconv.Atoi(start)
	if err != nil {
		return hunkRange{}, false
	}
	c, err := strconv.Atoi(count)
	if err != nil {
		return hunkRange{}, false
	}
	return hunkRange{start: s, count: c}, true
}

// splitDiffLabels names the two panes from the diff's file markers: "---"
// describes the version on the left and "+++" the one on the right, each
// carrying its version in parentheses ("(revision 42)", "(working copy)"). The
// first pair found names both panes, since a multi-file diff compares the same
// two versions throughout. Generic names stand in when a diff carries no markers.
func splitDiffLabels(diff string) (left, right string) {
	left, right = "original", "modified"
	gotLeft, gotRight := false, false
	for _, ln := range strings.Split(diff, "\n") {
		switch {
		case !gotLeft && strings.HasPrefix(ln, "---"):
			if v := diffMarkerVersion(ln); v != "" {
				left, gotLeft = v, true
			}
		case !gotRight && strings.HasPrefix(ln, "+++"):
			if v := diffMarkerVersion(ln); v != "" {
				right, gotRight = v, true
			}
		}
		if gotLeft && gotRight {
			break
		}
	}
	return left, right
}

// diffMarkerVersion pulls the parenthesized version off a "---"/"+++" file
// marker, e.g. "--- src/main.go\t(revision 42)" -> "revision 42".
func diffMarkerVersion(ln string) string {
	open := strings.LastIndex(ln, "(")
	shut := strings.LastIndex(ln, ")")
	if open < 0 || shut < open {
		return ""
	}
	return strings.TrimSpace(ln[open+1 : shut])
}
