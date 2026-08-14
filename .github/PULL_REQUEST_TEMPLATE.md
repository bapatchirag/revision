## What this changes

<!-- A short description, and the issue it closes. -->

Closes #

## How it was tested

<!-- The keys you pressed, the working copy you used, or the test that now covers it. -->

## Checklist

- [ ] `make fmt`, `make lint` and `make test` all pass locally
- [ ] Golden files were regenerated with `go test ./... -update`, and the diff contains
      only files this change should have touched
- [ ] New UI components have an interface assertion, a golden test, a `teatest` harness,
      and an entry in `cmd/gallery`
- [ ] Keymap changes are reflected in the `?` overlay, and `make site-data` was re-run
- [ ] Documentation under `site/` was updated if this changes behaviour a page describes
- [ ] No new dependency, or the PR explains why it is needed
