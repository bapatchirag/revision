---
title: Updating revision
description: Keep the revision binary current, in-app or from the command line.
---

Release builds check for a newer version on startup. Development builds — anything from
`go install`, `make build`, or a local cross-compile — never check for or apply updates.

## The startup prompt

When a newer release is available, `revision` offers three choices:

| Choice | What it does |
|---|---|
| **Update with cURL** | Re-runs the [install script](/guides/installation/), replacing the binary in place. |
| **Update with Go** | Runs `go install github.com/bapatchirag/revision/cmd/revision@latest`. |
| **Don't update this time** | Dismisses the prompt for this session. |

Pick one with the arrow keys and <kbd>enter</kbd>, or press <kbd>esc</kbd> to dismiss.

## From the command line

```sh
revision --update                      # check, then prompt for a method
revision --update --update-with curl   # non-interactive: use the install script
revision --update --update-with go     # non-interactive: use go install
```

The non-interactive forms are the ones to use from a script or a dotfiles bootstrap.

## Which method should I use?

Use the method you installed with.

- **cURL** replaces the binary wherever the install script put it. It needs write access
  to that directory — the same one the original install used.
- **Go** writes to `$GOBIN` (or `$GOPATH/bin`). If that is a *different* directory that
  also happens to be on your `PATH`, you can end up with two copies and the shell picking
  whichever comes first. `command -v revision` tells you which one wins.

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
