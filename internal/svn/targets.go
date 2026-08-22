package svn

import (
	"context"
	"os"
	"strings"
)

// runPaths runs one invocation naming every path, falling back to one
// invocation per path when that comes back non-zero.
//
// The fallback is not belt and braces: svn walks its targets in order and
// abandons the run at the first one it refuses, so the targets before it are
// done and the ones after are never looked at. Without a second pass a single
// bad path would decide how far down the set the action got — which is the shape
// of failure revision spent a release removing.
//
// Re-running over paths the first pass already dealt with is safe for every
// caller here: assigning a changelist a path is already in, removing one it is
// not in, and `svn add --force` over something already versioned all do nothing
// and exit zero.
//
// args builds the argv around a trailing target list.
func (c *Client) runPaths(ctx context.Context, paths []string, args func(tail ...string) []string) []PathError {
	if len(paths) == 0 {
		return nil
	}
	if len(paths) == 1 || !targetable(paths) {
		return c.runEachPath(ctx, paths, args)
	}
	if err := c.runTargets(ctx, paths, args); err == nil {
		return nil
	}
	return c.runEachPath(ctx, paths, args)
}

// runTargets names the whole set through svn's --targets, a file of one path per
// line. It is what lets a single invocation carry more paths than a command line
// can hold, and it keeps the command log to one readable entry rather than a
// screenful of arguments. The file is removed however the run ends.
func (c *Client) runTargets(ctx context.Context, paths []string, args func(tail ...string) []string) error {
	f, err := os.CreateTemp("", "revision-targets-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	_, err = f.WriteString(strings.Join(paths, "\n") + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	_, err = c.run(ctx, args("--targets", name)...)
	return err
}

// runEachPath runs one invocation per path, so each is judged on its own and
// carries back its own reason for being refused.
func (c *Client) runEachPath(ctx context.Context, paths []string, args func(tail ...string) []string) []PathError {
	var failed []PathError
	for _, p := range paths {
		if _, err := c.run(ctx, args(p)...); err != nil {
			failed = append(failed, PathError{Path: p, Err: err})
		}
	}
	return failed
}

// targetable reports whether paths can be named in a --targets file, which
// separates them by newline and offers no way to escape one. A path holding a
// newline would be read as two, so such a set is run a path at a time instead,
// where the path is one argument and cannot be split.
func targetable(paths []string) bool {
	for _, p := range paths {
		if strings.ContainsAny(p, "\n\r") {
			return false
		}
	}
	return true
}
