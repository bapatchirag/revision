package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/config"
	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/svn"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

// sourceTree builds a directory holding the given subdirectory names plus a
// regular file, and returns its path.
func sourceTree(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, n := range names {
		if err := os.Mkdir(filepath.Join(root, n), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return root
}

// modelAtRoot builds a sized model whose working copy — and so the floor the
// source path may not go above — is root.
func modelAtRoot(t *testing.T, root string) *Model {
	t.Helper()
	info := &svn.Info{URL: "https://svn.example.com/repo/trunk", WorkingCopyRoot: root, Revision: "42"}
	m := New(svn.New(root), info, selfupdate.Build{}, config.Default())
	m.workDir = root
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(*Model)
}

func TestSourcePathOptionsListsDirectories(t *testing.T) {
	root := sourceTree(t, "alpha", "beta", ".svn")

	opts, more := sourcePathOptions(root+string(filepath.Separator), "")
	if more {
		t.Fatalf("did not expect the list to be capped: %v", opts)
	}
	want := []string{filepath.Dir(root), filepath.Join(root, "alpha"), filepath.Join(root, "beta")}
	if strings.Join(opts, ",") != strings.Join(want, ",") {
		t.Fatalf("options = %v, want %v", opts, want)
	}
}

func TestSourcePathOptionsWithholdsParentAtFloor(t *testing.T) {
	root := sourceTree(t, "alpha")

	opts, _ := sourcePathOptions(root+string(filepath.Separator), root)
	want := []string{filepath.Join(root, "alpha")}
	if strings.Join(opts, ",") != strings.Join(want, ",") {
		t.Fatalf("options = %v, want %v", opts, want)
	}
}

func TestSourcePathOptionsNarrowsByPrefix(t *testing.T) {
	root := sourceTree(t, "alpha", "beta")

	opts, _ := sourcePathOptions(filepath.Join(root, "al"), root)
	want := []string{filepath.Join(root, "alpha")}
	if strings.Join(opts, ",") != strings.Join(want, ",") {
		t.Fatalf("options = %v, want %v", opts, want)
	}
}

func TestSourcePathOptionsRevealsHiddenOnDot(t *testing.T) {
	root := sourceTree(t, "alpha", ".svn")

	opts, _ := sourcePathOptions(root+string(filepath.Separator)+".", root)
	want := []string{filepath.Join(root, ".svn")}
	if strings.Join(opts, ",") != strings.Join(want, ",") {
		t.Fatalf("options = %v, want %v", opts, want)
	}
}

func TestSourcePathOptionsCapsLongListing(t *testing.T) {
	names := make([]string, 0, sourcePathOptionLimit+2)
	for i := range sourcePathOptionLimit + 2 {
		names = append(names, string(rune('a'+i))+"dir")
	}
	root := sourceTree(t, names...)

	opts, more := sourcePathOptions(root+string(filepath.Separator), root)
	if !more {
		t.Fatal("expected the listing to report that it was capped")
	}
	if len(opts) != sourcePathOptionLimit {
		t.Fatalf("options = %d, want %d", len(opts), sourcePathOptionLimit)
	}
}

func TestChangeSourcePathOpensOnCurrentSource(t *testing.T) {
	m := sizedModel(t)
	m.client = svn.New("/home/alice/work/wc/internal/app")
	// The launch directory is deliberately elsewhere: the prompt follows the
	// source, not where revision was started.
	m.workDir = "/home/alice/elsewhere"

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	m = next.(*Model)

	if !m.retargeting {
		t.Fatal("expected the source-path prompt to be open")
	}
	if want := "/home/alice/work/wc/internal/app/"; m.pathEditor.Value() != want {
		t.Fatalf("prompt value = %q, want %q", m.pathEditor.Value(), want)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "Change source path") {
		t.Fatalf("expected the prompt on screen, got:\n%s", view)
	}
}

func TestChangeSourcePathLocksWorkingCopyRoot(t *testing.T) {
	m := sizedModel(t)
	m.client = svn.New("/home/alice/work/wc/internal")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	m = next.(*Model)

	for range 40 {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = next.(*Model)
	}

	if got := m.pathEditor.Value(); got != "/home/alice/work/wc" {
		t.Fatalf("value = %q, want the root to survive deletion", got)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = next.(*Model)
	if got := m.pathEditor.Value(); got != "/home/alice/work/wc" {
		t.Fatalf("value = %q, want the root to survive a word delete", got)
	}
}

func TestSubmitSourcePathRejectsPathAboveRoot(t *testing.T) {
	m := sizedModel(t)
	m.openSourcePathAt("/home/alice/work/wc/")

	if cmd := m.submitSourcePath("/home/alice/work"); cmd != nil {
		t.Fatal("expected a path above the working copy root to be rejected")
	}
	if !m.retargeting {
		t.Fatal("expected the prompt to stay open so the path can be corrected")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "must stay inside") {
		t.Fatalf("expected the out-of-bounds warning, got:\n%s", view)
	}
}

func TestChangeSourcePathEscapeClosesPrompt(t *testing.T) {
	m := sizedModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	m = next.(*Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*Model)
	next, _ = m.Update(cmd())
	m = next.(*Model)

	if m.retargeting {
		t.Fatal("expected esc to close the source-path prompt")
	}
}

func TestSubmitSourcePathSameDirectoryIsInert(t *testing.T) {
	m := sizedModel(t)
	m.retargeting = true

	if cmd := m.submitSourcePath("/home/alice/work/wc"); cmd != nil {
		t.Fatal("expected no probe for the directory already in use")
	}
	if m.retargeting {
		t.Fatal("expected the prompt to close")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "already reading") {
		t.Fatalf("expected a notice that the path is unchanged, got:\n%s", view)
	}
}

func TestSubmitSourcePathResolvesRelativeToCurrent(t *testing.T) {
	m := sizedModel(t)
	if got := m.resolveSourcePath("../other"); got != "/home/alice/work/other" {
		t.Fatalf("resolved = %q, want %q", got, "/home/alice/work/other")
	}
	if got := m.resolveSourcePath("/elsewhere/wc/"); got != "/elsewhere/wc" {
		t.Fatalf("resolved = %q, want %q", got, "/elsewhere/wc")
	}
}

func TestApplySourceChangeReRootsSession(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "old.txt", State: svn.StateModified}})
	next, _ := m.Update(logLoadedMsg{page: 1, entries: []svn.LogEntry{{Revision: "50", Message: "old history"}}})
	m = next.(*Model)
	m.setFilter(panelFiles, "old")
	m.collapsedDirs["sub"] = true

	client := svn.New("/home/alice/work/other")
	info := &svn.Info{URL: "https://svn.example.com/repo/branches/wip", WorkingCopyRoot: "/home/alice/work/other", Revision: "7"}
	next, cmd := m.Update(sourceChangedMsg{client: client, info: info})
	m = next.(*Model)

	if m.client.Dir != "/home/alice/work/other" {
		t.Fatalf("client dir = %q, want %q", m.client.Dir, "/home/alice/work/other")
	}
	if m.launchDir != "/home/alice/work/other" {
		t.Fatalf("launch dir = %q, want the new source", m.launchDir)
	}
	if m.wcRevision != "7" {
		t.Fatalf("revision = %q, want 7", m.wcRevision)
	}
	if len(m.fileItems) != 0 || len(m.collapsedDirs) != 0 || len(m.filters) != 0 {
		t.Fatalf("expected the old working copy's state to be dropped: items=%d dirs=%d filters=%d",
			len(m.fileItems), len(m.collapsedDirs), len(m.filters))
	}
	if len(m.log.Items()) != 0 || m.logPage != 1 || m.logAnchors != nil {
		t.Fatalf("expected history to be cleared and repaged: rows=%d page=%d anchors=%v",
			len(m.log.Items()), m.logPage, m.logAnchors)
	}
	if cmd == nil {
		t.Fatal("expected the new working copy to be loaded")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"/home/alice/work/other", "branches/wip", "source path:"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n---\n%s", want, view)
		}
	}
	for _, gone := range []string{"old.txt", "old history"} {
		if strings.Contains(view, gone) {
			t.Errorf("expected the previous working copy's %q to be gone\n---\n%s", gone, view)
		}
	}
}

