#!/usr/bin/env bash
# One-time setup: point GitHub Pages at the "Deploy Pages" Actions
# workflow (.github/workflows/pages.yml) instead of a branch/folder.
# After this runs once, every push to main that touches docs/ redeploys
# the site automatically.
#
# Requires: gh (https://cli.github.com/), authenticated, run from inside
# the portop repo once it has a GitHub remote.
#
# Usage: scripts/setup-pages.sh

set -euo pipefail

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh (GitHub CLI) is not installed. See https://cli.github.com/" >&2
  exit 1
fi

repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null || true)"
if [ -z "$repo" ]; then
  echo "error: could not determine the GitHub repo (are you inside it, and is 'gh' authenticated?)" >&2
  exit 1
fi

echo "Enabling GitHub Pages (Actions-driven) on $repo..."

if gh api "repos/$repo/pages" >/dev/null 2>&1; then
  gh api --method PUT "repos/$repo/pages" -f "build_type=workflow" >/dev/null
  echo "Pages source updated to GitHub Actions."
else
  gh api --method POST "repos/$repo/pages" -f "build_type=workflow" >/dev/null
  echo "Pages site created, source set to GitHub Actions."
fi

echo "Push to main (touching docs/) or run the 'Deploy Pages' workflow manually to publish."
echo "URL: https://$(echo "$repo" | cut -d/ -f1).github.io/$(echo "$repo" | cut -d/ -f2)/"
