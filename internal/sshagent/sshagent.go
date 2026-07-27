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
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
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
