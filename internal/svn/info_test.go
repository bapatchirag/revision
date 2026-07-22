package svn

import "testing"

func TestParseInfo(t *testing.T) {
	info, err := parseInfo(readFixture(t, "info.xml"))
	if err != nil {
		t.Fatalf("parseInfo: %v", err)
	}

	checks := map[string]struct{ got, want string }{
		"Path":            {info.Path, "."},
		"Revision":        {info.Revision, "42"},
		"URL":             {info.URL, "https://svn.example.com/repo/trunk"},
		"RepositoryRoot":  {info.RepositoryRoot, "https://svn.example.com/repo"},
		"WorkingCopyRoot": {info.WorkingCopyRoot, "/home/alice/work/wc"},
	}
	for field, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", field, c.got, c.want)
		}
	}
}

func TestParseInfoNoEntries(t *testing.T) {
	if _, err := parseInfo([]byte(`<?xml version="1.0"?><info></info>`)); err == nil {
		t.Fatal("expected error when info has no entries")
	}
}

func TestParseInfoInvalid(t *testing.T) {
	if _, err := parseInfo([]byte("<info><oops>")); err == nil {
		t.Fatal("expected error for malformed xml")
	}
}
