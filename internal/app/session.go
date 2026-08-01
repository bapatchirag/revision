package app

import (
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bapatchirag/revision/internal/cache"
	"github.com/bapatchirag/revision/internal/svn"
)

// Diff cache bounds: enough to hold a large review's worth of patches, while
// keeping one enormous diff — or a long session — from growing the process
// without limit.
const (
	diffCacheEntries = 256
	diffCacheBytes   = 8 << 20 // 8 MiB
	// diffFailTTL is how long a failed diff is remembered, so a broken path is
	// not re-run on every keypress while a transient failure still clears itself.
	diffFailTTL = 10 * time.Second
)

// History cache bounds: a generous walk back through the log, and the changed
// paths of every revision looked at along the way.
const (
	logPageCacheEntries = 32
	logPageCacheBytes   = 4 << 20 // 4 MiB
	revDetailEntries    = 512
	revDetailBytes      = 4 << 20 // 4 MiB
)

// diffKey identifies a cached diff: the path it was produced for, and whether
// that path was a directory row — whose diff spans every change beneath it —
// rather than a single file.
type diffKey struct {
	path string
	dir  bool
}

// diffEntry is one cached diff. failed marks text as a load-failure notice
// rather than a patch (and expires bounds how long that failure is trusted), and
// stamp fingerprints the working-copy state the diff was produced from: an entry
// whose stamp no longer matches is stale and is never served.
type diffEntry struct {
	text    string
	failed  bool
	stamp   string
	expires time.Time
}

// logKey identifies a cached page of history: the revision it starts after
// (empty for the first page) and the page size it was fetched with, since a
// different size puts different revisions on every page.
type logKey struct {
	anchor string
	limit  int
}

// historyPage is one cached page of revisions, with whether a further page
// follows it.
type historyPage struct {
	entries []svn.LogEntry
	more    bool
}

// sessionStore holds what revision can reuse for the life of the process. It is
// in-memory only, so Close leaves nothing behind. It is owned by Model and only
// touched from Update, so it needs no locking of its own.
type sessionStore struct {
	diffs      *cache.LRU[diffKey, diffEntry]
	logPages   *cache.LRU[logKey, historyPage]
	revDetails *cache.LRU[string, []svn.ChangedPath]
}

// newSessionStore returns an empty store with the caches bounded.
func newSessionStore() *sessionStore {
	return &sessionStore{
		diffs: cache.New[diffKey, diffEntry](diffCacheEntries, diffCacheBytes, func(e diffEntry) int {
			return len(e.text) + len(e.stamp)
		}),
		logPages:   cache.New[logKey, historyPage](logPageCacheEntries, logPageCacheBytes, historyPageSize),
		revDetails: cache.New[string, []svn.ChangedPath](revDetailEntries, revDetailBytes, changedPathsSize),
	}
}

func historyPageSize(p historyPage) int {
	n := 0
	for _, e := range p.entries {
		n += len(e.Revision) + len(e.Author) + len(e.Message)
	}
	return n
}

func changedPathsSize(paths []svn.ChangedPath) int {
	n := 0
	for _, p := range paths {
		n += len(p.Action) + len(p.Path)
	}
	return n
}

// Diff returns the cached diff for k when it was produced from the working-copy
// state stamp describes and has not expired. A stale or expired entry is dropped
// as it is read, so it can never be served.
func (s *sessionStore) Diff(k diffKey, stamp string) (diffEntry, bool) {
	e, ok := s.diffs.Get(k)
	if !ok {
		return diffEntry{}, false
	}
	if e.stamp != stamp || (e.failed && time.Now().After(e.expires)) {
		s.diffs.Delete(k)
		return diffEntry{}, false
	}
	return e, true
}

// PutDiff records a loaded diff against the state it was produced from. A
// failure is only trusted for diffFailTTL.
func (s *sessionStore) PutDiff(k diffKey, e diffEntry) {
	if e.failed {
		e.expires = time.Now().Add(diffFailTTL)
	}
	s.diffs.Put(k, e)
}

