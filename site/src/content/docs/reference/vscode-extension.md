---
title: VS Code extension
description: Launch the revision TUI in an editor terminal from VS Code.
---

:::caution[Not published yet]
The extension is built and works from a local install, but it is not on the VS Code
Marketplace or Open VSX yet. Publishing it is on the
[roadmap](https://github.com/bapatchirag/revision#roadmap).
:::

The extension is a thin launcher: it opens `revision` in a VS Code **editor** terminal —
a full-width tab, not the narrow panel at the bottom — using the workspace folder as the
working directory.

## Command

| Command | Palette entry |
|---|---|
| `revision.open` | **Revision: Open** |

## Settings

| Setting | Default | Description |
|---|---|---|
| `revision.binaryPath` | `""` | Absolute path to the `revision` binary. Empty uses the bundled binary, falling back to `revision` on your `PATH`. |
| `revision.workingDirectory` | `""` | Working directory to launch `revision` in. Empty uses the first workspace folder. |

## How the binary is found

1. `revision.binaryPath`, if set and pointing at a file that exists.
2. The binary bundled with the extension, made executable if it is not already.
3. `revision` on your `PATH`.

If none of those resolves, the extension shows an error pointing at the
[install instructions](/guides/installation/) rather than opening an empty terminal.

## Over a remote connection

The extension runs where the terminal runs. With **Remote-SSH**, a dev container, or WSL,
that is the server — so the `revision` binary has to be installed there, not on your
workstation.

That is also the arrangement the [`native` editor mode](/workflows/editor/#in-vs-code-over-a-remote-connection)
is built for: <kbd>e</kbd> inside the TUI opens the remote file as a tab in your local
window.

## Building it locally

```sh
cd extension
npm install && npm run compile
```

Then run **Developer: Install Extension from Location…** from the command palette and
point it at the `extension/` directory.
