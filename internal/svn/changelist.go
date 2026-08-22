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

// AddToChangelistPaths assigns every path to the named changelist, taking one
// invocation for the lot rather than one each — svn costs tens of milliseconds
// to start, which a directory's worth of files turns into seconds of them.
// Assigning nothing runs no command, svn refusing an invocation naming no path.
// What each path came to is on the result.
func (c *Client) AddToChangelistPaths(ctx context.Context, changelist string, paths []string) []PathError {
	return c.runPaths(ctx, paths, func(tail ...string) []string {
		return append([]string{"changelist", changelist}, tail...)
	})
}

// RemoveFromChangelistPaths removes every path from whatever changelist it
// belongs to, unstaging the set in one invocation.
func (c *Client) RemoveFromChangelistPaths(ctx context.Context, paths []string) []PathError {
	return c.runPaths(ctx, paths, func(tail ...string) []string {
		return append([]string{"changelist", "--remove"}, tail...)
	})
}
