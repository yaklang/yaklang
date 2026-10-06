#!/usr/bin/env bash
set -euo pipefail

# Bound build snapshots without letting one PR evict main's warm dependencies.
# Older snapshots of the same namespace/ref are redundant; prune those first.
# Under size/count pressure protect the remaining main and current-ref entries.
CACHE_KEY_PREFIX="${CACHE_KEY_PREFIX:-}"
CACHE_KEEP="${CACHE_KEEP:-0}"
CACHE_MAX_AGE_DAYS="${CACHE_MAX_AGE_DAYS:-0}"
CACHE_MAX_SIZE_GB="${CACHE_MAX_SIZE_GB:-0}"
CACHE_PROTECTED_REF="${CACHE_PROTECTED_REF:-refs/heads/main}"
GITHUB_TOKEN="${GITHUB_TOKEN:-}"
REPOSITORY="${REPOSITORY:-${GITHUB_REPOSITORY:-}}"

if [[ -z "$CACHE_KEY_PREFIX" || -z "$GITHUB_TOKEN" || -z "$REPOSITORY" ]]; then
  echo "::warning::Cache prefix, token or repository is unset, skipping cache prune"
  exit 0
fi
for value in "$CACHE_KEEP" "$CACHE_MAX_AGE_DAYS" "$CACHE_MAX_SIZE_GB"; do
  if [[ ! "$value" =~ ^[0-9]+$ ]]; then
    echo "::error::Cache limits must be nonnegative integers"
    exit 1
  fi
done

api() {
  curl -fsSL --connect-timeout 10 --max-time 60 \
    -H "Accept: application/vnd.github+json" \
    -H "Authorization: Bearer $GITHUB_TOKEN" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "$@"
}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
encoded=$(printf '%s' "$CACHE_KEY_PREFIX" | jq -sRr @uri)
# Complete the inventory before any deletion. An API/page failure must not prune
# from an incomplete list. The API's per-page order is not a global age order.
page=1
while :; do
  api "https://api.github.com/repos/$REPOSITORY/actions/caches?key=$encoded&per_page=100&page=$page" > "$work/page-$page.json"
  jq -e '.actions_caches | type == "array"' "$work/page-$page.json" >/dev/null
  count=$(jq '.actions_caches | length' "$work/page-$page.json")
  [[ "$count" -lt 100 ]] && break
  page=$((page + 1))
done

cutoff=""
if (( CACHE_MAX_AGE_DAYS > 0 )); then
  cutoff=$(date -u -d "$CACHE_MAX_AGE_DAYS days ago" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-${CACHE_MAX_AGE_DAYS}d +%Y-%m-%dT%H:%M:%SZ)
fi
jq -s --arg prefix "$CACHE_KEY_PREFIX" --arg cutoff "$cutoff" \
  --arg protected "$CACHE_PROTECTED_REF" --arg current "${GITHUB_REF:-}" \
  --argjson max_bytes "$((CACHE_MAX_SIZE_GB * 1024 * 1024 * 1024))" \
  --argjson max_count "$CACHE_KEEP" '
  [.[].actions_caches[] | select(.key | startswith($prefix))]
  | unique_by(.id) | sort_by(.created_at, .id) | reverse
  | reduce .[] as $entry ({seen: {}, keep: [], remove: []};
      # Strip only the refresh suffix, retaining toolchain/dependencies/role/ref.
      ($entry.ref + "|" + ($entry.key
        | sub("-[a-f0-9]{40}(-[0-9]+)?$"; "")
        | sub("-[0-9]{4}-W[0-9]{2}$"; ""))) as $group
      | if .seen[$group] then
          .remove += [$entry + {reason: "superseded snapshot"}]
        else
          .seen[$group] = true
          | if $cutoff != "" and $entry.created_at < $cutoff then
              .remove += [$entry + {reason: "age limit"}]
            else .keep += [$entry] end
        end)
  | .bytes = ([.keep[].size_in_bytes] | add // 0)
  | .count = (.keep | length)
  | . as $inventory
  | reduce ($inventory.keep | sort_by(.last_accessed_at // .created_at, .id)[]) as $entry (.;
      if (($max_bytes > 0 and .bytes > $max_bytes) or ($max_count > 0 and .count > $max_count))
        and $entry.ref != $protected and $entry.ref != $current
      then .remove += [$entry + {reason: "size/count limit"}]
        | .bytes -= $entry.size_in_bytes | .count -= 1
      else . end)
  | {remove, retained_bytes: .bytes, retained_count: .count}
' "$work"/page-*.json > "$work/plan.json"

# Plans include no credentials and are safe to inspect before performing deletes.
deleted=0
while IFS=$'\t' read -r cid reason; do
  if api -X DELETE "https://api.github.com/repos/$REPOSITORY/actions/caches/$cid" >/dev/null; then
    echo "Deleted cache $cid ($reason)"
    deleted=$((deleted + 1))
  else
    echo "::warning::Failed to delete cache $cid ($reason)"
  fi
done < <(jq -r '.remove[] | [.id, .reason] | @tsv' "$work/plan.json")
echo "Pruned $deleted caches under $CACHE_KEY_PREFIX"
jq -r '"Retaining \(.retained_count) snapshots (\(.retained_bytes) bytes), including protected refs"' "$work/plan.json"
if (( CACHE_MAX_SIZE_GB > 0 )) && jq -e --argjson max "$((CACHE_MAX_SIZE_GB * 1024 * 1024 * 1024))" '.retained_bytes > $max' "$work/plan.json" >/dev/null; then
  echo "::warning::Protected caches exceed the size target; retained instead of evicting active suite caches"
fi
