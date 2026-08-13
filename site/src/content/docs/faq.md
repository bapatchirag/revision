---
title: FAQ
description: Why revision exists, what it touches, where it runs, and where the name came from.
---

## Why not just use git, or git-svn?

If you can move to git, move to git. Plenty of people cannot: the repository, the release
process and the audit trail are organisational decisions that sit well above any one
developer's preference. `git-svn` bridges the two, but it asks you to keep a second
history in step with the first and to translate between two models every time something
goes wrong.

`revision` takes the other road. SVN stays the source of truth, and the day-to-day work
over it gets a proper interface instead. A tool that only has to speak one language can
speak it plainly — a changelist is a changelist here, not a stand-in for something else —
and, frankly, it looks better doing it.

## Is SVN dead?

No. Apache shipped [Subversion 1.15.0-rc3](https://subversion.apache.org/news) on
20 July 2026 — the project is still cutting releases in the open. Large centralised
codebases, with per-path access control and decades of history behind them, have not gone
anywhere either, and the tooling around them has to keep up.

## Does this replace the `svn` binary?

No — it drives it. Every action shells out to the `svn` already on your `PATH` and reads
what comes back; nothing about the working copy format is reimplemented underneath. Your
`~/.subversion/config`, your cached credentials, your hooks and your server behave exactly
as they do on the command line. Remove `revision` and the working copy is untouched —
which also means anything it did can be undone with `svn` directly.

## Is it safe? Does it cache anything behind my back?

Every command it runs is listed in the [Command Log](/guides/panels/) panel as it happens,
with its full argument list, its exit code and its output. Nothing runs that you cannot
read.

There is no shadow copy of the working copy and no index of its own — state lives in SVN,
and `revision` re-reads it. The only files it owns are under `~/.config/revision/`: your
[settings](/reference/configuration/) and a note of when it last checked for a release.
Diffs reach the disk only when you [ask for one](/workflows/diffs/#saving-a-diff), where
you ask for it. An [svn+ssh passphrase](/operations/authentication/) is handed to
`ssh-add` through the environment and is never written down.

## Is there a Windows build?

Not today, and it is not a focus. Windows already has SVN clients that a terminal UI will
not beat on their own ground — TortoiseSVN puts the whole thing in Explorer, with more
than this can offer. Prebuilt binaries cover macOS and Linux; see
[Requirements](/guides/requirements/).

Nothing rules it out, though. If you want it, say so on the
[issue tracker](https://github.com/bapatchirag/revision/issues).

## Why is it called `revision` and not lazysvn?

While `lazysvn` is an obvious choice (given the inspiration), this is just to avoid being overly derivative.
