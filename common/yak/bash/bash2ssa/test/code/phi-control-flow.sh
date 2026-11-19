#!/usr/bin/env bash
# Control-flow fixture for SSA Phi regression coverage:
# if/elif/else, for, while, until, nesting and short-circuit assignment.
set -euo pipefail

TARGET="${1:-/tmp/phi-target}"
COUNT=0
RESULT=""

if [ -f "$TARGET" ]; then
  RESULT="file"
elif [ -d "$TARGET" ]; then
  RESULT="dir"
else
  RESULT="missing"
fi

for item in alpha beta; do
  COUNT=$((COUNT + 1))
  if [ "$item" = alpha ]; then
    RESULT="$RESULT:$item"
  else
    RESULT="$RESULT:other"
  fi
done

while read -r line; do
  COUNT=$((COUNT + 1))
  [ -n "$line" ] && RESULT="$RESULT:$line"
done < "$TARGET"

until [ "$COUNT" -ge 10 ]; do
  COUNT=$((COUNT + 1))
done

for ((index = 0; index < 3; index++)); do
  COUNT=$((COUNT + index))
done

echo "$RESULT $COUNT"