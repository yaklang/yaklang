#!/usr/bin/env bash
set -euo pipefail

# Delete Go caches matching a key prefix. Pruning is driven by age and total
# size, not by entry count: several PRs can run CI at once, so a count cap
# would evict one PR's cache just because another PR published more entries.
#   - CACHE_MAX_AGE_DAYS: drop entries older than this (0 = disabled)
#   - CACHE_MAX_SIZE_GB:  keep the newest entries until total size fits (0 = disabled)
#   - CACHE_KEEP:         optional hard count cap, newest-first (0 = disabled)
# GitHub evicts caches by LRU once the repository quota is reached, which can
# drop the entry a running job is about to restore; pruning explicitly keeps
# the quota predictable.
CACHE_KEY_PREFIX="${CACHE_KEY_PREFIX:-}"
CACHE_KEEP="${CACHE_KEEP:-0}"
CACHE_MAX_AGE_DAYS="${CACHE_MAX_AGE_DAYS:-0}"
CACHE_MAX_SIZE_GB="${CACHE_MAX_SIZE_GB:-0}"
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

# Collect "created_at size id" triples, newest first, across all pages.
entries=()
page=1
while [[ $page -le 10 ]]; do
  resp=$(api "https://api.github.com/repos/$REPOSITORY/actions/caches?key=$encoded&per_page=100&page=$page" || echo '{"actions_caches":[]}')
  count=$(printf '%s' "$resp" | jq '.actions_caches | length')
  if [[ "$count" == "0" ]]; then
    break
  fi
  while IFS= read -r line; do
    [[ -n "$line" ]] && entries+=("$line")
  done < <(printf '%s' "$resp" | jq -r '.actions_caches[] | "\(.created_at) \(.size_in_bytes) \(.id)"' | sort -r)
  page=$((page + 1))
done

if [[ ${#entries[@]} -eq 0 ]]; then
  echo "No caches matched prefix $CACHE_KEY_PREFIX"
  exit 0
fi

total=${#entries[@]}
deleted=0

delete_entry() {
  local cid="$1" reason="$2"
  if api -X DELETE "https://api.github.com/repos/$REPOSITORY/actions/caches/$cid" >/dev/null; then
    echo "Deleted cache $cid ($reason)"
    deleted=$((deleted + 1))
  else
    echo "::warning::Failed to delete cache $cid ($reason)"
  fi
}

# 1) Age bound: drop anything older than CACHE_MAX_AGE_DAYS.
if (( CACHE_MAX_AGE_DAYS > 0 )); then
  cutoff=$(date -u -d "$CACHE_MAX_AGE_DAYS days ago" +%s 2>/dev/null || date -u -v-${CACHE_MAX_AGE_DAYS}d +%s)
  remaining=()
  for entry in "${entries[@]}"; do
    created=$(printf '%s' "$entry" | awk '{print $1}')
    cid=$(printf '%s' "$entry" | awk '{print $3}')
    created_ts=$(date -u -d "$created" +%s 2>/dev/null || date -u -j -f '%Y-%m-%dT%H:%M:%SZ' "$created" +%s 2>/dev/null || echo 0)
    if (( created_ts > 0 && created_ts < cutoff )); then
      delete_entry "$cid" "older than ${CACHE_MAX_AGE_DAYS}d"
    else
      remaining+=("$entry")
    fi
  done
  entries=("${remaining[@]}")
fi

# 2) Size bound: keep the newest entries until total size fits.
if (( CACHE_MAX_SIZE_GB > 0 )); then
  max_bytes=$((CACHE_MAX_SIZE_GB * 1024 * 1024 * 1024))
  total_bytes=0
  for entry in "${entries[@]}"; do
    size=$(printf '%s' "$entry" | awk '{print $2}')
    total_bytes=$((total_bytes + size))
  done
  while (( total_bytes > max_bytes && ${#entries[@]} > 1 )); do
    last=${entries[${#entries[@]}-1]}
    size=$(printf '%s' "$last" | awk '{print $2}')
    cid=$(printf '%s' "$last" | awk '{print $3}')
    delete_entry "$cid" "size cap ${CACHE_MAX_SIZE_GB}GB"
    total_bytes=$((total_bytes - size))
    unset 'entries[${#entries[@]}-1]'
    entries=("${entries[@]}")
  done
fi

# 3) Optional hard count cap, newest-first.
if (( CACHE_KEEP > 0 && ${#entries[@]} > CACHE_KEEP )); then
  for ((i=CACHE_KEEP; i<${#entries[@]}; i++)); do
    cid=$(printf '%s' "${entries[$i]}" | awk '{print $3}')
    delete_entry "$cid" "count cap $CACHE_KEEP"
  done
fi

if (( deleted > 0 )); then
  echo "Pruned $deleted of $total caches under $CACHE_KEY_PREFIX (age ${CACHE_MAX_AGE_DAYS}d, size ${CACHE_MAX_SIZE_GB}GB, keep $CACHE_KEEP)"
else
  echo "Caches under $CACHE_KEY_PREFIX: $total (age ${CACHE_MAX_AGE_DAYS}d, size ${CACHE_MAX_SIZE_GB}GB, keep $CACHE_KEEP) - nothing to prune"
fi
