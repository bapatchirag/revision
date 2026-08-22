package svn

import "context"

// Add schedules an unversioned path for addition (svn add PATH), turning an
// untracked file into a versioned, added one. Directories are added
// recursively, matching svn's default.
func (c *Client) Add(ctx context.Context, path string) error {
	_, err := c.run(ctx, "add", path)
	return err
}

// AddPaths schedules every path for addition in one invocation rather than one
// each. Adding nothing runs no command.
//
// --force is what makes naming a path that is already versioned harmless: svn
// passes over it and exits zero, where without it the whole invocation is
// refused (E200009). That matters twice over — a set can hold both a directory
// and something under it, which the recursive add reaches first, and a run that
// has to be retried a path at a time would otherwise fail on everything the
// first pass had already added.
func (c *Client) AddPaths(ctx context.Context, paths []string) []PathError {
	return c.runPaths(ctx, paths, func(tail ...string) []string {
		return append([]string{"add", "--force"}, tail...)
	})
}
