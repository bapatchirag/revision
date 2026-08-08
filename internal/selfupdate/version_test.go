package selfupdate

import (
	"errors"
	"net/http"
	"net/http/httptest"
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
// the update environment it inherited into env.txt.
func stubPath(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		body := "#!/bin/sh\n" +
			"echo \"$0 $@\" >> " + filepath.Join(dir, "argv.txt") + "\n" +
			"echo \"REVISION_VERSION=$REVISION_VERSION\" >> " + filepath.Join(dir, "env.txt") + "\n" +
			"echo \"REVISION_INSTALL_DIR=$REVISION_INSTALL_DIR\" >> " + filepath.Join(dir, "env.txt") + "\n" +
			"echo \"GOBIN=$GOBIN\" >> " + filepath.Join(dir, "env.txt") + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatalf("write stub %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

// stubExecutable pretends the running binary lives at path.
func stubExecutable(t *testing.T, path string) {
	t.Helper()
	prev := executablePath
	executablePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { executablePath = prev })
}

// failPath is stubPath with stubs that exit non-zero, so a failing update can be
// checked for being reported as one.
func failPath(t *testing.T, names ...string) string {
	t.Helper()
	dir := stubPath(t, names...)
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
			t.Fatalf("write failing stub %s: %v", name, err)
		}
	}
	return dir
}

// withRaw serves body as the install script published with tag, for the
// duration of a test.
func withRaw(t *testing.T, tag, body string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+repo+"/"+tag+"/install.sh" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(srv.Close)

	prev := rawBase
	rawBase = srv.URL
	t.Cleanup(func() { rawBase = prev })
}

func TestFetchInstallScriptTakesItFromTheTag(t *testing.T) {
	const want = "#!/bin/sh\necho installed\n"
	withRaw(t, "v1.4.0", want, http.StatusOK)

	path, cleanup, err := fetchInstallScript(Release{Tag: "v1.4.0"})
	if err != nil {
		t.Fatalf("fetchInstallScript: %v", err)
	}
	if got := readFile(t, path); got != want {
		t.Errorf("script = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat script: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("script mode = %v, want it readable by its owner alone", info.Mode().Perm())
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the downloaded script should not outlive the update")
	}
}

func TestFetchInstallScriptReportsAFailedDownload(t *testing.T) {
	// The script is served for v1.4.0 only, so another tag 404s — as a release
	// published before the script was pinned would.
	withRaw(t, "v1.4.0", "", http.StatusOK)

	if _, _, err := fetchInstallScript(Release{Tag: "v9.9.9"}); err == nil {
		t.Fatal("expected an error when the script cannot be downloaded")
	} else if !strings.Contains(err.Error(), "v9.9.9") {
		t.Errorf("err = %v, want the tag named", err)
	}
}

func TestRunReportsAFailingInstallScript(t *testing.T) {
	// A `curl … | sh` pipeline exits with the status of the shell, which is 0
	// even when the download produced nothing. Running the script directly is
	// what makes its failure visible.
	failPath(t, "sh")
	stubExecutable(t, filepath.Join(t.TempDir(), "revision"))
	withRaw(t, "v1.4.0", "#!/bin/sh\nexit 3\n", http.StatusOK)

	if err := Run(MethodCurl, Release{Tag: "v1.4.0"}); err == nil {
		t.Fatal("expected a failing install script to be reported")
	}
}

func TestInstallDirFollowsTheRunningBinary(t *testing.T) {
	home := t.TempDir()
	stubExecutable(t, filepath.Join(home, "revision"))
	got, err := installDir()
	if err != nil {
		t.Fatalf("installDir: %v", err)
	}
	if got != home {
		t.Errorf("installDir() = %q, want %q", got, home)
	}
}

func TestInstallDirResolvesSymlinks(t *testing.T) {
	// A binary reached through a link on the PATH must still be replaced where
	// it really lives, or the link keeps pointing at the old file.
	real := t.TempDir()
	linked := t.TempDir()
	target := filepath.Join(real, "revision")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(linked, "revision")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	stubExecutable(t, link)
	got, err := installDir()
	if err != nil {
		t.Fatalf("installDir: %v", err)
	}
	want, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatalf("resolve want: %v", err)
	}
	if got != want {
		t.Errorf("installDir() = %q, want the link's target directory %q", got, want)
	}
}

func TestRunReportsAnUnlocatableBinary(t *testing.T) {
	dir := stubPath(t, "go")
	prev := executablePath
	executablePath = func() (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { executablePath = prev })

	err := Run(MethodGo, Release{Tag: "v1.4.0"})
	if err == nil || !strings.Contains(err.Error(), "locating the running binary") {
		t.Errorf("err = %v, want the lookup failure reported", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "argv.txt")); err == nil {
		t.Error("nothing should have been run without a target directory")
	}
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
		home := t.TempDir()
		stubExecutable(t, filepath.Join(home, "revision"))
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
		env := readFile(t, filepath.Join(dir, "env.txt"))
		if !strings.Contains(env, "GOBIN="+home) {
			t.Errorf("env = %q, want GOBIN aimed at %s", env, home)
		}
	})

	t.Run("curl", func(t *testing.T) {
		dir := stubPath(t, "sh")
		home := t.TempDir()
		stubExecutable(t, filepath.Join(home, "revision"))
		withRaw(t, rel.Tag, "#!/bin/sh\nexit 0\n", http.StatusOK)
		if err := Run(MethodCurl, rel); err != nil {
			t.Fatalf("Run(MethodCurl): %v", err)
		}
		got := readFile(t, filepath.Join(dir, "argv.txt"))
		if !strings.Contains(got, "install.sh") {
			t.Errorf("argv = %q, want the downloaded script run directly", got)
		}
		if strings.Contains(got, "|") {
			t.Errorf("argv = %q, want no pipeline to swallow the exit status", got)
		}
		env := readFile(t, filepath.Join(dir, "env.txt"))
		if !strings.Contains(env, "REVISION_VERSION="+rel.Tag) {
			t.Errorf("env = %q, want the install script pinned to %s", env, rel.Tag)
		}
		if !strings.Contains(env, "REVISION_INSTALL_DIR="+home) {
			t.Errorf("env = %q, want the install script aimed at %s", env, home)
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
