package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/component"
)

// repoSwitchID identifies the switch-repository prompt on emitted messages.
const repoSwitchID = "repository"

// repoOptionLimit caps how many working copies the prompt lists at once, so a
// directory full of checkouts cannot grow the overlay past the screen.
const repoOptionLimit = 8

// repoScanLimit caps how many working copies the scan collects. Typing narrows
// the list, so more than the prompt shows is worth holding — but not a directory
// full of them without end.
const repoScanLimit = 64

// repoScanDepth is how many levels below a scan root the walk descends. Its job
// is to terminate a symlink cycle, which the seen set cannot: a/link/link/…
// names a fresh path every time. It is deliberately far deeper than any layout
// needs, because the deadline is what actually decides how much gets covered.
const repoScanDepth = 8

// repoScanBudget caps how many directories the walk opens, bounding the memory
// its queue can take on a filesystem fast enough to outrun the deadline.
const repoScanBudget = 8192

// repoScanUp is how many levels above the checkout the scan also starts from, so
// a sibling of the current tree — or one under a sibling of its parent — is
// offered as well as what lies inside it.
const repoScanUp = 3

// repoScanTimeout bounds how long the scan runs. It is what limits the wait on a
// slow or network filesystem, where the budget would never be reached. The scan
// is off the UI goroutine, so this is the delay before the list fills in, not a
// freeze.
const repoScanTimeout = 3 * time.Second

// openRepoSwitch shows the prompt that re-points revision at another working
// copy and starts the scan behind it. The prompt opens straight away and the
// list fills in when the walk lands, so a slow filesystem delays the suggestions
// rather than the keystroke. The switch applies to the session only and is never
// persisted.
func (m *Model) openRepoSwitch() tea.Cmd {
	m.repos = nil
	m.scanningRepos = true
	m.openRepoSwitchAt("")
	ctx, gen := m.gens.repos.begin(repoScanTimeout)
	return scanReposCmd(ctx, m.repoScanRoots(), gen)
}

// applyRepos takes a scan's result. One that lands after its prompt has closed is
// still kept, so reopening shows it without walking again.
func (m *Model) applyRepos(msg reposFoundMsg) tea.Cmd {
	if m.gens.repos.stale(msg.gen) {
		return nil
	}
	m.scanningRepos = false
	m.repos = msg.repos
	if m.switchingRepo {
		m.repoEditor.SetTitle(m.repoPromptTitle())
		m.refreshRepoOptions()
	}
	return nil
}

// repoPromptTitle names the prompt, saying so while the scan behind it is still
// running, since the list stays empty until it lands.
func (m *Model) repoPromptTitle() string {
	if m.scanningRepos {
		return "Switch repository — scanning…"
	}
	return "Switch repository"
}

// openRepoSwitchAt shows the switch-repository prompt seeded with dir. It is also
// how a rejected path is handed back to the user, so a typo can be fixed in
// place rather than retyped; the working copies already found are kept.
func (m *Model) openRepoSwitchAt(dir string) {
	m.switchingRepo = true
	m.repoEditor.SetTitle(m.repoPromptTitle())
	m.repoEditor.Reset()
	m.repoEditor.SetValue(dir)
	m.refreshRepoOptions()
	m.repoEditor.Focus()
	m.sizeRepoSwitch()
}

// closeRepoSwitch hides the switch-repository prompt.
func (m *Model) closeRepoSwitch() {
	m.switchingRepo = false
	m.repoEditor.Blur()
}

// sizeRepoSwitch sizes the switch-repository prompt (only its width matters; the
// height follows the input and option rows). Like the source-path prompt it
// holds absolute paths, so it takes nearly the whole screen.
func (m *Model) sizeRepoSwitch() {
	m.repoEditor.SetSize(clamp(m.width-6, 40, 100), 0)
}

// refreshRepoOptions re-lists the discovered working copies, narrowed by what has
// been typed, so the list follows the input.
func (m *Model) refreshRepoOptions() {
	opts, more := matchRepos(m.repos, m.repoEditor.Value())
	head := "Working copies:"
	if more {
		head = "Working copies (type to narrow):"
	}
	m.repoEditor.SetOptions(head, opts)
}

// repoScanRoots are the directories searched for working copies: the directory
// revision was launched in, the root of the checkout it is reading, and that
// checkout's parents up to repoScanUp levels above it — so a checkout beside the
// current tree is offered as readily as one inside it. They are ordered nearest
// first, which is the order the walk covers them in and so the order the
// suggestions arrive in.
func (m *Model) repoScanRoots() []string {
	var roots []string
	if m.workDir != "" {
		roots = append(roots, m.workDir)
	}
	dir := m.sourceFloor()
	if dir == "" {
		return roots
	}
	roots = append(roots, dir)
	for range repoScanUp {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
		roots = append(roots, dir)
	}
	return roots
}

