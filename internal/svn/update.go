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
	out, err := c.run(ctx, "update")
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
