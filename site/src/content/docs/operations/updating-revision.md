---
title: Updating revision
description: Keep the revision binary current, in-app or from the command line.
---

Release builds check for a newer version on startup. A build counts as a release when it
came from the release pipeline, or from `go install …@<tag>` at a published tag.
Everything else — `make build`, a local cross-compile, or `go install …@main` at an
untagged commit — is a development build, and never checks for or applies updates.

## How often it checks

At most once a day. The answer is remembered in
`~/.config/revision/update-check.json`, so launching `revision` repeatedly costs one
call to the GitHub API rather than one per launch. A check that fails — offline, or
rate-limited — is remembered too, and is not retried for six hours.

GitHub allows anonymous callers 60 requests an hour per address, which a shared address
can exhaust without you running `revision` at all. Set `GITHUB_TOKEN` and the check is
made with it, against your own much larger allowance.

`revision --update` ignores all of this and always asks.

## The startup prompt

When a newer release is available, `revision` offers three choices:

| Choice | What it does |
|---|---|
| **Update with cURL** | Runs the [install script](/guides/installation/) as it was published with that release, replacing the binary in place. |
| **Update with Go** | Runs `go install github.com/bapatchirag/revision/cmd/revision@<tag>` for that release. |
| **Don't update this time** | Dismisses the prompt for this session. |

Pick one with the arrow keys and <kbd>enter</kbd>, or press <kbd>esc</kbd> to dismiss.

Either way the update is pinned to the release the prompt named, installed over the
binary you are running, and checked afterwards: `revision` only reports success once the
new binary reports the new version.

## From the command line

```sh
revision --update                      # check, then prompt for a method
revision --update --update-with curl   # non-interactive: use the install script
revision --update --update-with go     # non-interactive: use go install
```

The non-interactive forms are the ones to use from a script or a dotfiles bootstrap.
`--update-with` on its own is an error rather than a no-op, so a typo cannot quietly
launch the TUI instead of updating.

## Which method should I use?

The install script, unless you have a reason not to. Both methods install over the
binary you are running — the Go path is pointed at that directory too, so it cannot
leave a second copy in `$GOBIN` for your `PATH` to choose between — but they differ in
what they cost and what they check.

- **cURL** downloads the prebuilt binary and verifies it against the SHA-256 the release
  publishes. It needs `curl` or `wget`, and write access to the install directory.
- **Go** compiles from source. It needs a Go toolchain new enough for the module's `go`
  directive — an older one downloads a matching toolchain first, and `GOTOOLCHAIN=local`
  fails outright — plus the network and time that compiling takes. Nothing is
  checksum-verified by the script, though the module proxy verifies the source.

Either way, an install directory you cannot write to now fails loudly instead of
silently putting the new binary somewhere else.

## Verifying

```sh
revision --version
```

Release notes for each version are on the
[Releases page](https://github.com/bapatchirag/revision/releases). The Status panel's
About view links there too — focus the Status panel with <kbd>1</kbd>.

## Pinning a version

The install script honours `REVISION_VERSION`:

```sh
REVISION_VERSION=v1.4.0 sh -c "$(curl -fsSL https://raw.githubusercontent.com/bapatchirag/revision/main/install.sh)"
```

Use that to roll back if a release misbehaves — and please
[open an issue](https://github.com/bapatchirag/revision/issues) if it does.

## Not to be confused with

<kbd>u</kbd> inside the app updates the **working copy**, not the binary. See
[Updating the working copy](/workflows/updating/).
