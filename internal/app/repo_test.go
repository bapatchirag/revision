package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

// checkoutTree builds a directory holding one working copy per name — a
// directory with a .svn inside — beside a plain directory that is not one, and
// returns its path.
func checkoutTree(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n, ".svn"), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "plain"), 0o755); err != nil {
		t.Fatalf("mkdir plain: %v", err)
	}
	return root
}

// scanRepos runs the walk with a context that is not going to expire mid-test.
func scanRepos(t *testing.T, roots ...string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), repoScanTimeout)
	defer cancel()
	return discoverRepos(ctx, roots)
}

// openRepoPrompt presses W and runs the scan it starts, so the option list is
// filled the way it is in a running session.
func openRepoPrompt(t *testing.T, m *Model) *Model {
	t.Helper()
	next, cmd := pressRune(t, m, 'W')
	if cmd == nil {
		t.Fatal("expected W to start a scan for working copies")
	}
	after, _ := next.Update(cmd())
	return after.(*Model)
}

func TestDiscoverReposFindsCheckoutsBeneathARoot(t *testing.T) {
	root := checkoutTree(t, "alpha", "beta", ".hidden")

	got := scanRepos(t, root)

	want := []string{filepath.Join(root, "alpha"), filepath.Join(root, "beta")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("repos = %v, want %v", got, want)
	}
}

func TestDiscoverReposIncludesTheRootItselfOnce(t *testing.T) {
	root := checkoutTree(t)
	if err := os.Mkdir(filepath.Join(root, ".svn"), 0o755); err != nil {
		t.Fatalf("mkdir .svn: %v", err)
	}

	// The same directory reached twice — as a scan root and as a sibling of the
	// current checkout — must be listed once.
	got := scanRepos(t, root, root+string(filepath.Separator), "/nowhere/at/all", "")

	if want := []string{root}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("repos = %v, want %v", got, want)
	}
}

func TestDiscoverReposReachesTrunkAndBranchCheckouts(t *testing.T) {
	// The layout svn encourages: the checkouts are the trunk and the branch, two
	// and three levels under the directory holding the project.
	root := checkoutTree(t, filepath.Join("project", "trunk"), filepath.Join("project", "branches", "wip"))

	got := scanRepos(t, root)

	// Nearest first: the trunk is a level above the branch, so it leads.
	want := []string{
		filepath.Join(root, "project", "trunk"),
		filepath.Join(root, "project", "branches", "wip"),
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("repos = %v, want %v", got, want)
	}
}

func TestDiscoverReposReadsARootThatIsItselfACheckout(t *testing.T) {
	// The normal launch: the current directory is the checkout revision is
	// reading, with other checkouts under it. Pruning at the root would leave
	// nothing from inside the current directory to offer.
	root := checkoutTree(t)
	if err := os.Mkdir(filepath.Join(root, ".svn"), 0o755); err != nil {
		t.Fatalf("mkdir .svn: %v", err)
	}
	nested := filepath.Join(root, "projects", "alpha")
	if err := os.MkdirAll(filepath.Join(nested, ".svn"), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", nested, err)
	}

	got := scanRepos(t, root)

	if want := []string{root, nested}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("repos = %v, want %v", got, want)
	}
}

func TestDiscoverReposStopsAtACheckout(t *testing.T) {
	root := checkoutTree(t, "alpha")
	// An external, or a checkout dropped inside another: everything below a
	// working copy belongs to it, so the walk must not go looking.
	if err := os.MkdirAll(filepath.Join(root, "alpha", "vendor", ".svn"), 0o755); err != nil {
		t.Fatalf("mkdir vendor: %v", err)
	}

	got := scanRepos(t, root)

	if want := []string{filepath.Join(root, "alpha")}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("repos = %v, want %v", got, want)
	}
}

func TestDiscoverReposCoversANearRootBeforeAFarOne(t *testing.T) {
	// A root high above the checkout can be slow — an automount point, a network
	// share — and the deadline is shared, so the near root has to be finished
	// before the far one is started or it could be starved of it.
	near := checkoutTree(t, "alpha")
	far := checkoutTree(t, "beta")

	got := scanRepos(t, near, far)

	want := []string{filepath.Join(near, "alpha"), filepath.Join(far, "beta")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("repos = %v, want the near root's checkouts first: %v", got, want)
	}
}

func TestDiscoverReposStopsAtTheDepthLimit(t *testing.T) {
	parts := make([]string, repoScanDepth+1)
	for i := range parts {
		parts[i] = fmt.Sprintf("level%d", i)
	}
	root := checkoutTree(t, filepath.Join(parts...))

	if got := scanRepos(t, root); len(got) != 0 {
		t.Fatalf("repos = %v, want nothing past the depth limit", got)
	}
}

