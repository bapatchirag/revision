package svn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pickyStub returns a Client whose svn stub records every invocation, refuses
// any naming one of bad with the given stderr, and otherwise announces each
// target as reverted. It is what pins that a revert svn aborts still reaches the
// targets listed after the one it choked on.
func pickyStub(t *testing.T, stderr string, bad ...string) (*Client, func() []string) {
	t.Helper()
	dir := t.TempDir()
	argv := filepath.Join(dir, "argv.txt")
	bin := filepath.Join(dir, "svn-stub")
	pattern := "\x00none\x00"
	if len(bad) > 0 {
		pattern = strings.Join(bad, "|")
	}
	script := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %s
for a in "$@"; do
  case "$a" in
  %s) echo "%s" >&2; exit 1 ;;
  esac
done
for a in "$@"; do
  case "$a" in
  revert|--depth|infinity|--non-interactive) ;;
  *) echo "Reverted '$a'" ;;
  esac
done
exit 0
`, argv, pattern, stderr)
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

// TestRevertPathsRetriesWhatTheAbortedRunLeft pins the failure RevertPaths
// exists to survive: svn walks a multi-target revert in order and abandons the
// whole process at the first target it refuses, so everything after it is never
// looked at. Falling back to one target per invocation is what gets the rest
// reverted, and what names the one that would not go.
func TestRevertPathsRetriesWhatTheAbortedRunLeft(t *testing.T) {
	c, calls := pickyStub(t, "svn: E155010: The node 'b.txt' was not found.", "b.txt")

	res := c.RevertPaths(context.Background(), []string{"a.txt", "b.txt", "c.txt"})

	if len(res.Failed) != 1 || res.Failed[0].Path != "b.txt" {
		t.Errorf("Failed = %v, want only the path svn refused", res.Failed)
	}
	if !equalPaths(res.Reverted, []string{"a.txt", "c.txt"}) {
		t.Errorf("Reverted = %v, want the paths either side of it", res.Reverted)
	}
	argv := calls()
	if len(argv) != 4 {
		t.Fatalf("argv = %q, want the batch attempt then one invocation per path", argv)
	}
	if argv[0] != "revert --depth infinity a.txt b.txt c.txt --non-interactive" {
		t.Errorf("argv[0] = %q, want the whole set attempted first", argv[0])
	}
	if argv[3] != "revert --depth infinity c.txt --non-interactive" {
		t.Errorf("argv[3] = %q, want c.txt asked for on its own", argv[3])
	}
}

// TestRevertPathsStopsWhenTheWorkingCopyItselfRefuses pins the one failure worth
// giving up on: a locked working copy refuses every path alike, so asking it
// once per file only makes the user wait to be told so.
func TestRevertPathsStopsWhenTheWorkingCopyItselfRefuses(t *testing.T) {
	c, calls := pickyStub(t, "svn: E155004: Working copy locked.", "a.txt", "b.txt", "c.txt")

	res := c.RevertPaths(context.Background(), []string{"a.txt", "b.txt", "c.txt"})

	if len(res.Reverted) != 0 {
		t.Errorf("Reverted = %v, want nothing reverted", res.Reverted)
	}
	if len(res.Failed) != 3 {
		t.Errorf("Failed = %v, want every path accounted for", res.Failed)
	}
	if !IsLockedError(res.Err()) {
		t.Errorf("Err() = %v, want the lock reported", res.Err())
	}
	if argv := calls(); len(argv) != 2 {
		t.Errorf("argv = %q, want the batch attempt and one retry before giving up", argv)
	}
}

// TestRevertPathsReportsWhatSvnPassedOver pins the quiet half of the same
// failure: svn announces a path it will not touch and exits zero all the same,
// so an exit code taken for the whole answer reports a revert that discarded
// nothing as a success.
func TestRevertPathsReportsWhatSvnPassedOver(t *testing.T) {
	c, _ := stubClient(t, "Reverted 'a.txt'\nSkipped 'gone.txt'\n", 0)

	res := c.RevertPaths(context.Background(), []string{"a.txt", "gone.txt"})

	if !equalPaths(res.Skipped, []string{"gone.txt"}) {
		t.Errorf("Skipped = %v, want the path svn passed over", res.Skipped)
	}
	if !equalPaths(res.Reverted, []string{"a.txt"}) {
		t.Errorf("Reverted = %v, want only what svn actually took", res.Reverted)
	}
}

// TestRevertPathsCountsWhatADirectoryTookWithIt pins that pruning a covered path
// out of the invocation does not lose it from the answer: the recursive revert
// of its parent is what reverted it.
func TestRevertPathsCountsWhatADirectoryTookWithIt(t *testing.T) {
	c, calls := stubClient(t, "Reverted 'sub'\n", 0)

	res := c.RevertPaths(context.Background(), []string{"sub", "sub/x.txt", "sub/deep/y.txt"})

	if !equalPaths(res.Reverted, []string{"sub", "sub/x.txt", "sub/deep/y.txt"}) {
		t.Errorf("Reverted = %v, want every path asked for", res.Reverted)
	}
	want := "revert --depth infinity sub --non-interactive"
	if got := calls(); len(got) != 1 || got[0] != want {
		t.Errorf("argv = %q, want only the directory named", got)
	}
}

func TestCoverPathsGroupsDescendantsUnderTheirHighestAncestor(t *testing.T) {
	cases := map[string]struct {
		paths []string
		want  []PathCover
	}{
		"a directory carries its whole subtree": {
			[]string{"mpte", "mpte/Makefile", "mpte/rust/src/lib.rs"},
			[]PathCover{{Lead: "mpte", Paths: []string{"mpte", "mpte/Makefile", "mpte/rust/src/lib.rs"}}},
		},
		"the highest ancestor wins, not the nearest": {
			[]string{"a", "a/b", "a/b/c.txt"},
			[]PathCover{{Lead: "a", Paths: []string{"a", "a/b", "a/b/c.txt"}}},
		},
		"a sibling sharing a prefix stands on its own": {
			[]string{"mpte", "mpte-compute", "mpte/Makefile"},
			[]PathCover{
				{Lead: "mpte", Paths: []string{"mpte", "mpte/Makefile"}},
				{Lead: "mpte-compute", Paths: []string{"mpte-compute"}},
			},
		},
		"a covered path is placed even when it comes first": {
			[]string{"a/b.txt", "a"},
			[]PathCover{{Lead: "a", Paths: []string{"a", "a/b.txt"}}},
		},
		"duplicates collapse": {
			[]string{"a.txt", "a.txt"},
			[]PathCover{{Lead: "a.txt", Paths: []string{"a.txt"}}},
		},
		"unrelated paths each lead": {
			[]string{"a.txt", "sub/b.txt"},
			[]PathCover{
				{Lead: "a.txt", Paths: []string{"a.txt"}},
				{Lead: "sub/b.txt", Paths: []string{"sub/b.txt"}},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := CoverPaths(tc.paths)
			if len(got) != len(tc.want) {
				t.Fatalf("CoverPaths = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i].Lead != tc.want[i].Lead || !equalPaths(got[i].Paths, tc.want[i].Paths) {
					t.Errorf("cover %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseRevertOutputIgnoresEverythingElse(t *testing.T) {
	res := parseRevertOutput("Reverted 'a.txt'\r\nSkipped 'b.txt'\nsome unrelated chatter\nReverted\n")
	if !equalPaths(res.Reverted, []string{"a.txt"}) {
		t.Errorf("Reverted = %v, want the one path svn named", res.Reverted)
	}
	if !equalPaths(res.Skipped, []string{"b.txt"}) {
		t.Errorf("Skipped = %v, want the one path svn passed over", res.Skipped)
	}
}

func TestRevertResultErrIsNilWhenNothingWasRefused(t *testing.T) {
	if err := (RevertResult{Reverted: []string{"a.txt"}}).Err(); err != nil {
		t.Errorf("Err() = %v, want nil", err)
	}
	res := RevertResult{Failed: []PathError{{Path: "a.txt", Err: errors.New("refused")}}}
	if res.Err() == nil {
		t.Error("Err() must report a refusal")
	}
}
