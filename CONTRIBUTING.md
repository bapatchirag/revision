# Contributing to revision

Issues and pull requests are welcome — bug reports, feature ideas, and documentation fixes
all help.

- [Issue tracker](https://github.com/bapatchirag/revision/issues)
- [Pull requests](https://github.com/bapatchirag/revision/pulls)

By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Before you start

Open an issue first for anything larger than a bug fix. It is much cheaper to agree on an
approach in an issue than to unpick a finished branch — especially for changes to the
layout or the keymap, where every golden test moves.

Read [Architecture](https://bapatchirag.github.io/revision/develop/architecture/) to find
the right layer for your change.

## Setting up

```sh
git clone https://github.com/bapatchirag/revision.git
cd revision
make build
make test
```

You need:

- A Go toolchain matching the version in [`go.mod`](go.mod).
- `svn` on your `PATH` — the tests that drive a real repository skip without it.
- [`golangci-lint`](https://golangci-lint.run/welcome/install/) for `make lint`.
- Node 22+ only if you are changing the website under `site/`.

[Building from source](https://bapatchirag.github.io/revision/develop/building/) lists
every `make` target.

## The gates

```sh
make fmt
make lint
make test
```

All three must pass before a PR is ready. CI runs the same checks, plus a cross-compile of
every release target, `shellcheck` over `install.sh`, and a build and link check of the
website.

## Conventions

**Respect the layering.** `internal/tui` may not import `internal/svn` or `internal/app`.
A guard test enforces it, so a violation shows up as a test failure rather than a review
comment.

**New UI components follow the existing contract:**

1. A compile-time interface assertion.
2. A golden test over `View()`.
3. A `teatest` harness driving it as a real Bubble Tea program.

Add the component to `cmd/gallery` too, so it can be previewed in isolation with
`make run-gallery`.

**Goldens are regenerated, not hand-edited:**

```sh
go test ./... -update
```

Read the resulting diff. An unexpected file in it means the change reached further than
intended — that is the signal the golden tests exist to give you.

**Keybindings live in one table.** Adding a binding means adding it to the help menu, and
the `?` overlay has no scrolling — its height is its item count, and at 80×24 it is already
full. A new row usually means merging two existing ones. The website's keybindings
reference is generated from that table, so run `make site-data` and commit the result; CI
fails on drift.

## Commits and pull requests

- Keep a PR to one change. Split unrelated fixes into their own branches.
- Write the commit subject in the imperative mood — "Add a hunk-level stage key", not
  "Added…".
- Reference the issue the PR closes.
- Update the documentation in the same PR when behaviour a page describes changes.

## Documentation

The website lives in `site/`. Every page has an **Edit this page** link at the bottom that
opens the right source file on GitHub. Prefer adding to an existing page over creating a
new one.

## Security

Do not open a public issue for a vulnerability. Follow the
[security policy](SECURITY.md) instead.

## Licence

`revision` is [MIT](LICENSE) licensed. Contributions are accepted under the same terms.
