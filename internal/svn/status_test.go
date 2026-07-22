package svn

import (
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestParseStatus(t *testing.T) {
	items, err := parseStatus(readFixture(t, "status.xml"))
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}

	want := []StatusItem{
		{Path: "added.txt", State: StateAdded, PropState: StateNone, Revision: "-1", Changelist: "revision:staged"},
		{Path: "committed.txt", State: StateModified, PropState: StateNone, Revision: "1"},
		{Path: "untracked.txt", State: StateUnversioned, PropState: StateNone},
	}
	if len(items) != len(want) {
		t.Fatalf("got %d items, want %d: %+v", len(items), len(want), items)
	}
	for i, w := range want {
		if items[i] != w {
			t.Errorf("item[%d] = %+v, want %+v", i, items[i], w)
		}
	}
}

func TestParseStatusEmpty(t *testing.T) {
	items, err := parseStatus([]byte(`<?xml version="1.0"?><status><target path="."></target></status>`))
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("got %d items, want 0", len(items))
	}
}

func TestParseStatusInvalid(t *testing.T) {
	if _, err := parseStatus([]byte("<status><oops>")); err == nil {
		t.Fatal("expected error for malformed xml")
	}
}
