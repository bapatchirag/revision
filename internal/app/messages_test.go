package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/selfupdate"
	"github.com/bapatchirag/revision/internal/svn"
)

// cmdClient returns a client whose svn binary is a stub that prints out on
// stdout and exits with code, so the tea.Cmd closures below can be run for real
// against both a succeeding and a failing svn. Dir is the temp directory the
// stub lives in, which doubles as the working copy under test.
func cmdClient(t *testing.T, out string, code int) *svn.Client {
	t.Helper()
	dir := t.TempDir()
	return cmdClientIn(t, dir, out, code)
}

// cmdClientIn is cmdClient rooted at an existing directory, for the commands
// that need files staged beside the stub.
func cmdClientIn(t *testing.T, dir, out string, code int) *svn.Client {
	t.Helper()
	body := filepath.Join(t.TempDir(), "stdout.txt")
	if err := os.WriteFile(body, []byte(out), 0o644); err != nil {
		t.Fatalf("write stub output: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "svn-stub")
	script := fmt.Sprintf("#!/bin/sh\ncat %s\nexit %d\n", body, code)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub binary: %v", err)
	}
	return &svn.Client{Dir: dir, Bin: bin}
}

// pickyClient returns a client whose svn stub records every invocation and
// refuses any that names one of bad, together with a reader for what it was
// asked to run. It is what a fan-out is checked against: that the files after
// the one svn would not take were still attempted.
//
// It expands --targets the way svn does, reading the paths out of the file it
// names, so a batched invocation is judged on the paths it actually carries
// rather than on the temporary file it carries them in.
func pickyClient(t *testing.T, bad ...string) (*svn.Client, func() []string) {
	t.Helper()
	dir := t.TempDir()
	argv := filepath.Join(t.TempDir(), "argv.txt")
	bin := filepath.Join(t.TempDir(), "svn-stub")
	// An empty set has to match nothing, and an empty case pattern is a syntax
	// error, so it falls back to a path no test uses.
	pattern := "\x00none\x00"
	if len(bad) > 0 {
		pattern = strings.Join(bad, "|")
	}
	script := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %s
named=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--targets" ]; then named="$named $(cat "$a")"; fi
  prev="$a"
done
for a in "$@" $named; do
  case "$a" in
  %s) echo "svn: E155007: '$a'" >&2; exit 1 ;;
  esac
done
exit 0
`, argv, pattern)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub binary: %v", err)
	}
	calls := func() []string {
		b, err := os.ReadFile(argv)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	}
	return &svn.Client{Dir: dir, Bin: bin}, calls
}

// run executes a command's closure, failing when the command is nil.
func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	return cmd()
}

// msgOf runs cmd and asserts its message is of type T.
func msgOf[T tea.Msg](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	got, ok := run(t, cmd).(T)
	if !ok {
		var want T
		t.Fatalf("message is %T, want %T", got, want)
	}
	return got
}

func TestErrMsgError(t *testing.T) {
	if got := (errMsg{err: errors.New("kaboom")}).Error(); got != "kaboom" {
		t.Errorf("Error() = %q, want the wrapped error's text", got)
	}
}

func TestSuperseded(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if !superseded(cancelled) {
		t.Error("a cancelled context is superseded")
	}
	if superseded(context.Background()) {
		t.Error("a live context is not superseded")
	}
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
	defer stop()
	if superseded(expired) {
		t.Error("an expired deadline is a real failure, not a supersession")
	}
}

func TestBatchOutcomeLabel(t *testing.T) {
	var one batchOutcome
	one.ok("a.txt")
	if got := one.label(); got != "a.txt" {
		t.Errorf("label(1) = %q, want the sole path", got)
	}
	var many batchOutcome
	for _, p := range []string{"a.txt", "b.txt", "c.txt"} {
		many.ok(p)
	}
	if got := many.label(); got != "3 files" {
		t.Errorf("label(3) = %q, want a count", got)
	}
}

func TestBatchLabel(t *testing.T) {
	if got := batchLabel(1, "a.txt"); got != "a.txt" {
		t.Errorf("batchLabel(1) = %q, want the sole path", got)
	}
	if got := batchLabel(4, "a.txt"); got != "4 files" {
		t.Errorf("batchLabel(4) = %q, want a count", got)
	}
}

func TestPatchTrialErr(t *testing.T) {
	cases := map[string]struct {
		res  svn.PatchResult
		want string
	}{
		"nothing to apply": {svn.PatchResult{}, "nothing in it to apply"},
		"partly applied":   {svn.PatchResult{Applied: []string{"a.txt"}, Skipped: []string{"b.txt"}}, ""},
		"all skipped":      {svn.PatchResult{Skipped: []string{"a.txt", "b.txt"}}, "svn cannot find 2 files"},
		"only conflicted":  {svn.PatchResult{Conflicted: []string{"a.txt"}}, "not one of its changes applies"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := patchTrialErr(tc.res)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("patchTrialErr = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("patchTrialErr = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestProbeSourceCmd(t *testing.T) {
	const infoXML = `<?xml version="1.0"?>
<info><entry path="." revision="42" kind="dir">
<url>https://svn.example.com/repo/trunk</url>
<wc-info><wcroot-abspath>/wc</wcroot-abspath></wc-info>
</entry></info>`

	got := msgOf[sourceChangedMsg](t, probeSourceCmd(cmdClient(t, infoXML, 0), fromSourcePath))
	if got.err != nil {
		t.Fatalf("probe: %v", got.err)
	}
	if got.info == nil || got.info.Revision != "42" {
		t.Errorf("info = %+v, want the parsed entry", got.info)
	}

	bad := msgOf[sourceChangedMsg](t, probeSourceCmd(cmdClient(t, "", 1), fromRepoSwitch))
	if bad.err == nil {
		t.Error("a directory svn cannot read must be reported as an error")
	}
	if bad.from != fromRepoSwitch {
		t.Error("the reply must carry the prompt the probe was asked for")
	}
}

func TestStartupNoticeCmd(t *testing.T) {
	got := msgOf[startupNoticeMsg](t, startupNoticeCmd("using the default theme"))
	if got.text != "using the default theme" {
		t.Errorf("text = %q, want the notice", got.text)
	}
}

func TestSSHCmdsReportFailure(t *testing.T) {
	checked := msgOf[sshCheckedMsg](t, sshCheckCmd(""))
	if checked.err == nil {
		t.Error("an empty key path cannot be checked and must error")
	}
	if checked.loaded {
		t.Error("loaded must be false when the check failed")
	}

	added := msgOf[sshAddedMsg](t, sshAddCmd("", "hunter2"))
	if added.err == nil {
		t.Error("an empty key path cannot be added and must error")
	}
}

func TestCheckUpdateCmdIsInertOnDevBuilds(t *testing.T) {
	if msg := checkUpdateCmd(selfupdate.Build{Version: "dev"})(); msg != nil {
		t.Errorf("a development build must produce no message, got %#v", msg)
	}
}

func TestLoadStatusCmd(t *testing.T) {
	const statusXML = `<?xml version="1.0"?>
<status><target path="."><entry path="a.txt"><wc-status item="modified" props="none"/></entry></target></status>`

	got := msgOf[statusLoadedMsg](t, loadStatusCmd(context.Background(), cmdClient(t, statusXML, 0), 7))
	if got.gen != 7 {
		t.Errorf("gen = %d, want the stamp it was issued with", got.gen)
	}
	if len(got.items) != 1 || got.items[0].Path != "a.txt" {
		t.Errorf("items = %+v, want the single parsed entry", got.items)
	}

	failed := msgOf[errMsg](t, loadStatusCmd(context.Background(), cmdClient(t, "", 1), 1))
	if failed.err == nil {
		t.Error("a failing svn status must surface as an error")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if msg := loadStatusCmd(ctx, cmdClient(t, "", 1), 1)(); msg != nil {
		t.Errorf("a superseded load must report nothing, got %#v", msg)
	}
}

func TestLoadDiffCmd(t *testing.T) {
	const patch = "Index: a.txt\n@@ -1 +1 @@\n-old\n+new\n"

	got := msgOf[diffLoadedMsg](t, loadDiffCmd(context.Background(), cmdClient(t, patch, 0), diffKey{path: "a.txt"}, 3))
	if got.path != "a.txt" || got.dir || got.gen != 3 || got.diff != patch {
		t.Errorf("msg = %+v, want the diff keyed by its request", got)
	}

	// The synthetic root diffs the whole working copy but stays keyed by "/".
	root := msgOf[diffLoadedMsg](t, loadDiffCmd(context.Background(), cmdClient(t, patch, 0), diffKey{path: fileTreeRoot, dir: true}, 4))
	if root.path != fileTreeRoot || !root.dir {
		t.Errorf("msg = %+v, want it keyed by the synthetic root", root)
	}

	failed := msgOf[diffLoadedMsg](t, loadDiffCmd(context.Background(), cmdClient(t, "", 1), diffKey{path: "a.txt"}, 5))
	if failed.err == nil {
		t.Error("a diff failure rides on the message rather than tearing down the UI")
	}
}

func TestSaveDiffCmd(t *testing.T) {
	const patch = "Index: a.txt\n@@ -1 +1 @@\n-old\n+new"
	out := t.TempDir()

	got := msgOf[diffSavedMsg](t, saveDiffCmd(cmdClient(t, patch, 0), []string{"a.txt"}, out, "wip.diff"))
	if got.err != nil {
		t.Fatalf("save: %v", got.err)
	}
	body, err := os.ReadFile(got.path)
	if err != nil {
		t.Fatalf("read saved diff: %v", err)
	}
	if string(body) != patch+"\n" {
		t.Errorf("saved %q, want the diff with a trailing newline", body)
	}

	failed := msgOf[diffSavedMsg](t, saveDiffCmd(cmdClient(t, "", 1), nil, out, "wip.diff"))
	if failed.err == nil {
		t.Error("a diff that svn refused must not be written")
	}
	if failed.path != filepath.Join(out, "wip.diff") {
		t.Errorf("path = %q, want the destination it was attempting", failed.path)
	}
}

func TestSavedDiffCmds(t *testing.T) {
	dir := t.TempDir()
	patch := filepath.Join(dir, "wip.diff")
	if err := os.WriteFile(patch, []byte("@@ -1 +1 @@\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	listed := msgOf[savedDiffsLoadedMsg](t, loadSavedDiffsCmd(dir, 1))
	if listed.err != nil || len(listed.files) != 1 || listed.files[0].Name != "wip.diff" {
		t.Errorf("listed = %+v (err %v), want the one saved patch", listed.files, listed.err)
	}

	read := msgOf[savedDiffReadMsg](t, readSavedDiffCmd(patch, 2))
	if read.err != nil || read.text != "@@ -1 +1 @@\n" || read.gen != 2 {
		t.Errorf("read = %+v, want the file's contents", read)
	}

	missing := msgOf[savedDiffReadMsg](t, readSavedDiffCmd(filepath.Join(dir, "gone.diff"), 3))
	if missing.err == nil {
		t.Error("an unreadable patch must carry its error rather than panic")
	}

	deleted := msgOf[savedDiffDeletedMsg](t, deleteSavedDiffCmd(patch, "wip.diff"))
	if deleted.err != nil || deleted.name != "wip.diff" {
		t.Errorf("deleted = %+v, want the removal to succeed", deleted)
	}
	if _, err := os.Stat(patch); !os.IsNotExist(err) {
		t.Error("the patch file should be gone")
	}
	if again := msgOf[savedDiffDeletedMsg](t, deleteSavedDiffCmd(patch, "wip.diff")); again.err == nil {
		t.Error("removing a patch twice must report the second failure")
	}
}

func TestRejectCmds(t *testing.T) {
	dir := t.TempDir()
	rej := filepath.Join(dir, "a.txt.svnpatch.rej")
	if err := os.WriteFile(rej, []byte("@@ -1,1 +1,1 @@\n-old\n+new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	listed := msgOf[rejectsLoadedMsg](t, loadRejectsCmd(dir, 1))
	if listed.err != nil || len(listed.files) != 1 {
		t.Errorf("listed = %+v (err %v), want the one reject", listed.files, listed.err)
	}

	read := msgOf[rejectReadMsg](t, readRejectCmd(rej, 2))
	if read.err != nil || !strings.Contains(read.text, "+new") {
		t.Errorf("read = %+v, want the reject's contents", read)
	}

	missing := msgOf[rejectReadMsg](t, readRejectCmd(filepath.Join(dir, "gone.rej"), 3))
	if missing.err == nil {
		t.Error("an unreadable reject must carry its error")
	}

	deleted := msgOf[rejectDeletedMsg](t, deleteRejectCmd(rej, "a.txt.svnpatch.rej"))
	if deleted.err != nil {
		t.Errorf("delete: %v", deleted.err)
	}
	if again := msgOf[rejectDeletedMsg](t, deleteRejectCmd(rej, "a.txt.svnpatch.rej")); again.err == nil {
		t.Error("removing a reject twice must report the second failure")
	}
}

func TestApplyPatchCmd(t *testing.T) {
	// A working copy holding the file the patch expects to find.
	wc := t.TempDir()
	if err := os.WriteFile(filepath.Join(wc, "a.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := filepath.Join(t.TempDir(), "wip.diff")
	if err := os.WriteFile(patch, []byte("--- a.txt\t(revision 1)\n+++ a.txt\t(working copy)\n@@ -1 +1 @@\n-old\n+new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("unreadable patch", func(t *testing.T) {
		got := msgOf[patchAppliedMsg](t, applyPatchCmd(cmdClientIn(t, wc, "", 0), filepath.Join(wc, "gone.diff"), "gone.diff", wc))
		if got.err == nil {
			t.Error("a patch that cannot be read must be reported")
		}
	})

	t.Run("taken from elsewhere", func(t *testing.T) {
		elsewhere := t.TempDir()
		got := msgOf[patchAppliedMsg](t, applyPatchCmd(cmdClientIn(t, elsewhere, "", 0), patch, "wip.diff", elsewhere))
		if got.err == nil || !strings.Contains(got.err.Error(), "another directory") {
			t.Errorf("err = %v, want the patch refused as foreign", got.err)
		}
	})

	t.Run("dry run fails", func(t *testing.T) {
		got := msgOf[patchAppliedMsg](t, applyPatchCmd(cmdClientIn(t, wc, "", 1), patch, "wip.diff", wc))
		if got.err == nil {
			t.Error("a dry run svn refused must be reported")
		}
	})

	t.Run("nothing would land", func(t *testing.T) {
		client := cmdClientIn(t, wc, "Skipped missing target: 'a.txt'\n", 0)
		got := msgOf[patchAppliedMsg](t, applyPatchCmd(client, patch, "wip.diff", wc))
		if got.err == nil || !strings.Contains(got.err.Error(), "cannot find") {
			t.Errorf("err = %v, want the patch refused after the trial", got.err)
		}
	})

	t.Run("applied", func(t *testing.T) {
		client := cmdClientIn(t, wc, "U         a.txt\n", 0)
		got := msgOf[patchAppliedMsg](t, applyPatchCmd(client, patch, "wip.diff", wc))
		if got.err != nil {
			t.Fatalf("apply: %v", got.err)
		}
		if len(got.res.Applied) != 1 || got.res.Applied[0] != "a.txt" {
			t.Errorf("res = %+v, want the applied target", got.res)
		}
	})
}

func TestLoadConflictCmd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(conflictedFile), 0o644); err != nil {
		t.Fatal(err)
	}

	got := msgOf[mergeLoadedMsg](t, loadConflictCmd(path, "main.go"))
	if got.err != nil {
		t.Fatalf("load: %v", got.err)
	}
	if got.doc == nil || len(got.doc.regions) != 1 {
		t.Errorf("doc = %+v, want the one conflict region", got.doc)
	}

	missing := msgOf[mergeLoadedMsg](t, loadConflictCmd(filepath.Join(dir, "gone.go"), "gone.go"))
	if missing.err == nil || missing.rel != "gone.go" {
		t.Errorf("msg = %+v, want the unreadable file named in the failure", missing)
	}
}

func TestLoadRejectMergeCmd(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "main.go")
	rej := target + ".svnpatch.rej"
	if err := os.WriteFile(target, []byte(targetFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rej, []byte("@@ -4,1 +4,1 @@\n-\tsetup()\n+\tsetUp()\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := msgOf[mergeLoadedMsg](t, loadRejectMergeCmd(rej, "main.go.svnpatch.rej"))
	if got.err != nil {
		t.Fatalf("load: %v", got.err)
	}
	if got.doc == nil || got.doc.kind != mergeReject || got.rel != "main.go" {
		t.Errorf("msg = %+v, want a reject doc named for its target", got)
	}

	noReject := msgOf[mergeLoadedMsg](t, loadRejectMergeCmd(filepath.Join(dir, "gone.go.svnpatch.rej"), "gone.go.svnpatch.rej"))
	if noReject.err == nil {
		t.Error("an unreadable reject must be reported")
	}

	// A reject whose target has since been deleted has nothing to resolve against.
	orphan := filepath.Join(dir, "removed.go.svnpatch.rej")
	if err := os.WriteFile(orphan, []byte("@@ -1 +1 @@\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	noTarget := msgOf[mergeLoadedMsg](t, loadRejectMergeCmd(orphan, "removed.go.svnpatch.rej"))
	if noTarget.err == nil || noTarget.rel != "removed.go" {
		t.Errorf("msg = %+v, want the missing target named in the failure", noTarget)
	}
}

func TestWriteMergeCmd(t *testing.T) {
	t.Run("conflict resolves through svn", func(t *testing.T) {
		client := cmdClient(t, "", 0)
		path := filepath.Join(client.Dir, "main.go")
		if err := os.WriteFile(path, []byte(conflictedFile), 0o644); err != nil {
			t.Fatal(err)
		}
		doc := conflictDoc(path, "main.go", conflictedFile)
		doc.regions[0].choice = chooseLeft

		got := msgOf[mergeWrittenMsg](t, writeMergeCmd(client, doc))
		if got.err != nil {
			t.Fatalf("write: %v", got.err)
		}
		if got.kind != mergeConflict || got.count != 1 {
			t.Errorf("msg = %+v, want one resolved conflict", got)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), conflictMineMarker) {
			t.Error("the written file must have its markers resolved away")
		}
	})

	t.Run("reject clears the .rej", func(t *testing.T) {
		client := cmdClient(t, "", 0)
		target := filepath.Join(client.Dir, "main.go")
		rej := target + ".svnpatch.rej"
		if err := os.WriteFile(target, []byte(targetFile), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rej, []byte("@@ -4,1 +4,1 @@\n-\tsetup()\n+\tsetUp()\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		doc := rejectDoc(rej, "main.go", "@@ -4,1 +4,1 @@\n-\tsetup()\n+\tsetUp()\n", target, targetFile)

		got := msgOf[mergeWrittenMsg](t, writeMergeCmd(client, doc))
		if got.err != nil {
			t.Fatalf("write: %v", got.err)
		}
		if got.kind != mergeReject {
			t.Errorf("kind = %v, want a reject", got.kind)
		}
		if _, err := os.Stat(rej); !os.IsNotExist(err) {
			t.Error("the reject must be removed once its hunks are decided")
		}
	})

	t.Run("unwritable target", func(t *testing.T) {
		client := cmdClient(t, "", 0)
		// A path under a file, which can never be created.
		blocked := filepath.Join(client.Dir, "main.go", "nested.go")
		if err := os.WriteFile(filepath.Join(client.Dir, "main.go"), []byte(conflictedFile), 0o644); err != nil {
			t.Fatal(err)
		}
		doc := conflictDoc(blocked, "nested.go", conflictedFile)

		if got := msgOf[mergeWrittenMsg](t, writeMergeCmd(client, doc)); got.err == nil {
			t.Error("a file that cannot be written must report its failure")
		}
	})

	t.Run("svn resolve fails", func(t *testing.T) {
		client := cmdClient(t, "", 1)
		path := filepath.Join(client.Dir, "main.go")
		doc := conflictDoc(path, "main.go", conflictedFile)

		if got := msgOf[mergeWrittenMsg](t, writeMergeCmd(client, doc)); got.err == nil {
			t.Error("a resolve svn refused must be reported")
		}
	})
}

func TestWriteDiffReportsAnUncreatableDirectory(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "out")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := writeDiff(filepath.Join(blocker, "nested"), "wip.diff", "@@")
	if got.err == nil {
		t.Error("a directory that cannot be created must be reported")
	}
}

func TestLogCmds(t *testing.T) {
	const logXML = `<?xml version="1.0"?>
<log><logentry revision="42"><author>alice</author><date>2024-01-02T03:04:05.000000Z</date><msg>first</msg></logentry></log>`

	page := msgOf[logLoadedMsg](t, loadLogCmd(context.Background(), cmdClient(t, logXML, 0), "", 1, 50, 9))
	if page.err != nil || page.page != 1 || page.gen != 9 {
		t.Fatalf("msg = %+v, want the first page", page)
	}
	if len(page.entries) != 1 || page.entries[0].Revision != "42" {
		t.Errorf("entries = %+v, want the single revision", page.entries)
	}

	failed := msgOf[logLoadedMsg](t, loadLogCmd(context.Background(), cmdClient(t, "", 1), "", 2, 50, 1))
	if failed.err == nil {
		t.Error("a log failure stays on the message so it is confined to the Log panel")
	}

	head := msgOf[headLoadedMsg](t, headRevisionCmd(cmdClient(t, logXML, 0)))
	if head.err != nil || head.rev != "42" {
		t.Errorf("head = %+v, want the newest revision", head)
	}
	if bad := msgOf[headLoadedMsg](t, headRevisionCmd(cmdClient(t, "", 1))); bad.err == nil {
		t.Error("a failing head read must be reported")
	}

	detail := msgOf[revisionDetailMsg](t, loadRevisionDetailCmd(context.Background(), cmdClient(t, logXML, 0), "42", 6))
	if detail.err != nil || detail.rev != "42" || detail.gen != 6 {
		t.Errorf("detail = %+v, want the revision it was asked for", detail)
	}
	if bad := msgOf[revisionDetailMsg](t, loadRevisionDetailCmd(context.Background(), cmdClient(t, "", 1), "42", 1)); bad.err == nil {
		t.Error("a revision that cannot be described must carry its error")
	}
}

func TestStageCmd(t *testing.T) {
	ok := cmdClient(t, "", 0)

	staged := msgOf[stagedMsg](t, stageCmd(ok, "revision:staged", stageAction{path: "a.txt", stage: true}, 11))
	if staged.outcome.err() != nil || staged.token != 11 || staged.outcome.label() != "a.txt" {
		t.Errorf("msg = %+v, want a clean stage", staged)
	}

	unstaged := msgOf[stagedMsg](t, stageCmd(ok, "revision:staged", stageAction{path: "a.txt"}, 12))
	if unstaged.outcome.err() != nil {
		t.Errorf("msg = %+v, want a clean unstage", unstaged)
	}

	added := msgOf[stagedMsg](t, stageCmd(ok, "revision:staged", stageAction{path: "new.txt", add: true, stage: true}, 13))
	if added.outcome.err() != nil {
		t.Errorf("staging an untracked file should svn add it first: %v", added.outcome.err())
	}

	failed := msgOf[stagedMsg](t, stageCmd(cmdClient(t, "", 1), "revision:staged", stageAction{path: "new.txt", add: true, stage: true}, 14))
	if failed.outcome.err() == nil || failed.token != 14 {
		t.Errorf("msg = %+v, want the failure carrying its token so the optimistic change can be undone", failed)
	}
}

func TestStageManyCmd(t *testing.T) {
	acts := []stageAction{
		{path: "a.txt", add: true, stage: true},
		{path: "b.txt", stage: true},
		{path: "c.txt"},
	}

	got := msgOf[stagedMsg](t, stageManyCmd(cmdClient(t, "", 0), "revision:staged", acts, 21))
	if got.outcome.err() != nil || got.token != 21 || len(got.outcome.done) != 3 {
		t.Errorf("msg = %+v, want every file staged", got)
	}
}

// TestStageManyCmdCarriesOnPastARefusal pins that one file svn will not stage no
// longer strands the files listed after it: the run reports what it could not do
// and the rest are staged all the same.
func TestStageManyCmdCarriesOnPastARefusal(t *testing.T) {
	client, calls := pickyClient(t, "b.txt")
	acts := []stageAction{{path: "a.txt", stage: true}, {path: "b.txt", stage: true}, {path: "c.txt", stage: true}}

	got := msgOf[stagedMsg](t, stageManyCmd(client, "revision:staged", acts, 22))
	if got.outcome.err() == nil {
		t.Fatal("a file svn refused has to be reported")
	}
	if len(got.outcome.failed) != 1 || got.outcome.failed[0].path != "b.txt" {
		t.Errorf("failed = %+v, want only the file svn refused", got.outcome.failed)
	}
	if len(got.outcome.done) != 2 {
		t.Errorf("done = %q, want the other two staged regardless", got.outcome.done)
	}
	argv := strings.Join(calls(), "\n")
	if !strings.Contains(argv, "c.txt") {
		t.Errorf("svn was never asked about c.txt — the refusal blocked it:\n%s", argv)
	}
}

// TestStageManyCmdBatchesTheInvocations pins the shape of what runs: a mixed set
// costs three invocations rather than one or two per file, with the add ahead of
// the changelist assignment, since svn will not changelist a file it does not
// yet track.
func TestStageManyCmdBatchesTheInvocations(t *testing.T) {
	client, calls := pickyClient(t)
	acts := []stageAction{
		{path: "new1.txt", add: true, stage: true},
		{path: "mod1.txt", stage: true},
		{path: "new2.txt", add: true, stage: true},
		{path: "off1.txt"},
		{path: "mod2.txt", stage: true},
		{path: "off2.txt"},
	}

	got := msgOf[stagedMsg](t, stageManyCmd(client, "revision:staged", acts, 51))
	if got.outcome.err() != nil || len(got.outcome.done) != len(acts) {
		t.Fatalf("outcome = %+v, want every action landed", got.outcome)
	}
	ran := calls()
	if len(ran) != 3 {
		t.Fatalf("svn ran %d times, want one invocation per kind:\n%s", len(ran), strings.Join(ran, "\n"))
	}
	if !strings.HasPrefix(ran[0], "add --force ") {
		t.Errorf("first invocation = %q, want the add ahead of the rest", ran[0])
	}
	if !strings.HasPrefix(ran[1], "changelist revision:staged ") {
		t.Errorf("second invocation = %q, want the changelist assignment", ran[1])
	}
	if !strings.HasPrefix(ran[2], "changelist --remove ") {
		t.Errorf("third invocation = %q, want the changelist removal", ran[2])
	}
	for _, call := range ran {
		if !strings.Contains(call, "--targets ") {
			t.Errorf("invocation %q names its paths inline, want them in a targets file", call)
		}
	}
}

// TestStageManyCmdRunsNothingForAnEmptySet locks the early return: svn refuses an
// invocation naming no path, so a kind with nothing in it must not be run at all.
func TestStageManyCmdRunsNothingForAnEmptySet(t *testing.T) {
	client, calls := pickyClient(t)
	got := msgOf[stagedMsg](t, stageManyCmd(client, "revision:staged", nil, 52))
	if got.outcome.err() != nil || len(got.outcome.done) != 0 {
		t.Errorf("outcome = %+v, want nothing done and nothing refused", got.outcome)
	}
	if ran := calls(); len(ran) != 0 {
		t.Errorf("svn ran %d times for an empty set:\n%s", len(ran), strings.Join(ran, "\n"))
	}
}

// TestStageManyCmdKeepsASinglePathReadable pins that staging one file still
// names it on the command line: it is the common case, and a targets file would
// leave the command log showing a temporary path instead of the file staged.
func TestStageManyCmdKeepsASinglePathReadable(t *testing.T) {
	client, calls := pickyClient(t)
	msgOf[stagedMsg](t, stageManyCmd(client, "revision:staged", []stageAction{{path: "a.txt", stage: true}}, 53))
	ran := calls()
	if len(ran) != 1 || ran[0] != "changelist revision:staged a.txt --non-interactive" {
		t.Errorf("ran %q, want one invocation naming the file itself", ran)
	}
}

// TestStageManyCmdSkipsTheChangelistWhenTheAddIsRefused pins the ordering
// dependency: a file svn would not add is not then asked to join a changelist,
// which it is not yet tracked well enough to do.
func TestStageManyCmdSkipsTheChangelistWhenTheAddIsRefused(t *testing.T) {
	client, calls := pickyClient(t, "bad.txt")
	acts := []stageAction{
		{path: "bad.txt", add: true, stage: true},
		{path: "good.txt", add: true, stage: true},
	}

	got := msgOf[stagedMsg](t, stageManyCmd(client, "revision:staged", acts, 54))
	if len(got.outcome.failed) != 1 || got.outcome.failed[0].path != "bad.txt" {
		t.Errorf("failed = %+v, want only the file svn would not add", got.outcome.failed)
	}
	if len(got.outcome.done) != 1 || got.outcome.done[0] != "good.txt" {
		t.Errorf("done = %q, want the other file added and staged", got.outcome.done)
	}
	for _, call := range calls() {
		if strings.HasPrefix(call, "changelist ") && strings.Contains(call, "bad.txt") {
			t.Errorf("a file that could not be added was still changelisted: %q", call)
		}
	}
}

func TestCommitCmd(t *testing.T) {
	got := msgOf[committedMsg](t, commitCmd(cmdClient(t, "Committed revision 43.\n", 0), "msg", "revision:staged", 31))
	if got.err != nil || got.revision != "43" || got.token != 31 {
		t.Errorf("msg = %+v, want the new revision", got)
	}

	failed := msgOf[committedMsg](t, commitCmd(cmdClient(t, "", 1), "msg", "", 32))
	if failed.err == nil || failed.token != 32 {
		t.Errorf("msg = %+v, want the failure carrying its token", failed)
	}
}

func TestAssignChangelistCmd(t *testing.T) {
	targets := []changelistTarget{{path: "a.txt", add: true}, {path: "b.txt"}}

	got := msgOf[stagedMsg](t, assignChangelistCmd(cmdClient(t, "", 0), "feature", targets, 41))
	if got.outcome.err() != nil || got.changelist != "feature" || got.outcome.label() != "2 files" {
		t.Errorf("msg = %+v, want a two-file assignment", got)
	}

	one := msgOf[stagedMsg](t, assignChangelistCmd(cmdClient(t, "", 0), "feature", targets[1:], 42))
	if one.outcome.err() != nil || one.outcome.label() != "b.txt" {
		t.Errorf("msg = %+v, want the sole path named", one)
	}

	// The file svn refuses is named, and the one after it is assigned anyway.
	picky, _ := pickyClient(t, "a.txt")
	partial := msgOf[stagedMsg](t, assignChangelistCmd(picky, "feature", targets, 43))
	if partial.outcome.err() == nil || partial.outcome.label() != "b.txt" {
		t.Errorf("msg = %+v, want a.txt reported and b.txt assigned regardless", partial)
	}
}

func TestRevertCmds(t *testing.T) {
	got := msgOf[revertedMsg](t, revertCmd(cmdClient(t, "", 0), "a.txt", 51))
	if got.outcome.err() != nil || got.outcome.label() != "a.txt" || got.token != 51 {
		t.Errorf("msg = %+v, want a clean revert", got)
	}
	if failed := msgOf[revertedMsg](t, revertCmd(cmdClient(t, "", 1), "a.txt", 52)); failed.outcome.err() == nil {
		t.Error("a revert svn refused must be reported")
	}

	many := msgOf[revertedMsg](t, revertManyCmd(cmdClient(t, "", 0), []string{"a.txt", "b.txt"}, 53))
	if many.outcome.err() != nil || many.outcome.label() != "2 files" {
		t.Errorf("msg = %+v, want a two-file summary", many)
	}

	one := msgOf[revertedMsg](t, revertManyCmd(cmdClient(t, "", 0), []string{"a.txt"}, 54))
	if one.outcome.err() != nil || one.outcome.label() != "a.txt" {
		t.Errorf("msg = %+v, want the sole path named", one)
	}
}

// TestRevertManyCmdCarriesOnPastARefusal pins the failure that started all this:
// svn walks a multi-target revert in order and abandons the process at the first
// target it refuses, leaving everything after it untouched. The retry a path at
// a time is what rescues the rest.
func TestRevertManyCmdCarriesOnPastARefusal(t *testing.T) {
	client, calls := pickyClient(t, "b.txt")

	got := msgOf[revertedMsg](t, revertManyCmd(client, []string{"a.txt", "b.txt", "c.txt"}, 55))
	if len(got.outcome.failed) != 1 || got.outcome.failed[0].path != "b.txt" {
		t.Errorf("failed = %+v, want only the path svn refused", got.outcome.failed)
	}
	if len(got.outcome.done) != 2 {
		t.Errorf("done = %q, want a.txt and c.txt reverted regardless", got.outcome.done)
	}
	argv := calls()
	if len(argv) != 4 {
		t.Errorf("argv = %q, want the batch attempt and then one invocation per path", argv)
	}
}

// TestRevertReportsWhatSvnPassedOver pins the quiet half of the same failure: a
// path svn skips is announced on stdout and the command still exits zero, so
// taking the exit code for the whole answer reports a revert that discarded
// nothing as a success.
func TestRevertReportsWhatSvnPassedOver(t *testing.T) {
	client := cmdClient(t, "Reverted 'a.txt'\nSkipped 'gone.txt'\n", 0)

	got := msgOf[revertedMsg](t, revertManyCmd(client, []string{"a.txt", "gone.txt"}, 56))
	if got.outcome.err() == nil {
		t.Fatal("a path svn passed over has to be reported, not counted as reverted")
	}
	if len(got.outcome.failed) != 1 || got.outcome.failed[0].path != "gone.txt" {
		t.Errorf("failed = %+v, want the skipped path", got.outcome.failed)
	}
	if got.outcome.label() != "a.txt" {
		t.Errorf("done = %q, want only the path svn actually reverted", got.outcome.done)
	}
}

func TestDeleteCmds(t *testing.T) {
	client := cmdClient(t, "", 0)
	untracked := filepath.Join(client.Dir, "junk.txt")
	if err := os.WriteFile(untracked, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	versioned := msgOf[deletedMsg](t, deleteCmd(client, deleteAction{path: "a.txt"}, 61))
	if versioned.outcome.err() != nil || versioned.outcome.label() != "a.txt" || versioned.token != 61 {
		t.Errorf("msg = %+v, want a clean svn delete", versioned)
	}

	// RemoveUnversioned resolves against the working copy, so the path is relative.
	removed := msgOf[deletedMsg](t, deleteCmd(client, deleteAction{path: "junk.txt", unversioned: true}, 62))
	if removed.outcome.err() != nil {
		t.Fatalf("removing an untracked file: %v", removed.outcome.err())
	}
	if _, err := os.Stat(untracked); !os.IsNotExist(err) {
		t.Error("an untracked file is removed from disk rather than scheduled")
	}

	if failed := msgOf[deletedMsg](t, deleteCmd(cmdClient(t, "", 1), deleteAction{path: "a.txt"}, 63)); failed.outcome.err() == nil {
		t.Error("a delete svn refused must be reported")
	}
}

func TestDeleteManyCmd(t *testing.T) {
	client := cmdClient(t, "", 0)
	if err := os.WriteFile(filepath.Join(client.Dir, "junk.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	acts := []deleteAction{{path: "a.txt"}, {path: "junk.txt", unversioned: true}}

	got := msgOf[deletedMsg](t, deleteManyCmd(client, acts, 71))
	if got.outcome.err() != nil || got.outcome.label() != "2 files" {
		t.Errorf("msg = %+v, want a two-file summary", got)
	}

	one := msgOf[deletedMsg](t, deleteManyCmd(client, acts[:1], 72))
	if one.outcome.err() != nil || one.outcome.label() != "a.txt" {
		t.Errorf("msg = %+v, want the sole path named", one)
	}
}

// TestDeleteManyCmdCarriesOnPastARefusal pins that a file svn will not delete no
// longer strands the files listed after it.
func TestDeleteManyCmdCarriesOnPastARefusal(t *testing.T) {
	client, calls := pickyClient(t, "b.txt")
	acts := []deleteAction{{path: "a.txt"}, {path: "b.txt"}, {path: "c.txt"}}

	got := msgOf[deletedMsg](t, deleteManyCmd(client, acts, 73))
	if len(got.outcome.failed) != 1 || got.outcome.failed[0].path != "b.txt" {
		t.Errorf("failed = %+v, want only the file svn refused", got.outcome.failed)
	}
	if len(got.outcome.done) != 2 {
		t.Errorf("done = %q, want the other two deleted regardless", got.outcome.done)
	}
	if argv := strings.Join(calls(), "\n"); !strings.Contains(argv, "c.txt") {
		t.Errorf("svn was never asked to delete c.txt — the refusal blocked it:\n%s", argv)
	}
}

// TestDeleteManyCmdLeavesOutWhatADirectoryTakes pins the overlap both ways of
// deleting recurse into: svn status reports a scheduled-add directory alongside
// every file under it, and naming a child after its parent has gone fails with
// E155007 — which used to sink the rest of the run. The child still counts as
// deleted, because the directory took it.
func TestDeleteManyCmdLeavesOutWhatADirectoryTakes(t *testing.T) {
	client, calls := pickyClient(t)
	acts := []deleteAction{{path: "sub"}, {path: "sub/x.txt"}, {path: "sub/y.txt"}, {path: "other.txt"}}

	got := msgOf[deletedMsg](t, deleteManyCmd(client, acts, 74))
	if got.outcome.err() != nil {
		t.Fatalf("delete: %v", got.outcome.err())
	}
	if len(got.outcome.done) != 4 {
		t.Errorf("done = %q, want every path counted, children included", got.outcome.done)
	}
	argv := calls()
	if len(argv) != 2 {
		t.Errorf("argv = %q, want one delete for sub and one for other.txt", argv)
	}
	if strings.Contains(strings.Join(argv, "\n"), "sub/x.txt") {
		t.Errorf("sub/x.txt was named to svn after its parent had gone:\n%q", argv)
	}
}

func TestUpdateCmds(t *testing.T) {
	got := msgOf[updatedMsg](t, updateCmd(cmdClient(t, "Updated to revision 44.\n", 0)))
	if got.err != nil || got.revision != "44" || got.toRevision {
		t.Errorf("msg = %+v, want a plain update to HEAD", got)
	}
	if failed := msgOf[updatedMsg](t, updateCmd(cmdClient(t, "", 1))); failed.err == nil {
		t.Error("an update svn refused must be reported")
	}

	pinned := msgOf[updatedMsg](t, updateToRevisionCmd(cmdClient(t, "Updated to revision 40.\n", 0), "40"))
	if pinned.err != nil || pinned.revision != "40" || !pinned.toRevision {
		t.Errorf("msg = %+v, want the update marked as targeting a revision", pinned)
	}
	if failed := msgOf[updatedMsg](t, updateToRevisionCmd(cmdClient(t, "", 1), "40")); failed.err == nil {
		t.Error("an update to a revision svn refused must be reported")
	}
}
