package svn

import (
	"context"
	"path"
	"strings"
)

// RevertResult is what a revert managed, path by path. Reverted names the paths
// svn restored, Skipped the ones it passed over untouched — a path it does not
// track — and Failed the ones it refused, each with svn's reason.
//
// Skipped is worth as much as Failed here: svn reports it and exits zero, so
// without it a revert that did nothing at all reads as a success.
type RevertResult struct {
	Reverted []string
	Skipped  []string
	Failed   []PathError
}

// Err collapses the refusals into one error, or nil when nothing was refused.
func (r RevertResult) Err() error { return collapse(r.Failed) }

// RevertPaths discards local modifications across the given paths, restoring
// them to their pristine committed state. On a scheduled add it un-schedules the
// add (leaving the file unversioned on disk); on a scheduled delete it restores
// the item. Reverting nothing is not an error and runs no command, since svn
// refuses an invocation naming no path.
//
// The revert always recurses: svn will not revert a directory scheduled for
// addition at the default depth (E155038, "without reverting children").
//
// It takes one invocation for the whole set, which is what an untroubled revert
// costs. svn walks its targets in order and aborts the process at the first one
// it refuses, leaving every target after it untouched — so a run that comes back
// non-zero is retried a path at a time, which cannot let one bad path decide the
// fate of the rest. What each path came to is on the result.
//
// It leaves a trace behind: a file that was a scheduled add is un-scheduled but
// stays on disk as unversioned, so a caller clearing a working copy has to
// remove those itself.
func (c *Client) RevertPaths(ctx context.Context, paths []string) RevertResult {
	covers := CoverPaths(paths)
	if len(covers) == 0 {
		return RevertResult{}
	}
	out, err := c.run(ctx, revertArgs(coverLeads(covers))...)
	if err != nil {
		return c.revertEach(ctx, covers)
	}
	skipped := set(parseRevertOutput(string(out)).Skipped)
	var res RevertResult
	for _, cv := range covers {
		if skipped[cv.Lead] {
			res.Skipped = append(res.Skipped, cv.Paths...)
			continue
		}
		res.Reverted = append(res.Reverted, cv.Paths...)
	}
	return res
}

// revertEach reverts one target per invocation, so each is judged on its own. It
// is what RevertPaths falls back to once the single invocation has aborted, and
// is safe to run over targets that attempt already reverted: reverting a path
// with nothing left to discard prints nothing and exits zero.
//
// A failure that is about the working copy rather than the path — it is locked,
// or svn cannot authenticate — ends the pass. Every target left would fail the
// same way, and spawning svn once more for each of them only makes the user wait
// to be told so.
func (c *Client) revertEach(ctx context.Context, covers []PathCover) RevertResult {
	var res RevertResult
	for i, cv := range covers {
		out, err := c.run(ctx, revertArgs([]string{cv.Lead})...)
		switch {
		case err == nil && len(parseRevertOutput(string(out)).Skipped) > 0:
			res.Skipped = append(res.Skipped, cv.Paths...)
		case err == nil:
			res.Reverted = append(res.Reverted, cv.Paths...)
		case BlocksEveryPath(err):
			for _, rest := range covers[i:] {
				res.addFailed(rest.Paths, err)
			}
			return res
		default:
			res.addFailed(cv.Paths, err)
		}
	}
	return res
}

// addFailed records one reason against every path it kept from being reverted.
func (r *RevertResult) addFailed(paths []string, err error) {
	for _, p := range paths {
		r.Failed = append(r.Failed, PathError{Path: p, Err: err})
	}
}

// revertArgs is the invocation that reverts the given targets.
func revertArgs(targets []string) []string {
	return append([]string{"revert", "--depth", "infinity"}, targets...)
}

// parseRevertOutput reads the per-path lines `svn revert` prints, which name the
// path in quotes: "Reverted 'a.txt'" for one it restored, "Skipped 'a.txt'" for
// one it will not touch. A path with nothing to discard is announced by neither.
func parseRevertOutput(out string) RevertResult {
	var res RevertResult
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		p, ok := quotedPath(line)
		if !ok {
			continue
		}
		switch {
		case strings.HasPrefix(line, "Reverted"):
			res.Reverted = append(res.Reverted, p)
		case strings.HasPrefix(line, "Skipped"):
			res.Skipped = append(res.Skipped, p)
		}
	}
	return res
}

// set collects paths for membership tests.
func set(paths []string) map[string]bool {
	out := make(map[string]bool, len(paths))
	for _, p := range paths {
		out[p] = true
	}
	return out
}

// PathCover is one path an operation is actually run on, together with every
// path in the same set that rides along with it — itself included. A recursive
// command reaches those through the lead, so the lead's verdict is theirs.
type PathCover struct {
	Lead  string
	Paths []string
}

// CoverPaths groups paths under the ancestor in the same set a recursive
// operation reaches them through, keeping the order they were given in and
// dropping exact duplicates.
//
// Running a command on both an ancestor and something beneath it is not merely
// redundant: svn acts on the ancestor first, which takes the descendant with it,
// and naming that descendant afterwards fails — E155010 for a revert, E155007
// for a delete — and takes the whole invocation's exit code with it. svn status
// reports a scheduled-add directory alongside every file under it, so a set
// holding both is the ordinary case rather than a corner one.
//
// It works by set membership rather than by sorting, because a bytewise sort
// does not keep a subtree contiguous: "a-b" sorts between "a" and "a/c".
func CoverPaths(paths []string) []PathCover {
	have := set(paths)
	index := make(map[string]int, len(paths))
	var out []PathCover
	for _, p := range paths {
		if _, seen := index[p]; seen || topAncestorIn(p, have) != "" {
			continue
		}
		index[p] = len(out)
		out = append(out, PathCover{Lead: p, Paths: []string{p}})
	}
	placed := make(map[string]bool, len(paths))
	for _, p := range paths {
		lead := topAncestorIn(p, have)
		if lead == "" || placed[p] {
			continue
		}
		placed[p] = true
		out[index[lead]].Paths = append(out[index[lead]].Paths, p)
	}
	return out
}

// coverLeads is the paths the covers are run on.
func coverLeads(covers []PathCover) []string {
	leads := make([]string, 0, len(covers))
	for _, cv := range covers {
		leads = append(leads, cv.Lead)
	}
	return leads
}

// topAncestorIn returns the highest directory above p that is itself in have, or
// "" when none is. The highest rather than the nearest: with both "a" and "a/b"
// present, a command run on "a" is what reaches "a/b/c".
func topAncestorIn(p string, have map[string]bool) string {
	top := ""
	for {
		dir := path.Dir(p)
		if dir == p {
			return top
		}
		if have[dir] {
			top = dir
		}
		p = dir
	}
}
