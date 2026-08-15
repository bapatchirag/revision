package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/keymap"
)

// openConfirm arms the shared modal with a prompt and shows it; the pending
// command runs when the user confirms.
func (m *Model) openConfirm(title, message string) {
	m.confirming = true
	m.modal.SetPrompt(title, message)
	m.modal.Focus()
	m.sizeModal()
}

// closeConfirm hides the confirmation modal.
func (m *Model) closeConfirm() {
	m.confirming = false
	m.modal.Blur()
}

// openHelp shows the keybindings help menu as a centered overlay.
func (m *Model) openHelp() tea.Cmd {
	m.helping = true
	m.menu.Focus()
	m.sizeMenu()
	return nil
}

// closeHelp hides the help menu.
func (m *Model) closeHelp() {
	m.helping = false
	m.menu.Blur()
}

// overlayActive reports whether any modal, editor, or menu is currently on
// screen, so a background event (like the update check completing) knows not to
// steal focus.
func (m *Model) overlayActive() bool {
	return m.aborting || m.unlocking || m.editing || m.naming || m.savingDiff || m.retargeting || m.switchingRepo || m.splitting || m.merging || m.confirming || m.helping || m.updating || m.configuring
}

// updateHeld reports whether now is the wrong moment to interrupt with the
// update prompt: something else owns the screen or the keyboard, or the working
// copy has not finished arriving. Wider than overlayActive, because the filter
// bar and the update progress modal both take keys without being overlays.
func (m *Model) updateHeld() bool {
	return m.overlayActive() || m.updatingWC || m.filtering || m.loading
}

// offerUpdate shows the prompt for a release, or holds it until the screen is
// free.
func (m *Model) offerUpdate(rel selfupdate.Release) {
	if m.updateHeld() {
		m.deferredUpdate = &rel
		return
	}
	m.deferredUpdate = nil
	m.openUpdate(rel)
}

// deferUpdate takes the prompt off the screen and holds its release for the next
// quiet moment, for when something with a stronger claim to the screen arrives.
func (m *Model) deferUpdate() {
	if !m.updating {
		return
	}
	rel := m.updateRel
	m.closeUpdate()
	m.deferredUpdate = &rel
}

// retakeUpdate offers a held release again once nothing is in its way.
func (m *Model) retakeUpdate() {
	if m.deferredUpdate == nil || m.updateHeld() {
		return
	}
	rel := *m.deferredUpdate
	m.deferredUpdate = nil
	m.openUpdate(rel)
}

// openUpdate shows the startup update prompt for the given release as a centered
// overlay, titling it with the new version.
func (m *Model) openUpdate(rel selfupdate.Release) {
	m.updateRel = rel
	m.updating = true
	m.updateMenu.SetTitle("Update available: " + rel.Tag)
	m.updateMenu.Focus()
	m.sizeUpdateMenu()
}

// closeUpdate hides the update prompt without applying an update.
func (m *Model) closeUpdate() {
	m.updating = false
	m.updateMenu.Blur()
}

// chooseUpdate handles a selection in the update prompt. The first two items
// record the chosen method and quit so the update runs after the TUI tears down
// (a self-replacing binary cannot be updated cleanly while it is on screen); the
// last item just dismisses the prompt.
func (m *Model) chooseUpdate(index int) tea.Cmd {
	switch index {
	case 0:
		m.updateMethod = selfupdate.MethodCurl
		m.updateChosen = true
		return tea.Quit
	case 1:
		m.updateMethod = selfupdate.MethodGo
		m.updateChosen = true
		return tea.Quit
	default:
		m.closeUpdate()
		return nil
	}
}

// showToast displays a transient notice; it stays until the next interaction.
func (m *Model) showToast(text string, level component.Level) {
	m.toast.Show(text, level)
	m.showingToast = true
}

// dismissToast hides the current toast.
func (m *Model) dismissToast() { m.showingToast = false }

// failureText renders an action failure for a toast. An svn authentication
// failure collapses to a short, actionable hint instead of a raw multi-line svn
// error dump.
func failureText(action string, err error) string {
	if svn.IsAuthError(err) {
		return action + " failed: " + svn.AuthHint
	}
	return action + " failed: " + err.Error()
}

