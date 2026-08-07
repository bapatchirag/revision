package app

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Update offers each message to the handlers in turn — system, load, mutation,
// UI — then to whatever owns the keyboard, and finally to the focused panel.
// Exactly one handler claims each message type; TestEveryMessageIsClaimedOnce
// holds that line.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Refresh the command log from any invocations that finished since the last
	// message; every svn command completes by delivering a message here.
	m.syncCommandLog()
	if cmd, ok := m.systemEvent(msg); ok {
		return m, cmd
	}
	if cmd, ok := m.loadEvent(msg); ok {
		return m, cmd
	}
	if cmd, ok := m.mutationEvent(msg); ok {
		return m, cmd
	}
	if cmd, ok := m.uiEvent(msg); ok {
		return m, cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		if cmd, handled := m.routeKey(k); handled {
			return m, cmd
		}
	}
	return m, m.panels[m.focus.Index()].Update(msg)
}
