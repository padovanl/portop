# Contributing

## Branching model

- `main` — always releasable. Protected: no direct pushes, no force-pushes,
  no deletion. Every change lands via a pull request from `develop` (or a
  `release/*`/`hotfix/*` branch). Every push to `main` is tagged and
  published as a release.
- `develop` — integration branch for day-to-day work. Feature branches
  (`feat/...`, `fix/...`) target `develop` via pull request.

```
feat/my-thing ──PR──▶ develop ──PR──▶ main ──tag──▶ release
```

## Pull requests

- Branch off `develop` (or off `main` only for a `hotfix/*`).
- Keep the change focused; unrelated cleanups belong in their own PR.
- A PR can only be merged once all required checks are green:
  - **unit tests** (`go test ./...`)
  - **e2e tests** (`go test -tags=e2e ./e2e/...`, exercises the real
    compiled binary against live sockets)
  - **release dry-run** (`goreleaser release --snapshot --clean`, proves the
    `.deb`/`.tar.gz` artifacts still build)
  - **lint** (`go vet` + `gofmt`)
- These are enforced by GitHub branch protection on `main` (and required for
  `develop` too) — see `scripts/setup-branch-protection.sh`.

## Local development

```sh
make build   # go build ./cmd/portop
make test    # unit tests
make e2e     # black-box tests against the compiled binary
make lint    # go vet + gofmt check
make release # local snapshot release build (deb + tar.gz, not published)
```

`make e2e` opens real listening sockets and shells out to the compiled
binary; like portop itself, it only runs on Linux (the scanner reads
`/proc/net` directly).

## Releasing

Releases are cut from `main` only. Merge `develop` into `main` via PR, then
push a `vX.Y.Z` tag on `main`:

```sh
git checkout main && git pull
git tag vX.Y.Z
git push origin vX.Y.Z
```

The `release` GitHub Actions workflow builds the `.deb` and `.tar.gz`
artifacts with GoReleaser and publishes them to the GitHub release for that
tag.

## Code style

- Run `gofmt` before committing (CI checks it).
- No comments explaining *what* code does — only *why*, when it's
  non-obvious (a workaround, an invariant, a constraint from `/proc`'s
  format).
- Keep packages under `internal/` small and single-purpose; see the existing
  layout (`scanner`, `procinfo`, `systemdinfo`, `docker`, `dnscache`, `ui`,
  `cli`) for the pattern.
