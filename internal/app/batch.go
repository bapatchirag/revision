package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
)

// The budget a fan-out runs under grows with the set it was given, from
// batchTimeoutBase for a single file up to batchTimeoutMax. A flat deadline is
// worse than a generous one here: exec kills svn outright when the deadline
// expires, and an svn killed mid-operation leaves the working copy locked, so
// every command after it fails too until the user runs `svn cleanup`.
const (
	batchTimeoutBase = 30 * time.Second
	batchTimeoutPer  = 2 * time.Second
	batchTimeoutMax  = 10 * time.Minute
)

// batchTimeout is how long an action over n paths may run before it is
// abandoned.
func batchTimeout(n int) time.Duration {
	d := batchTimeoutBase + time.Duration(n)*batchTimeoutPer
	if d > batchTimeoutMax {
		return batchTimeoutMax
	}
	return d
}

// batchFailure is one path a fan-out could not act on, and why.
type batchFailure struct {
	path string
	err  error
}

// batchOutcome is what a fan-out over several paths actually managed. Every
// action is attempted, so one path svn refuses never decides the fate of the
// rest: done names what landed, failed names what did not and carries the reason
// each was refused.
type batchOutcome struct {
	done   []string
	failed []batchFailure
}

// ok records a path the action landed on.
func (b *batchOutcome) ok(path string) { b.done = append(b.done, path) }

// fail records a path the action was refused for, and reports whether err was
// non-nil — so a caller can write `if o.add(p, err) { continue }` around the
// work that only makes sense once the path is dealt with.
func (b *batchOutcome) fail(path string, err error) bool {
	if err == nil {
		return false
	}
	b.failed = append(b.failed, batchFailure{path: path, err: err})
	return true
}

// add records one path's result either way, reporting whether it failed.
func (b *batchOutcome) add(path string, err error) bool {
	if b.fail(path, err) {
		return true
	}
	b.ok(path)
	return false
}

// singleOutcome is the outcome of an action on one path, so a command acting on
// a single file reports itself the same way a fan-out does.
func singleOutcome(path string, err error) batchOutcome {
	var out batchOutcome
	out.add(path, err)
	return out
}

// revertOutcome reads a revert's per-path result as a fan-out outcome. A path
// svn passed over counts against the revert as much as one it refused: svn says
// so and exits zero, and taking that for success is how a revert that discarded
// nothing reads as one that worked.
func revertOutcome(res svn.RevertResult) batchOutcome {
	out := batchOutcome{done: res.Reverted}
	for _, p := range res.Skipped {
		out.fail(p, errSkipped)
	}
	for _, f := range res.Failed {
		out.fail(f.Path, f.Err)
	}
	return out
}

// errSkipped is why a path svn passed over was not reverted.
var errSkipped = errors.New("svn passed over it — nothing here it tracks")

// err collapses the failures into one error for the callers that can only carry
// one, naming the first path and saying how many others went with it. It is nil
// when every path landed, so an outcome doubles as an error value.
func (b batchOutcome) err() error {
	switch len(b.failed) {
	case 0:
		return nil
	case 1:
		return b.failed[0]
	default:
		return fmt.Errorf("%s (and %d more)", b.failed[0], len(b.failed)-1)
	}
}

func (f batchFailure) Error() string { return f.path + ": " + f.err.Error() }

func (f batchFailure) Unwrap() error { return f.err }

// toast describes a finished fan-out: what landed on how many files, and what
// it was refused for. action names the deed for a failure ("revert"), done names
// it for a success ("reverted"). A run where nothing landed is a plain failure;
// one where some did is a warning naming what is left, since the working copy
// has moved and the rest has not.
func (b batchOutcome) toast(action, done string) (string, component.Level) {
	switch {
	case len(b.failed) == 0:
		return done + " " + b.label(), component.LevelSuccess
	case len(b.done) == 0:
		return failureText(action, b.err()), component.LevelError
	default:
		return done + " " + fileCount(len(b.done)) + ", " + fileCount(len(b.failed)) +
			" refused — " + b.err().Error(), component.LevelWarning
	}
}

// label names what a fan-out landed on: the sole path when it touched one file,
// otherwise a count.
func (b batchOutcome) label() string {
	if len(b.done) == 1 {
		return b.done[0]
	}
	return fileCount(len(b.done))
}
