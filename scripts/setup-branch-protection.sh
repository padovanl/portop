#!/usr/bin/env bash
# Configures GitHub branch protection on main/develop to match
# CONTRIBUTING.md: no direct pushes, PRs required, and the CI checks
# (lint, unit tests, e2e tests, release dry-run) must pass before merging.
#
# Requires: gh (https://cli.github.com/), authenticated (`gh auth login`),
# and to be run from inside the portop repo once it has a GitHub remote.
#
# Usage: scripts/setup-branch-protection.sh

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

echo "Configuring branch protection on $repo..."

protect() {
  local branch="$1"
  gh api \
    --method PUT \
    -H "Accept: application/vnd.github+json" \
    "repos/$repo/branches/$branch/protection" \
    -f "required_status_checks[strict]=true" \
    -f "required_status_checks[checks][][context]=lint" \
    -f "required_status_checks[checks][][context]=unit tests" \
    -f "required_status_checks[checks][][context]=e2e tests" \
    -f "required_status_checks[checks][][context]=release dry-run (proof of release)" \
    -F "enforce_admins=true" \
    -f "required_pull_request_reviews[required_approving_review_count]=0" \
    -F "required_pull_request_reviews[dismiss_stale_reviews]=true" \
    -F "restrictions=null" \
    -F "allow_force_pushes=false" \
    -F "allow_deletions=false" \
    -F "required_linear_history=true"
  echo "  $branch protected."
}

protect main
protect develop

echo "Done. Direct pushes to main/develop are now blocked; changes must go"
echo "through a pull request with lint, unit tests, e2e tests and the"
echo "release dry-run all green."