func TestApplySourceChangeHonorsRootDisplayScope(t *testing.T) {
	cfg := config.Default()
	cfg.DisplayFrom = config.DisplayFromRoot
	m := sizedModelCfg(t, cfg)

	client := svn.New("/home/alice/work/other/sub")
	info := &svn.Info{URL: "https://svn.example.com/repo/trunk", WorkingCopyRoot: "/home/alice/work/other", Revision: "7"}
	next, _ := m.Update(sourceChangedMsg{client: client, info: info})
	m = next.(*Model)

	if m.client.Dir != "/home/alice/work/other" {
		t.Fatalf("client dir = %q, want the new working copy's root", m.client.Dir)
	}
}

func TestApplySourceChangeFailureKeepsCurrentSource(t *testing.T) {
	m := loadItems(t, sizedModel(t), []svn.StatusItem{{Path: "kept.txt", State: svn.StateModified}})

	bad := svn.New("/home/alice/work/wc/not-versioned")
	next, _ := m.Update(sourceChangedMsg{client: bad, err: errors.New("not a working copy")})
	m = next.(*Model)

	if m.client.Dir != "/home/alice/work/wc" {
		t.Fatalf("client dir = %q, want the original source", m.client.Dir)
	}
	if len(m.fileItems) != 1 {
		t.Fatalf("expected the loaded files to survive, got %d", len(m.fileItems))
	}
	if !m.retargeting {
		t.Fatal("expected the prompt to reopen so the path can be corrected")
	}
	if got := m.pathEditor.Value(); got != "/home/alice/work/wc/not-versioned" {
		t.Fatalf("prompt value = %q, want the rejected path", got)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "not a working copy") {
		t.Fatalf("expected the failure on screen, got:\n%s", view)
	}
}

func TestSourcePathPromptRelistsWhileTyping(t *testing.T) {
	root := sourceTree(t, "alpha", "beta")
	m := modelAtRoot(t, root)
	m.openSourcePathAt(root + string(filepath.Separator))
	// One row per suggestion, so narrowing the list shortens the box.
	before := strings.Count(m.pathEditor.View(), "\n")

	next, _ := m.Update(keyRunes("a"))
	m = next.(*Model)

	if after := strings.Count(m.pathEditor.View(), "\n"); after != before-1 {
		t.Fatalf("suggestions did not narrow while typing: %d rows before, %d after", before, after)
	}
}

func TestSourcePathPromptSubmitsThroughMessage(t *testing.T) {
	m := sizedModel(t)
	m.openSourcePathAt("/home/alice/work/wc")

	next, cmd := m.Update(uimsg.SubmitMsg{ID: sourcePathID, Value: "  "})
	m = next.(*Model)

	if cmd != nil {
		t.Fatal("expected a blank path to be rejected")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "cannot be empty") {
		t.Fatalf("expected the blank-path warning, got:\n%s", view)
	}
}
