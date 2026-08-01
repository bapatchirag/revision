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

func TestLogPageArgs(t *testing.T) {
	cases := []struct {
		name   string
		anchor string
		limit  int
		want   []string
	}{
		{
			name:  "first page over-fetches one row to detect a further page",
			limit: 50,
			want:  []string{"log", "--xml", "--verbose", "--limit", "51", ".@HEAD"},
		},
		{
			name:   "anchored page also covers the repeated anchor revision",
			anchor: "31",
			limit:  50,
			want:   []string{"log", "--xml", "--verbose", "-r", "31:1", "--limit", "52", ".@HEAD"},
		},
		{
			name:  "no limit leaves it to svn",
			limit: 0,
			want:  []string{"log", "--xml", "--verbose", ".@HEAD"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := logPageArgs(tc.anchor, tc.limit)
			if len(got) != len(tc.want) {
				t.Fatalf("args = %q, want %q", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("args = %q, want %q", got, tc.want)
				}
			}
		})
	}
}

func TestLogPageFrom(t *testing.T) {
	entries := func(revs ...string) []LogEntry {
		out := make([]LogEntry, 0, len(revs))
		for _, r := range revs {
			out = append(out, LogEntry{Revision: r})
		}
		return out
	}
	revisions := func(in []LogEntry) []string {
		out := make([]string, 0, len(in))
		for _, e := range in {
			out = append(out, e.Revision)
		}
		return out
	}
	cases := []struct {
		name     string
		entries  []LogEntry
		anchor   string
		limit    int
		want     []string
		wantMore bool
	}{
		{
			name:     "surplus row signals a further page and is dropped",
			entries:  entries("9", "8", "7"),
			limit:    2,
			want:     []string{"9", "8"},
			wantMore: true,
		},
		{
			name:    "an exactly full page is the last page",
			entries: entries("9", "8"),
			limit:   2,
			want:    []string{"9", "8"},
		},
		{
			name:    "a short page is the last page",
			entries: entries("9"),
			limit:   2,
			want:    []string{"9"},
		},
		{
			name:     "the repeated anchor revision is dropped",
			entries:  entries("7", "6", "5", "4"),
			anchor:   "7",
			limit:    2,
			want:     []string{"6", "5"},
			wantMore: true,
		},
		{
			name:    "anchored last page",
			entries: entries("7", "6"),
			anchor:  "7",
			limit:   2,
			want:    []string{"6"},
		},
		{
			name:    "nothing beyond the anchor",
			entries: entries("7"),
			anchor:  "7",
			limit:   2,
			want:    []string{},
		},
		{
			name:    "no limit keeps everything",
			entries: entries("9", "8", "7"),
			limit:   0,
			want:    []string{"9", "8", "7"},
		},
		{
			name:    "empty result",
			entries: entries(),
			limit:   2,
			want:    []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, more := logPageFrom(tc.entries, tc.anchor, tc.limit)
			gotRevs := revisions(got)
			if len(gotRevs) != len(tc.want) {
				t.Fatalf("revisions = %q, want %q", gotRevs, tc.want)
			}
			for i := range gotRevs {
				if gotRevs[i] != tc.want[i] {
					t.Fatalf("revisions = %q, want %q", gotRevs, tc.want)
				}
			}
			if more != tc.wantMore {
				t.Errorf("more = %v, want %v", more, tc.wantMore)
			}
		})
	}
}
