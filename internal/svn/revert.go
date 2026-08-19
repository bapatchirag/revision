package svn

import "context"

// Revert discards local modifications to path (svn revert PATH), restoring it to
// its pristine committed state. On a scheduled add it un-schedules the add
// (leaving the file unversioned on disk); on a scheduled delete it restores the
// item.
func (c *Client) Revert(ctx context.Context, path string) error {
	_, err := c.run(ctx, "revert", path)
	return err
}

// RevertPaths discards local modifications across the given paths in one
// invocation, recursing into any directory among them. Reverting nothing is not
// an error and runs no command, since svn refuses an invocation naming no path.
//
// It leaves the same traces behind as Revert: a file that was a scheduled add is
// un-scheduled but stays on disk as unversioned, so a caller clearing a working
// copy has to remove those itself.
func (c *Client) RevertPaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := c.run(ctx, append([]string{"revert", "--depth", "infinity"}, paths...)...)
	return err
}
