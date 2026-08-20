package svn

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubClient returns a Client whose svn binary is a shell script that records
// every argument list it was invoked with, prints out on stdout, and exits with
// code. It lets the thin command wrappers be checked for the argv they build
// without Subversion being installed.
func stubClient(t *testing.T, out string, code int) (*Client, func() []string) {
	t.Helper()
	dir := t.TempDir()
	body := filepath.Join(dir, "stdout.txt")
	if err := os.WriteFile(body, []byte(out), 0o644); err != nil {
		t.Fatalf("write stub output: %v", err)
	}
	argv := filepath.Join(dir, "argv.txt")
	bin := filepath.Join(dir, "svn-stub")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\ncat %s\nexit %d\n", argv, body, code)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub binary: %v", err)
	}
	calls := func() []string {
		b, err := os.ReadFile(argv)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	}
	return &Client{Dir: dir, Bin: bin}, calls
}

func TestBinaryFallsBackToTheDefault(t *testing.T) {
	if got := (&Client{}).binary(); got != DefaultBinary {
		t.Errorf("binary() = %q, want %q when Bin is unset", got, DefaultBinary)
	}
	if got := (&Client{Bin: "/opt/svn"}).binary(); got != "/opt/svn" {
		t.Errorf("binary() = %q, want the configured path", got)
	}
}

func TestWithUserActionRidesOnTheRecord(t *testing.T) {
	c, _ := stubClient(t, "", 0)
	var got []CommandRecord
	c.Recorder = func(r CommandRecord) { got = append(got, r) }

	if err := c.Add(context.Background(), "a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := c.Add(WithUserAction(context.Background()), "a.txt"); err != nil {
		t.Fatalf("Add (user action): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("recorded %d commands, want 2", len(got))
	}
	if got[0].UserAction {
		t.Error("a plain context must not mark the record as a user action")
	}
	if !got[1].UserAction {
		t.Error("WithUserAction must mark the record as a user action")
	}
	if got[0].Subcommand != "add" {
		t.Errorf("Subcommand = %q, want the svn subcommand", got[0].Subcommand)
	}
}

// TestCommandWrappersBuildTheirArgv pins the argv each thin wrapper produces,
// which is the whole of what these one-line commands do.
func TestCommandWrappersBuildTheirArgv(t *testing.T) {
	cases := map[string]struct {
		call func(*Client) error
		want string
	}{
		"add": {
			func(c *Client) error { return c.Add(context.Background(), "a.txt") },
			"add a.txt --non-interactive",
		},
		"add to changelist": {
			func(c *Client) error { return c.AddToChangelist(context.Background(), "feature", "a.txt") },
			"changelist feature a.txt --non-interactive",
		},
		"remove from changelist": {
			func(c *Client) error { return c.RemoveFromChangelist(context.Background(), "a.txt") },
			"changelist --remove a.txt --non-interactive",
		},
		"delete": {
			func(c *Client) error { return c.Delete(context.Background(), "a.txt") },
			"delete --force a.txt --non-interactive",
		},
		"revert": {
			func(c *Client) error { return c.RevertPaths(context.Background(), []string{"a.txt"}) },
			"revert --depth infinity a.txt --non-interactive",
		},
		"revert paths": {
			func(c *Client) error { return c.RevertPaths(context.Background(), []string{"a.txt", "sub"}) },
			"revert --depth infinity a.txt sub --non-interactive",
		},
		"resolve": {
			func(c *Client) error { return c.Resolve(context.Background(), "a.txt") },
			"resolve --accept working a.txt --non-interactive",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c, calls := stubClient(t, "", 0)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got := calls(); len(got) != 1 || got[0] != tc.want {
				t.Errorf("argv = %q, want %q", got, tc.want)
			}

			bad, _ := stubClient(t, "", 1)
			if err := tc.call(bad); err == nil {
				t.Error("a command svn refused must return an error")
			}
		})
	}
}

