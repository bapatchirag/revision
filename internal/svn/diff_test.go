package svn

import (
	"reflect"
	"testing"
)

func TestBinarySkipsNamesTheFilesADiffLeavesOut(t *testing.T) {
	const patch = `Index: text.txt
===================================================================
--- text.txt	(revision 1)
+++ text.txt	(working copy)
@@ -1 +1 @@
-old
+new
Index: bin.dat
===================================================================
Cannot display: file marked as a binary type.
svn:mime-type = application/octet-stream
Index: other.txt
===================================================================
--- other.txt	(revision 1)
+++ other.txt	(working copy)
@@ -1 +1 @@
-a
+b
`
	got := BinarySkips(patch)
	if want := []string{"bin.dat"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BinarySkips = %v, want %v", got, want)
	}
}

func TestBinarySkipsFindsEveryBinaryFile(t *testing.T) {
	const patch = `Index: one.bin
===================================================================
Cannot display: file marked as a binary type.
svn:mime-type = application/octet-stream
Index: two.bin
===================================================================
Cannot display: file marked as a binary type.
svn:mime-type = image/png
`
	got := BinarySkips(patch)
	if want := []string{"one.bin", "two.bin"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BinarySkips = %v, want %v", got, want)
	}
}

func TestBinarySkipsIgnoresATextOnlyDiff(t *testing.T) {
	const patch = `Index: text.txt
===================================================================
--- text.txt	(revision 1)
+++ text.txt	(working copy)
@@ -1 +1 @@
-old
+new
`
	if got := BinarySkips(patch); len(got) != 0 {
		t.Errorf("BinarySkips = %v, want none", got)
	}
}

func TestBinarySkipsIgnoresTheNoticeWithoutAFile(t *testing.T) {
	// The notice only names a file by the Index: header above it, so one with no
	// header before it belongs to nothing and must not be reported.
	if got := BinarySkips(binaryNotice + "\n"); len(got) != 0 {
		t.Errorf("BinarySkips = %v, want none", got)
	}
}

func TestBinarySkipsReadsCarriageReturns(t *testing.T) {
	patch := "Index: bin.dat\r\n===\r\n" + binaryNotice + "\r\n"
	got := BinarySkips(patch)
	if want := []string{"bin.dat"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BinarySkips = %v, want %v", got, want)
	}
}

func TestBinarySkipsOnAnEmptyDiff(t *testing.T) {
	if got := BinarySkips(""); len(got) != 0 {
		t.Errorf("BinarySkips = %v, want none", got)
	}
}
