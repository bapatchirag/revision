// Package msg holds the decoupled messages that components emit through
// returned commands. Components never call app or domain logic directly; they
// announce intent here and the composition layer decides what to do.
package msg

// SelectedMsg is emitted when a list-like component's selection changes.
type SelectedMsg struct {
	ID    string // identifies the emitting component
	Index int
}

// ActivatedMsg is emitted when the user activates the current selection
// (e.g. presses enter on a list row or menu item).
type ActivatedMsg struct {
	ID    string
	Index int
}

// SubmitMsg is emitted when an editor component submits its value.
type SubmitMsg struct {
	ID    string
	Value string
}

// ConfirmMsg is emitted when a modal confirmation is accepted.
type ConfirmMsg struct {
	ID string
}

// DismissMsg is emitted when a popup is dismissed or cancelled.
type DismissMsg struct {
	ID string
}
