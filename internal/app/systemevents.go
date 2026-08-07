package app

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/sshagent"
	"github.com/bapatchirag/revision/internal/tui/component"
)

// systemEvent handles the messages about the process rather than the working
// copy: the terminal resizing, a load failing outright, the background watcher
// and update check, and the SSH handshake the first load waits on. It reports
// whether it owned the message.
func (m *Model) systemEvent(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		if m.editing {
			m.sizeEditor()
		}
		if m.naming {
			m.sizeNameEditor()
		}
		if m.savingDiff {
			m.sizeDiffEditor()
		}
		if m.retargeting {
			m.sizeSourcePath()
		}
		if m.splitting {
			m.sizeSplitDiff()
		}
		if m.merging {
			m.sizeMerge()
		}
		if m.confirming {
			m.sizeModal()
		}
		if m.helping {
			m.sizeMenu()
		}
		if m.updating {
			m.sizeUpdateMenu()
		}
		if m.configuring {
			m.sizeForm()
		}
		if m.unlocking {
			m.sizeUnlock()
		}
		if m.updatingWC {
			m.sizeProgress()
		}
		return nil, true

	case errMsg:
		m.loading = false
		m.err = msg.err
		m.refreshChrome()
		return nil, true

	case workingCopyChangedMsg:
		return m.observeWorkingCopy(msg), true

	case updateAvailableMsg:
		// Offer the update only when nothing else is on screen, so the prompt
		// never steals focus from an in-flight commit, confirmation, or menu.
		if !m.overlayActive() {
			m.openUpdate(msg.rel)
		}
		return nil, true

	case startupNoticeMsg:
		// A one-time notice surfaced at launch (e.g. config values reset during
		// reconciliation). It behaves like any toast: it clears on the next key.
		m.showToast(msg.text, component.LevelWarning)
		return nil, true

	case sshCheckedMsg:
		switch {
		case msg.err != nil:
			// The agent is unreachable or ssh-add is missing: there is nothing to
			// unlock and the key is required, so surface the error and quit.
			return m.abort("ssh-agent unavailable: " + msg.err.Error()), true
		case msg.loaded:
			return m.beginInitialLoad(), true
		default:
			m.openUnlock()
			return nil, true
		}

	case sshAddedMsg:
		if !m.unlocking {
			return nil, true
		}
		m.adding = false
		if msg.err != nil {
			if errors.Is(msg.err, sshagent.ErrAgentUnreachable) {
				return m.abort("ssh-agent unavailable: " + msg.err.Error()), true
			}
			m.passAttempts++
			if m.passAttempts >= maxPassphraseAttempts {
				return m.abort(fmt.Sprintf("SSH key not added after %d attempts; it is required for this working copy", m.passAttempts)), true
			}
			m.showToast(fmt.Sprintf("wrong passphrase (%d/%d) — try again", m.passAttempts, maxPassphraseAttempts), component.LevelError)
			m.passEditor.Reset()
			m.passEditor.Focus()
			return nil, true
		}
		m.showToast("SSH key added", component.LevelSuccess)
		m.closeUnlock()
		return m.beginInitialLoad(), true

	case sourceChangedMsg:
		return m.applySourceChange(msg), true
	}
	return nil, false
}
