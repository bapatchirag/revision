# revision

A lazygit-style terminal UI for Subversion (SVN). `revision` gives you a fast, keyboard-driven interface over the `svn` command line — review changes, stage with changelists, commit, update, and browse history without leaving your terminal.

> **Status:** early development. Features are being built out phase by phase (see the roadmap in the repo).

## Why

SVN's command line is powerful but verbose for day-to-day work. `revision` wraps it in a focused TUI — inspired by [lazygit](https://github.com/jesseduffield/lazygit) — so common tasks are a keystroke away. It shells out to your existing `svn` binary, so it respects your working copy, credentials, and configuration.

## Features

- Working-copy **status** view, grouped by state and changelist
- Per-file **diff** viewer
- **Staging** via SVN changelists (a git-index-like workflow)
- **Commit** staged changes
- **Update** the working copy
- **Add / revert / delete** files
- **Log** / history viewer

## Requirements

- The [`svn`](https://subversion.apache.org/) command-line client on your `PATH`
- Run `revision` from inside an SVN working copy (or pass `--path`)

## Install

`revision` is a single self-contained binary. The VS Code extension is an optional launcher.

### Quick install (Linux / macOS)

```sh
curl -fsSL https://raw.githubusercontent.com/bapatchirag/revision/main/install.sh | sh
```

### With Go

```sh
go install github.com/bapatchirag/revision/cmd/revision@latest
```

### Prebuilt binaries

Download the binary for your platform from the [Releases](https://github.com/bapatchirag/revision/releases) page and put it on your `PATH`.

### VS Code extension (optional)

Install **revision** from the VS Code Marketplace, then run **Revision: Open** from the Command Palette to launch the TUI in an editor terminal. The extension bundles the binary for supported platforms and otherwise uses `revision` from your `PATH`.

## Usage

```sh
# from inside an SVN working copy
revision

# or point it at a working copy
revision --path /path/to/working-copy
```

Flags:

- `--path <dir>` — working copy to operate on (default: current directory)
- `--version` — print version and exit
- `--help` — show help

### Keybindings

Keybindings are shown in the footer and via `?`. (Full reference coming as features land.)

## How staging works

SVN has no local staging index. `revision` emulates one using an SVN **changelist** named `revision:staged`: staging a file adds it to that changelist, unstaging removes it, and committing operates on the staged set. This maps a git-like stage/commit flow onto native SVN.

## Building from source

```sh
git clone https://github.com/bapatchirag/revision.git
cd revision
make build      # builds ./bin/revision
make test
```

Cross-compile static binaries:

```sh
make cross      # dist/revision-darwin-arm64 and dist/revision-linux-amd64
```

## Contributing

Issues and PRs are welcome. Please run `make test` and `make lint` before submitting.

## License

[MIT](LICENSE) &copy; Chirag Bapat
