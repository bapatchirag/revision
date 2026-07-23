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
