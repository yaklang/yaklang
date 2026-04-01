#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_ID="${ARTIFACT_ID:-}"
GITHUB_TOKEN="${GITHUB_TOKEN:-}"
REPOSITORY="${REPOSITORY:-${GITHUB_REPOSITORY:-}}"
ARTIFACT_LABEL="${ARTIFACT_LABEL:-artifact}"

if [[ -z "$ARTIFACT_ID" ]]; then
  echo "::warning::No ${ARTIFACT_LABEL} artifact id found, skip deletion"
  exit 0
fi

if [[ -z "$GITHUB_TOKEN" || -z "$REPOSITORY" ]]; then
  echo "ERROR: GITHUB_TOKEN and REPOSITORY must be set"
  exit 1
fi

curl -fsSL \
  -X DELETE \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer $GITHUB_TOKEN" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "https://api.github.com/repos/$REPOSITORY/actions/artifacts/$ARTIFACT_ID" \
  && echo "Deleted ${ARTIFACT_LABEL} artifact $ARTIFACT_ID" \
  || echo "::warning::Failed to delete ${ARTIFACT_LABEL} artifact $ARTIFACT_ID"
