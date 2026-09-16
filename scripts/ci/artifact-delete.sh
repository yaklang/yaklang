#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_ID="${ARTIFACT_ID:-}"
GITHUB_TOKEN="${GITHUB_TOKEN:-}"
REPOSITORY="${REPOSITORY:-${GITHUB_REPOSITORY:-}}"
ARTIFACT_LABEL="${ARTIFACT_LABEL:-artifact}"

# Deleting a prepared artifact is hygiene, not verification: it reclaims storage
# after a suite has already consumed the artifact. Nothing downstream depends on
# it, so every failure mode below degrades to a warning instead of a non-zero
# exit that would turn an otherwise green run red.
if [[ -z "$ARTIFACT_ID" ]]; then
  echo "::warning::No ${ARTIFACT_LABEL} artifact id found, skip deletion"
  exit 0
fi

if [[ -z "$GITHUB_TOKEN" || -z "$REPOSITORY" ]]; then
  echo "::warning::GITHUB_TOKEN or REPOSITORY is unset, skip deletion of ${ARTIFACT_LABEL} artifact $ARTIFACT_ID"
  exit 0
fi

curl -fsSL --connect-timeout 10 --max-time 60 \
  -X DELETE \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer $GITHUB_TOKEN" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "https://api.github.com/repos/$REPOSITORY/actions/artifacts/$ARTIFACT_ID" \
  && echo "Deleted ${ARTIFACT_LABEL} artifact $ARTIFACT_ID" \
  || echo "::warning::Failed to delete ${ARTIFACT_LABEL} artifact $ARTIFACT_ID (already gone, expired, or read-only token); nothing depends on this"