func TestRemoveUnversionedResolvesAgainstTheWorkingCopy(t *testing.T) {
	c, _ := stubClient(t, "", 0)
	path := filepath.Join(c.Dir, "junk", "a.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := c.RemoveUnversioned("junk"); err != nil {
		t.Fatalf("RemoveUnversioned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "junk")); !os.IsNotExist(err) {
		t.Error("the unversioned tree should be gone from disk")
	}
}

func TestDiffOmitsAnEmptyPath(t *testing.T) {
	const patch = "Index: a.txt\n@@ -1 +1 @@\n-old\n+new\n"

	c, calls := stubClient(t, patch, 0)
	got, err := c.Diff(context.Background(), "")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if got != patch {
		t.Errorf("Diff = %q, want svn's output verbatim", got)
	}
	if argv := calls(); len(argv) != 1 || argv[0] != "diff --non-interactive" {
		t.Errorf("argv = %q, want the whole working copy diffed", argv)
	}

	c, calls = stubClient(t, patch, 0)
	if _, err := c.Diff(context.Background(), "a.txt"); err != nil {
		t.Fatalf("Diff(path): %v", err)
	}
	if argv := calls(); len(argv) != 1 || argv[0] != "diff a.txt --non-interactive" {
		t.Errorf("argv = %q, want the named path diffed", argv)
	}

	bad, _ := stubClient(t, "", 1)
	if _, err := bad.Diff(context.Background(), "a.txt"); err == nil {
		t.Error("a diff svn refused must return an error")
	}
}

func TestDiffPaths(t *testing.T) {
	const patch = "Index: a.txt\n@@ -1 +1 @@\n-old\n+new\n"

	cases := map[string]struct {
		paths []string
		want  string
	}{
		"none":   {nil, "diff --non-interactive"},
		"single": {[]string{"a.txt"}, "diff a.txt --non-interactive"},
		"many":   {[]string{"a.txt", "sub/b.txt"}, "diff a.txt sub/b.txt --non-interactive"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c, calls := stubClient(t, patch, 0)
			got, err := c.DiffPaths(context.Background(), tc.paths)
			if err != nil {
				t.Fatalf("DiffPaths: %v", err)
			}
			if got != patch {
				t.Errorf("DiffPaths = %q, want svn's output verbatim", got)
			}
			if argv := calls(); len(argv) != 1 || argv[0] != tc.want {
				t.Errorf("argv = %q, want %q", argv, tc.want)
			}
		})
	}

	bad, _ := stubClient(t, "", 1)
	if _, err := bad.DiffPaths(context.Background(), []string{"a.txt"}); err == nil {
		t.Error("a diff svn refused must return an error")
	}
}

func TestDiffPathsForPatchingAsksForCopiedContent(t *testing.T) {
	const patch = "Index: a.txt\n@@ -0,0 +1 @@\n+new\n"

	cases := map[string]struct {
		paths []string
		want  string
	}{
		"none":   {nil, "diff --show-copies-as-adds --non-interactive"},
		"single": {[]string{"a.txt"}, "diff --show-copies-as-adds a.txt --non-interactive"},
		"many":   {[]string{"a.txt", "sub/b.txt"}, "diff --show-copies-as-adds a.txt sub/b.txt --non-interactive"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c, calls := stubClient(t, patch, 0)
			got, err := c.DiffPathsForPatching(context.Background(), tc.paths)
			if err != nil {
				t.Fatalf("DiffPathsForPatching: %v", err)
			}
			if got != patch {
				t.Errorf("DiffPathsForPatching = %q, want svn's output verbatim", got)
			}
			if argv := calls(); len(argv) != 1 || argv[0] != tc.want {
				t.Errorf("argv = %q, want %q", argv, tc.want)
			}
		})
	}

	bad, _ := stubClient(t, "", 1)
	if _, err := bad.DiffPathsForPatching(context.Background(), []string{"a.txt"}); err == nil {
		t.Error("a diff svn refused must return an error")
	}
}

func TestRevertPathsWithNothingToRevertRunsNothing(t *testing.T) {
	// svn refuses an invocation naming no path, so an empty set has to stop here
	// rather than reach the command line.
	c, calls := stubClient(t, "", 0)
	if err := c.RevertPaths(context.Background(), nil); err != nil {
		t.Fatalf("RevertPaths(nil) = %v, want no error", err)
	}
	if got := calls(); len(got) != 0 {
		t.Errorf("argv = %q, want no svn invocation", got)
	}
}

// TestRevertPathsPrunesCoveredPaths pins why RevertPaths does not hand svn every
// path it is given. The revert recurses, so a directory named alongside
// something beneath it reverts the descendant on the way past and then fails on
// it with E155010, sinking the exit code of an invocation that did the work.
func TestRevertPathsPrunesCoveredPaths(t *testing.T) {
	cases := map[string]struct {
		paths []string
		want  []string
	}{
		"a directory swallows its subtree": {
			[]string{"mpte", "mpte/Makefile", "mpte/rust-crate", "mpte/rust-crate/src/lib.rs"},
			[]string{"mpte"},
		},
		"a sibling sharing a prefix survives": {
			// "mpte-compute" sorts between "mpte" and "mpte/Makefile" bytewise, so
			// this is what a prune walking a sorted list gets wrong.
			[]string{"mpte", "mpte-compute", "mpte/Makefile"},
			[]string{"mpte", "mpte-compute"},
		},
		"a covered path goes even when it comes first": {
			[]string{"mpte/rust-crate/src/lib.rs", "mpte"},
			[]string{"mpte"},
		},
		"duplicates collapse": {
			[]string{"a.txt", "a.txt"},
			[]string{"a.txt"},
		},
		"the working-copy root swallows everything": {
			[]string{".", "a.txt", "sub/b.txt"},
			[]string{"."},
		},
		"unrelated paths all survive": {
			[]string{"a.txt", "sub/b.txt", "other/c.txt"},
			[]string{"a.txt", "sub/b.txt", "other/c.txt"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c, calls := stubClient(t, "", 0)
			if err := c.RevertPaths(context.Background(), tc.paths); err != nil {
				t.Fatalf("RevertPaths: %v", err)
			}
			want := "revert --depth infinity " + strings.Join(tc.want, " ") + " --non-interactive"
			if got := calls(); len(got) != 1 || got[0] != want {
				t.Errorf("argv = %q, want [%q]", got, want)
			}
		})
	}
}

