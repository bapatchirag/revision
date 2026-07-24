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

A lazygit-style terminal UI for Subversion (SVN). `revision` gives you a fast, keyboard-driven interface over the `svn` command line — review changes, stage with changelists, commit, update, and browse history without leaving your terminal.

<p align="center">
  <img src="docs/hero.gif" alt="revision in action — browsing diffs, staging, named changelists, history, and committing" width="100%" />
</p>

## Why

SVN's command line is powerful but verbose for day-to-day work. `revision` wraps it in a focused TUI — inspired by [lazygit](https://github.com/jesseduffield/lazygit) — so common tasks are a keystroke away. It shells out to your existing `svn` binary, so it respects your working copy, credentials, and configuration.

## Features

- **lazygit-style layout** — a left column of Status, Files, and Log panels beside a Main detail view, with number-key and `Tab` focus switching
- Working-copy **status** at a glance — changes colored by state, over a live repo/revision header
- Per-file **diff** viewer that follows your selection in the Main panel
- **Staging** via a SVN changelist (a git-index-like workflow) — stage or unstage with a single keystroke
- **Named changelists** — group related changes in a tabbed Changelists view, drill into any list, and commit it as a unit
- **Commit** the staged set (or a chosen changelist) through an inline message editor
- **Update** the working copy to HEAD
- **Add / revert / delete** files, with confirmation prompts before anything destructive
- Read-only **log / history** viewer with per-revision detail (author, date, message, changed paths)
- **Discoverable keybindings** — a contextual footer plus a full `?` help menu
- **Toast notifications** for every action, success or failure
- **Non-blocking authentication** — clear, actionable hints instead of a hung credential prompt

## Requirements

