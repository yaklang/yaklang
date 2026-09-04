#!/usr/bin/env bash
set -euo pipefail

# Delete Go caches matching a key prefix, keeping only the newest $CACHE_KEEP
# entries. GitHub evicts caches by LRU once the repository quota is reached,
# which can drop the entry a running job is about to restore; pruning explicitly
# keeps the quota predictable.
CACHE_KEY_PREFIX="${CACHE_KEY_PREFIX:-}"
CACHE_KEEP="${CACHE_KEEP:-3}"
GITHUB_TOKEN="${GITHUB_TOKEN:-}"
REPOSITORY="${REPOSITORY:-${GITHUB_REPOSITORY:-}}"

if [[ -z "$CACHE_KEY_PREFIX" ]]; then
  echo "ERROR: CACHE_KEY_PREFIX must be set"
  exit 1
fi

if [[ -z "$GITHUB_TOKEN" || -z "$REPOSITORY" ]]; then
  echo "ERROR: GITHUB_TOKEN and REPOSITORY must be set"
  exit 1
fi

api() {
  curl -fsSL \
    -H "Accept: application/vnd.github+json" \
    -H "Authorization: Bearer $GITHUB_TOKEN" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "$@"
}

encoded=$(printf '%s' "$CACHE_KEY_PREFIX" | jq -sRr @uri)

# List every page that matches the prefix so old entries are not left behind.
page=1
ids=()
while [[ $page -le 10 ]]; do
  resp=$(api "https://api.github.com/repos/$REPOSITORY/actions/caches?key=$encoded&per_page=100&page=$page" || echo '{"actions_caches":[]}')
  count=$(printf '%s' "$resp" | jq '.actions_caches | length')
  if [[ "$count" == "0" ]]; then
    break
  fi
  while IFS= read -r id; do
    [[ -n "$id" ]] && ids+=("$id")
  done < <(printf '%s' "$resp" | jq -r '.actions_caches[] | "\(.created_at) \(.id)"' | sort -r | awk '{print $2}')
  page=$((page + 1))
done

if [[ ${#ids[@]} -eq 0 ]]; then
  echo "No caches matched prefix $CACHE_KEY_PREFIX"
  exit 0
fi

# ids were appended per page already sorted newest-first; re-sort the whole set.
mapfile -t ordered < <(printf '%s\n' "${ids[@]}" | awk '{print NR"\t"$0}' | sort -n | cut -f2)

total=${#ordered[@]}
keep=$CACHE_KEEP
if (( total <= keep )); then
  echo "Caches under $CACHE_KEY_PREFIX: $total (keep $keep) - nothing to prune"
  exit 0
fi

for ((i=keep; i<total; i++)); do
  cid="${ordered[$i]}"
  if api -X DELETE "https://api.github.com/repos/$REPOSITORY/actions/caches/$cid" >/dev/null; then
    echo "Deleted cache $cid"
  else
    echo "::warning::Failed to delete cache $cid"
  fi
done

echo "Pruned $((total - keep)) of $total caches under $CACHE_KEY_PREFIX (kept $keep newest)"
