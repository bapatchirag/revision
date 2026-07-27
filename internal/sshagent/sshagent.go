// Package sshagent inspects the running ssh-agent so revision can tell whether
// the SSH key used for svn+ssh access is already loaded before it starts talking
// to a remote repository. It is deliberately domain-agnostic: it knows nothing
// about the TUI, the SVN layer, or the app composition, and simply shells out to
// the system ssh-add.
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

// KeyLoaded reports whether the SSH private key at keyPath is currently held by
// the running ssh-agent. It runs `ssh-add -l` and matches keyPath — with a
// leading ~ expanded to the user's home directory — against the identities the
// agent lists.
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

	return keyListed(stdout.String(), path), nil
}

// keyListed reports whether listing (the output of `ssh-add -l`) names the
// identity at path. Each line is "<bits> <fingerprint> <comment> (<type>)", and
// a key added from a file carries that file's path as its comment, so an exact
// match on any whitespace-delimited field identifies the key without the
// substring false positives (id_rsa vs id_rsa2) a plain contains check invites.
func keyListed(listing, path string) bool {
	for _, line := range strings.Split(listing, "\n") {
		for _, field := range strings.Fields(line) {
			if field == path {
				return true
			}
		}
	}
	return false
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
