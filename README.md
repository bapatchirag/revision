<p align="center">
  <img src="docs/logo.png" alt="revision logo" width="120" />
</p>

<h1 align="center">revision</h1>

<p align="center">
  <a href="https://github.com/bapatchirag/revision/actions/workflows/ci.yml">
    <img src="https://github.com/bapatchirag/revision/actions/workflows/ci.yml/badge.svg" alt="CI status" />
  </a>
  <a href="https://github.com/bapatchirag/revision/blob/main/go.mod">
    <img src="https://img.shields.io/github/go-mod/go-version/bapatchirag/revision" alt="Go version" />
  </a>
  <a href="https://github.com/bapatchirag/revision/blob/main/LICENSE">
    <img src="https://img.shields.io/badge/license-MIT-yellow.svg" alt="MIT License" />
  </a>
</p>

<p align="center">
  <b><a href="https://bapatchirag.github.io/revision/">Documentation</a></b> ·
  <a href="https://bapatchirag.github.io/revision/guides/quick-start/">Quick start</a> ·
  <a href="https://bapatchirag.github.io/revision/reference/keybindings/">Keybindings</a> ·
  <a href="https://bapatchirag.github.io/revision/reference/configuration/">Configuration</a>
</p>

A lazygit-style terminal UI for Subversion (SVN). `revision` gives you a fast, keyboard-driven interface over the `svn` command line — review changes, stage with changelists, commit, update, and browse history without leaving your terminal.

<p align="center">
  <img src="site/public/demos/hero.gif" alt="revision in action — working-copy status, colour-coded diffs with in-place search, filtering, directory staging, named changelists, history and update-to-revision, the svn command log, live theming, and committing" width="100%" />
</p>

## Why

SVN's command line is powerful but verbose for day-to-day work. `revision` wraps it in a focused TUI — inspired by [lazygit](https://github.com/jesseduffield/lazygit) — so common tasks are a keystroke away. It shells out to your existing `svn` binary, so it respects your working copy, credentials, and configuration.

## Features

- **lazygit-style layout** — Status, Files and Log panels down the left, a Main detail view beside them, and the `svn` command log beneath; switch with the number keys or `Tab`
- **Changed files as a collapsible directory tree**, grouped under their folders, with the visible count in the panel footer
- **Colour-coded diffs** that follow your selection — read one side by side with `s`, save it to a file with `w`, or highlight a directory for the combined diff of everything beneath it
- **Staging** built on a native SVN changelist — `space` stages a file or a whole subtree, `c` commits the set through an inline message editor
- **Named changelists** — group the staged set into real SVN changelists, drill into one in a tabbed view, and commit it on its own
- **Update** to HEAD with `u`, or to any revision picked in the Log panel — conflicts are spelled out before you confirm
- **Resolve conflicts side by side** with `m` — each conflict, or each hunk a patch could not place, laid out two panes wide with a key to take either side, both, or your editor
- **Add, revert and delete** a single file or every change beneath a directory, with confirmation prompts
- **Instant staging** — the row restyles on the keypress while `svn` confirms behind it, and a failure puts the previous state back; revert, delete and commit mark their rows as in flight rather than claiming a success they do not have yet
- **Live refresh** — an edit made outside `revision` reaches the Files and diff panels on its own, with the cursor and scroll where you left them; `L` turns the watcher off
- **Reads once, and only what you look at** — diffs and history pages are cached for the session, so startup costs a single `svn status` and revisiting a file is instant
- **Filter or search any panel** with `/` — `rev:`, `user:`, `state:` and `cl:` parameters plus free text, and `n` / `N` to jump between matches
- **Open the highlighted file** in vim, nvim, nano, or the editor around you — including a VS Code tab when `revision` is running on a remote host
- **Themes and in-app settings** — six colour schemes, edited with `S` and saved to your config file
- **svn+ssh ready**, self-updating release builds, a contextual footer and a full `?` keybindings menu

## Requirements

- The [`svn`](https://subversion.apache.org/) command-line client on your `PATH`
- Run `revision` from inside an SVN working copy (or pass `--path`)
- OpenSSH's `ssh-add` and `ssh-keygen` if your working copy is served over `svn+ssh://`

## Install

`revision` is a single self-contained binary.

### Quick install (Linux / macOS)

```sh
curl -fsSL https://raw.githubusercontent.com/bapatchirag/revision/main/install.sh | sh
```

The script detects your OS and architecture, downloads the matching binary from the latest release, and installs it without `sudo` (falling back to `~/.local/bin`). Prebuilt binaries cover macOS on Apple silicon and Intel, and Linux on x86-64 and 64-bit ARM.

### With Go

```sh
go install github.com/bapatchirag/revision/cmd/revision@latest
```

### Prebuilt binaries

Download the binary for your platform from the [Releases](https://github.com/bapatchirag/revision/releases) page and put it on your `PATH`. Each release publishes `revision-darwin-arm64`, `revision-darwin-amd64`, `revision-linux-amd64` and `revision-linux-arm64`.

### Updating

Release builds check for a newer version on startup and offer to upgrade themselves; `revision --update` does the same from the command line. See [Updating revision](https://bapatchirag.github.io/revision/operations/updating-revision/).

## Quick start

```sh
cd /path/to/working-copy
revision                 # or from anywhere: revision --path /path/to/working-copy
```

- `1` `2` `3` `4` `0` or `Tab` — move between the Status, Files, Log, Command Log and Main panels
- `space` — stage the selected file, or every change beneath the selected directory
- `c` — commit the staged set · `u` — update the working copy
- `/` — filter or search the focused panel · `R` — refresh
- `S` — settings and themes · `?` — every keybinding
- `q` — quit

Full flag list: `revision --help`.

## Documentation

**[bapatchirag.github.io/revision](https://bapatchirag.github.io/revision/)** — installation,
workflows, the full keybindings reference, configuration, troubleshooting, and the architecture
notes.

## Contributing

Issues and pull requests are welcome — bug reports, feature ideas, and documentation fixes all help.
See [Contributing](https://bapatchirag.github.io/revision/develop/contributing/) for the layout of
the tree and what to run before opening a PR.

## License

[MIT](LICENSE) &copy; Chirag Bapat