// discoverRepos finds the SVN working copies at and beneath roots. Each root is
// covered in full before the next is started, and the roots arrive nearest
// first, so a slow directory high above the checkout cannot starve the one the
// user is standing in — and the result comes back in that same order, which is
// what to show when only the head of the list fits. Within a root it walks
// breadth first, stopping at every working copy it reaches below the root:
// since svn 1.7 the root of a checkout is the only directory carrying a .svn, so
// everything under one belongs to it. The roots themselves are always read,
// since revision can only have been started inside a checkout. Hidden
// directories are skipped, the result is deduplicated and capped at
// repoScanLimit, and the walk gives up on its deadline, its open budget or the
// depth limit. A directory that cannot be read is passed over rather than
// reported: the list is a convenience, and a path can always be typed instead.
func discoverRepos(ctx context.Context, roots []string) []string {
	seen := map[string]bool{}
	var found []string
	opened := 0
	for _, root := range roots {
		if root == "" || len(found) >= repoScanLimit || ctx.Err() != nil {
			continue
		}
		var level []string
		queue := func(dir string) {
			if !seen[dir] {
				seen[dir] = true
				level = append(level, dir)
			}
		}
		queue(filepath.Clean(root))
		for depth := 0; depth <= repoScanDepth && len(level) > 0 && len(found) < repoScanLimit; depth++ {
			this := level
			level = nil
			for _, dir := range this {
				if len(found) >= repoScanLimit || ctx.Err() != nil {
					break
				}
				if isWorkingCopy(dir) {
					found = append(found, dir)
					// A root is where the user is standing, so it is read even though it
					// is a checkout — and it nearly always is one, since revision only
					// starts on a working copy. Anywhere else a checkout owns everything
					// below it.
					if depth > 0 {
						continue
					}
				}
				if depth == repoScanDepth || opened >= repoScanBudget {
					continue
				}
				opened++
				entries, err := os.ReadDir(dir)
				if err != nil {
					continue
				}
				for _, e := range entries {
					// Entries are not filtered by type: a symlink to a checkout reads as
					// one, and isWorkingCopy rules out everything that is not a directory.
					if !strings.HasPrefix(e.Name(), ".") {
						queue(filepath.Join(dir, e.Name()))
					}
				}
			}
		}
	}
	return found
}

// isWorkingCopy reports whether dir is the root of an SVN working copy, which
// since svn 1.7 is the single directory in a checkout holding a .svn directory.
func isWorkingCopy(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ".svn"))
	return err == nil && fi.IsDir()
}

// matchRepos narrows the discovered working copies to those whose path contains
// q, case-insensitively, capped at repoOptionLimit. It reports whether the list
// was cut short, which typing narrows.
func matchRepos(repos []string, q string) ([]string, bool) {
	q = strings.ToLower(strings.TrimSpace(q))
	var opts []string
	for _, r := range repos {
		if q != "" && !strings.Contains(strings.ToLower(r), q) {
			continue
		}
		if len(opts) == repoOptionLimit {
			return opts, true
		}
		opts = append(opts, r)
	}
	return opts, false
}

// submitRepoPath resolves the entered path and asks svn whether it is a working
// copy; the verdict arrives on sourceChangedMsg, so the session is only switched
// onto a directory known to be one. A path that is not a directory at all is
// refused here, without troubling svn. Either way the prompt stays open on a
// refusal, so the path can be corrected.
func (m *Model) submitRepoPath(value string) tea.Cmd {
	if m.client == nil {
		return nil
	}
	dir := strings.TrimSpace(value)
	if dir == "" {
		m.showToast("repository path cannot be empty", component.LevelWarning)
		return nil
	}
	dir = m.resolveSourcePath(dir)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		m.showToast("not a directory: "+dir, component.LevelWarning)
		return nil
	}
	m.closeRepoSwitch()
	if dir == m.client.Dir {
		m.showToast("already reading "+dir, component.LevelInfo)
		return nil
	}
	// Copy rather than build a fresh client so the command-log recorder and the
	// configured svn binary carry over to the candidate working copy.
	next := *m.client
	next.Dir = dir
	m.loading = true
	m.refreshChrome()
	return probeSourceCmd(&next, fromRepoSwitch)
}
