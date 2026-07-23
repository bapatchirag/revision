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

// ViewSelectedMsg is emitted when a multi-view container switches its active
// named view (e.g. the user cycles the tabs with [ or ]).
type ViewSelectedMsg struct {
	ID    string // identifies the emitting container
	Index int    // position of the now-active view
	Name  string // name of the now-active view
}

// SubViewPoppedMsg is emitted when a multi-view container pops back out of an
// unnamed sub-view (a drill-down cascade), e.g. on esc.
type SubViewPoppedMsg struct {
	ID    string // identifies the emitting container
	Depth int    // remaining drill depth after the pop (0 at the base view)
}
