package svn

import (
	"context"
	"path"
)

// RevertPaths discards local modifications across the given paths in one
// invocation, restoring them to their pristine committed state. On a scheduled
// add it un-schedules the add (leaving the file unversioned on disk); on a
// scheduled delete it restores the item. Reverting nothing is not an error and
// runs no command, since svn refuses an invocation naming no path.
//
// The revert always recurses: svn will not revert a directory scheduled for
// addition at the default depth (E155038, "without reverting children").
//
// It leaves a trace behind: a file that was a scheduled add is un-scheduled but
// stays on disk as unversioned, so a caller clearing a working copy has to
// remove those itself.
func (c *Client) RevertPaths(ctx context.Context, paths []string) error {
	targets := pruneCovered(paths)
	if len(targets) == 0 {
		return nil
	}
	_, err := c.run(ctx, append([]string{"revert", "--depth", "infinity"}, targets...)...)
	return err
}

// pruneCovered drops the paths a recursive revert already reaches through an
// ancestor in the same set, along with exact duplicates. Naming both is not
// merely redundant: svn reverts the ancestor first, which leaves the descendant
// unversioned, and reverting that then fails with E155010 and takes the whole
// invocation's exit code with it.
//
// It works by set membership rather than by sorting, because a bytewise sort
// does not keep a subtree contiguous: "a-b" sorts between "a" and "a/c".
func pruneCovered(paths []string) []string {
	have := make(map[string]bool, len(paths))
	for _, p := range paths {
		have[p] = true
	}
	out := make([]string, 0, len(paths))
	kept := make(map[string]bool, len(paths))
	for _, p := range paths {
		if kept[p] || hasAncestorIn(p, have) {
			continue
		}
		kept[p] = true
		out = append(out, p)
	}
	return out
}

// hasAncestorIn reports whether any directory above p is itself in have.
func hasAncestorIn(p string, have map[string]bool) bool {
	for {
		dir := path.Dir(p)
		if dir == p {
			return false
		}
		if have[dir] {
			return true
		}
		p = dir
	}
}
