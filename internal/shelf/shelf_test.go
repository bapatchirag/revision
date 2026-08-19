package shelf

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDirNameCarriesSubversionsIgnoredPrefix(t *testing.T) {
	// The ".#" prefix is what keeps the store out of svn status and svn diff via
	// Subversion's built-in ".#*" global-ignore. Renaming the directory without
	// it would silently make every shelf show up as an unversioned file.
	if !strings.HasPrefix(DirName, ".#") {
		t.Errorf("DirName = %q, want a %q prefix so svn ignores the store", DirName, ".#")
	}
}

func TestDirIsUnderTheWorkingCopyRoot(t *testing.T) {
	if got, want := Dir("/wc"), filepath.Join("/wc", DirName); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestNewIDLeadsWithTheTimestamp(t *testing.T) {
	at := time.Date(2026, 8, 19, 14, 25, 30, 0, time.UTC)
	if got := NewID(at); !strings.HasPrefix(got, "20260819-142530-") {
		t.Errorf("NewID() = %q, want a 20260819-142530- prefix", got)
	}
}

func TestNewIDsWithinTheSameSecondDiffer(t *testing.T) {
	at := time.Date(2026, 8, 19, 14, 25, 30, 0, time.UTC)
	seen := map[string]bool{}
	for range 100 {
		id := NewID(at)
		if seen[id] {
			t.Fatalf("NewID() repeated %q within the same second", id)
		}
		seen[id] = true
	}
}

func TestNewIDsSortChronologically(t *testing.T) {
	early := NewID(time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC))
	late := NewID(time.Date(2026, 8, 19, 17, 0, 0, 0, time.UTC))
	if early >= late {
		t.Errorf("NewID ordering: %q should sort before %q", early, late)
	}
}

func TestSafeRelRejectsEscapes(t *testing.T) {
	for _, rel := range []string{"", "..", "../outside", "a/../../outside", "/etc/passwd"} {
		if got, err := safeRel(rel); err == nil {
			t.Errorf("safeRel(%q) = %q, want an error", rel, got)
		}
	}
}

func TestSafeRelNormalizes(t *testing.T) {
	got, err := safeRel("a/./b/../c.txt")
	if err != nil {
		t.Fatalf("safeRel: %v", err)
	}
	if want := "a/c.txt"; got != want {
		t.Errorf("safeRel = %q, want %q", got, want)
	}
}

func TestValidIDRejectsPathSeparatorsAndDots(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../escape", "a/b", `a\b`} {
		if err := validID(id); err == nil {
			t.Errorf("validID(%q) = nil, want an error", id)
		}
	}
	if err := validID("20260819-142530-abc123"); err != nil {
		t.Errorf("validID on a real id: %v", err)
	}
}
