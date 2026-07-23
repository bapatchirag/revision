package svn

import "strings"

// AuthHint is a short, actionable message for an svn authentication failure.
// Because revision always runs svn with --non-interactive (so it never blocks on
// a hidden credential prompt), a command that needs credentials fails outright;
// this tells the user how to recover.
const AuthHint = "authentication required — cache SVN credentials, then retry"

// authSignatures are lower-cased substrings that mark an svn failure as an
// authentication or authorization problem. Kept specific so a plain network or
// path error is not misreported as an auth failure.
var authSignatures = []string{
	"authentication failed",
	"authorization failed",
	"no more credentials",
	"interactive prompting is disabled",
	"username or password",
	"e170001", // authorization failed
	"e215004", // no more credentials / auth failed in a non-interactive context
}

// IsAuthError reports whether err looks like an svn authentication or
// authorization failure. revision runs svn --non-interactive, so a command that
// needs credentials fails instead of prompting; callers use this to surface an
// actionable hint (see AuthHint) rather than a raw svn error dump.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, sig := range authSignatures {
		if strings.Contains(msg, sig) {
			return true
		}
	}
	return false
}