// helpMenuItems is the keybindings reference shown by the "?" help menu. The
// table itself lives in keymap.HelpSections, which also feeds the website's
// keybindings page; this only lays it out as menu rows.
func helpMenuItems() []component.MenuItem {
	sections := keymap.HelpSections()
	items := make([]component.MenuItem, 0, len(sections))
	for _, s := range sections {
		items = append(items, component.MenuSection(s.Title))
		for _, b := range s.Bindings {
			items = append(items, component.MenuItem{Label: b.Action, Key: b.KeyHint()})
		}
	}
	return items
}

// updateMenuItems are the choices shown in the startup update prompt. Their
// order is load-bearing: chooseUpdate maps index 0/1/2 to curl / go / dismiss.
func updateMenuItems() []component.MenuItem {
	return []component.MenuItem{
		{Label: "Update with cURL"},
		{Label: "Update with Go"},
		{Label: "Don't update this time"},
	}
}

// sizeEditor sizes the commit editor to a centered portion of the screen.
func (m *Model) sizeEditor() {
	w := clamp(m.width*3/5, 40, max(m.width-4, 40))
	h := clamp(m.height/2, 8, max(m.height-4, 8))
	m.editor.SetSize(w, h)
}

// sizeNameEditor sizes the changelist-name prompt (only its width matters; the
// height follows the input and option rows).
func (m *Model) sizeNameEditor() {
	w := clamp(m.width/2, 30, max(m.width-6, 30))
	m.nameEditor.SetSize(w, 0)
}

// sizeDiffEditor sizes the save-diff file-name prompt like the changelist-name
// prompt.
func (m *Model) sizeDiffEditor() {
	w := clamp(m.width/2, 30, max(m.width-6, 30))
	m.diffEditor.SetSize(w, 0)
}

// sizeModal sizes the confirmation modal to a centered portion of the screen
// (only its width matters; the height follows the wrapped message).
func (m *Model) sizeModal() {
	w := clamp(m.width/2, 34, max(m.width-6, 34))
	m.modal.SetSize(w, 0)
}

// showUpdating raises the progress modal for the staged update message and marks
// an update in flight; the updatedMsg handler clears it when svn returns.
func (m *Model) showUpdating() {
	m.updatingWC = true
	m.progress.SetPrompt("", m.updateProgress)
	m.progress.Focus()
	m.sizeProgress()
}

// sizeProgress widths the update-progress modal to fit its one-line message,
// capped to the screen so a narrow terminal wraps instead of overflowing.
func (m *Model) sizeProgress() {
	w := clamp(lipgloss.Width(m.updateProgress)+4, 34, max(m.width-6, 34))
	m.progress.SetSize(w, 0)
}

// sizeMenu sizes the help menu to a centered portion of the screen (only its
// width matters; the height follows the item count). It is laid out in two
// columns so the grouped keybindings fit a short terminal.
func (m *Model) sizeMenu() {
	m.menu.SetColumns(2)
	m.menu.SetSize(clamp(m.width*4/5, 60, max(m.width-6, 60)), 0)
}

// sizeUpdateMenu sizes the startup update prompt like the help menu (width
// only; the height follows the three choices).
func (m *Model) sizeUpdateMenu() {
	m.updateMenu.SetSize(clamp(m.width/2, 40, max(m.width-6, 40)), 0)
}

// sizeForm sizes the settings editor to a centered portion of the screen (only
// its width matters; the height follows the field count).
func (m *Model) sizeForm() {
	m.form.SetSize(clamp(m.width*3/5, 40, max(m.width-4, 40)), 0)
}

// sizeHideRules sizes the rules editor to the settings editor's width, with a
// height that scrolls a long rule set rather than growing past the screen.
func (m *Model) sizeHideRules() {
	w := clamp(m.width*3/5, 40, max(m.width-4, 40))
	h := clamp(m.height/2, 8, max(m.height-4, 8))
	m.rulesEditor.SetSize(w, h)
}
