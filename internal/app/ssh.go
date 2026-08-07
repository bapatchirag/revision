package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/tui/component"
)

// submitUnlock adds the configured SSH key to the agent with the entered
// passphrase. It locks the input and shows a processing notice so the wait for
// ssh-add is visible and can't be interrupted; the result arrives on
// sshAddedMsg, which starts the deferred initial load on success.
func (m *Model) submitUnlock(passphrase string) tea.Cmd {
	m.adding = true
	m.passEditor.Blur()
	m.showToast("Adding SSH key…", component.LevelInfo)
	return sshAddCmd(m.cfg.SSHKeyPath, passphrase)
}

// openUnlock shows the SSH passphrase overlay so the configured key can be
// unlocked and added to the agent before the initial load runs.
func (m *Model) openUnlock() {
	m.unlocking = true
	m.adding = false
	m.passEditor.Reset()
	m.passEditor.Focus()
	m.sizeUnlock()
}

// closeUnlock hides the passphrase overlay.
func (m *Model) closeUnlock() {
	m.unlocking = false
	m.adding = false
	m.passEditor.Blur()
}

// abort tears down the passphrase overlay and shows reason plus a quit hint in a
// centered error toast; the next keypress quits. It is used for every
// unrecoverable SSH outcome — an unreachable agent, a cancelled prompt, or too
// many wrong passphrases — since the key is required to proceed.
func (m *Model) abort(reason string) tea.Cmd {
	m.aborting = true
	m.closeUnlock()
	wrapW := clamp(m.width-8, 24, 60)
	wrapped := lipgloss.NewStyle().Width(wrapW).Render(reason)
	m.toast.Show(wrapped+"\n\nPress any key to quit and try again", component.LevelError)
	return nil
}

// sizeUnlock sizes the passphrase prompt (only its width matters).
func (m *Model) sizeUnlock() {
	w := clamp(m.width/2, 30, max(m.width-6, 30))
	m.passEditor.SetSize(w, 0)
}
