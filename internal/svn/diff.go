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

// DiffPaths returns the unified diff of local modifications across the given
// paths as a single document. It is how a set of files that need not share a
// directory — the members of a changelist — is diffed. An empty set diffs the
// whole working copy.
func (c *Client) DiffPaths(ctx context.Context, paths []string) (string, error) {
	out, err := c.run(ctx, append([]string{"diff"}, paths...)...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
