package sshagent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubTools installs the named shell scripts as the only executables on PATH, so
// KeyLoaded and AddKey talk to a simulated ssh-add / ssh-keygen instead of the
// real ones. A name mapped to an empty body is left off PATH entirely, which is
// how the "tool not installed" branches are reached.
func stubTools(t *testing.T, scripts map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		if body == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatalf("write stub %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

// keyFile writes a placeholder private key and returns its path. The stubs never
// read it; it exists only so the paths under test are real.
func keyFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, []byte("PRIVATE KEY\n"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return path
}

// sleepBin is the absolute path to sleep. A stub that has to block cannot just
// say "sleep": stubTools makes its directory the whole of PATH, and sleep is an
// external binary, so the stub's shell would not find it. Resolve it here, while
// the real PATH is still in place.
func sleepBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not found on PATH")
	}
	return path
}

const stubFingerprint = "SHA256:abc123"

// listsFingerprint is an ssh-keygen stub reporting stubFingerprint.
const keygenPrints = "echo '256 " + stubFingerprint + " me@host (ED25519)'\n"

func TestKeyLoadedRejectsAnEmptyPath(t *testing.T) {
	if _, err := KeyLoaded(context.Background(), "   "); err == nil {
		t.Error("an empty key path cannot be checked and must error")
	}
}

func TestKeyLoadedNeedsItsTools(t *testing.T) {
	key := keyFile(t)

	t.Run("no ssh-add", func(t *testing.T) {
		stubTools(t, map[string]string{"ssh-keygen": keygenPrints})
		_, err := KeyLoaded(context.Background(), key)
		if err == nil || !strings.Contains(err.Error(), "ssh-add not found") {
			t.Errorf("err = %v, want the missing ssh-add reported", err)
		}
	})

	t.Run("no ssh-keygen", func(t *testing.T) {
		stubTools(t, map[string]string{"ssh-add": "exit 0\n"})
		_, err := KeyLoaded(context.Background(), key)
		if err == nil || !strings.Contains(err.Error(), "ssh-keygen not found") {
			t.Errorf("err = %v, want the missing ssh-keygen reported", err)
		}
	})
}

func TestKeyFingerprintFailures(t *testing.T) {
	key := keyFile(t)

	t.Run("ssh-keygen refuses the key", func(t *testing.T) {
		stubTools(t, map[string]string{
			"ssh-add":    "exit 0\n",
			"ssh-keygen": "echo 'is not a public key file' >&2\nexit 1\n",
		})
		_, err := KeyLoaded(context.Background(), key)
		if err == nil || !strings.Contains(err.Error(), "not a public key file") {
			t.Errorf("err = %v, want ssh-keygen's own stderr", err)
		}
	})

	t.Run("ssh-keygen fails silently", func(t *testing.T) {
		stubTools(t, map[string]string{
			"ssh-add":    "exit 0\n",
			"ssh-keygen": "exit 1\n",
		})
		_, err := KeyLoaded(context.Background(), key)
		if err == nil || !strings.Contains(err.Error(), "ssh-keygen -lf") {
			t.Errorf("err = %v, want the exec error stand in for the missing stderr", err)
		}
	})

	t.Run("unexpected output", func(t *testing.T) {
		stubTools(t, map[string]string{
			"ssh-add":    "exit 0\n",
			"ssh-keygen": "echo 256\n",
		})
		_, err := KeyLoaded(context.Background(), key)
		if err == nil || !strings.Contains(err.Error(), "unexpected output") {
			t.Errorf("err = %v, want the short listing rejected", err)
		}
	})
}

// TestKeyFingerprintPrefersThePublicHalf checks that a .pub sitting next to the
// key is what ssh-keygen is pointed at, since it can be read without the
// passphrase.
func TestKeyFingerprintPrefersThePublicHalf(t *testing.T) {
	key := keyFile(t)
	if err := os.WriteFile(key+".pub", []byte("ssh-ed25519 AAAA me@host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubTools(t, map[string]string{
		// Echo back the path ssh-keygen was handed, in the field the parser reads.
		"ssh-keygen": "echo \"256 $2 me@host (ED25519)\"\n",
	})

	got, err := keyFingerprint(context.Background(), key)
	if err != nil {
		t.Fatalf("keyFingerprint: %v", err)
	}
	if got != key+".pub" {
		t.Errorf("fingerprinted %q, want the .pub beside the key", got)
	}
}

