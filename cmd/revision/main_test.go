package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bapatchirag/revision/internal/selfupdate"
)

// stubPath makes dir the whole of PATH, installing an executable shell script
// for each name whose body is the mapped value. It is how the svn and update
// tools the CLI shells out to are simulated.
func stubPath(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatalf("write stub %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)
}

// stubInstalled puts a stand-in for the installed binary next to the test
// binary. That directory is where the update lands and where selfupdate re-runs
// it to confirm the new version is really in place.
func stubInstalled(t *testing.T, version string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary: %v", err)
	}
	path := filepath.Join(filepath.Dir(exe), "revision")
	body := "#!/bin/sh\necho 'revision " + version + "'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Skipf("cannot write next to the test binary: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
}

// withStdin replaces os.Stdin with a pipe carrying input for the duration of fn.
func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = orig
		_ = r.Close()
	}()
	go func() {
		_, _ = io.WriteString(w, input)
		_ = w.Close()
	}()
	fn()
}

// captureOutput redirects both standard streams for the duration of fn and
// returns everything written to them.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	outOrig, errOrig := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	fn()
	os.Stdout, os.Stderr = outOrig, errOrig
	_ = w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured output: %v", err)
	}
	return string(b)
}

func TestCheckFlags(t *testing.T) {
	cases := []struct {
		name       string
		doUpdate   bool
		updateWith string
		wantErr    bool
	}{
		{"no flags", false, "", false},
		{"update alone", true, "", false},
		{"update with a method", true, "curl", false},
		// Without --update the method was read by nobody and the TUI launched
		// as if it had not been asked for.
		{"method without update", false, "go", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkFlags(tc.doUpdate, tc.updateWith)
			if tc.wantErr && err == nil {
				t.Fatal("expected the combination to be rejected")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("checkFlags: %v", err)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "--update --update-with "+tc.updateWith) {
				t.Errorf("err = %v, want the working command spelled out", err)
			}
		})
	}
}

func TestUsageNamesTheFlags(t *testing.T) {
	// flag.PrintDefaults reads the command line's own flag set, which the test
	// binary owns, so give usage a set holding the real flags.
	orig := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = orig })
	fs := flag.NewFlagSet("revision", flag.ContinueOnError)
	fs.String("path", ".", "path to the SVN working copy to operate on")
	fs.Bool("version", false, "print version and exit")
	fs.Bool("update", false, "check for a newer release and update the binary")
	fs.String("update-with", "", "update method for --update: 'curl' or 'go' (default: prompt)")
	flag.CommandLine = fs

	got := captureOutput(t, usage)
	for _, want := range []string{"revision", "Usage:", "-path", "-version", "-update", "-update-with"} {
		if !strings.Contains(got, want) {
			t.Errorf("usage output missing %q:\n%s", want, got)
		}
	}
}

func TestRunRequiresSVNOnPath(t *testing.T) {
	stubPath(t, nil)
	err := run(".", selfupdate.Build{Version: "dev", Channel: "dev"})
	if err == nil || !strings.Contains(err.Error(), "not found on your PATH") {
		t.Errorf("err = %v, want the missing svn reported", err)
	}
}

func TestRunRejectsANonWorkingCopy(t *testing.T) {
	stubPath(t, map[string]string{
		"svn": "echo \"svn: E155007: '/tmp' is not a working copy\" >&2\nexit 1\n",
	})
	err := run(t.TempDir(), selfupdate.Build{Version: "dev", Channel: "dev"})
	if err == nil || !strings.Contains(err.Error(), "does not appear to be an SVN working copy") {
		t.Errorf("err = %v, want the directory rejected", err)
	}
}

func TestRunSurfacesAnAuthFailureAsAHint(t *testing.T) {
	stubPath(t, map[string]string{
		"svn": "echo 'svn: E170013: Authentication failed' >&2\nexit 1\n",
	})
	err := run(t.TempDir(), selfupdate.Build{Version: "dev", Channel: "dev"})
	if err == nil || !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("err = %v, want the actionable auth hint", err)
	}
}

func TestRunUpdateIsInertOnADevelopmentBuild(t *testing.T) {
	var err error
	got := captureOutput(t, func() {
		err = runUpdate(selfupdate.Build{Version: "dev", Channel: "dev"}, "curl")
	})
	if err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
	if !strings.Contains(got, "development build") {
		t.Errorf("output = %q, want the development build explained", got)
	}
}

func TestResolveMethod(t *testing.T) {
	cases := map[string]struct {
		in      string
		want    selfupdate.Method
		wantOK  bool
		wantErr bool
	}{
		"curl":       {"curl", selfupdate.MethodCurl, true, false},
		"go":         {"go", selfupdate.MethodGo, true, false},
		"mixed case": {"  CURL ", selfupdate.MethodCurl, true, false},
		"unknown":    {"brew", 0, false, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok, err := resolveMethod(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveMethod(%q) = nil error, want it rejected", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveMethod(%q): %v", tc.in, err)
			}
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("resolveMethod(%q) = %v/%v, want %v/%v", tc.in, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestResolveMethodFallsBackToThePrompt(t *testing.T) {
	withStdin(t, "2\n", func() {
		var (
			got selfupdate.Method
			ok  bool
			err error
		)
		captureOutput(t, func() { got, ok, err = resolveMethod("") })
		if err != nil {
			t.Fatalf("resolveMethod(\"\"): %v", err)
		}
		if !ok || got != selfupdate.MethodGo {
			t.Errorf("resolveMethod(\"\") = %v/%v, want the prompted go method", got, ok)
		}
	})
}

func TestPromptMethod(t *testing.T) {
	cases := map[string]struct {
		input  string
		want   selfupdate.Method
		wantOK bool
	}{
		"curl":         {"1\n", selfupdate.MethodCurl, true},
		"go":           {"2\n", selfupdate.MethodGo, true},
		"declined":     {"3\n", 0, false},
		"unrecognized": {"nope\n", 0, false},
		"eof":          {"", 0, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			withStdin(t, tc.input, func() {
				var (
					got selfupdate.Method
					ok  bool
				)
				out := captureOutput(t, func() { got, ok = promptMethod() })
				if !strings.Contains(out, "Select [1-3]") {
					t.Errorf("prompt = %q, want the choices offered", out)
				}
				if got != tc.want || ok != tc.wantOK {
					t.Errorf("promptMethod() = %v/%v, want %v/%v", got, ok, tc.want, tc.wantOK)
				}
			})
		})
	}
}

func TestApplyUpdate(t *testing.T) {
	rel := selfupdate.Release{Tag: "v1.4.0", Version: "1.4.0"}

	stubInstalled(t, rel.Version)
	stubPath(t, map[string]string{"go": "exit 0\n"})
	var err error
	out := captureOutput(t, func() { err = applyUpdate(selfupdate.MethodGo, rel) })
	if err != nil {
		t.Fatalf("applyUpdate: %v", err)
	}
	if !strings.Contains(out, rel.Tag) {
		t.Errorf("output = %q, want the release being installed named", out)
	}
	if !strings.Contains(out, "Update complete") {
		t.Errorf("output = %q, want the restart hint", out)
	}

	stubPath(t, nil)
	captureOutput(t, func() { err = applyUpdate(selfupdate.MethodGo, rel) })
	if err == nil || !strings.Contains(err.Error(), "update failed") {
		t.Errorf("err = %v, want the missing tool reported as a failed update", err)
	}
}
