package svn

import "context"

// Add schedules an unversioned path for addition (svn add PATH), turning an
// untracked file into a versioned, added one. Directories are added
// recursively, matching svn's default.
func (c *Client) Add(ctx context.Context, path string) error {
	_, err := c.run(ctx, "add", path)
	return err
}
