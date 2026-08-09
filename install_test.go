// Package revision holds the test suite for install.sh, the shell script the
// documented one-liner and the self-update path both run. The script is what
// actually replaces the binary on a user's machine, so it is exercised here as
// a script — run against a local server standing in for the release download —
// rather than reasoned about.
package revision

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// checksums is the release's checksums.txt, in the format goreleaser writes.
func checksums(body string) string {
	sum := sha256.Sum256([]byte(body))
	return fmt.Sprintf("%s  revision-darwin-arm64\n%s  revision-darwin-amd64\n%s  revision-linux-amd64\n%s  revision-linux-arm64\n",
		hex.EncodeToString(sum[:]), hex.EncodeToString(sum[:]),
		hex.EncodeToString(sum[:]), hex.EncodeToString(sum[:]))
}

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
	return releaseServerSigning(t, tag, checksums(installed))
}

// releaseServerSigning is releaseServer with the published checksums.txt spelled
// out, so a release that lists the wrong digest can be served. A sums of ""
// publishes no checksums.txt at all.
func releaseServerSigning(t *testing.T, tag, sums string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/bapatchirag/revision/releases/latest/download/"
		if tag != "" {
			want = "/bapatchirag/revision/releases/download/" + tag + "/"
		}
		name, ok := strings.CutPrefix(r.URL.Path, want)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if name == "checksums.txt" {
			if sums == "" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(sums))
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

	// Staging happens inside the install directory so the move is a rename;
	// nothing of it may survive.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the install directory: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "revision" {
			t.Errorf("install left %q behind", e.Name())
		}
	}
}

func TestInstallScriptStagesInsideTheInstallDirectory(t *testing.T) {
	// A rename is only atomic within one filesystem, so the download has to be
	// staged beside its destination rather than under $TMPDIR. The directory is
	// sampled while the script is blocked on the checksums request, which is the
	// one moment staging is guaranteed to be in progress.
	dir := t.TempDir()
	sums := checksums(installed)
	staged := make(chan []string, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/checksums.txt") {
			staged <- dirNames(dir)
			_, _ = w.Write([]byte(sums))
			return
		}
		_, _ = w.Write([]byte(installed))
	}))
	t.Cleanup(srv.Close)

	out, err := runInstall(t, "REVISION_BASE_URL="+srv.URL, "REVISION_INSTALL_DIR="+dir)
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}

	var during []string
	select {
	case during = <-staged:
	default:
		t.Fatalf("the checksums were never requested\n%s", out)
	}
	found := false
	for _, name := range during {
		if strings.HasPrefix(name, ".revision-install.") {
			found = true
		}
	}
	if !found {
		t.Errorf("the install directory held %v mid-install, want the staging directory among them", during)
	}
}

// dirNames lists what a directory holds, for asserting on work in progress.
func dirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestInstallScriptRejectsAMismatchedChecksum(t *testing.T) {
	dir := t.TempDir()
	out, err := runInstall(t,
		"REVISION_BASE_URL="+releaseServerSigning(t, "", checksums("something else entirely")),
		"REVISION_INSTALL_DIR="+dir,
	)
	if err == nil {
		t.Fatalf("expected a mismatched checksum to exit non-zero\n%s", out)
	}
	if !strings.Contains(out, "checksum mismatch") {
		t.Errorf("output = %q, want the mismatch reported", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "revision")); err == nil {
		t.Error("a mismatched download must not be installed")
	}
}

func TestInstallScriptRefusesWithoutChecksums(t *testing.T) {
	// Fail closed: an unverifiable binary is not one to install over the copy
	// already on the machine, and there is no flag to say otherwise.
	dir := t.TempDir()
	out, err := runInstall(t,
		"REVISION_BASE_URL="+releaseServerSigning(t, "", ""),
		"REVISION_INSTALL_DIR="+dir,
	)
	if err == nil {
		t.Fatalf("expected a release without checksums to exit non-zero\n%s", out)
	}
	if !strings.Contains(out, "cannot verify") {
		t.Errorf("output = %q, want the reason reported", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "revision")); err == nil {
		t.Error("an unverifiable download must not be installed")
	}
}

func TestInstallScriptRefusesWhenTheAssetIsNotListed(t *testing.T) {
	dir := t.TempDir()
	out, err := runInstall(t,
		"REVISION_BASE_URL="+releaseServerSigning(t, "", "abc123  revision-plan9-vax\n"),
		"REVISION_INSTALL_DIR="+dir,
	)
	if err == nil {
		t.Fatalf("expected an unlisted asset to exit non-zero\n%s", out)
	}
	if !strings.Contains(out, "not listed") {
		t.Errorf("output = %q, want the reason reported", out)
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
