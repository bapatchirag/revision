---
title: Building from source
description: Compile, test and cross-compile the revision tree.
---

## Get the source

```sh
git clone https://github.com/bapatchirag/revision.git
cd revision
```

You need a Go toolchain matching the version in
[`go.mod`](https://github.com/bapatchirag/revision/blob/main/go.mod), and `svn` on your
`PATH` to run the integration tests.

## Build

```sh
make build      # compiles ./bin/revision
make test
```

`make build` produces a development build: it reports its version as `dev` and never
[self-updates](/operations/updating-revision/).

## Make targets

| Target | What it does |
|---|---|
| `make build` | Compile `./bin/revision`. |
| `make run` | Run the TUI from source. |
| `make run-gallery` | Preview the reusable components in isolation. |
| `make test` | Run all tests. |
| `make cover` | Run tests with a coverage summary. |
| `make vet` | Run `go vet`. |
| `make lint` | Run `golangci-lint` (must be installed separately). |
| `make fmt` | `gofmt` the tree. |
| `make tidy` | Tidy `go.mod` / `go.sum`. |
| `make cross` | Build static binaries for every release target. |
| `make clean` | Remove build artifacts. |

## Cross-compiling

```sh
make cross
```

That writes one static binary per release target:

```text
dist/revision-darwin-arm64
dist/revision-darwin-amd64
dist/revision-linux-amd64
dist/revision-linux-arm64
```

These are the four targets the [install script](/guides/installation/) serves; `make
build-darwin-arm64` and its three siblings build them one at a time. Locally
cross-compiled binaries are still development builds — the release channel is stamped in
by the release pipeline, not by `make cross`.

## Running it

```sh
./bin/revision --path /path/to/working-copy
```

Or `make run`, which builds and starts it in the current directory.

## The component gallery

```sh
make run-gallery
```

`cmd/gallery` renders every reusable component from `internal/tui/component` on its own,
without any SVN data. It is the fastest way to see a component change — no working copy
required.

## Tests

```sh
make test
```

Some tests drive a real `svn` binary against a throwaway repository created with
`svnadmin`. They skip automatically when `svn` and `svnadmin` are not on the `PATH`, so
the suite passes on a machine without Subversion — it just covers less.

Golden tests compare rendered output byte for byte. Regenerate them after an intentional
UI change:

```sh
go test ./... -update
```

Read the diff before committing regenerated goldens. An unexpected file in that diff means
the change reached further than intended.

## Before opening a PR

```sh
make fmt && make lint && make test
```

See [Contributing](/develop/contributing/).
