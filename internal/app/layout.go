package app

// layout sizes the panels and bar for the current terminal dimensions.
func (m *Model) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	const barHeight = 1
	bodyHeight := max(m.height-barHeight, 3)

	leftWidth := clamp(m.width*2/5, 24, m.width-20)
	rightWidth := m.width - leftWidth

	// Tall enough for the Status panel's six rows plus its border.
	statusHeight := clamp(8, 3, max(bodyHeight-6, 3))
	rest := bodyHeight - statusHeight
	filesHeight := rest / 2
	logHeight := rest - filesHeight

	m.panels[panelStatus].SetSize(leftWidth, statusHeight)
	m.panels[panelFiles].SetSize(leftWidth, filesHeight)
	m.panels[panelLog].SetSize(leftWidth, logHeight)

	// The right column is Main alone, or Main above the command log when it is
	// shown; the split keeps the two columns the same overall height.
	mainHeight := bodyHeight
	if m.showCmdLog {
		cmdLogHeight := clamp(bodyHeight/4, 6, 12)
		if cmdLogHeight > bodyHeight-3 {
			cmdLogHeight = max(bodyHeight-3, 0)
		}
		mainHeight = bodyHeight - cmdLogHeight
		m.panels[panelCmdLog].SetSize(rightWidth, cmdLogHeight)
	}
	m.panels[panelMain].SetSize(rightWidth, mainHeight)
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
