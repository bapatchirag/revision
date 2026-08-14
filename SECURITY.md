# Security Policy

## Supported versions

`revision` is maintained on a rolling basis: only the
[latest release](https://github.com/bapatchirag/revision/releases/latest) receives security
fixes. If you are on an older build, run `revision --update` or reinstall before reporting.

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Report it privately through GitHub's
[private vulnerability reporting](https://github.com/bapatchirag/revision/security/advisories/new)
— the **Security** tab of the repository, then **Report a vulnerability**. If that is not
available to you, email **bapatchirag@gmail.com** instead.

Include as much of the following as you can:

- The version (`revision --version`) and the platform you saw it on.
- The repository URL scheme in use (`http://`, `https://`, `svn+ssh://`, `file://`).
- Steps to reproduce, or a proof of concept.
- What an attacker gains — the impact you believe the issue has.

You will get an acknowledgement of the report, an assessment once the issue has been
reproduced, and a note when a fix ships. Please give the fix a chance to land before
disclosing publicly. Credit is given in the release notes unless you would rather stay
anonymous.

## Scope

`revision` is a terminal client that shells out to your existing `svn` binary. The areas
most worth your attention:

- **Command construction** (`internal/svn`) — argument handling for paths, revisions,
  changelists and commit messages passed to `svn`.
- **Credential and key handling** (`internal/sshagent`) — the `ssh-add` / `ssh-keygen`
  flow used for `svn+ssh://` working copies, and anything that could leak a passphrase
  into the command log, a log file, or the terminal.
- **Configuration and file writes** (`internal/config`, saved diffs, patch and reject
  files) — path handling and permissions on the files `revision` creates.
- **Install and self-update** (`install.sh`, `internal/selfupdate`) — the download path
  for release binaries. `install.sh` fetches over HTTPS and verifies each asset against
  the release's `checksums.txt`; `revision --update` re-runs that same script.

Out of scope:

- Vulnerabilities in Subversion, OpenSSH, or your terminal emulator — report those
  upstream.
- Anything that requires an attacker to already have write access to your working copy,
  your config file, or your shell.
- Findings from automated scanners without a demonstrated impact on `revision`.
