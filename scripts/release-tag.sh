#!/usr/bin/env bash
#
# kafSIEM
# Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
# See NOTICE for provenance and LICENSE for repository-local terms.

set -euo pipefail

level="${1:-patch}"

if ! command -v git >/dev/null 2>&1; then
  echo "git is required"
  exit 1
fi

current_tag="$(git tag --list 'v*' --sort=-version:refname | head -n 1)"
if [[ -z "${current_tag}" ]]; then
  current_tag="v0.0.0"
fi

version="${current_tag#v}"
IFS='.' read -r major minor patch <<<"${version}"

case "${level}" in
  major)
    major=$((major + 1))
    minor=0
    patch=0
    ;;
  minor)
    minor=$((minor + 1))
    patch=0
    ;;
  patch)
    patch=$((patch + 1))
    ;;
  *)
    echo "Usage: $0 [patch|minor|major]"
    exit 1
    ;;
esac

next_tag="v${major}.${minor}.${patch}"

if git rev-parse "${next_tag}" >/dev/null 2>&1; then
  echo "Tag ${next_tag} already exists"
  exit 1
fi

git tag -a "${next_tag}" -m "Release ${next_tag}"
git push origin "${next_tag}"
echo "Created ${next_tag}"
