---
title: Contributing
description: How to propose a change to revision.
---

Issues and pull requests are welcome — bug reports, feature ideas, and documentation
fixes all help.

- [Issue tracker](https://github.com/bapatchirag/revision/issues)
- [Pull requests](https://github.com/bapatchirag/revision/pulls)

By taking part you agree to the
[Code of Conduct](https://github.com/bapatchirag/revision/blob/main/CODE_OF_CONDUCT.md).
Found a vulnerability? Do not open an issue — follow the
[security policy](https://github.com/bapatchirag/revision/security/policy) instead.

## Before you start

Open an issue first for anything larger than a bug fix. It is much cheaper to agree on an
approach in an issue than to unpick a finished branch — especially for changes to the
layout or the keymap, where every golden test moves.

Read [Architecture](/develop/architecture/) to find the right layer for your change.

## The gates

```sh
make fmt
make lint
make test
```

All three must pass before a PR is ready. CI runs the same checks.

`make lint` needs
[`golangci-lint`](https://golangci-lint.run/welcome/install/) installed separately.

CI also runs a cross-compile of every release target, `shellcheck` over `install.sh`, a
build and link check of this website, and three security gates a PR has to be clean of:

| Gate | What it reads |
|---|---|
| `govulncheck` over `./...`, after `go mod tidy -diff` and `go mod verify` | Known advisories against code the build can actually reach, standard library included |
| Dependency review, on pull requests, failing at moderate severity | A vulnerable dependency in the PR that adds it — `go.mod` and the site's npm tree alike |
| CodeQL with the `security-extended` queries, plus a weekly run | This repository's own code, which the other two never look at |

Only the first can be run locally, with
[`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck).

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
the `?` overlay has no scrolling — its height is its item count, and at 80×24 it is
already full. A new row usually means merging two existing ones.

## Tests that need `svn`

Some tests drive a real `svn` binary against a throwaway repository created with
`svnadmin`. They skip automatically when neither is on the `PATH`. Install Subversion
locally if you are changing anything under `internal/svn` — otherwise you are running with
a large part of the suite silently skipped.

## Documentation

This site lives in `site/`. Every page has an **Edit this page** link at the bottom that
opens the right source file on GitHub.

If your change affects behaviour a page describes, update the page in the same PR. Prefer
adding to the existing page over creating a new one.

## Licence

`revision` is [MIT](https://github.com/bapatchirag/revision/blob/main/LICENSE) licensed.
Contributions are accepted under the same terms.
