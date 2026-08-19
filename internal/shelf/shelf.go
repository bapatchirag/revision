// Package shelf stores sets of local changes lifted out of a Subversion working
// copy so they can be put back later. It is deliberately ignorant of both
// Subversion and the terminal UI: an entry is a directory of plain files, and
// what those files mean is the caller's business.
package shelf

import (
	"crypto/rand"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

// DirName is the store's directory, created at the working copy's root. The
// ".#" prefix is load-bearing: it matches the ".#*" glob in Subversion's
// built-in global-ignores, so the store stays out of svn status, svn diff and
// svn add without a property being set on the working copy or a line of
// configuration from the user.
const DirName = ".#revision-shelves"

// formatVersion is the on-disk layout an entry is written in. Scan passes over
// anything newer, so an older revision meets a store a newer one wrote by
// showing fewer entries rather than by misreading them.
const formatVersion = 1

// Names within an entry's directory. tmpPrefix marks a directory still being
// assembled by Save; Scan skips it, as it skips anything else dot-prefixed.
const (
	metaFile     = "meta.json"
	patchFile    = "changes.patch"
	untrackedDir = "untracked"
	tmpPrefix    = ".tmp-"
)

const (
	dirPerm  fs.FileMode = 0o755
	filePerm fs.FileMode = 0o644
)

// Dir returns the store directory for a working copy rooted at wcRoot.
func Dir(wcRoot string) string { return filepath.Join(wcRoot, DirName) }

// FileRec is one versioned file an entry captured. svn's patch format carries
// neither a file's status nor the changelist it belonged to, so both are
// recorded here for the restore to put back.
type FileRec struct {
	Path       string `json:"path"`
	State      string `json:"state"`
	Changelist string `json:"changelist,omitempty"`
}

// Entry is one shelved set of changes: the patch that reproduces them, the
// unversioned files that no patch could carry, and enough about where they came
// from to warn when they are being put back somewhere else.
type Entry struct {
	Version      int       `json:"version"`
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Created      time.Time `json:"created"`
	BaseRevision string    `json:"baseRevision,omitempty"`
	Files        []FileRec `json:"files,omitempty"`
	// Untracked lists the unversioned files copied into the entry verbatim, by
	// their path relative to the working copy's root. svn diff does not report an
	// unversioned file at all, so its bytes are the only way to carry it.
	Untracked []string `json:"untracked,omitempty"`
	// SkippedBinary lists files the capture left in the working copy because svn
	// diff will not express them ("Cannot display: file marked as a binary
	// type."). They are named so the caller can say what it did not take.
	SkippedBinary []string `json:"skippedBinary,omitempty"`
}

// Payload is an unversioned file to copy into an entry verbatim: Rel is where it
// sits relative to the working copy's root, Src the path its bytes come from.
type Payload struct {
	Rel string
	Src string
}

// NewID returns an identifier for an entry created at t. The timestamp leads so
// a directory listing reads in order, and the random tail keeps two entries
// shelved within the same second apart.
func NewID(t time.Time) string {
	return t.Format("20060102-150405") + "-" + strings.ToLower(rand.Text()[:6])
}

// validID rejects an identifier that would not stay inside the store. IDs reach
// Drop and ReadPatch from a caller's own state, but they end as filesystem
// paths, so they are checked rather than trusted.
func validID(id string) error {
	if id == "" {
		return fmt.Errorf("shelf: empty entry id")
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("shelf: invalid entry id %q", id)
	}
	return nil
}

// safeRel normalizes a payload's path and rejects one that would escape the
// entry it is being written into.
func safeRel(rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("shelf: empty payload path")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("shelf: payload path %q is absolute", rel)
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("shelf: payload path %q escapes the entry", rel)
	}
	return clean, nil
}
