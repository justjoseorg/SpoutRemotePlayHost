#!/usr/bin/env bash
# Prints the release notes for VERSION (MAJOR.MINOR.PATCH).
# Minor releases (PATCH = 0) use their hand-written "## MAJOR.MINOR" section of CHANGELOG.md
# and fail if it is missing. Patch releases use the title and number of the merged PR.
# Usage: release-notes.sh VERSION [PR_TITLE PR_NUMBER]
set -euo pipefail
version=$1
minor=${version%.*}
patch=${version##*.}
if [ "$patch" = 0 ]; then
  notes=$(awk -v h="## ${minor}" '{ sub(/\r$/, "") } $0 == h { f = 1; next } /^## / { if (f) exit } f' CHANGELOG.md 2>/dev/null || true)
  if [ -z "$(printf '%s' "$notes" | tr -d '[:space:]')" ]; then
    echo "CHANGELOG.md has no '## ${minor}' section: minor releases need a written changelog." >&2
    exit 1
  fi
  printf '%s\n' "$notes"
else
  printf '%s (#%s)\n' "${2:-}" "${3:-}"
fi