func TestDiscoverReposStopsOnADeadline(t *testing.T) {
	root := checkoutTree(t, "alpha")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if got := discoverRepos(ctx, []string{root}); len(got) != 0 {
		t.Fatalf("repos = %v, want nothing once the deadline has passed", got)
	}
}

func TestRepoScanRootsClimbAboveTheCheckout(t *testing.T) {
	// revision only starts on a working copy, so the launch directory is nearly
	// always inside one. The checkout's root joins it, then its parents, so a
	// checkout beside the current tree is offered as readily as one inside it.
	m := &Model{
		info:    &svn.Info{WorkingCopyRoot: "/home/alice/work/team/wc"},
		workDir: "/home/alice/work/team/wc/projects",
	}
	want := "/home/alice/work/team/wc/projects,/home/alice/work/team/wc,/home/alice/work/team,/home/alice/work,/home/alice"
	if got := m.repoScanRoots(); strings.Join(got, ",") != want {
		t.Fatalf("roots = %v, want %s", got, want)
	}
}

func TestRepoScanRootsStopAtTheFilesystemRoot(t *testing.T) {
	m := &Model{info: &svn.Info{WorkingCopyRoot: "/wc"}}

	if want, got := "/wc,/", strings.Join(m.repoScanRoots(), ","); got != want {
		t.Fatalf("roots = %s, want %s", got, want)
	}
}

func TestMatchReposNarrowsAndCaps(t *testing.T) {
	repos := []string{"/work/Alpha", "/work/beta", "/other/alpha-two"}

	got, more := matchRepos(repos, " ALPHA ")
	if more {
		t.Errorf("did not expect the list to be capped: %v", got)
	}
	if want := "/work/Alpha,/other/alpha-two"; strings.Join(got, ",") != want {
		t.Errorf("matches = %v, want %s", got, want)
	}

	many := make([]string, 0, repoOptionLimit+2)
	for i := range repoOptionLimit + 2 {
		many = append(many, "/work/"+string(rune('a'+i)))
	}
	got, more = matchRepos(many, "")
	if !more || len(got) != repoOptionLimit {
		t.Errorf("matches = %d (capped %v), want %d and a cap", len(got), more, repoOptionLimit)
	}
}

func TestSwitchRepositoryListsDiscoveredWorkingCopies(t *testing.T) {
	root := checkoutTree(t, "alpha", "beta")
	// The prompt shows full paths and truncates them to fit, so the scan runs
	// from "." to keep the listed paths short enough to assert on.
	t.Chdir(root)
	m := sizedModel(t)
	m.workDir = "."

	m = openRepoPrompt(t, m)

	if !m.switchingRepo {
		t.Fatal("expected the switch-repository prompt to be open")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Switch repository", "alpha", "beta"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n---\n%s", want, view)
		}
	}
	if strings.Contains(view, "plain") {
		t.Errorf("expected a directory that is not a checkout to be left out\n---\n%s", view)
	}
}

func TestSwitchRepositoryListsCheckoutsUnderTheLaunchDirectory(t *testing.T) {
	root := checkoutTree(t)
	if err := os.Mkdir(filepath.Join(root, ".svn"), 0o755); err != nil {
		t.Fatalf("mkdir .svn: %v", err)
	}
	nested := filepath.Join(root, "projects", "alpha")
	if err := os.MkdirAll(filepath.Join(nested, ".svn"), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", nested, err)
	}

	m := sizedModel(t)
	// Launched inside the checkout it is reading, which is the normal case: a
	// checkout under the current directory still has to be offered.
	m.info = &svn.Info{WorkingCopyRoot: root}
	m.workDir = filepath.Join(root, "projects")

	m = openRepoPrompt(t, m)

	if !slices.Contains(m.repos, nested) {
		t.Fatalf("repos = %v, want the checkout under the launch directory %q", m.repos, nested)
	}
}

func TestSwitchRepositoryOpensBeforeTheScanLands(t *testing.T) {
	root := checkoutTree(t, "alpha")
	t.Chdir(root)
	m := sizedModel(t)
	m.workDir = "."

	next, cmd := pressRune(t, m, 'W')
	m = next

	if !m.switchingRepo {
		t.Fatal("expected the prompt to open without waiting for the scan")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "scanning") {
		t.Fatalf("expected the prompt to say the scan is still running, got:\n%s", view)
	}

	after, _ := m.Update(cmd())
	m = after.(*Model)

	view := stripANSI(m.View())
	if strings.Contains(view, "scanning") || !strings.Contains(view, "alpha") {
		t.Fatalf("expected the list to fill in once the scan lands, got:\n%s", view)
	}
}

