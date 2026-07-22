package svn

import "context"

// Diff returns the unified diff of local modifications for the given path,
// relative to the working copy. An empty path diffs the entire working copy.
func (c *Client) Diff(ctx context.Context, path string) (string, error) {
	args := []string{"diff"}
	if path != "" {
		args = append(args, path)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
