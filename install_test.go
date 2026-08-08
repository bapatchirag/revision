package revision
// Package revision holds the test suite for install.sh, the shell script the
// documented one-liner and the self-update path both run. The script is what
// actually replaces the binary on a user's machine, so it is exercised here as
// a script — run against a local server standing in for the release download —
// rather than reasoned about.
package revision

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// installed is the body of the fake release asset the stub server hands out. It
// only has to be something the script can make executable and move.
const installed = "#!/bin/sh\necho 'revision 9.9.9'\n"

// shBin is the absolute path to a POSIX shell, resolved before any test starts
// rewriting PATH.
func shBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not found on PATH")
	}
	return path
}

// releaseServer serves the release assets for tag, or every tag when it is
// empty. It returns the base URL to point the script at.
func releaseServer(t *testing.T, tag string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/bapatchirag/revision/releases/latest/download/"
		if tag != "" {
			want = "/bapatchirag/revision/releases/download/" + tag + "/"
		}
		if !strings.HasPrefix(r.URL.Path, want) {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(installed))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// runInstall runs the script with the given environment added, returning its
// combined output and error.
func runInstall(t *testing.T, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(shBin(t), "install.sh")
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallScriptInstallsTheAsset(t *testing.T) {
	dir := t.TempDir()
	out, err := runInstall(t,
		"REVISION_BASE_URL="+releaseServer(t, ""),
		"REVISION_INSTALL_DIR="+dir,
	)
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}

	dest := filepath.Join(dir, "revision")
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read the installed binary: %v\n%s", err, out)
	}
	if string(body) != installed {
		t.Errorf("installed %q, want the downloaded asset", body)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat the installed binary: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("installed mode = %v, want it executable", info.Mode().Perm())
	}
	if !strings.Contains(out, dest) {
		t.Errorf("output = %q, want the install location reported", out)
	}
}

func TestInstallScriptHonoursTheRequestedVersion(t *testing.T) {
	// The server serves v1.4.0 alone, so the script reaching it at all is the
	// evidence that the pinned tag ends up in the URL.
	dir := t.TempDir()
	out, err := runInstall(t,
		"REVISION_BASE_URL="+releaseServer(t, "v1.4.0"),
		"REVISION_INSTALL_DIR="+dir,
		"REVISION_VERSION=v1.4.0",
	)
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "revision")); err != nil {
		t.Errorf("the pinned release was not installed: %v\n%s", err, out)
	}
}

func TestInstallScriptReportsAFailedDownload(t *testing.T) {
	dir := t.TempDir()
	out, err := runInstall(t,
		"REVISION_BASE_URL="+releaseServer(t, "v1.4.0"),
		"REVISION_INSTALL_DIR="+dir,
		"REVISION_VERSION=v9.9.9", // never served, so the download 404s
	)
	if err == nil {
		t.Fatalf("expected a failed download to exit non-zero\n%s", out)
	}
	if !strings.Contains(out, "download failed") {
		t.Errorf("output = %q, want the failure reported", out)
	}
	if !strings.Contains(out, "go install") {
		t.Errorf("output = %q, want the build-from-source fallback offered", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "revision")); err == nil {
		t.Error("a failed download must not leave a binary behind")
	}
}

func TestInstallScriptReportsAnUnwritableDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write to any directory")
	}

	out, err := runInstall(t,
		"REVISION_BASE_URL="+releaseServer(t, ""),
		"REVISION_INSTALL_DIR="+dir,
	)
	if err == nil {
		t.Fatalf("expected an unwritable directory to exit non-zero\n%s", out)
	}
	if !strings.Contains(out, "REVISION_INSTALL_DIR") {
		t.Errorf("output = %q, want the way out named", out)
	}
}

func TestInstallScriptRejectsAnUnsupportedHost(t *testing.T) {
	cases := []struct {
		name   string
		uname  string
		reason string
	}{
		{"os", "echo Plan9\n", "unsupported OS"},
		{"arch", "echo Linux\n", "unsupported architecture"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// uname reports the machine for -m and the system otherwise; a stub
			// that ignores its argument answers the same for both, so a stub
			// naming an OS fails the architecture check and vice versa.
			stub := t.TempDir()
			if err := os.WriteFile(filepath.Join(stub, "uname"), []byte("#!/bin/sh\n"+tc.uname), 0o755); err != nil {
				t.Fatalf("write uname stub: %v", err)
			}

			out, err := runInstall(t, "PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"))
			if err == nil {
				t.Fatalf("expected an unsupported host to exit non-zero\n%s", out)
			}
			if !strings.Contains(out, tc.reason) {
				t.Errorf("output = %q, want %q", out, tc.reason)
			}
		})
	}
}