- The [`svn`](https://subversion.apache.org/) command-line client on your `PATH`
- Run `revision` from inside an SVN working copy (or pass `--path`)

## Install

`revision` is a single self-contained binary.

### Quick install (Linux / macOS)

```sh
curl -fsSL https://raw.githubusercontent.com/bapatchirag/revision/main/install.sh | sh
```

The script detects your OS and architecture, downloads the matching binary from the latest release, and installs it without `sudo` (falling back to `~/.local/bin`).

### With Go

```sh
go install github.com/bapatchirag/revision/cmd/revision@latest
```

### Prebuilt binaries

Download the binary for your platform from the [Releases](https://github.com/bapatchirag/revision/releases) page and put it on your `PATH`.

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

The footer shows the most common actions, and `?` opens the full keybindings menu at any time.

| Key | Action |
|-----|--------|
| `1` / `2` / `3` / `0` | Focus the Status / Files / Log / Main panel |
| `Tab` / `Shift+Tab` | Cycle focus between panels |
| `↑`/`k`, `↓`/`j` | Move the selection up / down |
| `g` / `G` | Jump to the top / bottom of a list |
| `K` / `J` | Scroll the Main panel up / down a page |
| `←`/`h`, `→`/`l` | Scroll the Main panel left / right (one column) |
| `Home` / `End` | Jump to the start / end of the line in the Main panel |
| `[` / `]` | Switch the Files panel between the Changes and Changelists views |
| `space` | Stage / unstage the selected file (an untracked file is `svn add`ed first) |
| `n` | Assign the staged files — or just the selected file when nothing is staged — to a named changelist |
| `enter` | Expand the selected changelist into its files |
| `c` | Commit the staged files, or the selected changelist (opens the message editor) |
| `r` | Revert the selected file (with confirmation) |
| `d` | Delete the selected file (with confirmation) |
| `u` | Update the working copy to the latest revision |
| `R` | Refresh status and history |
| `?` | Toggle the keybindings help |
| `q` / `Ctrl+C` | Quit |

In the commit editor, `Ctrl+S` submits and `Esc` cancels. In the changelist-name prompt, `Tab` toggles between typing a new name and picking an existing changelist. In a confirmation dialog, `Enter`/`y` confirms and `Esc`/`n` cancels.

## How staging works

SVN has no local staging index. `revision` emulates one using an SVN **changelist** named `revision:staged`: staging a file adds it to that changelist, unstaging removes it, and `c` commits the staged set as a unit. This maps a git-like stage/commit flow onto native SVN.

You can also group work into **named changelists** with `n`: it moves the staged files (or just the selected file when nothing is staged) into a real SVN changelist, which appears in the Changelists view and can be committed on its own. A file belongs to at most one changelist at a time — unstage it (`space`) before moving it elsewhere.

## Authentication

`revision` always runs `svn` with `--non-interactive`, so it never blocks on a hidden credential prompt. If a command needs credentials that aren't cached, it fails fast with a clear hint instead of hanging.

Cache your credentials once by running an `svn` command yourself in the working copy (for example `svn info` or `svn update`). SVN stores them, and `revision` uses them on subsequent actions.

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

## Roadmap

`revision` already covers the everyday SVN workflow. On the horizon:

- **VS Code extension** — a bundled launcher that opens the TUI in an editor terminal, published to the VS Code Marketplace and Open VSX. The scaffolding exists but isn't ready yet.
- **Configuration file** — settings from a config file (e.g. `~/.config/revision/config.toml`): default working-copy path, log limit, an external `$EDITOR` for commit messages, and keybinding overrides.
- **Theming** — selectable, user-customizable color themes loaded from that configuration.
- **Diff export & patching** — save a file's or a changelist's diff as a patch and apply one (`svn diff` → patch → `svn patch`), plus line- and hunk-level staging.
- **ssh-agent support** — seamless authentication for `svn+ssh://` working copies, alongside smoother handling of cached credentials.
- **Branches & tags** — create and switch between them as server-side copies (`svn copy` / `svn switch`).
- **More review tools** — blame / annotate, revision search and filtering, and conflict-resolution helpers.

Have an idea or want to help build one of these? Contributions are welcome.

## Contributing

Issues and pull requests are welcome — bug reports, feature ideas, and documentation fixes all help.

### Project layout

`revision` is a Go module with a layered, lazygit-inspired architecture:

- `cmd/revision` — the CLI entry point (flag parsing, working-copy detection, launching the TUI).
- `cmd/gallery` — a standalone gallery that renders each reusable UI component in isolation (`make run-gallery`).
- `internal/svn` — a thin wrapper over the `svn` binary that parses `--xml` output into typed values; always runs `--non-interactive`.
- `internal/tui` — the domain-agnostic UI foundation: reusable components plus theme, keymap, focus, layout, and messages. It must never import `internal/svn` or `internal/app` (a reusability-guard test enforces this).
- `internal/app` — the composition layer that adapts SVN data into components and arranges the lazygit layout. It is the only package that knows both sides.

### Development

```sh
make build        # compile ./bin/revision
make run          # run the TUI from source
make run-gallery  # preview the reusable components in isolation
make test         # run all tests
make lint         # run golangci-lint (must be installed)
make fmt          # gofmt the tree
```

Before opening a PR, please make sure `make fmt`, `make lint`, and `make test` all pass. New UI components should follow the existing contracts — compile-time interface assertions, a golden test over `View()`, and a teatest harness — and nothing under `internal/tui` may reach into the SVN or app layers.

Some tests drive a real `svn` binary against a throwaway repository; they skip automatically when `svn`/`svnadmin` aren't on the `PATH`.

### Regenerating the demo

The hero GIF is produced with [VHS](https://github.com/charmbracelet/vhs) from a scripted tape:

```sh
vhs docs/demo.tape   # writes docs/hero.gif
```

The tape builds a throwaway SVN working copy via `docs/demo-setup.sh`, so it needs `vhs`, `svn`/`svnadmin`, and a Go toolchain.

## License

[MIT](LICENSE) &copy; Chirag Bapat
