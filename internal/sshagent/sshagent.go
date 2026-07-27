// Package sshagent inspects the running ssh-agent so revision can tell whether
// the SSH key used for svn+ssh access is already loaded before it starts talking
// to a remote repository. It is deliberately domain-agnostic: it knows nothing
// about the TUI, the SVN layer, or the app composition, and simply shells out to
// the system ssh-add and ssh-keygen.
package sshagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// KeyLoaded reports whether the SSH key at keyPath is currently held by the
// running ssh-agent. It fingerprints the key with `ssh-keygen -lf` — expanding a
// leading ~ to the user's home directory — and looks for that fingerprint among
// the identities `ssh-add -l` lists.
//
// Matching on the fingerprint, rather than the key path, is what makes the check
// reliable: `ssh-add -l` labels each identity with the key's comment (typically
// user@host, taken from the .pub file), not its file path, so a path match would
// miss any normally generated key and make revision prompt to add a key that is
// in fact already loaded.
//
// A reachable agent that holds no identities is not an error: it simply means
// the key is not loaded, so KeyLoaded returns (false, nil). An unreachable agent
// or a missing ssh-add binary is returned as an error, since the key's state
// cannot be determined.
func KeyLoaded(ctx context.Context, keyPath string) (bool, error) {
	path, err := expandPath(keyPath)
	if err != nil {
		return false, err
	}
	if path == "" {
		return false, errors.New("ssh key path is empty")
	}
	if _, err := exec.LookPath("ssh-add"); err != nil {
		return false, fmt.Errorf("ssh-add not found on PATH: %w", err)
	}

	fingerprint, err := keyFingerprint(ctx, path)
	if err != nil {
		return false, err
	}

	cmd := exec.CommandContext(ctx, "ssh-add", "-l")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// ssh-add -l exits 1 when the agent is reachable but holds no
		// identities; that is a definitive "not loaded", not a failure.
		if exitCode(err) == 1 {
			return false, nil
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return false, fmt.Errorf("ssh-add -l: %s", msg)
	}

	return fingerprintListed(stdout.String(), fingerprint), nil
}

// keyFingerprint returns the SHA256 fingerprint (the "SHA256:…" field) of the key
// at path, as reported by `ssh-keygen -lf`. It prefers the public key file
// (path + ".pub") when one exists next to path, since ssh-keygen reads it without
// needing the passphrase; otherwise it reads path directly, whose public half
// OpenSSH stores in the clear even for an encrypted private key. Stdin is closed
// so a key ssh-keygen cannot read fails fast instead of blocking on a prompt.
func keyFingerprint(ctx context.Context, path string) (string, error) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		return "", fmt.Errorf("ssh-keygen not found on PATH: %w", err)
	}
	target := path
	if pub := path + ".pub"; fileExists(pub) {
		target = pub
	}

	cmd := exec.CommandContext(ctx, "ssh-keygen", "-lf", target)
	cmd.Stdin = nil // never block waiting for a passphrase prompt
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("ssh-keygen -lf %s: %s", target, msg)
	}

	// Output is "<bits> <fingerprint> <comment> (<type>)"; the fingerprint is the
	// second whitespace-delimited field.
	fields := strings.Fields(stdout.String())
	if len(fields) < 2 {
		return "", fmt.Errorf("ssh-keygen -lf %s: unexpected output %q", target, strings.TrimSpace(stdout.String()))
	}
	return fields[1], nil
}

// fingerprintListed reports whether listing (the output of `ssh-add -l`) names an
// identity with the given fingerprint. Each line is
// "<bits> <fingerprint> <comment> (<type>)", so an exact match on any
// whitespace-delimited field finds the fingerprint without the substring false
// positives (SHA256:abc123 vs SHA256:abc1234) a plain contains check invites.
func fingerprintListed(listing, fingerprint string) bool {
	for _, line := range strings.Split(listing, "\n") {
		for _, field := range strings.Fields(line) {
			if field == fingerprint {
				return true
			}
		}
	}
	return false
}

// fileExists reports whether p names an existing regular file (not a directory).
func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// expandPath resolves a leading ~ or ~/ in p to the current user's home
// directory and trims surrounding whitespace. Any other path is returned
// unchanged, and a blank path expands to "".
func expandPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	if p == "~" {
		return home, nil
	}
	return filepath.Join(home, p[2:]), nil
}

// askpassSentinelEnv marks a revision subprocess that ssh-add invoked as its
// SSH_ASKPASS helper; when set, the process must print the passphrase and exit
// instead of starting the TUI.
const askpassSentinelEnv = "REVISION_ASKPASS"

