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

func TestInfoBranch(t *testing.T) {
	cases := map[string]struct {
		url, root string
		want      string
	}{
		"trunk":         {"https://svn.example.com/repo/trunk", "https://svn.example.com/repo", "trunk"},
		"trunk subdir":  {"https://svn.example.com/repo/trunk/cmd/app", "https://svn.example.com/repo", "trunk"},
		"branch":        {"https://svn.example.com/repo/branches/feature-x", "https://svn.example.com/repo", "branches/feature-x"},
		"branch subdir": {"https://svn.example.com/repo/branches/feature-x/cmd", "https://svn.example.com/repo", "branches/feature-x"},
		"tag":           {"https://svn.example.com/repo/tags/v1.0", "https://svn.example.com/repo", "tags/v1.0"},
		"project trunk": {"https://svn.example.com/repo/proj/trunk", "https://svn.example.com/repo", "trunk"},
		"no root":       {"https://svn.example.com/repo/branches/feature-x", "", "branches/feature-x"},
		"bare branches": {"https://svn.example.com/repo/branches", "https://svn.example.com/repo", ""},
		"repo root":     {"https://svn.example.com/repo", "https://svn.example.com/repo", ""},
		"no layout":     {"https://svn.example.com/repo/src", "https://svn.example.com/repo", ""},
		"empty":         {"", "", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			info := &Info{URL: c.url, RepositoryRoot: c.root}
			if got := info.Branch(); got != c.want {
				t.Errorf("Branch(%q) = %q, want %q", c.url, got, c.want)
			}
		})
	}
}

func TestInfoIsOverSSH(t *testing.T) {
	cases := map[string]struct {
		url  string
		want bool
	}{
		"svn+ssh":       {"svn+ssh://host/repo/trunk", true},
		"svn+ssh upper": {"SVN+SSH://host/repo/trunk", true},
		"https":         {"https://svn.example.com/repo/trunk", false},
		"http":          {"http://svn.example.com/repo", false},
		"file":          {"file:///srv/repo", false},
		"svn":           {"svn://host/repo", false},
		"empty":         {"", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			info := &Info{URL: c.url}
			if got := info.IsOverSSH(); got != c.want {
				t.Errorf("IsOverSSH(%q) = %v, want %v", c.url, got, c.want)
			}
		})
	}
}
