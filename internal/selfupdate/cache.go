package selfupdate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// checkInterval is how long a successful check stands for. The prompt is a
	// convenience, so a day-old answer is good enough.
	checkInterval = 24 * time.Hour

	// failureBackoff keeps a rate-limited or offline host from asking again on
	// every launch. GitHub allows 60 unauthenticated calls an hour per address,
	// which a busy shell can burn through on its own.
	failureBackoff = 6 * time.Hour
)

// now is the clock, replaceable in tests.
var now = time.Now

// checkMemo records the last check so a launch soon after one costs nothing.
type checkMemo struct {
	CheckedAt time.Time `json:"checked_at"`
	Tag       string    `json:"tag,omitempty"`
	URL       string    `json:"url,omitempty"`
	Failed    bool      `json:"failed,omitempty"`
}

// CheckCached is Check with its last answer remembered at path, so repeated
// launches do not each cost a call to GitHub. A failure is remembered too and
// backs the next attempt off instead of retrying every time. An empty path
// turns the memo off, which makes every call a live check.
func CheckCached(ctx context.Context, b Build, path string) (Release, bool, error) {
	if !b.IsRelease() {
		return Release{}, false, nil
	}
	if memo, ok := readMemo(path); ok && !memo.stale() {
		return memo.answer(b)
	}

	rel, newer, err := Check(ctx, b)
	if err != nil {
		writeMemo(path, checkMemo{CheckedAt: now(), Failed: true})
		return Release{}, false, err
	}
	writeMemo(path, checkMemo{CheckedAt: now(), Tag: rel.Tag, URL: rel.URL})
	return rel, newer, nil
}

// stale reports whether the memo has aged out and the check should run again.
func (c checkMemo) stale() bool {
	age := now().Sub(c.CheckedAt)
	// A clock that moved backwards would otherwise pin the memo indefinitely.
	if age < 0 {
		return true
	}
	if c.Failed {
		return age >= failureBackoff
	}
	return age >= checkInterval
}

// answer replies from the memo alone. A remembered failure reports no update
// rather than an error: the startup check is silent by design, and --update
// bypasses the memo entirely.
func (c checkMemo) answer(b Build) (Release, bool, error) {
	if c.Failed || c.Tag == "" {
		return Release{}, false, nil
	}
	rel := Release{Tag: c.Tag, Version: strings.TrimPrefix(c.Tag, "v"), URL: c.URL}
	newer, err := isNewer(rel, b)
	return rel, newer, err
}

// readMemo reads the record of the last check. A missing, unreadable or corrupt
// file just means the check has to run.
func readMemo(path string) (checkMemo, bool) {
	if path == "" {
		return checkMemo{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return checkMemo{}, false
	}
	var memo checkMemo
	if err := json.Unmarshal(b, &memo); err != nil {
		return checkMemo{}, false
	}
	return memo, true
}

// writeMemo records a check. Failing to write costs one call on the next launch
// and nothing else, so there is nothing worth reporting.
func writeMemo(path string, memo checkMemo) {
	if path == "" {
		return
	}
	b, err := json.Marshal(memo)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o644)
}
