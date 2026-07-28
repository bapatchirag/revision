package svn

import (
	"context"
	"regexp"
)

// updatedRevisionRE matches the revision line svn prints when an update
// finishes, e.g. "Updated to revision 42." or, when already current,
// "At revision 42.".
var updatedRevisionRE = regexp.MustCompile(`(?m)^(?:Updated to|At) revision (\d+)\.`)

// Update brings the working copy up to date with the repository (svn update)
// and returns the revision it is now at, parsed from svn's output (empty when
// none was reported).
func (c *Client) Update(ctx context.Context) (string, error) {
	return c.update(ctx, "update")
}

// UpdateToRevision moves the working copy to a specific revision (svn update -r
// <rev>), which may take it backwards or forwards in history. It returns the
// revision the working copy is now at, parsed from svn's output.
func (c *Client) UpdateToRevision(ctx context.Context, rev string) (string, error) {
	return c.update(ctx, "update", "-r", rev)
}

// update runs an svn update variant and returns the revision it reports.
func (c *Client) update(ctx context.Context, args ...string) (string, error) {
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	return parseUpdatedRevision(string(out)), nil
}

// parseUpdatedRevision extracts the revision number from svn update output.
func parseUpdatedRevision(out string) string {
	if m := updatedRevisionRE.FindStringSubmatch(out); len(m) == 2 {
		return m[1]
	}
	return ""
}
