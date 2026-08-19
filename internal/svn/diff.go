package svn

import (
	"context"
	"strconv"
	"strings"
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

// DiffPathsForPatching returns the same diff as DiffPaths, but one that svn
// patch can put back in full.
//
// A copied or moved file is identical to its source, so svn reports it as a
// header with no hunks under it. Read back as a patch that says to delete the
// original and create nothing in its place, which loses the file. Asking for
// copies as adds spells the content out instead, at the cost of recording the
// move as a delete and an add rather than as a copy.
func (c *Client) DiffPathsForPatching(ctx context.Context, paths []string) (string, error) {
	out, err := c.run(ctx, append([]string{"diff", "--show-copies-as-adds"}, paths...)...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// binaryNotice is what svn prints in place of the hunks of a file it will not
// express as text.
const binaryNotice = "Cannot display: file marked as a binary type."

// BinarySkips lists the files a diff names but carries no content for. svn will
// not render a file it considers binary, so a patch holding one puts every other
// file back and passes over that one in silence; naming them is what lets a
// caller say what it could not take.
func BinarySkips(patch string) []string {
	var skips []string
	var current string
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimRight(line, "\r")
		if path, ok := strings.CutPrefix(line, "Index: "); ok {
			current = strings.TrimSpace(path)
			continue
		}
		if current != "" && strings.HasPrefix(line, binaryNotice) {
			skips = append(skips, current)
			current = ""
		}
	}
	return skips
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
