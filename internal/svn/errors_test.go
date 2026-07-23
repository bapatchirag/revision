package svn

import (
	"errors"
	"testing"
)

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{
			"non-interactive prompt disabled",
			errors.New("svn update: E215004: Authentication failed and interactive prompting is disabled; see the --force-interactive option"),
			true,
		},
		{
			"no more credentials",
			errors.New("svn commit: E170001: No more credentials or we tried too many times.\nAuthentication realm: <https://svn.example.com:443> Repo"),
			true,
		},
		{
			"authorization failed",
			errors.New("svn commit: E170001: Authorization failed"),
			true,
		},
		{
			"authentication failed lowercase already",
			errors.New("svn info: authentication failed"),
			true,
		},
		{
			"plain network error is not auth",
			errors.New("svn info: E170013: Unable to connect to a repository at URL 'https://svn.example.com/repo'"),
			false,
		},
		{
			"path error is not auth",
			errors.New("svn diff: E155010: The node 'missing.txt' was not found."),
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAuthError(tc.err); got != tc.want {
				t.Errorf("IsAuthError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
