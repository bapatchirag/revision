package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
)

func TestBatchOutcomeErrNamesTheFirstAndCountsTheRest(t *testing.T) {
	var none batchOutcome
	none.ok("a.txt")
	if err := none.err(); err != nil {
		t.Errorf("err() = %v, want nil when everything landed", err)
	}

	var one batchOutcome
	one.add("a.txt", errors.New("refused"))
	if got := one.err(); got == nil || got.Error() != "a.txt: refused" {
		t.Errorf("err() = %v, want the sole refusal", got)
	}

	var many batchOutcome
	for _, p := range []string{"a.txt", "b.txt", "c.txt"} {
		many.add(p, errors.New("refused"))
	}
	if got := many.err(); got == nil || got.Error() != "a.txt: refused (and 2 more)" {
		t.Errorf("err() = %v, want the first named and the rest counted", got)
	}

	wrapped := errors.New("refused")
	if !errors.Is(batchFailure{path: "a.txt", err: wrapped}, wrapped) {
		t.Error("a batchFailure has to unwrap to svn's own error")
	}
}

// TestBatchOutcomeToastTellsPartFromWhole pins the three things a fan-out can
// come to. A run where some files landed is the one that matters: it is a
// warning rather than a failure, because the working copy has moved.
func TestBatchOutcomeToastTellsPartFromWhole(t *testing.T) {
	var whole batchOutcome
	whole.ok("a.txt")
	whole.ok("b.txt")
	text, level := whole.toast("revert", "reverted")
	if text != "reverted 2 files" || level != component.LevelSuccess {
		t.Errorf("toast = %q/%v, want a plain success", text, level)
	}

	var part batchOutcome
	part.ok("a.txt")
	part.add("b.txt", errors.New("refused"))
	text, level = part.toast("revert", "reverted")
	if level != component.LevelWarning {
		t.Errorf("level = %v, want a warning when only part of it landed", level)
	}
	if !strings.Contains(text, "reverted 1 file") || !strings.Contains(text, "b.txt") {
		t.Errorf("toast = %q, want both what landed and what did not", text)
	}

	var nothing batchOutcome
	nothing.add("a.txt", errors.New("refused"))
	text, level = nothing.toast("revert", "reverted")
	if level != component.LevelError || !strings.HasPrefix(text, "revert failed") {
		t.Errorf("toast = %q/%v, want a plain failure", text, level)
	}
}

// TestBatchToastCollapsesAnActionableFailure pins that the hint survives the
// fan-out wrapping: a locked working copy is the same advice however many files
// were asked for.
func TestBatchToastCollapsesAnActionableFailure(t *testing.T) {
	var out batchOutcome
	out.add("a.txt", errors.New("svn: E155004: Working copy locked."))
	text, _ := out.toast("revert", "reverted")
	if !strings.Contains(text, svn.LockHint) {
		t.Errorf("toast = %q, want the cleanup hint instead of svn's dump", text)
	}
}

func TestSingleOutcomeReadsLikeAFanOutOfOne(t *testing.T) {
	if got := singleOutcome("a.txt", nil); got.err() != nil || got.label() != "a.txt" {
		t.Errorf("singleOutcome(ok) = %+v, want the path recorded as done", got)
	}
	if got := singleOutcome("a.txt", errors.New("refused")); got.err() == nil || len(got.done) != 0 {
		t.Errorf("singleOutcome(err) = %+v, want the path recorded as refused", got)
	}
}

// TestRevertOutcomeCountsASkipAsARefusal pins the quiet failure: svn announces a
// path it passed over and exits zero, so counting it as reverted is how a revert
// that discarded nothing reads as one that worked.
func TestRevertOutcomeCountsASkipAsARefusal(t *testing.T) {
	got := revertOutcome(svn.RevertResult{
		Reverted: []string{"a.txt"},
		Skipped:  []string{"gone.txt"},
		Failed:   []svn.PathError{{Path: "bad.txt", Err: errors.New("refused")}},
	})
	if len(got.done) != 1 || got.done[0] != "a.txt" {
		t.Errorf("done = %v, want only what svn reverted", got.done)
	}
	if len(got.failed) != 2 {
		t.Errorf("failed = %+v, want the skipped path counted alongside the refused one", got.failed)
	}
}

// TestBatchTimeoutGrowsWithTheSet pins why the budget is not flat: exec kills
// svn when the deadline expires, and an svn killed mid-operation leaves the
// working copy locked for everything after it.
func TestBatchTimeoutGrowsWithTheSet(t *testing.T) {
	one, many := batchTimeout(1), batchTimeout(500)
	if one < batchTimeoutBase {
		t.Errorf("batchTimeout(1) = %v, want at least the base budget", one)
	}
	if many <= one {
		t.Errorf("batchTimeout(500) = %v, want more than the %v a single file gets", many, one)
	}
	if got := batchTimeout(1_000_000); got != batchTimeoutMax {
		t.Errorf("batchTimeout(huge) = %v, want the cap %v", got, batchTimeoutMax)
	}
}