func TestDiffRevisionUsesChangeShorthand(t *testing.T) {
	const patch = "Index: a.txt\n@@ -0,0 +1 @@\n+a1\n"

	c, calls := stubClient(t, patch, 0)
	got, err := c.DiffRevision(context.Background(), "7")
	if err != nil {
		t.Fatalf("DiffRevision: %v", err)
	}
	if got != patch {
		t.Errorf("DiffRevision = %q, want svn's output verbatim", got)
	}
	// No path: the diff is scoped to the client's directory, so its paths come
	// out relative to whatever the display scope rooted it at.
	if argv := calls(); len(argv) != 1 || argv[0] != "diff -c 7 --non-interactive" {
		t.Errorf("argv = %q, want the single revision diffed", argv)
	}

	bad, _ := stubClient(t, "", 1)
	if _, err := bad.DiffRevision(context.Background(), "7"); err == nil {
		t.Error("a revision svn refused must return an error")
	}
}

func TestDiffRevisionsOrdersTheRangeForwards(t *testing.T) {
	cases := map[string]struct {
		from, to string
		want     string
	}{
		"ascending":     {"2", "4", "diff -r 2:4"},
		"descending":    {"4", "2", "diff -r 2:4"},
		"same revision": {"4", "4", "diff -r 4:4"},
		"multi-digit":   {"9", "100", "diff -r 9:100"},
		// Left alone for svn to reject: guessing at an order here would turn a
		// clear error into a silently wrong diff.
		"unparsable": {"head", "4", "diff -r head:4"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := strings.Join(diffRevisionsArgs(tc.from, tc.to), " "); got != tc.want {
				t.Errorf("diffRevisionsArgs(%q, %q) = %q, want %q", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestDiffRevisionsRunsTheRange(t *testing.T) {
	const patch = "Index: a.txt\n@@ -1 +1,2 @@\n a1\n+a2\n"

	c, calls := stubClient(t, patch, 0)
	got, err := c.DiffRevisions(context.Background(), "4", "2")
	if err != nil {
		t.Fatalf("DiffRevisions: %v", err)
	}
	if got != patch {
		t.Errorf("DiffRevisions = %q, want svn's output verbatim", got)
	}
	if argv := calls(); len(argv) != 1 || argv[0] != "diff -r 2:4 --non-interactive" {
		t.Errorf("argv = %q, want the ordered range diffed", argv)
	}

	bad, _ := stubClient(t, "", 1)
	if _, err := bad.DiffRevisions(context.Background(), "2", "4"); err == nil {
		t.Error("a range svn refused must return an error")
	}
}

func TestRevisionDetailAndHeadRevisionErrors(t *testing.T) {
	const empty = `<?xml version="1.0"?><log></log>`

	c, _ := stubClient(t, empty, 0)
	if _, err := c.RevisionDetail(context.Background(), "42"); err == nil {
		t.Error("a revision svn has no entry for must be an error")
	}
	rev, err := c.HeadRevision(context.Background())
	if err != nil {
		t.Fatalf("HeadRevision on an empty log: %v", err)
	}
	if rev != "" {
		t.Errorf("HeadRevision = %q, want empty when there is no history", rev)
	}

	malformed, _ := stubClient(t, "<log", 0)
	if _, err := malformed.RevisionDetail(context.Background(), "42"); err == nil {
		t.Error("malformed log XML must be an error")
	}
	if _, err := malformed.HeadRevision(context.Background()); err == nil {
		t.Error("malformed log XML must be an error")
	}

	failing, _ := stubClient(t, "", 1)
	if _, err := failing.RevisionDetail(context.Background(), "42"); err == nil {
		t.Error("a failing svn log must be an error")
	}
	if _, err := failing.HeadRevision(context.Background()); err == nil {
		t.Error("a failing svn log must be an error")
	}
}