func TestSupersededRepoScanIsDropped(t *testing.T) {
	m := sizedModel(t)
	m.openRepoSwitchAt("")
	m.repos = []string{"/home/alice/work/wc"}

	next, _ := m.Update(reposFoundMsg{repos: []string{"/stale/wc"}, gen: 99})
	m = next.(*Model)

	if slices.Contains(m.repos, "/stale/wc") {
		t.Fatalf("repos = %v, want a scan from an earlier prompt to be ignored", m.repos)
	}
}

func TestSwitchRepositoryNarrowsListWhileTyping(t *testing.T) {
	root := checkoutTree(t, "alpha", "beta")
	t.Chdir(root)
	m := sizedModel(t)
	m.workDir = "."
	m = openRepoPrompt(t, m)
	// One row per working copy, so narrowing the list shortens the box.
	before := strings.Count(m.repoEditor.View(), "\n")

	next, _ := pressRune(t, m, 'l')
	m = next

	if after := strings.Count(m.repoEditor.View(), "\n"); after != before-1 {
		t.Fatalf("the list did not narrow while typing: %d rows before, %d after", before, after)
	}
}

func TestSubmitRepoPathRefusesWhatIsNotADirectory(t *testing.T) {
	m := sizedModel(t)
	m.openRepoSwitchAt("")

	if cmd := m.submitRepoPath("  "); cmd != nil {
		t.Fatal("expected a blank path to be rejected")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "cannot be empty") {
		t.Fatalf("expected the blank-path warning, got:\n%s", view)
	}

	if cmd := m.submitRepoPath(filepath.Join(t.TempDir(), "missing")); cmd != nil {
		t.Fatal("expected a path that is not a directory to be rejected")
	}
	if !m.switchingRepo {
		t.Fatal("expected the prompt to stay open so the path can be corrected")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "not a directory") {
		t.Fatalf("expected the bad-path warning, got:\n%s", view)
	}
}

func TestSubmitRepoPathProbesADirectory(t *testing.T) {
	dir := t.TempDir()
	m := sizedModel(t)
	m.openRepoSwitchAt("")

	cmd := m.submitRepoPath(dir)

	if cmd == nil {
		t.Fatal("expected the directory to be probed before switching")
	}
	if m.switchingRepo {
		t.Fatal("expected the prompt to close while the probe runs")
	}
	if m.client.Dir == dir {
		t.Fatal("expected the session to stay put until svn confirms the working copy")
	}
}

func TestRepoSwitchFailureReopensTheRepositoryPrompt(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "kept.txt", State: svn.StateModified}})

	bad := svn.New("/home/alice/work/not-versioned")
	next, _ := m.Update(sourceChangedMsg{client: bad, from: fromRepoSwitch, err: errors.New("not a working copy")})
	m = next.(*Model)

	if m.client.Dir != "/home/alice/work/wc" {
		t.Fatalf("client dir = %q, want the original working copy", m.client.Dir)
	}
	if m.retargeting {
		t.Fatal("a rejected repository must not reopen the source-path prompt")
	}
	if !m.switchingRepo {
		t.Fatal("expected the repository prompt to reopen so the path can be corrected")
	}
	if got := m.repoEditor.Value(); got != "/home/alice/work/not-versioned" {
		t.Fatalf("prompt value = %q, want the rejected path", got)
	}
}

func TestRepoSwitchSuccessNamesTheRepository(t *testing.T) {
	m := sizedModel(t)

	client := svn.New("/home/alice/work/other")
	info := &svn.Info{URL: "https://svn.example.com/other/trunk", WorkingCopyRoot: "/home/alice/work/other", Revision: "7"}
	next, _ := m.Update(sourceChangedMsg{client: client, info: info, from: fromRepoSwitch})
	m = next.(*Model)

	if m.client.Dir != "/home/alice/work/other" {
		t.Fatalf("client dir = %q, want the new working copy", m.client.Dir)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "repository: /home/alice/work/other") {
		t.Fatalf("expected the switch to be confirmed by name, got:\n%s", view)
	}
}

func TestSwitchRepositoryEscapeClosesPrompt(t *testing.T) {
	m := sizedModel(t)
	next, _ := pressRune(t, m, 'W')
	m = next
	next2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next2.(*Model)
	next2, _ = m.Update(cmd())
	m = next2.(*Model)

	if m.switchingRepo {
		t.Fatal("expected esc to close the switch-repository prompt")
	}
}

func TestRepoSwitchPromptSubmitsThroughMessage(t *testing.T) {
	m := sizedModel(t)
	m.openRepoSwitchAt("")

	next, cmd := m.Update(uimsg.SubmitMsg{ID: repoSwitchID, Value: "  "})
	m = next.(*Model)

	if cmd != nil {
		t.Fatal("expected a blank path to be rejected")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "cannot be empty") {
		t.Fatalf("expected the blank-path warning, got:\n%s", view)
	}
}
