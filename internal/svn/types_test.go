package svn

import "testing"

func TestFileStateCode(t *testing.T) {
	cases := map[FileState]string{
		StateModified:    "M",
		StateAdded:       "A",
		StateDeleted:     "D",
		StateReplaced:    "R",
		StateUnversioned: "?",
		StateConflicted:  "C",
		StateMissing:     "!",
		StateIgnored:     "I",
		StateExternal:    "X",
		StateNormal:      " ",
		StateNone:        " ",
	}
	for st, want := range cases {
		if got := st.Code(); got != want {
			t.Errorf("Code(%s) = %q, want %q", st, got, want)
		}
	}
}

func TestMapState(t *testing.T) {
	cases := map[string]FileState{
		"":            StateNone,
		"none":        StateNone,
		"normal":      StateNormal,
		"modified":    StateModified,
		"added":       StateAdded,
		"deleted":     StateDeleted,
		"unversioned": StateUnversioned,
		"conflicted":  StateConflicted,
		"weird-value": FileState("weird-value"),
	}
	for in, want := range cases {
		if got := mapState(in); got != want {
			t.Errorf("mapState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsDirty(t *testing.T) {
	dirty := []FileState{StateModified, StateAdded, StateDeleted, StateReplaced, StateConflicted}
	for _, s := range dirty {
		if !s.IsDirty() {
			t.Errorf("IsDirty(%s) = false, want true", s)
		}
	}
	clean := []FileState{StateNormal, StateNone, StateUnversioned, StateIgnored}
	for _, s := range clean {
		if s.IsDirty() {
			t.Errorf("IsDirty(%s) = true, want false", s)
		}
	}
}
