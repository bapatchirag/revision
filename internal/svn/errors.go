package svn

import (
	"fmt"
	"strings"
)

// AuthHint is a short, actionable message for an svn authentication failure.
// Because revision always runs svn with --non-interactive (so it never blocks on
// a hidden credential prompt), a command that needs credentials fails outright;
// this tells the user how to recover.
const AuthHint = "authentication required — cache SVN credentials, then retry"

// LockHint is a short, actionable message for a working copy svn will not touch
// because a lock is still on it. The lock is nearly always the residue of an
// earlier operation that did not finish — one revision itself abandoned when it
// ran past its deadline, since a killed svn has no chance to release it.
const LockHint = "working copy locked — run `svn cleanup`, then retry"

// authSignatures are lower-cased substrings that mark an svn failure as an
// authentication or authorization problem. Kept specific so a plain network or
// path error is not misreported as an auth failure.
var authSignatures = []string{
	"authentication failed",
	"authorization failed",
	"no more credentials",
	"interactive prompting is disabled",
	"username or password",
	"e170001", // authorization failed
	"e215004", // no more credentials / auth failed in a non-interactive context
}

// lockSignatures are lower-cased substrings that mark an svn failure as a
// working copy still under lock.
var lockSignatures = []string{
	"e155004", // working copy locked
	"run 'svn cleanup'",
	"is already locked",
}

// IsAuthError reports whether err looks like an svn authentication or
// authorization failure. revision runs svn --non-interactive, so a command that
// needs credentials fails instead of prompting; callers use this to surface an
// actionable hint (see AuthHint) rather than a raw svn error dump.
func IsAuthError(err error) bool { return matches(err, authSignatures) }

// IsLockedError reports whether err is svn refusing to work in a locked working
// copy (see LockHint).
func IsLockedError(err error) bool { return matches(err, lockSignatures) }

// BlocksEveryPath reports whether a failure is about the working copy rather
// than the path it happened to be reported for, so every other path would fail
// the same way. It is what lets a fan-out stop early instead of running the same
// doomed command once per file.
func BlocksEveryPath(err error) bool { return IsAuthError(err) || IsLockedError(err) }

// Hint returns the short, actionable message for a failure revision recognises,
// in place of svn's own multi-line dump. It reports false for everything else,
// whose own text is the best account there is.
func Hint(err error) (string, bool) {
	switch {
	case IsAuthError(err):
		return AuthHint, true
	case IsLockedError(err):
		return LockHint, true
	default:
		return "", false
	}
}

// matches reports whether err's text holds any of the given lower-cased marks.
func matches(err error, signatures []string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, sig := range signatures {
		if strings.Contains(msg, sig) {
			return true
		}
	}
	return false
}

// PathError is one path an operation was refused for, and svn's reason for
// refusing it. It is what lets an operation over a set of paths report which of
// them it could not carry out, rather than only that it did not finish.
type PathError struct {
	Path string
	Err  error
}

func (e PathError) Error() string { return e.Path + ": " + e.Err.Error() }

func (e PathError) Unwrap() error { return e.Err }

// collapse folds the refusals from one operation into a single error for a
// caller that can only carry one, naming the first and counting the rest. It is
// nil when nothing was refused.
func collapse(failed []PathError) error {
	switch len(failed) {
	case 0:
		return nil
	case 1:
		return failed[0]
	default:
		return fmt.Errorf("%s (and %d more)", failed[0], len(failed)-1)
	}
}
