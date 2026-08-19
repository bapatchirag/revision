package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
)

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:                      "0B",
		1:                      "1B",
		1023:                   "1023B",
		1024:                   "1.0KB",
		1536:                   "1.5KB",
		1024 * 1024:            "1.0MB",
		1024 * 1024 * 1024:     "1.0GB",
		1024 * 1024 * 1024 * 3: "3.0GB",
	}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
	// A terabyte still names a unit rather than running off the end of "KMGT".
	if got := humanSize(1024 * 1024 * 1024 * 1024); !strings.HasSuffix(got, "TB") {
		t.Errorf("humanSize(1TiB) = %q, want a TB suffix", got)
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cases := map[string]string{
		"~":            home,
		"~/work/wc":    filepath.Join(home, "work", "wc"),
		"/absolute":    "/absolute",
		"relative/dir": "relative/dir",
		"":             "",
		"~alice/work":  "~alice/work", // another user's home is left as written
	}
	for in, want := range cases {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSourceFloor(t *testing.T) {
	m := &Model{info: &svn.Info{WorkingCopyRoot: "/home/alice/work/wc/"}}
	if got := m.sourceFloor(); got != "/home/alice/work/wc" {
		t.Errorf("sourceFloor() = %q, want the cleaned working-copy root", got)
	}

	// Without a root, the current source stands in for it.
	m = &Model{client: svn.New("/home/alice/work/wc/src/")}
	if got := m.sourceFloor(); got != "/home/alice/work/wc/src" {
		t.Errorf("sourceFloor() = %q, want the cleaned client directory", got)
	}

	if got := (&Model{}).sourceFloor(); got != "" {
		t.Errorf("sourceFloor() = %q, want empty when nothing is known", got)
	}
}

func TestWithinSource(t *testing.T) {
	cases := []struct {
		floor, p string
		want     bool
	}{
		{"", "/anywhere", true},
		{"/wc", "/wc", true},
		{"/wc", "/wc/src", true},
		{"/wc", "/wc/src/deep", true},
		{"/wc", "/other", false},
		{"/wc", "/", false},
	}
	for _, tc := range cases {
		if got := withinSource(tc.floor, tc.p); got != tc.want {
			t.Errorf("withinSource(%q, %q) = %v, want %v", tc.floor, tc.p, got, tc.want)
		}
	}
}

func TestAbsPath(t *testing.T) {
	m := &Model{client: svn.New("/home/alice/work/wc")}
	if got := m.absPath("src/a.go"); got != "/home/alice/work/wc/src/a.go" {
		t.Errorf("absPath(rel) = %q, want it resolved against the client directory", got)
	}
	if got := m.absPath("/tmp/a.go"); got != "/tmp/a.go" {
		t.Errorf("absPath(abs) = %q, want it left alone", got)
	}
	if got := (&Model{}).absPath("src/a.go"); got != "src/a.go" {
		t.Errorf("absPath without a client = %q, want the path unchanged", got)
	}
}

func TestNewCommandLogFallsBackToTheDefaultLimit(t *testing.T) {
	for _, limit := range []int{0, -5} {
		if got := newCommandLog(limit).limit; got != commandLogLimit {
			t.Errorf("newCommandLog(%d).limit = %d, want the default %d", limit, got, commandLogLimit)
		}
	}
	if got := newCommandLog(3).limit; got != 3 {
		t.Errorf("newCommandLog(3).limit = %d, want 3", got)
	}
}

// TestCommandLogEvictsTheOldest drives the ring past its capacity.
func TestCommandLogEvictsTheOldest(t *testing.T) {
	l := newCommandLog(2)
	for _, c := range []string{"svn status", "svn diff", "svn log"} {
		l.record(svn.CommandRecord{Command: c, Output: "discarded"})
	}
	got := l.snapshot()
	if len(got) != 2 {
		t.Fatalf("kept %d entries, want the ring capped at 2", len(got))
	}
	if got[0].Command != "svn diff" || got[1].Command != "svn log" {
		t.Errorf("entries = %+v, want the oldest dropped", got)
	}
	if got[0].Output != "" {
		t.Error("output should be discarded rather than retained")
	}
}

// TestPanelCycleCoversTheSidePanelsOnly walks the cycle both ways: it visits
// Status, Files and Log and nothing else, and pressed on a panel outside it
// returns to the side panel driving Main.
func TestPanelCycleCoversTheSidePanelsOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyMsg
	}{
		{"tab", tea.KeyMsg{Type: tea.KeyTab}},
		{"shift+tab", tea.KeyMsg{Type: tea.KeyShiftTab}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := sizedModel(t)
			seen := map[int]bool{}
			for range 6 {
				m = stepModel(t, m, tc.key)
				seen[m.focus.Index()] = true
			}
			for _, p := range sidePanels {
				if !seen[p] {
					t.Errorf("panel %d should be in the cycle, reached %v", p, seen)
				}
			}
			if seen[panelMain] || seen[panelCmdLog] {
				t.Errorf("Main and the command log are outside the cycle, reached %v", seen)
			}
		})
	}
}