// askpassValueEnv carries the passphrase to the SSH_ASKPASS helper. It is set
// only on the short-lived ssh-add child's environment, never on revision's own
// process, so the secret lives no longer than that child.
const askpassValueEnv = "REVISION_SSH_PASSPHRASE"

// askpassOnceEnv names a marker file that makes the helper answer exactly once.
// ssh-add re-invokes the askpass program in an unbounded loop, stopping only
// when it receives an empty answer, so a helper that always returned the same
// passphrase would spin forever on a wrong one. The first invocation claims the
// marker (removing it) and returns the passphrase; any retry finds it gone and
// returns an empty answer, so ssh-add gives up after a single attempt.
const askpassOnceEnv = "REVISION_ASKPASS_ONCE"

// ErrAgentUnreachable reports that the ssh-agent could not be contacted, as
// opposed to a recoverable failure such as a wrong passphrase. Callers use it to
// tell "the agent is down" apart from "that passphrase was wrong".
var ErrAgentUnreachable = errors.New("ssh-agent is not reachable")

// IsAskpass reports whether this process was invoked by ssh-add as its
// SSH_ASKPASS helper. The program's entry point must check this before doing
// anything else and, when true, call RunAskpass and exit.
func IsAskpass() bool {
	return os.Getenv(askpassSentinelEnv) == "1"
}

// RunAskpass emits, on stdout, the passphrase AddKey passed through the
// environment — what ssh-add expects from an SSH_ASKPASS helper — but only for
// the first invocation. It claims the single-use marker named by askpassOnceEnv;
// a retry (which ssh-add issues after a wrong passphrase) finds the marker gone
// and returns an empty answer, so ssh-add stops instead of looping. The caller
// must exit immediately afterward.
func RunAskpass() {
	if once := os.Getenv(askpassOnceEnv); once == "" || os.Remove(once) != nil {
		fmt.Println()
		return
	}
	fmt.Println(os.Getenv(askpassValueEnv))
}

// AddKey loads the SSH private key at keyPath into the running ssh-agent,
// decrypting it with passphrase. It runs `ssh-add <keyPath>` once and feeds the
// passphrase through an SSH_ASKPASS helper — this same binary, re-invoked — so
// the passphrase is never typed on the terminal the TUI owns and never written
// to disk. The passphrase is placed only on the child's environment, so it lives
// no longer than the ssh-add process, and a single-use marker makes ssh-add try
// exactly once rather than loop on a wrong passphrase.
//
// It relies on SSH_ASKPASS_REQUIRE=force (OpenSSH 8.4+) to keep ssh-add off the
// terminal. An unreachable agent is reported as ErrAgentUnreachable; a wrong
// passphrase or missing key file is returned as an error carrying ssh-add's
// stderr.
func AddKey(ctx context.Context, keyPath, passphrase string) error {
	path, err := expandPath(keyPath)
	if err != nil {
		return err
	}
	if path == "" {
		return errors.New("ssh key path is empty")
	}
	if _, err := exec.LookPath("ssh-add"); err != nil {
		return fmt.Errorf("ssh-add not found on PATH: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate revision binary for the passphrase helper: %w", err)
	}

	// A private, empty marker file the helper consumes to answer exactly once. It
	// holds no secret; the passphrase itself travels only in the environment.
	once, err := os.CreateTemp("", "revision-askpass-*")
	if err != nil {
		return fmt.Errorf("create askpass marker: %w", err)
	}
	onceName := once.Name()
	_ = once.Close()
	defer func() { _ = os.Remove(onceName) }()

	cmd := exec.CommandContext(ctx, "ssh-add", path)
	cmd.Stdin = nil // never read the passphrase from the terminal
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	env := append(os.Environ(),
		"SSH_ASKPASS="+self,
		"SSH_ASKPASS_REQUIRE=force",
		askpassSentinelEnv+"=1",
		askpassValueEnv+"="+passphrase,
		askpassOnceEnv+"="+onceName,
	)
	// Older OpenSSH consults DISPLAY before using the askpass program; a non-empty
	// value keeps it on the askpass path when SSH_ASKPASS_REQUIRE is ignored.
	if os.Getenv("DISPLAY") == "" {
		env = append(env, "DISPLAY=:0")
	}
	cmd.Env = env

	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("ssh-add timed out adding %s", path)
	}
	if err != nil {
		out := strings.TrimSpace(stderr.String())
		// Exit code 2, or the classic connection message, means the agent itself
		// is unreachable rather than the passphrase being wrong.
		if exitCode(err) == 2 || strings.Contains(out, "authentication agent") {
			return ErrAgentUnreachable
		}
		if out == "" {
			out = err.Error()
		}
		return fmt.Errorf("ssh-add %s: %s", path, out)
	}
	return nil
}

// exitCode returns the process exit code carried by err, or -1 when err is not
// an exec exit error.
func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}
