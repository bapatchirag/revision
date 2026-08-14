package app

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Update offers each message to the handlers in turn — system, load, mutation,
// UI — then to whatever owns the keyboard or the pointer, and finally to the
// focused panel. Exactly one handler claims each message type;
// TestEveryMessageIsClaimedOnce holds that line.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.dispatch(msg)
	// Whatever the message did may have freed the screen, which is the moment a
	// held-back update prompt gets its turn. Checking here catches every way an
	// overlay can close without each of them having to remember.
	m.retakeUpdate()
	return m, cmd
}

func (m *Model) dispatch(msg tea.Msg) tea.Cmd {
	// Refresh the command log from any invocations that finished since the last
	// message; every svn command completes by delivering a message here.
	m.syncCommandLog()
	if cmd, ok := m.systemEvent(msg); ok {
		return cmd
	}
	if cmd, ok := m.loadEvent(msg); ok {
		return cmd
	}
	if cmd, ok := m.mutationEvent(msg); ok {
		return cmd
	}
	if cmd, ok := m.uiEvent(msg); ok {
		return cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		if cmd, handled := m.routeKey(k); handled {
			return cmd
		}
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		// The mouse stops here: it only ever moves focus.
		return m.routeMouse(mouse)
	}
	return m.panels[m.focus.Index()].Update(msg)
}
