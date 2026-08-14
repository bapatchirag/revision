package app

// barHeight is the single row the status bar — or the filter bar in its place —
// takes below the panels.
const barHeight = 1

// rect is where a panel sits on screen, in terminal cells: (x, y) is its
// top-left corner, border included.
type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool {
	return r.w > 0 && r.h > 0 && x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

// panelRects places every panel for the current terminal dimensions. baseView
// joins them without padding and the alternate screen starts at the origin, so
// these are screen coordinates too, which is what panelAt hit-tests a click
// against. The command log's rectangle stays empty while it is hidden.
func (m *Model) panelRects() [panelCount]rect {
	var r [panelCount]rect
	if m.width <= 0 || m.height <= 0 {
		return r
	}
	bodyHeight := max(m.height-barHeight, 3)

	leftWidth := clamp(m.width*2/5, 24, m.width-20)
	rightWidth := m.width - leftWidth

	// Tall enough for the Status panel's six rows plus its border.
	statusHeight := clamp(8, 3, max(bodyHeight-6, 3))
	rest := bodyHeight - statusHeight
	filesHeight := rest / 2
	logHeight := rest - filesHeight

	r[panelStatus] = rect{0, 0, leftWidth, statusHeight}
	r[panelFiles] = rect{0, statusHeight, leftWidth, filesHeight}
	r[panelLog] = rect{0, statusHeight + filesHeight, leftWidth, logHeight}

	// The right column is Main alone, or Main above the command log when it is
	// shown; the split keeps the two columns the same overall height.
	mainHeight := bodyHeight
	if m.showCmdLog {
		cmdLogHeight := clamp(bodyHeight/4, 6, 12)
		if cmdLogHeight > bodyHeight-3 {
			cmdLogHeight = max(bodyHeight-3, 0)
		}
		mainHeight = bodyHeight - cmdLogHeight
		r[panelCmdLog] = rect{leftWidth, mainHeight, rightWidth, cmdLogHeight}
	}
	r[panelMain] = rect{leftWidth, 0, rightWidth, mainHeight}
	return r
}

// layout sizes the panels and bar for the current terminal dimensions.
func (m *Model) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	for i, r := range m.panelRects() {
		if r.w > 0 && r.h > 0 {
			m.panels[i].SetSize(r.w, r.h)
		}
	}
	m.bar.SetSize(m.width, barHeight)
	m.searchBar.SetSize(m.width, barHeight)
	m.sizeToast()
	m.updateMain()
}

// sizeToast caps the notice box to the screen, leaving the one-column gutter
// overlayToast places it in. A long svn error wraps inside the box instead of
// drawing past the terminal edge.
func (m *Model) sizeToast() {
	m.toast.SetSize(max(m.width-2, 1), max(m.height-2, 3))
}

// refreshChrome recomputes the derived content in the Status panel, Main panel
// and status bar.
func (m *Model) refreshChrome() {
	m.updateStatus()
	m.updateMain()
	m.updateBar()
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
