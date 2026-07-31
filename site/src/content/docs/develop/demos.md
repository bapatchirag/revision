---
title: Regenerating demos
description: How the demo recordings are produced, and how to rebuild them.
---

The recordings are produced with [VHS](https://github.com/charmbracelet/vhs) from a
scripted tape, so they are reproducible and reviewable in a diff rather than being
screen-captured by hand.

## Prerequisites

| Tool | Why |
|---|---|
| `vhs` | Drives the recording. `brew install vhs`. |
| `ttyd`, `ffmpeg` | VHS's own dependencies, installed with it. |
| `svn`, `svnadmin` | The tape builds a throwaway repository and working copy. |
| A Go toolchain | The tape builds `revision` from the checkout it is recording. |

## Recording

```sh
vhs docs/demo.tape       # writes site/public/demos/hero.gif
npm --prefix site run demo   # derives hero.mp4 and hero-poster.png from it
```

The README embeds the GIF. The site plays the mp4, which is about a third of the size and
keeps the landing page inside its performance budget, with the poster frame standing in
when the reader has Reduce Motion turned on. Run the second command after every recording,
or the derived files go stale silently.

`docs/demo.tape` calls `docs/demo-setup.sh`, which creates a disposable SVN repository and
working copy in a temporary directory, populates it with the files the demo walks through,
and starts `revision` there. Nothing touches a real working copy.

## Editing a tape

A tape is a plain-text script of terminal actions:

```text
Type "revision"
Enter
Sleep 2s
Down
Space
```

Keep them deterministic. `Sleep` durations are the usual source of flaky recordings — if a
step sometimes captures a half-rendered frame, the sleep before it is too short.

Recordings are committed to the repository, so a PR that changes one shows the new file in
its diff. Re-record from a clean checkout to avoid capturing local state.

:::caution[Check what you recorded]
Before committing a recording, watch it end to end and confirm no personal paths,
hostnames or real repository URLs leaked into a frame. The setup script uses a temporary
directory precisely so they cannot — but a prompt string or a shell integration can still
put your hostname on screen.
:::

## Keeping them small

GIFs are committed to the repository, so size matters. Reduce the frame rate or the
recorded window before reaching for heavier compression, and prefer several short demos
over one long one.