func TestKeyLoaded(t *testing.T) {
	key := keyFile(t)

	cases := map[string]struct {
		sshAdd     string
		want       bool
		wantErr    string
		wantNoFail bool
	}{
		"listed": {
			sshAdd: "echo '256 " + stubFingerprint + " me@host (ED25519)'\n",
			want:   true,
		},
		"another key is loaded": {
			sshAdd: "echo '256 SHA256:zzz other@host (ED25519)'\n",
			want:   false,
		},
		"agent holds no identities": {
			// ssh-add -l exits 1 when reachable but empty: a definitive "not loaded".
			sshAdd: "echo 'The agent has no identities.'\nexit 1\n",
			want:   false,
		},
		"agent unreachable": {
			sshAdd:  "echo 'Could not open a connection to your authentication agent.' >&2\nexit 2\n",
			wantErr: "authentication agent",
		},
		"unreachable without stderr": {
			sshAdd:  "exit 2\n",
			wantErr: "ssh-add -l",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			stubTools(t, map[string]string{"ssh-add": tc.sshAdd, "ssh-keygen": keygenPrints})
			got, err := KeyLoaded(context.Background(), key)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("KeyLoaded: %v", err)
			}
			if got != tc.want {
				t.Errorf("KeyLoaded = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAddKeyRejectsAnEmptyPath(t *testing.T) {
	if err := AddKey(context.Background(), "  ", "hunter2"); err == nil {
		t.Error("an empty key path cannot be added and must error")
	}
}

func TestAddKeyNeedsSSHAdd(t *testing.T) {
	stubTools(t, nil)
	err := AddKey(context.Background(), keyFile(t), "hunter2")
	if err == nil || !strings.Contains(err.Error(), "ssh-add not found") {
		t.Errorf("err = %v, want the missing ssh-add reported", err)
	}
}

func TestAddKey(t *testing.T) {
	key := keyFile(t)

	cases := map[string]struct {
		sshAdd  string
		wantErr string
		unreach bool
	}{
		"loaded": {sshAdd: "exit 0\n"},
		"wrong passphrase": {
			sshAdd:  "echo 'Bad passphrase, try again' >&2\nexit 1\n",
			wantErr: "Bad passphrase",
		},
		"failed without stderr": {
			sshAdd:  "exit 1\n",
			wantErr: "ssh-add",
		},
		"agent down by exit code": {
			sshAdd:  "exit 2\n",
			unreach: true,
		},
		"agent down by message": {
			sshAdd:  "echo 'Could not open a connection to your authentication agent.' >&2\nexit 1\n",
			unreach: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			stubTools(t, map[string]string{"ssh-add": tc.sshAdd})
			err := AddKey(context.Background(), key, "hunter2")
			switch {
			case tc.unreach:
				if !errors.Is(err, ErrAgentUnreachable) {
					t.Errorf("err = %v, want ErrAgentUnreachable", err)
				}
			case tc.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("err = %v, want it to mention %q", err, tc.wantErr)
				}
			default:
				if err != nil {
					t.Errorf("AddKey: %v", err)
				}
			}
		})
	}
}

func TestAddKeyReportsATimeout(t *testing.T) {
	sleep := sleepBin(t)
	key := keyFile(t)
	stubTools(t, map[string]string{"ssh-add": sleep + " 10\n"})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := AddKey(ctx, key, "hunter2")
	if err == nil || !strings.Contains(err.Error(), "timed out adding") {
		t.Errorf("err = %v, want the deadline reported as a timeout", err)
	}
}

func TestExitCode(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("false not found on PATH")
	}
	err := exec.Command("false").Run()
	if got := exitCode(err); got != 1 {
		t.Errorf("exitCode(%v) = %d, want 1", err, got)
	}
	if got := exitCode(errors.New("not an exit error")); got != -1 {
		t.Errorf("exitCode(plain error) = %d, want -1", got)
	}
}
