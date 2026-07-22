// Package svn provides a thin, typed wrapper around the Subversion command-line
// client. It shells out to the system `svn` binary and parses its --xml output.
package svn

import "time"

// FileState is the working-copy status of a single path, mirroring the values
// reported by `svn status` in its `wc-status` item attribute.
type FileState string

const (
	StateNone        FileState = "none"
	StateNormal      FileState = "normal"
	StateAdded       FileState = "added"
	StateModified    FileState = "modified"
	StateDeleted     FileState = "deleted"
	StateReplaced    FileState = "replaced"
	StateUnversioned FileState = "unversioned"
	StateMissing     FileState = "missing"
	StateConflicted  FileState = "conflicted"
	StateIgnored     FileState = "ignored"
	StateExternal    FileState = "external"
	StateObstructed  FileState = "obstructed"
	StateIncomplete  FileState = "incomplete"
	StateMerged      FileState = "merged"
	StateUnknown     FileState = "unknown"
)

// Code returns the single-character status letter conventionally used by svn.
func (s FileState) Code() string {
	switch s {
	case StateModified:
		return "M"
	case StateAdded:
		return "A"
	case StateDeleted:
		return "D"
	case StateReplaced:
		return "R"
	case StateUnversioned:
		return "?"
	case StateMissing:
		return "!"
	case StateConflicted:
		return "C"
	case StateIgnored:
		return "I"
	case StateExternal:
		return "X"
	case StateObstructed:
		return "~"
	case StateIncomplete:
		return "!"
	case StateMerged:
		return "G"
	case StateNormal, StateNone, "":
		return " "
	default:
		return "?"
	}
}

// IsDirty reports whether the state represents a pending change that would be
// part of a commit (as opposed to normal/unversioned/ignored items).
func (s FileState) IsDirty() bool {
	switch s {
	case StateModified, StateAdded, StateDeleted, StateReplaced, StateConflicted, StateMissing, StateMerged:
		return true
	default:
		return false
	}
}

// StatusItem is a single entry from `svn status`.
type StatusItem struct {
	Path       string    // path relative to the working-copy target
	State      FileState // wc-status item
	PropState  FileState // wc-status props
	Revision   string    // working-copy revision, if reported
	Changelist string    // changelist name, if the item belongs to one
}

// Info is the subset of `svn info` we care about for a working copy.
type Info struct {
	Path            string
	WorkingCopyRoot string
	URL             string
	RepositoryRoot  string
	Revision        string
}

// ChangedPath is a single path affected by a revision, as reported by
// `svn log --verbose`.
type ChangedPath struct {
	Action string // "A", "M", "D", "R" as reported by svn
	Path   string // repository-relative path
}

// LogEntry is a single revision from `svn log`.
type LogEntry struct {
	Revision string
	Author   string
	Date     time.Time
	Message  string
	Paths    []ChangedPath
}

// mapState normalizes an svn status string into a FileState.
func mapState(s string) FileState {
	switch s {
	case "", "none":
		return StateNone
	case "normal":
		return StateNormal
	case "added":
		return StateAdded
	case "modified":
		return StateModified
	case "deleted":
		return StateDeleted
	case "replaced":
		return StateReplaced
	case "unversioned":
		return StateUnversioned
	case "missing":
		return StateMissing
	case "conflicted":
		return StateConflicted
	case "ignored":
		return StateIgnored
	case "external":
		return StateExternal
	case "obstructed":
		return StateObstructed
	case "incomplete":
		return StateIncomplete
	case "merged":
		return StateMerged
	default:
		return FileState(s)
	}
}
