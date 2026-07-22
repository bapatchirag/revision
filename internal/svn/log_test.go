package svn

import "testing"

func TestParseLog(t *testing.T) {
	entries, err := parseLog(readFixture(t, "log.xml"))
	if err != nil {
		t.Fatalf("parseLog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}

	first := entries[0]
	if first.Revision != "42" {
		t.Errorf("revision = %q, want 42", first.Revision)
	}
	if first.Author != "alice" {
		t.Errorf("author = %q, want alice", first.Author)
	}
	if first.Message != "Add feature and tweak existing file" {
		t.Errorf("message = %q", first.Message)
	}
	if first.Date.IsZero() {
		t.Error("expected a parsed date")
	}
	if y := first.Date.Year(); y != 2026 {
		t.Errorf("date year = %d, want 2026", y)
	}
	if len(first.Paths) != 2 {
		t.Fatalf("got %d changed paths, want 2", len(first.Paths))
	}
	if first.Paths[0].Action != "M" || first.Paths[0].Path != "/trunk/committed.txt" {
		t.Errorf("path[0] = %+v, want {M /trunk/committed.txt}", first.Paths[0])
	}
	if first.Paths[1].Action != "A" || first.Paths[1].Path != "/trunk/added.txt" {
		t.Errorf("path[1] = %+v, want {A /trunk/added.txt}", first.Paths[1])
	}
}

func TestParseLogEmpty(t *testing.T) {
	entries, err := parseLog([]byte(`<?xml version="1.0"?><log></log>`))
	if err != nil {
		t.Fatalf("parseLog: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0", len(entries))
	}
}

func TestParseLogInvalid(t *testing.T) {
	if _, err := parseLog([]byte("<log><oops>")); err == nil {
		t.Fatal("expected error for malformed xml")
	}
}
