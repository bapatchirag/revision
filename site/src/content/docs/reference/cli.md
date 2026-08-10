---
title: CLI flags
description: Every command-line flag revision accepts.
---

```sh
revision [flags]
```

With no flags, `revision` opens the TUI on the current directory.

## Flags

| Flag | Default | Description |
|---|---|---|
| `--path <dir>` | `.` | The SVN working copy to operate on. |
| `--version` | — | Print the version and exit. |
| `--update` | — | Check for a newer release and update the binary. Release builds only. |
| `--update-with <curl\|go>` | prompt | The method `--update` uses. Requires `--update`. |
| `--help` | — | Show usage and exit. |

### `--path`

```sh
revision --path /path/to/working-copy
```

The path is resolved to an absolute path before anything else runs. If it is not an SVN
working copy, `revision` says so and exits with status 1 rather than opening a UI it
cannot use.

This sets where `revision` *operates*; [`displayFrom`](/reference/configuration/#displayfrom)
controls how much of the working copy it *shows*.

Once the TUI is open, `P` re-scopes it to another directory inside that working copy for
the rest of the session — see
[changing the source path](/guides/panels/#changing-the-source-path). The new source is
never saved; the next launch uses `--path` again.

### `--version`

```sh
$ revision --version
revision v1.4.0
```

A `go install …@<tag>` build reports that tag, `make build` reports what `git describe`
gave it, and a plain `go build` reports `dev`.

### `--update` and `--update-with`

```sh
revision --update                      # check, then prompt for a method
revision --update --update-with curl   # non-interactive: run the release's install script
revision --update --update-with go     # non-interactive: go install …@<tag>
```

`--update-with` without `--update` is a usage error, not a silent no-op.

A build checks for and applies updates when it came from the release pipeline, or from
`go install …@<tag>` at a published tag. `make build`, a local cross-compile and an
untagged `go install` never do. See
[Updating revision](/operations/updating-revision/).

## Exit status

| Status | Meaning |
|---|---|
| `0` | Clean exit, including quitting the TUI. |
| `1` | `svn` is not on your `PATH`, the path is not a working copy, or an update failed. |
| `2` | The flags given do not go together. |

Errors are written to stderr, prefixed `revision:`.

## Startup checks

Before the UI appears, `revision`:

1. Looks for `svn` on your `PATH` and exits with a message if it is missing.
2. Resolves `--path` and runs `svn info` against it, with a 15-second timeout. An
   authentication failure produces an [actionable hint](/operations/authentication/)
   rather than a hang.
3. Loads [`config.json`](/reference/configuration/), reconciling anything missing or
   invalid and reporting the result in a startup toast.
4. Pins the colour profile when a named [theme](/reference/themes/) is selected.

## Environment

`revision` reads no configuration from the environment beyond what SVN and OpenSSH use
themselves. Two variables are worth knowing about:

| Variable | Used by |
|---|---|
| `XDG_CONFIG_HOME` | The [config file location](/reference/configuration/#where-the-file-lives). |
| `VISUAL` / `EDITOR` | The [`native` editor](/workflows/editor/#how-native-resolves) fallback. |

`REVISION_VERSION` and `REVISION_INSTALL_DIR` are read by the
[install script](/guides/installation/#environment-variables), not by the binary.

:::note
`revision` re-invokes itself as an `SSH_ASKPASS` helper while adding a key to your agent.
That mode is internal, activated only by an environment variable the parent process sets,
and exits before any flag parsing happens.
:::
