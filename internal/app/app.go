// Package app is the composition layer: it is the only package that knows both
// the SVN domain (internal/svn) and the reusable component library
// (internal/tui/component). It adapts SVN data into components and arranges them
// into the lazygit-style layout.
package app

import (
	"time"
)

// panel ring indices.
const (
	panelStatus = 0
	panelFiles  = 1
	panelLog    = 2
	// panelShelf lists the change sets taken out of the working copy. It sits
	// under the Log panel and, being a short list most of the time, shows only
	// its newest row until it is focused.
	panelShelf = 3
	panelMain  = 4
	// panelCmdLog is the command-log panel below Main. It is last in the focus
	// ring and is skipped by Tab while hidden.
	panelCmdLog = 5

	panelCount = 6
)

// stagedChangelist is the SVN changelist name revision uses to emulate a
// staging area: paths in it are "staged" and committed as a unit.
const stagedChangelist = "revision:staged"

// commitEditorID identifies the commit-message editor on emitted messages.
const commitEditorID = "commit"

// changelistEditorID identifies the changelist-name prompt on emitted messages.
const changelistEditorID = "changelist"

// diffNameEditorID identifies the save-diff file-name prompt on emitted messages.
const diffNameEditorID = "diff-name"

// shelfNameEditorID identifies the prompt that names a shelved change set on
// emitted messages.
const shelfNameEditorID = "shelf-name"

// splitDiffID identifies the side-by-side diff overlay on emitted messages.
const splitDiffID = "split-diff"

// mergeViewID identifies the conflict/reject resolution overlay on emitted
// messages.
const mergeViewID = "merge"

// passphraseEditorID identifies the SSH passphrase prompt on emitted messages.
const passphraseEditorID = "ssh-passphrase"

// maxPassphraseAttempts is how many wrong passphrases the SSH unlock overlay
// tolerates before giving up and exiting, since the key is required to proceed.
const maxPassphraseAttempts = 3

// filesViewsID identifies the Files panel's multi-view container on emitted
// messages (the Changes / Changelists tabs and their drill-downs).
const filesViewsID = "files-views"

// logViewsID identifies the Log panel's container on emitted messages. It holds
// a single view, so it exists only for the drill into a revision's files.
const logViewsID = "log-views"

// revFilesListID identifies the drilled-in revision file tree on emitted
// selection/activation messages.
const revFilesListID = "rev-files"

// changelistsListID / changelistFilesID identify the Changelists list and its
// drilled-in file list on emitted selection/activation messages.
const (
	changelistsListID = "changelists"
	changelistFilesID = "changelist-files"
)

// confirmModalID identifies the shared confirmation modal on emitted messages.
const confirmModalID = "confirm"

// helpMenuID identifies the keybindings help menu on emitted messages.
const helpMenuID = "help"

// updateMenuID identifies the startup update prompt on emitted messages.
const updateMenuID = "update"

// settingsFormID identifies the settings editor on emitted messages.
const settingsFormID = "settings"

// hideRulesEditorID identifies the hide-rules editor, opened from the settings
// editor, on emitted messages.
const hideRulesEditorID = "hide-rules"

// searchBarID identifies the panel filter input on emitted messages.
const searchBarID = "search"

// diffDebounce is how long the cursor must rest on a selection before its diff
// is asked of svn, so holding j/k through a tree runs one `svn diff` instead of
// one per row. A diff already cached is shown immediately, debounce or not.
const diffDebounce = 90 * time.Millisecond

// mainSource selects which side panel's selection drives the Main viewport.
type mainSource int

const (
	sourceFiles mainSource = iota
	sourceLog
	sourceStatus
	sourceShelf
)
