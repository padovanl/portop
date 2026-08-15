# Contributing

## Branching model

- `main` — always releasable. Protected: no direct pushes, no force-pushes,
  no deletion. Every change lands via a pull request from `develop` (or a
  `release/*`/`hotfix/*` branch). Pushing a `vX.Y.Z` tag on `main` triggers
  the release pipeline; the release workflow independently refuses to run
  if that tag isn't actually reachable from `main`, so there's no way to
  accidentally ship something that skipped `develop`.
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

## Releasing (maintainers)

This section only applies if you have push access to `main` — regular
contributors don't need any of this.

Releases are cut from `main` only, and only from a commit that arrived
there via a pull request from `develop` with every required check green.
To ship a release:

```sh
# 1. Open a PR from develop into main, wait for CI to go green, merge it.
git checkout main && git pull

# 2. Tag and push — this is the only step that actually triggers a release.
scripts/release.sh vX.Y.Z
```

`scripts/release.sh` refuses to run unless you're on `main`, the working
tree is clean, local `main` matches `origin/main`, and the tag doesn't
already exist locally or on the remote — then it tags and pushes after a
confirmation prompt. That push triggers the [release
workflow](.github/workflows/release.yml), which independently re-checks
the tag is reachable from `main` (so there's no way to accidentally ship
a tag that skipped the PR) before building the `.deb`/`.tar.gz` artifacts
with GoReleaser and publishing them to a new GitHub Release.

## Code style

- Run `gofmt` before committing (CI checks it).
- No comments explaining *what* code does — only *why*, when it's
  non-obvious (a workaround, an invariant, a constraint from `/proc`'s
  format).
- Keep packages under `internal/` small and single-purpose; see the existing
  layout (`scanner`, `procinfo`, `systemdinfo`, `docker`, `dnscache`, `ui`,
  `cli`) for the pattern.
