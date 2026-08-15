#!/usr/bin/env bash
# Tags and pushes a release. Usage: scripts/release.sh <version>
# <version> can be given with or without the leading "v" (e.g. 0.1.0 or v0.1.0).
set -euo pipefail

if [ $# -ne 1 ]; then
	echo "Usage: $0 <version>   e.g. $0 v0.1.0" >&2
	exit 1
fi

version="$1"
[[ "$version" == v* ]] || version="v${version}"

if ! [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
	echo "error: '$version' doesn't look like a valid semver tag (expected vX.Y.Z)" >&2
	exit 1
fi

branch="$(git rev-parse --abbrev-ref HEAD)"
if [ "$branch" != "main" ]; then
	echo "error: releases are cut from 'main' (you're on '$branch')" >&2
	exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
	echo "error: working tree is not clean, commit or stash changes first" >&2
	exit 1
fi

echo "Fetching origin..."
git fetch origin --quiet --tags main

if [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]; then
	ahead="$(git rev-list --count origin/main..HEAD)"
	behind="$(git rev-list --count HEAD..origin/main)"
	if [ "$behind" != "0" ]; then
		echo "error: local main is behind origin/main by ${behind} commit(s); run 'git pull origin main'" >&2
	else
		echo "error: local main is ahead of origin/main by ${ahead} commit(s); run 'git push origin main'" >&2
	fi
	exit 1
fi

if git rev-parse -q --verify "refs/tags/${version}" >/dev/null; then
	echo "error: tag '${version}' already exists locally" >&2
	exit 1
fi

if git ls-remote --exit-code --tags origin "refs/tags/${version}" >/dev/null 2>&1; then
	echo "error: tag '${version}' already exists on origin" >&2
	exit 1
fi

commit="$(git rev-parse --short HEAD)"
read -r -p "Create and push tag ${version} on ${commit}? [y/N] " confirm
if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
	echo "Aborted."
	exit 1
fi

git tag "$version"
git push origin "$version"

echo "Pushed ${version} — https://github.com/padovanl/portop/actions"
