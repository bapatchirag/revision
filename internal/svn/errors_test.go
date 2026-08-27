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

func TestIsLockedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{
			"locked working copy",
			errors.New("svn: E155004: Working copy '/home/alice/wc' locked."),
			true,
		},
		{
			"cleanup advice",
			errors.New("svn: E155037: Previous operation has not finished; run 'svn cleanup' if it was interrupted"),
			true,
		},
		{
			"already locked",
			errors.New("svn: E155004: '/home/alice/wc' is already locked."),
			true,
		},
		{
			"a path error is not a lock",
			errors.New("svn: E155007: '/home/alice/wc/a.txt' is not a working copy"),
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsLockedError(tc.err); got != tc.want {
				t.Errorf("IsLockedError(%v) = %v, want %v", tc.err, got, tc.want)
			}
			if got := BlocksEveryPath(tc.err); got != tc.want {
				t.Errorf("BlocksEveryPath(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}

	if !BlocksEveryPath(errors.New("svn: E215004: Authentication failed")) {
		t.Error("an auth failure blocks every path too — nothing else would fare better")
	}
}

func TestHintPrefersTheActionableMessage(t *testing.T) {
	if got, ok := Hint(errors.New("svn: E155004: Working copy locked.")); !ok || got != LockHint {
		t.Errorf("Hint(locked) = %q, %v, want the lock hint", got, ok)
	}
	if got, ok := Hint(errors.New("svn: E215004: Authentication failed")); !ok || got != AuthHint {
		t.Errorf("Hint(auth) = %q, %v, want the auth hint", got, ok)
	}
	if _, ok := Hint(errors.New("svn: E155010: The node 'a.txt' was not found.")); ok {
		t.Error("an error revision knows no way out of has to keep svn's own words")
	}
}

func TestCollapseNamesTheFirstAndCountsTheRest(t *testing.T) {
	if err := collapse(nil); err != nil {
		t.Errorf("collapse(nil) = %v, want nil", err)
	}

	one := collapse([]PathError{{Path: "a.txt", Err: errors.New("refused")}})
	if one == nil || one.Error() != "a.txt: refused" {
		t.Errorf("collapse(1) = %v, want the sole refusal", one)
	}

	many := collapse([]PathError{
		{Path: "a.txt", Err: errors.New("refused")},
		{Path: "b.txt", Err: errors.New("refused")},
		{Path: "c.txt", Err: errors.New("refused")},
	})
	if many == nil || many.Error() != "a.txt: refused (and 2 more)" {
		t.Errorf("collapse(3) = %v, want the first named and the rest counted", many)
	}

	wrapped := errors.New("refused")
	if !errors.Is(PathError{Path: "a.txt", Err: wrapped}, wrapped) {
		t.Error("a PathError has to unwrap to svn's own error")
	}
}
