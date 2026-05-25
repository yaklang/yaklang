#!/usr/bin/env bash
set -euo pipefail

# Prepare plain-text version list files for OSS upload.
# Handles corrupted downloads where history was stored as gzip bytes
# (optionally prefixed with a plain-text current-version line).

YAK_TAG="${1:?usage: prepare-version-info-for-oss.sh <YAK_TAG>}"

HISTORY_URL="${HISTORY_URL:-https://aliyun-oss.yaklang.com/yak/version-info/history_versions.txt}"
WORKDIR="${WORKDIR:-.}"

HISTORY_DL="${WORKDIR}/history_versions.downloaded.txt"
HISTORY_NORMALIZED="${WORKDIR}/history_versions.normalized.txt"
OUT_HISTORY="${WORKDIR}/new_history_versions.txt"
OUT_ACTIVE="${WORKDIR}/active_versions.txt"

VERSION_LINE_RE='^(dev/[0-9]{4}-[0-9]{2}-[0-9]{2}-[a-f0-9]+|[0-9]+(\.[0-9A-Za-z_-]+)+)$'

mkdir -p "$WORKDIR"

read_file_magic() {
  local file="$1"
  head -c2 "$file" | od -An -tx1 | tr -d ' \n'
}

download_history() {
  if ! curl -fsSI "$HISTORY_URL" | grep -qE 'HTTP/[0-9.]+ 200'; then
    echo "history_versions.txt not found at $HISTORY_URL, starting fresh"
    : > "$HISTORY_DL"
    return 0
  fi

  echo "Downloading history_versions.txt from $HISTORY_URL"
  # --compressed decodes HTTP Content-Encoding; normalize step still handles
  # objects that were uploaded to OSS as raw gzip bytes.
  curl -fsSL --compressed "$HISTORY_URL" -o "$HISTORY_DL"
}

normalize_history_file() {
  local src="$1"
  local dst="$2"

  if [ ! -s "$src" ]; then
    : > "$dst"
    return 0
  fi

  local magic rest_magic first_line_bytes rest_start
  magic="$(read_file_magic "$src")"

  if [ "$magic" = "1f8b" ]; then
    echo "Detected gzip-encoded history file, decompressing..."
    gzip -dc "$src" > "$dst"
    return 0
  fi

  first_line_bytes="$(head -1 "$src" | wc -c | tr -d ' ')"
  rest_start=$((first_line_bytes + 1))
  rest_magic="$(tail -c +"${rest_start}" "$src" | head -c2 | od -An -tx1 | tr -d ' \n')"

  if [ "$rest_magic" = "1f8b" ]; then
    echo "Detected hybrid history file (plain header + gzip body), repairing..."
    {
      head -1 "$src"
      tail -c +"${rest_start}" "$src" | gzip -dc
    } > "$dst"
    return 0
  fi

  cp "$src" "$dst"
}

merge_version_lists() {
  normalize_history_file "$HISTORY_DL" "$HISTORY_NORMALIZED"

  if ! printf '%s\n' "$YAK_TAG" | grep -Eq "$VERSION_LINE_RE"; then
    echo "ERROR: invalid current version tag: $YAK_TAG" >&2
    return 1
  fi

  {
    printf '%s\n' "$YAK_TAG"
    if [ -s "$HISTORY_NORMALIZED" ]; then
      cat "$HISTORY_NORMALIZED"
    fi
  } | awk -v version_re="$VERSION_LINE_RE" '
    function valid(line,    re) {
      re = version_re
      return line ~ re
    }
    {
      sub(/\r$/, "")
      if ($0 == "") next
      if (!valid($0)) {
        print "ERROR: invalid version line: " $0 > "/dev/stderr"
        exit 1
      }
      if (!seen[$0]++) print
    }
  ' > "$OUT_HISTORY"

  if [ ! -s "$OUT_HISTORY" ]; then
    echo "ERROR: merged version list is empty" >&2
    return 1
  fi

  head -n 100 "$OUT_HISTORY" > "$OUT_ACTIVE"

  local history_count active_count
  history_count="$(wc -l < "$OUT_HISTORY" | tr -d ' ')"
  active_count="$(wc -l < "$OUT_ACTIVE" | tr -d ' ')"
  echo "Prepared ${history_count} history versions, active list has ${active_count} entries"
}

validate_plaintext_version_file() {
  local file="$1"
  if [ ! -s "$file" ]; then
    echo "ERROR: version file is empty: $file" >&2
    return 1
  fi

  if grep -q $'\x1f\x8b' "$file"; then
    echo "ERROR: gzip magic found in plain version file: $file" >&2
    return 1
  fi

  if LC_ALL=C grep -n '[^[:print:][:space:]]' "$file" >/dev/null; then
    echo "ERROR: non-printable bytes found in version file: $file" >&2
    LC_ALL=C grep -n '[^[:print:][:space:]]' "$file" >&2 || true
    return 1
  fi

  local invalid
  invalid="$(grep -Ev "$VERSION_LINE_RE" "$file" || true)"
  if [ -n "$invalid" ]; then
    echo "ERROR: invalid version line(s) in $file:" >&2
    echo "$invalid" >&2
    return 1
  fi
}

main() {
  rm -f "$HISTORY_DL" "$HISTORY_NORMALIZED" "$OUT_HISTORY" "$OUT_ACTIVE"
  download_history
  merge_version_lists
  validate_plaintext_version_file "$OUT_HISTORY"
  validate_plaintext_version_file "$OUT_ACTIVE"
  echo "Version files ready:"
  echo "  history: $OUT_HISTORY ($(wc -l < "$OUT_HISTORY" | tr -d ' ') lines)"
  echo "  active:  $OUT_ACTIVE ($(wc -l < "$OUT_ACTIVE" | tr -d ' ') lines)"
  head -n 10 "$OUT_HISTORY"
}

main "$@"
