---
title: Regenerating demos
description: How the demo recordings are produced, and how to rebuild them.
---

The recordings are produced with [VHS](https://github.com/charmbracelet/vhs) from scripted
tapes, so they are reproducible and reviewable in a diff rather than being screen-captured
by hand.

## Prerequisites

| Tool | Why |
|---|---|
| `vhs` | Drives the recording. `brew install vhs`. |
| `ttyd`, `ffmpeg` | VHS's own dependencies, installed with it. |
| `svn`, `svnadmin` | The tapes build a throwaway repository and working copy. |
| A Go toolchain | The tapes build `revision` from the checkout they are recording. |

## Rebuilding everything

```sh
make demos
```

That records every tape in `docs/tapes/` and then derives the poster frames. It takes a few
minutes — each tape rebuilds the fixture from scratch.

## What is where

| File | What it is |
|---|---|
| `docs/tapes/common.tape` | Settings and hidden setup, sourced by every tape. Not runnable on its own. |
| `docs/tapes/config.json` | The config copied into the recording's isolated `XDG_CONFIG_HOME`. |
| `docs/tapes/*.tape` | One tape per feature, each writing `site/public/demos/<name>.mp4`. |
| `docs/demo.tape` | The landing-page hero, which is a GIF because the README embeds it. |
| `docs/demo-setup.sh` | Builds the throwaway repository and working copy every tape records against. |

The per-feature recordings are mp4 rather than GIF: the same footage as a GIF is several
times the size and becomes its page's largest contentful paint.
`site/scripts/make-demo.mjs` derives a poster frame for each one — that is what the page
paints before anything is fetched, and all that is ever shown to a reader with Reduce
Motion turned on.

The hero is the exception. It exists as a GIF first because the README embeds it, and the
same script cuts its mp4 and poster from that GIF. Recording it is a separate step:

```sh
vhs docs/demo.tape           # writes site/public/demos/hero.gif
npm --prefix site run demo   # derives the hero's mp4 and every poster
```

## Matching the site palette

A recording has to look like the page it sits on, so Cipher is pinned twice: `Set Theme` in
`common.tape` colours the terminal VHS renders into, and `docs/tapes/config.json` sets
`"theme": "cipher"` for `revision` itself. That config is copied into an isolated
`XDG_CONFIG_HOME` under the temporary directory, so a recording never reads or writes your
real `~/.config/revision/config.json`.

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
step sometimes captures a half-rendered frame, the sleep before it is too short. Prefer
`Wait+Screen` to a guessed `Sleep` when there is something on screen to wait for.

Anchor navigation instead of counting rows from wherever the cursor happens to be: jump to
the top of the panel with `g` first, then move a known number of rows.

Everything between `Hide` and `Show` still runs but is not recorded, so the fixture build,
the launch and any positioning belong there. The first visible frame is then a fully drawn
screen — which is also what the poster is cut from.

Recordings are committed to the repository, so a PR that changes one shows the new file in
its diff. Re-record from a clean checkout to avoid capturing local state.

:::caution[Check what you recorded]
Before committing a recording, watch it end to end and confirm no personal paths,
hostnames, usernames or real repository URLs leaked into a frame. The fixture lives in a
temporary directory and its commits are authored by a fixed name precisely so they cannot
— but a prompt string or a shell integration can still put your hostname on screen.
:::

## Keeping them small

Recordings are committed to the repository, so size matters. Reduce the frame rate or the
recorded window before reaching for heavier compression, and prefer several short demos
over one long one.
