// Package focus cycles input focus across a ring of Focusable components,
// guaranteeing that exactly one holds focus at a time.
package focus

import "github.com/bapatchirag/revision/internal/tui"

// Manager owns an ordered ring of focusable components.
type Manager struct {
	items []tui.Focusable
	index int
}

// New builds a manager over the given components, focusing the first one.
func New(items ...tui.Focusable) *Manager {
	m := &Manager{items: items}
	m.apply()
	return m
}

// Len reports how many components are in the ring.
func (m *Manager) Len() int { return len(m.items) }

// Index returns the focused component's position in the ring.
func (m *Manager) Index() int { return m.index }

// Current returns the focused component, or nil if the ring is empty.
func (m *Manager) Current() tui.Focusable {
	if len(m.items) == 0 {
		return nil
	}
	return m.items[m.index]
}

// Focus moves focus to component i, wrapping into range.
func (m *Manager) Focus(i int) {
	if len(m.items) == 0 {
		return
	}
	n := len(m.items)
	m.index = ((i % n) + n) % n
	m.apply()
}

// Next advances focus to the following component.
func (m *Manager) Next() { m.Focus(m.index + 1) }

// Prev moves focus to the preceding component.
func (m *Manager) Prev() { m.Focus(m.index - 1) }

func (m *Manager) apply() {
	for i, it := range m.items {
		if i == m.index {
			it.Focus()
		} else {
			it.Blur()
		}
	}
}
