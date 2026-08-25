package app

import (
	"strings"
	"unicode/utf8"

	"github.com/bapatchirag/revision/internal/svn"
)

// filterQuery is a parsed panel filter: zero or more key:value parameters plus
// the leftover free-text terms. It is produced by parseFilter and consumed by
// the panel-specific matchers (matchLogEntry, matchStatusItem).
type filterQuery struct {
	params map[string]string
	text   string
}

// logFilterKeys are the parameters the Log panel understands. Everything else in
// a log filter is free text, matched against the full commit message.
var logFilterKeys = map[string]bool{
	"rev":    true,
	"user":   true,
	"author": true,
	"path":   true,
	"date":   true,
}

// fileFilterKeys are the parameters the Files panel (Changes tree, Changelists
// list and drill) understands. Everything else is free text, matched against the
// file path.
var fileFilterKeys = map[string]bool{
	"state":      true,
	"cl":         true,
	"changelist": true,
}

// revFileFilterKeys are the parameters the drilled-in revision tree understands.
// A file in a range of history has a state the diff reported but no changelist,
// which belongs to the working copy alone.
var revFileFilterKeys = map[string]bool{
	"state": true,
}

// empty reports whether the query would match everything (no params, no text).
func (q filterQuery) empty() bool {
	return len(q.params) == 0 && q.text == ""
}

// parseFilter splits a raw filter string into recognized key:value parameters
// and free text. A token of the form key:value is treated as a parameter only
// when key is in allowed (compared case-insensitively) and value is non-empty;
// every other token — including an unrecognized key:value — becomes part of the
// free-text query, so a stray colon never silently drops a term. The free text
// preserves the user's casing; matchers fold case as needed.
func parseFilter(raw string, allowed map[string]bool) filterQuery {
	q := filterQuery{params: map[string]string{}}
	var text []string
	for _, tok := range strings.Fields(raw) {
		if k, v, ok := strings.Cut(tok, ":"); ok && v != "" && allowed[strings.ToLower(k)] {
			q.params[strings.ToLower(k)] = v
			continue
		}
		text = append(text, tok)
	}
	q.text = strings.Join(text, " ")
	return q
}

// matchLogEntry reports whether a revision satisfies every parameter of q and
// contains its free text somewhere in the full (multi-line) commit message. An
// empty query matches every entry.
func matchLogEntry(e svn.LogEntry, q filterQuery) bool {
	if q.empty() {
		return true
	}
	for k, v := range q.params {
		switch k {
		case "rev":
			if !revEquals(e.Revision, v) {
				return false
			}
		case "user", "author":
			if !containsFold(e.Author, v) {
				return false
			}
		case "path":
			if !anyPathContains(e.Paths, v) {
				return false
			}
		case "date":
			if e.Date.IsZero() || !strings.Contains(e.Date.Format("2006-01-02 15:04"), v) {
				return false
			}
		}
	}
	return containsFold(e.Message, q.text)
}

// matchStatusItem reports whether a working-copy file satisfies every parameter
// of q and contains its free text in the file path. An empty query matches every
// item.
func matchStatusItem(it svn.StatusItem, q filterQuery) bool {
	if q.empty() {
		return true
	}
	for k, v := range q.params {
		switch k {
		case "state":
			if !itemStateMatches(it, v) {
				return false
			}
		case "cl", "changelist":
			if !containsFold(displayCL(it.Changelist), v) {
				return false
			}
		}
	}
	return containsFold(it.Path, q.text)
}

// revEquals reports whether an svn revision string equals the user-typed value,
// tolerating an optional leading "r" on either side (rev:42 and rev:r42 both
// match revision "42").
func revEquals(rev, v string) bool {
	return strings.TrimPrefix(rev, "r") == strings.TrimPrefix(v, "r")
}

// stateMatches reports whether a file state matches the user-typed value, by its
// single-letter status code (case-insensitive) or a substring of the state name
// (so state:M and state:mod both select modified files). A one-character value is
// read as a code and nothing else: no state is named with a single letter, and
// falling back to a substring would have state:D also take in "modified".
func stateMatches(st svn.FileState, v string) bool {
	if v == "" {
		return true
	}
	if strings.EqualFold(st.Code(), v) {
		return true
	}
	if utf8.RuneCountInString(v) == 1 {
		return false
	}
	return containsFold(string(st), v)
}

// itemStateMatches reads a state filter against a working-copy item, extending
// stateMatches with the "+" history suffix the rows are drawn with: state:A+
// picks out the destinations of a copy or a move, and state:+ those of any code.
// A bare code means the row as it is drawn, so state:A leaves the A+ rows to
// state:A+; a state name is the same either way and still takes both. The suffix
// has no counterpart on the file sections of a revision diff, which carry no
// record of where their content came from.
func itemStateMatches(it svn.StatusItem, v string) bool {
	if code, ok := strings.CutSuffix(v, "+"); ok {
		return it.Copied && stateMatches(it.State, code)
	}
	if it.Copied && strings.EqualFold(it.State.Code(), v) {
		return false
	}
	return stateMatches(it.State, v)
}

// anyPathContains reports whether any changed path contains v (case-insensitive).
func anyPathContains(paths []svn.ChangedPath, v string) bool {
	for _, p := range paths {
		if containsFold(p.Path, v) {
			return true
		}
	}
	return false
}

// containsFold reports whether s contains substr, case-insensitively. An empty
// substr matches anything.
func containsFold(s, substr string) bool {
	if substr == "" {
		return true
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
