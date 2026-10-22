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
    `.deb`/`.rpm`/`.tar.gz` artifacts still build)
  - **lint** (`go vet` + `gofmt`)
- These are enforced by GitHub branch protection on `main` (and required for
  `develop` too) — see `scripts/setup-branch-protection.sh`.

## Local development

```sh
make build   # go build ./cmd/portop
make test    # unit tests
make e2e     # black-box tests against the compiled binary
make lint    # go vet + gofmt check
make release # local snapshot release build (deb + rpm + Linux/macOS tar.gz, not published)
```

`make e2e` opens real listening sockets and shells out to the compiled
binary on Linux and macOS. CI runs native macOS unit and end-to-end tests
on Apple Silicon and Intel runners.

### Desktop app

`desktop/` is a separate Go module (Wails v3) that reuses portop's internal
packages. Its window is plain HTML, CSS and JavaScript in `desktop/frontend/`,
with no build step. To work on it without the native toolkit:

```sh
cd desktop
go run ./cmd/devserver          # http://127.0.0.1:34115, frontend read from disk
go test ./backend
```

The native app needs GTK4 and WebKitGTK 6 headers on Linux
(`libgtk-4-dev libwebkitgtk-6.0-dev`, Ubuntu 24.04+), Xcode command line tools
on macOS, nothing extra on Windows. `desktop/build/package.sh VERSION` builds
and packages it for the current OS, as the Desktop workflow does on release.

## Documentation

The pages under `docs/docs/` are generated: edit `docs-src/<page>.html`, then
run `python3 scripts/build-docs.py`, which rebuilds the pages and the search
index and fails on a broken internal link. A new page also needs an entry in
the `NAV` list at the top of the script. CI runs `build-docs.py --check` to
catch pages that were edited directly or not rebuilt. Preview the site with
`python3 scripts/serve-pages.py`.

## Releasing (maintainers)

This section only applies if you have push access to `main` — regular
contributors don't need any of this.

Releases are cut from `main` only, and only from a commit that arrived
there via a pull request from `develop` with every required check green.
To ship a release:

```sh
# 1. Open a PR from develop into main, wait for CI to go green, merge it.
git checkout main && git pull

# 2. Optional: write the release notes. release-notes/ is gitignored.
$EDITOR release-notes/vX.Y.Z.md

# 3. Tag and push — this is the only step that actually triggers a release.
scripts/release.sh vX.Y.Z            # picks up release-notes/vX.Y.Z.md
scripts/release.sh vX.Y.Z --notes path/to/notes.md
```

The notes are stored in the annotated tag's message, not in a commit. The
release workflow publishes them at the top of the GitHub release, followed
by the commits since the previous tag, grouped into features, fixes and
other changes. A leading `# title` line in the notes file is dropped
because the release is already named `portop vX.Y.Z`. Without a notes file
the release lists commits only.

`scripts/release.sh` refuses to run unless you're on `main`, the working
tree is clean, local `main` matches `origin/main`, and the tag doesn't
already exist locally or on the remote — then it shows the notes, and tags
and pushes after a confirmation prompt. That push triggers the [release
workflow](.github/workflows/release.yml), which independently re-checks
the tag is reachable from `main` (so there's no way to accidentally ship
a tag that skipped the PR) before building the `.deb`/`.rpm`/`.tar.gz` artifacts
with GoReleaser and publishing them to a new GitHub Release.

Every GitHub Action is pinned to a commit SHA and GoReleaser to an exact
version, so a release never depends on a mutable upstream tag. The tag is
signed when git has a `user.signingkey`, and the workflow records a GitHub
artifact attestation for each published file. To check a download:

```sh
gh attestation verify portop_X.Y.Z_linux_amd64.tar.gz --repo padovanl/portop
git fetch --tags origin && git verify-tag vX.Y.Z   # needs the maintainer's public key
```

## Code style

- Run `gofmt` before committing (CI checks it).
- No comments explaining *what* code does — only *why*, when it's
  non-obvious (a workaround, an invariant, a constraint from `/proc`'s
  format).
- Keep packages under `internal/` small and single-purpose; see the existing
  layout (`scanner`, `procinfo`, `systemdinfo`, `docker`, `dnscache`, `ui`,
  `cli`) for the pattern.
