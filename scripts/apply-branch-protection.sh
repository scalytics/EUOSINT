#!/usr/bin/env bash
#
# EUOSINT
# Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
# See NOTICE for provenance and LICENSE for repository-local terms.

set -euo pipefail

branch="${1:-main}"

if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI is required"
  exit 1
fi

repo="${GITHUB_REPOSITORY:-}"
if [[ -z "${repo}" ]]; then
  repo="$(gh repo view --json nameWithOwner -q .nameWithOwner)"
fi

tmp_payload="$(mktemp)"
cat >"${tmp_payload}" <<JSON
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "CI / quality",
      "Docker / build",
      "CodeQL / analyze"
    ]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false,
    "required_approving_review_count": 1,
    "require_last_push_approval": false
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "block_creations": false,
  "required_conversation_resolution": true,
  "lock_branch": false,
  "allow_fork_syncing": true
}
JSON

gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  "repos/${repo}/branches/${branch}/protection" \
  --input "${tmp_payload}" >/dev/null

rm -f "${tmp_payload}"
echo "Applied branch protection to ${repo}:${branch}"
