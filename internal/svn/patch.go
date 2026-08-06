package svn

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// PatchResult summarizes what `svn patch` did with a patch file's targets — or,
// on a dry run, what it would do. Applied names the paths it changed cleanly,
// Conflicted those it could only leave in conflict (with their rejected hunks
// written out beside them), and Skipped those it could not find at all.
type PatchResult struct {
	Applied    []string
	Conflicted []string
	Skipped    []string
}

// Targets is the number of paths svn recognised in the patch, however each one
// fared. Zero means the file held nothing svn could read as a patch.
func (r PatchResult) Targets() int {
	return len(r.Applied) + len(r.Conflicted) + len(r.Skipped)
}

// Patch applies a patch file to the working copy (svn patch <file>). A dry run
// reports what the patch would do without changing a single file, so it can be
// tried out before it is let loose on the working copy.
func (c *Client) Patch(ctx context.Context, file string, dryRun bool) (PatchResult, error) {
	args := []string{"patch"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	out, err := c.run(ctx, append(args, file)...)
	if err != nil {
		return PatchResult{}, err
	}
	return parsePatchOutput(string(out)), nil
}

// patchStatusChars are the columns `svn patch` prints in front of a target it
// touched — content status then property status — either of which may be blank.
const patchStatusChars = " UGCAD"

// parsePatchOutput reads the per-target lines `svn patch` prints. A target it
// touched is announced by its two status columns and a path ("U         a.txt"),
// one it could not find by a "Skipped" line naming it in quotes. Everything else
// — the per-hunk notes and the closing conflict summary — is noise here.
func parsePatchOutput(out string) PatchResult {
	var res PatchResult
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if target, ok := skippedTarget(line); ok {
			res.Skipped = append(res.Skipped, target)
			continue
		}
		target, conflicted, ok := patchedTarget(line)
		switch {
		case !ok:
		case conflicted:
			res.Conflicted = append(res.Conflicted, target)
		default:
			res.Applied = append(res.Applied, target)
		}
	}
	return res
}

// skippedTarget pulls the path out of the line svn prints for a target it could
// not find ("Skipped missing target: 'a.txt'"), which always names it in quotes.
func skippedTarget(line string) (string, bool) {
	if !strings.HasPrefix(line, "Skipped") {
		return "", false
	}
	first, last := strings.Index(line, "'"), strings.LastIndex(line, "'")
	if first < 0 || last <= first {
		return "", false
	}
	return line[first+1 : last], true
}

// patchedTarget reads a target line: two status columns, then the path. A 'C' in
// either column means svn could not merge that side and left the target in
// conflict.
func patchedTarget(line string) (target string, conflicted, ok bool) {
	if len(line) < 4 || line[2] != ' ' {
		return "", false, false
	}
	content, props := line[0], line[1]
	if !isPatchStatus(content) || !isPatchStatus(props) || (content == ' ' && props == ' ') {
		return "", false, false
	}
	if target = strings.TrimSpace(line[2:]); target == "" {
		return "", false, false
	}
	return target, content == 'C' || props == 'C', true
}

func isPatchStatus(c byte) bool { return strings.IndexByte(patchStatusChars, c) >= 0 }

// PatchBelongsTo reports whether a patch was taken from dir, by looking for the
// files it expects to already be there. Every path in a patch is relative to the
// directory it was produced in, so one produced elsewhere resolves to nothing
// here — and svn, rather than say so, creates each missing target and rejects
// the patch's hunks into it. It answers false only when the patch names files it
// does not itself create and dir holds none of them. A patch that only adds
// files names no such target, cannot be placed this way, and is accepted.
func PatchBelongsTo(patch, dir string) bool {
	targets := patchTargets(patch)
	if len(targets) == 0 {
		return true
	}
	for _, t := range targets {
		if _, err := os.Stat(filepath.Join(dir, t)); err == nil {
			return true
		}
	}
	return false
}

// patchTargets lists the paths a patch expects to find already in place: the
// left-hand side of each file header ("--- a.txt\t(revision 1)"), minus the ones
// naming a file the patch creates, which svn writes as "(nonexistent)" and other
// diff formats as /dev/null.
func patchTargets(patch string) []string {
	var targets []string
	for _, line := range strings.Split(patch, "\n") {
		after, ok := strings.CutPrefix(line, "--- ")
		if !ok {
			continue
		}
		path, rev, _ := strings.Cut(after, "\t")
		path = strings.TrimSpace(path)
		if path == "" || path == "/dev/null" || strings.TrimSpace(rev) == "(nonexistent)" {
			continue
		}
		targets = append(targets, path)
	}
	return targets
}
