#!/usr/bin/env bash
# Tags and pushes a release. Usage: scripts/release.sh <version> [--notes FILE]
# <version> can be given with or without the leading "v" (e.g. 0.1.0 or v0.1.0).
#
# Release notes are optional Markdown that GoReleaser puts above the commit
# list on the GitHub release. They travel in the annotated tag's message, so
# the file itself never needs to be committed. Without --notes, the script
# uses release-notes/<version>.md when it exists (that folder is gitignored).
set -euo pipefail

usage() {
	echo "Usage: $0 <version> [--notes FILE]   e.g. $0 v0.1.0" >&2
	exit 1
}

version=""
notes=""
while [ $# -gt 0 ]; do
	case "$1" in
	--notes)
		[ $# -ge 2 ] || usage
		notes="$2"
		shift 2
		;;
	-*) usage ;;
	*)
		[ -z "$version" ] || usage
		version="$1"
		shift
		;;
	esac
done
[ -n "$version" ] || usage

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
git fetch origin --quiet --tags main develop

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

if ! git merge-base --is-ancestor origin/develop HEAD; then
	echo "error: main does not contain the latest origin/develop; merge develop into main before tagging" >&2
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

if [ -z "$notes" ] && [ -f "release-notes/${version}.md" ]; then
	notes="release-notes/${version}.md"
fi

# The first line of the tag message is its subject; GoReleaser publishes
# only the body. A leading "# title" line in the notes is dropped because
# the release already carries the name "portop <version>".
message="$(mktemp)"
trap 'rm -f "$message"' EXIT
printf 'portop %s\n' "$version" >"$message"
if [ -n "$notes" ]; then
	if [ ! -s "$notes" ]; then
		echo "error: release notes '${notes}' are missing or empty" >&2
		exit 1
	fi
	printf '\n' >>"$message"
	awk 'NR == 1 && /^# / { skip = 1; next } skip && NF == 0 { next } { skip = 0; print }' "$notes" >>"$message"
fi

commit="$(git rev-parse --short HEAD)"
if [ -n "$notes" ]; then
	echo "Release notes from ${notes}:"
	echo "----------------------------------------"
	tail -n +3 "$message"
	echo "----------------------------------------"
else
	echo "No release notes file; the release will list commits only."
fi
read -r -p "Create and push tag ${version} on ${commit}? [y/N] " confirm
if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
	echo "Aborted."
	exit 1
fi

# verbatim keeps Markdown headings, which git would otherwise strip as
# comment lines.
git tag -a "$version" --cleanup=verbatim -F "$message"
git push origin "$version"

echo "Pushed ${version} — https://github.com/padovanl/portop/actions"
