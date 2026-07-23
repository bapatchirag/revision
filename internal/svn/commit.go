package svn

import (
	"context"
	"regexp"
)

// committedRevisionRE matches the "Committed revision N." line svn prints on a
// successful commit.
var committedRevisionRE = regexp.MustCompile(`(?m)^Committed revision (\d+)\.`)

// Commit commits pending changes with the given log message and returns the new
// revision number parsed from svn's output (empty when none was reported). When
// changelist is non-empty only the members of that changelist are committed —
// SVN's changelist filter — which is how revision commits just the staged set.
func (c *Client) Commit(ctx context.Context, message, changelist string) (string, error) {
	args := []string{"commit", "-m", message}
	if changelist != "" {
		args = append(args, "--changelist", changelist)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	return parseCommittedRevision(string(out)), nil
}

// parseCommittedRevision extracts the revision number from svn commit output.
func parseCommittedRevision(out string) string {
	if m := committedRevisionRE.FindStringSubmatch(out); len(m) == 2 {
		return m[1]
	}
	return ""
}