// Reconcile drops every cached diff the working copy has moved out from under,
// keeping the rest. stamp re-derives the fingerprint a diff of that key would
// carry now; an entry survives only when it still matches.
func (s *sessionStore) Reconcile(stamp func(diffKey) string) {
	s.diffs.DeleteFunc(func(k diffKey, e diffEntry) bool { return e.stamp != stamp(k) })
}

// LogPage returns a page of history already fetched for k, so paging back over
// ground already covered costs no command.
func (s *sessionStore) LogPage(k logKey) (historyPage, bool) { return s.logPages.Get(k) }

// PutLogPage records a loaded page of history.
func (s *sessionStore) PutLogPage(k logKey, p historyPage) { s.logPages.Put(k, p) }

// PurgeLogPages forgets every cached page, for when the history itself has moved
// — a commit, an update, or an explicit refresh.
func (s *sessionStore) PurgeLogPages() { s.logPages.Purge() }

// RevDetail returns the changed paths already read for a revision. Revisions are
// immutable, so an entry here never goes stale.
func (s *sessionStore) RevDetail(rev string) ([]svn.ChangedPath, bool) { return s.revDetails.Get(rev) }

// PutRevDetail records the changed paths of one revision.
func (s *sessionStore) PutRevDetail(rev string, paths []svn.ChangedPath) {
	s.revDetails.Put(rev, paths)
}

// Purge empties every cache, so the next look at anything is answered by svn.
func (s *sessionStore) Purge() {
	s.diffs.Purge()
	s.logPages.Purge()
	s.revDetails.Purge()
}

// Close releases the session. Nothing is written to disk, so purging the caches
// is the whole of it; it is safe to call more than once.
func (s *sessionStore) Close() { s.Purge() }

// diffStampFor fingerprints the working-copy state a diff of k would be produced
// from: the working-copy revision, plus the status and on-disk size and
// modification time of the path — or of every item beneath it, for a directory
// row. Anything that could change the diff moves the stamp, so the cached entry
// is dropped rather than served. root is the directory the paths are relative
// to; an unreadable path stamps as absent, which is itself a change.
//
// Size and mtime are the same heuristic build tools use: an edit that preserves
// both, within the filesystem's timestamp resolution, is indistinguishable from
// no edit at all.
func diffStampFor(root, rev string, items []svn.StatusItem, k diffKey) string {
	h := fnv.New64a()
	writeStamp(h, rev)
	if !k.dir {
		writeItemStamp(h, root, k.path, findItem(items, k.path))
		return strconv.FormatUint(h.Sum64(), 16)
	}
	for i := range items {
		if !dirContains(k.path, items[i].Path) {
			continue
		}
		writeItemStamp(h, root, items[i].Path, &items[i])
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// writeItemStamp folds one path's status and on-disk state into the digest. A
// nil item means the path is no longer reported by svn status, which is a change
// in its own right.
func writeItemStamp(h io.Writer, root, path string, it *svn.StatusItem) {
	writeStamp(h, path)
	if it == nil {
		writeStamp(h, "absent")
	} else {
		writeStamp(h, string(it.State))
		writeStamp(h, it.Changelist)
	}
	if fi, err := os.Stat(filepath.Join(root, path)); err == nil {
		writeStamp(h, strconv.FormatInt(fi.Size(), 10))
		writeStamp(h, strconv.FormatInt(fi.ModTime().UnixNano(), 10))
	} else {
		writeStamp(h, "-")
	}
}

// writeStamp folds one field into the digest, delimited so neighbouring fields
// cannot run together into the same bytes.
func writeStamp(h io.Writer, s string) { _, _ = io.WriteString(h, s+"\x00") }

// findItem returns the status item for path, or nil when svn status no longer
// reports it.
func findItem(items []svn.StatusItem, path string) *svn.StatusItem {
	for i := range items {
		if items[i].Path == path {
			return &items[i]
		}
	}
	return nil
}

// dirContains reports whether the directory row dir holds path: the synthetic
// "/" root holds the whole working copy, any other row only what lies beneath it.
func dirContains(dir, path string) bool {
	return dir == fileTreeRoot || strings.HasPrefix(path, dir+"/")
}
