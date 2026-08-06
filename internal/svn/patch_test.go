package svn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePatchOutputClassifiesTargets(t *testing.T) {
	out := `U         a.txt
 U        props.txt
A         c.txt
D         gone.txt
C         sub/b.txt
>         rejected hunk @@ -1,1 +1,1 @@
Skipped missing target: 'elsewhere/d.txt'
Summary of conflicts:
  Text conflicts: 1
`
	res := parsePatchOutput(out)

	wantApplied := []string{"a.txt", "props.txt", "c.txt", "gone.txt"}
	if got := res.Applied; !equalPaths(got, wantApplied) {
		t.Errorf("Applied = %v, want %v", got, wantApplied)
	}
	if got := res.Conflicted; !equalPaths(got, []string{"sub/b.txt"}) {
		t.Errorf("Conflicted = %v, want [sub/b.txt]", got)
	}
	if got := res.Skipped; !equalPaths(got, []string{"elsewhere/d.txt"}) {
		t.Errorf("Skipped = %v, want [elsewhere/d.txt]", got)
	}
	if got := res.Targets(); got != 6 {
		t.Errorf("Targets() = %d, want 6", got)
	}
}

func TestParsePatchOutputEmpty(t *testing.T) {
	if got := parsePatchOutput("").Targets(); got != 0 {
		t.Errorf("a patch svn read nothing from should name no targets, got %d", got)
	}
}

func TestPatchBelongsToFindsTheDirectoryItWasTakenFrom(t *testing.T) {
	patch := `Index: a.txt
===================================================================
--- a.txt	(revision 1)
+++ a.txt	(working copy)
@@ -1 +1 @@
-one
+ONE
Index: sub/b.txt
===================================================================
--- sub/b.txt	(revision 1)
+++ sub/b.txt	(working copy)
@@ -1 +1 @@
-x
+y
`
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a.txt", "sub/b.txt"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(p)), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if !PatchBelongsTo(patch, root) {
		t.Error("a patch whose targets are all in the directory belongs to it")
	}
	if PatchBelongsTo(patch, filepath.Join(root, "sub")) {
		t.Error("the same patch is relative to the root, not to sub — it does not belong there")
	}
	if PatchBelongsTo(patch, t.TempDir()) {
		t.Error("a directory holding none of the patch's targets is not where it came from")
	}
}

func TestPatchBelongsToAcceptsAdditionOnlyPatch(t *testing.T) {
	patch := `Index: c.txt
===================================================================
--- c.txt	(nonexistent)
+++ c.txt	(working copy)
@@ -0,0 +1 @@
+new
`
	if !PatchBelongsTo(patch, t.TempDir()) {
		t.Error("a patch that only adds files names nothing that must already be there")
	}
}

func TestPatchTargetsSkipsCreatedFiles(t *testing.T) {
	patch := "--- a.txt\t(revision 1)\n--- c.txt\t(nonexistent)\n--- /dev/null\n"
	if got := patchTargets(patch); !equalPaths(got, []string{"a.txt"}) {
		t.Errorf("patchTargets = %v, want [a.txt]", got)
	}
}

func equalPaths(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
