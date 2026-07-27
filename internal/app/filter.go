package app

import (
	"strings"

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
			if !stateMatches(it.State, v) {
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
// (so state:M and state:mod both select modified files).
func stateMatches(st svn.FileState, v string) bool {
	if v == "" {
		return true
	}
	if strings.EqualFold(st.Code(), v) {
		return true
	}
	return containsFold(string(st), v)
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
