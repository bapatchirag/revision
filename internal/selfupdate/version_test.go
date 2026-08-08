package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtoiStrict(t *testing.T) {
	valid := map[string]int{"0": 0, "7": 7, "0042": 42}
	for in, want := range valid {
		got, err := atoiStrict(in)
		if err != nil {
			t.Errorf("atoiStrict(%q): unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("atoiStrict(%q) = %d, want %d", in, got, want)
		}
	}

	// A git-describe fragment must never masquerade as a version component.
	for _, in := range []string{"", "-1", "+1", " 1", "1 ", "1a", "g0a1b2c", "1.0"} {
		if _, err := atoiStrict(in); err == nil {
			t.Errorf("atoiStrict(%q) = nil error, want it rejected", in)
		}
	}
}

func TestComparePre(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "rc.1", 1},  // a release outranks its own pre-releases
		{"rc.1", "", -1}, //
		{"rc.1", "rc.1", 0},
		{"rc.1", "rc.2", -1},
		{"rc.2", "rc.1", 1},
	}
	for _, tc := range cases {
		if got := comparePre(tc.a, tc.b); got != tc.want {
			t.Errorf("comparePre(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSign(t *testing.T) {
	cases := map[int]int{-9: -1, -1: -1, 0: 0, 1: 1, 9: 1}
	for in, want := range cases {
		if got := sign(in); got != want {
			t.Errorf("sign(%d) = %d, want %d", in, got, want)
		}
	}
}

// TestComparePreOrdersPreReleases pins the ordering through the public entry
// point, where it decides whether an update is offered.
func TestComparePreOrdersPreReleases(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0-rc.2", "1.0.0-rc.1", 1},
		{"1.0.0-rc.1", "1.0.0-rc.1", 0},
	}
	for _, tc := range cases {
		got, err := compareVersions(tc.a, tc.b)
		if err != nil {
			t.Fatalf("compareVersions(%q, %q): %v", tc.a, tc.b, err)
		}
		if got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// stubPath makes the returned directory the whole of PATH, with an executable
// stub for each name that records the argv it was called with into argv.txt and
// the pinned version it inherited into env.txt.
func stubPath(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		body := "#!/bin/sh\n" +
			"echo \"$0 $@\" >> " + filepath.Join(dir, "argv.txt") + "\n" +
			"echo \"REVISION_VERSION=$REVISION_VERSION\" >> " + filepath.Join(dir, "env.txt") + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatalf("write stub %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

func TestExecUpdateRequiresItsTool(t *testing.T) {
	stubPath(t) // an empty PATH
	err := execUpdate("curl", nil, "sh", "-c", "true")
	if err == nil || !strings.Contains(err.Error(), "not found on your PATH") {
		t.Errorf("err = %v, want the missing tool named", err)
	}
}

func TestRunNeedsAReleaseToPinTo(t *testing.T) {
	dir := stubPath(t, "go")
	if err := Run(MethodGo, Release{}); err == nil {
		t.Fatal("expected an error without a release to install")
	}
	if _, err := os.Stat(filepath.Join(dir, "argv.txt")); err == nil {
		t.Error("nothing should have been run without a release")
	}
}

func TestRunBuildsTheRightCommand(t *testing.T) {
	rel := Release{Tag: "v1.4.0", Version: "1.4.0"}

	t.Run("go install", func(t *testing.T) {
		dir := stubPath(t, "go")
		if err := Run(MethodGo, rel); err != nil {
			t.Fatalf("Run(MethodGo): %v", err)
		}
		got := readFile(t, filepath.Join(dir, "argv.txt"))
		if !strings.Contains(got, "install") || !strings.Contains(got, goModule+"@"+rel.Tag) {
			t.Errorf("argv = %q, want `go install %s@%s`", got, goModule, rel.Tag)
		}
		if strings.Contains(got, "@latest") {
			t.Errorf("argv = %q, want the approved tag rather than @latest", got)
		}
	})

	t.Run("curl", func(t *testing.T) {
		dir := stubPath(t, "curl", "sh")
		if err := Run(MethodCurl, rel); err != nil {
			t.Fatalf("Run(MethodCurl): %v", err)
		}
		got := readFile(t, filepath.Join(dir, "argv.txt"))
		if !strings.Contains(got, "-c") || !strings.Contains(got, installURL) {
			t.Errorf("argv = %q, want the install script piped through sh", got)
		}
		env := readFile(t, filepath.Join(dir, "env.txt"))
		if !strings.Contains(env, "REVISION_VERSION="+rel.Tag) {
			t.Errorf("env = %q, want the install script pinned to %s", env, rel.Tag)
		}
	})
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the stub was never invoked: %v", err)
	}
	return string(b)
}
