package app

import (
	"sort"
	"strings"

	"github.com/bapatchirag/revision/internal/svn"
)

// statusScopeMax is the largest number of subtrees a targeted status read is
// taken over. Past it the read stops being the bargain it was: svn crawls each
// target, so a set spanning most of the working copy costs what reading all of
// it would, and the paths ride on the command line rather than in a targets file
// (`svn status` takes no --targets), which cannot grow without bound.
const statusScopeMax = 64

// statusScope reduces the paths an action changed to the subtrees a status read
// has to cover to learn what became of them: the highest ancestor of each, since
// `svn status` recurses and reading a directory covers everything beneath it.
//
// It returns nothing when the set is too large to be worth targeting, which asks
// the caller for a full read instead.
func statusScope(paths []string, items []svn.StatusItem) []string {
	if len(paths) == 0 {
		return nil
	}
	want := make(map[string]bool, len(paths))
	for _, p := range paths {
		want[p] = true
	}
	// Reverting half of a move rewrites the other half: svn drops the link from
	// the row left behind, which a read covering only the named paths never sees.
	for i := range items {
		if !want[items[i].Path] {
			continue
		}
		for _, partner := range []string{items[i].MovedFrom, items[i].MovedTo} {
			if partner != "" {
				want[partner] = true
			}
		}
	}
	all := make([]string, 0, len(want))
	for p := range want {
		all = append(all, p)
	}
	covers := svn.CoverPaths(all)
	if len(covers) > statusScopeMax {
		return nil
	}
	scope := make([]string, 0, len(covers))
	for _, cv := range covers {
		scope = append(scope, cv.Lead)
	}
	// Sorted so one set of paths always produces the same command line, whatever
	// order the map handed them back in.
	sort.Strings(scope)
	return scope
}

// scopeCovers reports whether path lies in scope: it is one of the paths itself
// or beneath one of them.
func scopeCovers(scope []string, path string) bool {
	for _, s := range scope {
		if path == s || strings.HasPrefix(path, s+"/") {
			return true
		}
	}
	return false
}

// spliceStatus puts the entries a targeted read returned in place of the ones
// the status on screen holds for the same scope, leaving the rest of it alone.
// A path in scope that fresh says nothing about is dropped: svn reports nothing
// for a file that has come clean or gone away, which is how a row is learned to
// be over.
//
// Both inputs are ordered by path, as every status read produces them, and the
// result keeps that order.
func spliceStatus(items []svn.StatusItem, scope []string, fresh []svn.StatusItem) []svn.StatusItem {
	out := make([]svn.StatusItem, 0, len(items)+len(fresh))
	next := 0
	for i := range items {
		if scopeCovers(scope, items[i].Path) {
			continue
		}
		for next < len(fresh) && fresh[next].Path < items[i].Path {
			out = append(out, fresh[next])
			next++
		}
		out = append(out, items[i])
	}
	return append(out, fresh[next:]...)
}
