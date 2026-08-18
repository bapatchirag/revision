package svn

import (
	"context"
	"strconv"
)

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

// DiffRevision returns the unified diff of what a single revision changed. It
// names no path, so the diff is scoped to the client's directory — the one the
// display scope roots it at — and the paths it reports are relative to that.
//
// `-c` is svn's own shorthand for the range ending at rev, which keeps the
// repository's first revision diffable: it has no predecessor to subtract one
// down to, and svn resolves the range against the empty tree instead.
func (c *Client) DiffRevision(ctx context.Context, rev string) (string, error) {
	out, err := c.run(ctx, diffRevisionArgs(rev)...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func diffRevisionArgs(rev string) []string {
	return []string{"diff", "-c", rev}
}

// DiffRevisions returns the unified diff between two revisions, scoped to the
// client's directory exactly as DiffRevision is.
//
// The range is literal: it compares the state at from with the state at to, so
// what from itself changed is already on the left-hand side and is not part of
// the result. The pair is ordered lowest first, so the diff reads forwards
// whichever way round the two were picked.
func (c *Client) DiffRevisions(ctx context.Context, from, to string) (string, error) {
	out, err := c.run(ctx, diffRevisionsArgs(from, to)...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// diffRevisionsArgs builds the range invocation, putting the lower revision
// first. A revision that is not a number is left where the caller put it, for
// svn to reject with its own message rather than be second-guessed here.
func diffRevisionsArgs(from, to string) []string {
	lo, hi := from, to
	a, errA := strconv.Atoi(from)
	b, errB := strconv.Atoi(to)
	if errA == nil && errB == nil && a > b {
		lo, hi = to, from
	}
	return []string{"diff", "-r", lo + ":" + hi}
}
