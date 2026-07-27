package app

import (
	"testing"
	"time"

	"github.com/bapatchirag/revision/internal/svn"
)

func TestParseFilterSplitsParamsAndText(t *testing.T) {
	q := parseFilter("rev:42 user:bob test search", logFilterKeys)
	if q.params["rev"] != "42" {
		t.Errorf("rev = %q, want 42", q.params["rev"])
	}
	if q.params["user"] != "bob" {
		t.Errorf("user = %q, want bob", q.params["user"])
	}
	if q.text != "test search" {
		t.Errorf("text = %q, want %q", q.text, "test search")
	}
}

func TestParseFilterUnknownKeyIsText(t *testing.T) {
	// A key that the panel does not recognize stays part of the free text rather
	// than being silently dropped.
	q := parseFilter("nope:x hello", logFilterKeys)
	if len(q.params) != 0 {
		t.Errorf("expected no params, got %v", q.params)
	}
	if q.text != "nope:x hello" {
		t.Errorf("text = %q, want %q", q.text, "nope:x hello")
	}
}

func TestParseFilterCaseInsensitiveKeys(t *testing.T) {
	q := parseFilter("REV:9 User:sam", logFilterKeys)
	if q.params["rev"] != "9" || q.params["user"] != "sam" {
		t.Errorf("params = %v, want rev=9 user=sam", q.params)
	}
}

func TestParseFilterEmptyValueIsText(t *testing.T) {
	q := parseFilter("rev: keep", logFilterKeys)
	if _, ok := q.params["rev"]; ok {
		t.Errorf("empty-value key should not be a param, params = %v", q.params)
	}
	if q.text != "rev: keep" {
		t.Errorf("text = %q, want %q", q.text, "rev: keep")
	}
}

func TestMatchLogEntryCombination(t *testing.T) {
	e := svn.LogEntry{
		Revision: "42",
		Author:   "bob",
		Message:  "Fix crash\n\nAddress the null pointer in the parser.",
	}
	if !matchLogEntry(e, parseFilter("rev:42 user:bob null pointer", logFilterKeys)) {
		t.Error("expected entry to match rev+user+message text")
	}
	// Free text searches the FULL message, not just its first line.
	if !matchLogEntry(e, parseFilter("parser", logFilterKeys)) {
		t.Error("free text should match text on a later message line")
	}
	if matchLogEntry(e, parseFilter("rev:43", logFilterKeys)) {
		t.Error("a non-matching revision should exclude the entry")
	}
	if matchLogEntry(e, parseFilter("user:alice", logFilterKeys)) {
		t.Error("a non-matching author should exclude the entry")
	}
}

func TestMatchLogEntryRevToleratesRPrefix(t *testing.T) {
	e := svn.LogEntry{Revision: "42"}
	if !matchLogEntry(e, parseFilter("rev:r42", logFilterKeys)) {
		t.Error("rev:r42 should match revision 42")
	}
	if matchLogEntry(e, parseFilter("rev:4", logFilterKeys)) {
		t.Error("rev is an exact match, not a substring")
	}
}

func TestMatchLogEntryPathAndDate(t *testing.T) {
	e := svn.LogEntry{
		Revision: "7",
		Date:     time.Date(2026, 7, 27, 10, 30, 0, 0, time.UTC),
		Paths:    []svn.ChangedPath{{Action: "M", Path: "internal/app/app.go"}},
	}
	if !matchLogEntry(e, parseFilter("path:app.go", logFilterKeys)) {
		t.Error("path param should match a changed path substring")
	}
	if !matchLogEntry(e, parseFilter("date:2026-07-27", logFilterKeys)) {
		t.Error("date param should match the formatted date")
	}
	if matchLogEntry(e, parseFilter("path:missing.go", logFilterKeys)) {
		t.Error("a non-matching path should exclude the entry")
	}
}

func TestMatchStatusItemStateAndPath(t *testing.T) {
	it := svn.StatusItem{Path: "internal/app/app.go", State: svn.StateModified, Changelist: "feature-x"}
	if !matchStatusItem(it, parseFilter("state:M", fileFilterKeys)) {
		t.Error("state:M should match a modified file by code")
	}
	if !matchStatusItem(it, parseFilter("state:modified", fileFilterKeys)) {
		t.Error("state:modified should match by state name")
	}
	if !matchStatusItem(it, parseFilter("cl:feature", fileFilterKeys)) {
		t.Error("cl:feature should match the changelist name")
	}
	if !matchStatusItem(it, parseFilter("app.go", fileFilterKeys)) {
		t.Error("free text should match the file path")
	}
	if matchStatusItem(it, parseFilter("state:A", fileFilterKeys)) {
		t.Error("state:A should not match a modified file")
	}
	if matchStatusItem(it, parseFilter("other.go", fileFilterKeys)) {
		t.Error("free text not in the path should exclude the item")
	}
}

func TestMatchStatusItemChangelistLabels(t *testing.T) {
	staged := svn.StatusItem{Path: "a.txt", State: svn.StateModified, Changelist: stagedChangelist}
	if !matchStatusItem(staged, parseFilter("cl:staged", fileFilterKeys)) {
		t.Error("cl:staged should match the staged bucket via its (staged) label")
	}
	unstaged := svn.StatusItem{Path: "b.txt", State: svn.StateModified}
	if !matchStatusItem(unstaged, parseFilter("cl:unstaged", fileFilterKeys)) {
		t.Error("cl:unstaged should match the default group via its (unstaged) label")
	}
}

func TestEmptyQueryMatchesEverything(t *testing.T) {
	if !matchLogEntry(svn.LogEntry{}, parseFilter("   ", logFilterKeys)) {
		t.Error("a blank log filter should match every entry")
	}
	if !matchStatusItem(svn.StatusItem{}, parseFilter("", fileFilterKeys)) {
		t.Error("a blank file filter should match every item")
	}
}
