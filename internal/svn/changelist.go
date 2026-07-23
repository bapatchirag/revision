package svn

import "context"

// AddToChangelist assigns path to the named changelist
// (svn changelist NAME PATH). Changelists are how revision emulates a staging
// area: membership marks a path as staged.
func (c *Client) AddToChangelist(ctx context.Context, changelist, path string) error {
	_, err := c.run(ctx, "changelist", changelist, path)
	return err
}

// RemoveFromChangelist removes path from whatever changelist it belongs to
// (svn changelist --remove PATH), i.e. unstages it.
func (c *Client) RemoveFromChangelist(ctx context.Context, path string) error {
	_, err := c.run(ctx, "changelist", "--remove", path)
	return err
}