// TestPanelCycleReturnsFromMain covers stepping off a panel that is not in the
// cycle: focus goes back to the side panel that drove Main, not to the top.
func TestPanelCycleReturnsFromMain(t *testing.T) {
	for _, from := range []int{panelMain, panelCmdLog} {
		m := sizedModel(t)
		m = stepModel(t, m, keyRunes("3")) // the Log panel now drives Main
		m.focus.Focus(from)
		m.afterFocusChange()
		m = stepModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
		if m.focus.Index() != panelLog {
			t.Errorf("tab from panel %d should return to the Log panel, got index %d", from, m.focus.Index())
		}
	}
}

func TestWatchCmdLooksAtTheWorkingCopy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := watchCmd(dir, []string{"a.txt"}, false, 7, time.Millisecond)().(workingCopyChangedMsg)
	if !ok {
		t.Fatal("watchCmd should report a working-copy look")
	}
	if got.gen != 7 {
		t.Errorf("gen = %d, want the stamp it was scheduled with", got.gen)
	}
	if got.err != nil {
		t.Fatalf("look: %v", got.err)
	}
	if got.tracked == "" {
		t.Error("a tracked fingerprint should have been taken")
	}
	if got.scanned {
		t.Error("a shallow look scans nothing")
	}

	// A full look also fingerprints everything beneath the root.
	full, ok := watchCmd(dir, nil, true, 8, time.Millisecond)().(workingCopyChangedMsg)
	if !ok {
		t.Fatal("watchCmd should report a working-copy look")
	}
	if !full.scanned || full.full == "" {
		t.Errorf("msg = %+v, want a full scan", full)
	}

	// Editing the file changes the fingerprint, which is the whole point.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("beta"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := watchCmd(dir, []string{"a.txt"}, false, 9, time.Millisecond)().(workingCopyChangedMsg)
	if after.tracked == got.tracked {
		t.Error("an edited file should change the tracked fingerprint")
	}
}

func TestOpenDetachedCmd(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "opener")
	if err := os.WriteFile(ok, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	noisy := filepath.Join(dir, "noisy")
	if err := os.WriteFile(noisy, []byte("#!/bin/sh\necho 'no application knows this file' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	silent := filepath.Join(dir, "silent")
	if err := os.WriteFile(silent, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := openDetachedCmd(editorLaunch{cmd: exec.Command(ok, "a.go")}, "a.go")().(editedMsg)
	if got.err != nil || !got.detached || got.name != "a.go" {
		t.Errorf("msg = %+v, want a clean detached open", got)
	}

	failed := openDetachedCmd(editorLaunch{cmd: exec.Command(noisy, "a.go")}, "a.go")().(editedMsg)
	if failed.err == nil || !strings.Contains(failed.err.Error(), "no application") {
		t.Errorf("err = %v, want the opener's own stderr preferred", failed.err)
	}

	quiet := openDetachedCmd(editorLaunch{cmd: exec.Command(silent, "a.go")}, "a.go")().(editedMsg)
	if quiet.err == nil {
		t.Error("an opener that failed without a word must still report a failure")
	}
}

func TestMergeLineFollowsTheOpenPage(t *testing.T) {
	m, _ := conflictModel(t)
	if got := m.mergeLine(); got != 0 {
		t.Errorf("mergeLine() = %d, want 0 while nothing is open", got)
	}

	m = resolveKey(t, m)
	if !m.merging {
		t.Fatal("the resolution overlay should be open")
	}
	if got := m.mergeLine(); got <= 0 {
		t.Errorf("mergeLine() = %d, want the line the open page is reading", got)
	}
}

func TestBlockCountPluralizes(t *testing.T) {
	if got := blockCount(1, "conflict"); got != "1 conflict" {
		t.Errorf("blockCount(1) = %q, want the singular", got)
	}
	if got := blockCount(3, "hunk"); got != "3 hunks" {
		t.Errorf("blockCount(3) = %q, want the plural", got)
	}
}

func TestMergeDoneText(t *testing.T) {
	got := mergeDoneText(mergeWrittenMsg{kind: mergeReject, rel: "a.go", aux: "/wc/a.go.svnpatch.rej", count: 2})
	for _, want := range []string{"2 hunks", "a.go", "a.go.svnpatch.rej", "cleared"} {
		if !strings.Contains(got, want) {
			t.Errorf("mergeDoneText = %q, want it to mention %q", got, want)
		}
	}
	got = mergeDoneText(mergeWrittenMsg{kind: mergeConflict, rel: "a.go", count: 1})
	if !strings.Contains(got, "resolved 1 conflict in a.go") {
		t.Errorf("mergeDoneText = %q, want the conflict wording", got)
	}
}
