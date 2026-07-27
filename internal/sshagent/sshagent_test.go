package sshagent

import (
	"path/filepath"
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

func TestKeyListed(t *testing.T) {
	const path = "/home/me/.ssh/id_rsa"
	cases := map[string]struct {
		listing string
		want    bool
	}{
		"present": {
			"4096 SHA256:abc123 /home/me/.ssh/id_rsa (RSA)",
			true,
		},
		"present among many": {
			"256 SHA256:zzz /home/me/.ssh/id_ed25519 (ED25519)\n" +
				"4096 SHA256:abc123 /home/me/.ssh/id_rsa (RSA)",
			true,
		},
		"absent": {
			"256 SHA256:zzz /home/me/.ssh/id_ed25519 (ED25519)",
			false,
		},
		"substring is not a match": {
			"4096 SHA256:abc123 /home/me/.ssh/id_rsa2 (RSA)",
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
			if got := keyListed(tc.listing, path); got != tc.want {
				t.Errorf("keyListed(%q, %q) = %v, want %v", tc.listing, path, got, tc.want)
			}
		})
	}
}
