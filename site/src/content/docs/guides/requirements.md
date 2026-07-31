---
title: Requirements
description: What revision needs before it will run.
---

## The svn client

`revision` shells out to the [Subversion](https://subversion.apache.org/) command-line
client for every operation. `svn` must be installed and on your `PATH`:

```sh
svn --version
```

If that fails, install Subversion first — `brew install subversion` on macOS,
`apt install subversion` or the equivalent on Linux. `revision` exits immediately with a
message rather than starting a UI it cannot use.

## A working copy

`revision` operates on an existing SVN working copy. Run it from inside one, or point it
at one:

```sh
revision --path /path/to/working-copy
```

It does not check out repositories; use `svn checkout` for that. If the target directory
is not a working copy, `revision` says so and exits.

## OpenSSH, for svn+ssh working copies

If your repository URL starts with `svn+ssh://`, `revision` uses OpenSSH's `ssh-add` and
`ssh-keygen` to check whether your key is already loaded in `ssh-agent`, and to add it if
it is not. Both ship with OpenSSH and are normally already present. See
[Authentication](/operations/authentication/) for the full flow.

## Platforms

| Platform | Support |
|---|---|
| macOS, Apple silicon (`darwin-arm64`) | Prebuilt binary |
| Linux, x86-64 (`linux-amd64`) | Prebuilt binary |
| Other Unix-like targets | Build from source with Go |
| Windows | Not supported |

The [install script](/guides/installation/) only serves the two prebuilt targets and
tells you to build from source on anything else. Anywhere else Go and `svn` both run,
`go install` should work — but it is not tested there.

## Terminal

Any terminal that supports an alternate screen buffer. The named
[themes](/reference/themes/) are true-colour, so they render identically everywhere,
including over SSH; the `auto` theme adapts to the palette your terminal advertises.

The layout is designed for 80×24 and grows from there. Narrower than 80 columns, panels
start to clip.
