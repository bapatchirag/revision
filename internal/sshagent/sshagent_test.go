package sshagent

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cases := map[string]struct {
		in   string
		want string
	}{
		"blank":            {"", ""},
		"whitespace":       {"   ", ""},
		"tilde only":       {"~", home},
		"tilde slash":      {"~/.ssh/id_rsa", filepath.Join(home, ".ssh", "id_rsa")},
		"tilde trimmed":    {"  ~/.ssh/id_rsa  ", filepath.Join(home, ".ssh", "id_rsa")},
		"absolute":         {"/etc/ssh/key", "/etc/ssh/key"},
		"relative":         {"keys/id_rsa", "keys/id_rsa"},
		"tilde other user": {"~alice/.ssh/id_rsa", "~alice/.ssh/id_rsa"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := expandPath(tc.in)
			if err != nil {
				t.Fatalf("expandPath(%q): unexpected error %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("expandPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFingerprintListed(t *testing.T) {
	const fingerprint = "SHA256:abc123"
	cases := map[string]struct {
		listing string
		want    bool
	}{
		"present": {
			"4096 SHA256:abc123 me@host (RSA)",
			true,
		},
		"present among many": {
			"256 SHA256:zzz other@host (ED25519)\n" +
				"4096 SHA256:abc123 me@host (RSA)",
			true,
		},
		"comment differs from path but fingerprint matches": {
			// ssh-add labels the identity with the key's comment, not its file
			// path — this is the case a path-based match got wrong.
			"4096 SHA256:abc123 /home/me/.ssh/id_rsa (RSA)",
			true,
		},
		"absent": {
			"256 SHA256:zzz me@host (ED25519)",
			false,
		},
		"substring is not a match": {
			"4096 SHA256:abc1234 me@host (RSA)",
			false,
		},
		"no identities": {
			"The agent has no identities.",
			false,
		},
		"empty": {"", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := fingerprintListed(tc.listing, fingerprint); got != tc.want {
				t.Errorf("fingerprintListed(%q, %q) = %v, want %v", tc.listing, fingerprint, got, tc.want)
			}
		})
	}
}

// TestKeyFingerprint checks that keyFingerprint returns the same SHA256
// fingerprint ssh-add reports, for a key whose comment is not its file path —
// the exact shape that made the old path-based check re-prompt for an
// already-loaded key.
func TestKeyFingerprint(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not available")
	}
	dir := t.TempDir()
	key := filepath.Join(dir, "id_ed25519")
	// A deliberately non-path comment, as a normally generated key carries.
	gen := exec.Command("ssh-keygen", "-t", "ed25519", "-f", key, "-N", "", "-C", "someone@example.com")
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen generate: %v\n%s", err, out)
	}

	ctx := context.Background()
	got, err := keyFingerprint(ctx, key)
	if err != nil {
		t.Fatalf("keyFingerprint: %v", err)
	}
	if !strings.HasPrefix(got, "SHA256:") {
		t.Fatalf("keyFingerprint = %q, want a SHA256:… fingerprint", got)
	}

	// The fingerprint must match whether computed from the private key or its
	// .pub, and it must be what a synthesized ssh-add -l line (labelled with the
	// comment, not the path) is recognized by.
	fromPub, err := keyFingerprint(ctx, key+".pub")
	if err != nil {
		t.Fatalf("keyFingerprint(.pub): %v", err)
	}
	if fromPub != got {
		t.Errorf("fingerprint from .pub = %q, from private key = %q; want equal", fromPub, got)
	}
	listing := "256 " + got + " someone@example.com (ED25519)"
	if !fingerprintListed(listing, got) {
		t.Errorf("fingerprintListed(%q, %q) = false, want true", listing, got)
	}
}

func TestIsAskpass(t *testing.T) {
	t.Setenv(askpassSentinelEnv, "")
	if IsAskpass() {
		t.Error("IsAskpass() = true with an empty sentinel, want false")
	}
	t.Setenv(askpassSentinelEnv, "1")
	if !IsAskpass() {
		t.Error("IsAskpass() = false with the sentinel set, want true")
	}
}

func TestRunAskpassAnswersOnce(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "once")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	t.Setenv(askpassValueEnv, "s3cret")
	t.Setenv(askpassOnceEnv, marker)

	// The first call answers with the passphrase and consumes the marker.
	if got := strings.TrimSpace(captureStdout(t, RunAskpass)); got != "s3cret" {
		t.Errorf("first RunAskpass = %q, want the passphrase", got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("first RunAskpass should consume the once-marker")
	}
	// A retry (what ssh-add does after a wrong passphrase) must answer empty so
	// ssh-add stops instead of looping.
	if got := strings.TrimSpace(captureStdout(t, RunAskpass)); got != "" {
		t.Errorf("second RunAskpass = %q, want empty (single-use)", got)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns what it
// wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = orig
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read helper output: %v", err)
	}
	return string(out)
}
