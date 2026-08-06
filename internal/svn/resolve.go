package svn

import "context"

// Resolve clears the conflict on path, accepting the file as it now stands in
// the working copy. It is what follows writing a merged file out by hand: svn
// stops treating the path as conflicted and removes the .mine/.rN artifacts it
// left beside it.
func (c *Client) Resolve(ctx context.Context, path string) error {
	_, err := c.run(ctx, "resolve", "--accept", "working", path)
	return err
}
