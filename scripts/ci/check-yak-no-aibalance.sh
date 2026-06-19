#!/usr/bin/env bash
set -euo pipefail

# Guard: the core yak engine binary (common/yak/cmd) must NOT depend on the
# heavy aibalance server package (common/aibalance). The dependency direction is
# one-way: aibalance may import yak, but yak must stay small and never pull in
# aibalance. The lightweight client packages (common/ai/aibalance and
# common/aibalanceclient) are allowed and intentionally excluded by exact match.

TARGET_PKG="${TARGET_PKG:-github.com/yaklang/yaklang/common/yak/cmd}"
FORBIDDEN_PKG="${FORBIDDEN_PKG:-github.com/yaklang/yaklang/common/aibalance}"

echo "checking that ${TARGET_PKG} does not depend on ${FORBIDDEN_PKG}"

DEPS="$(go list -deps "${TARGET_PKG}")"

# Exact whole-line match so common/ai/aibalance and common/aibalanceclient are
# not mistakenly treated as violations.
if printf '%s\n' "${DEPS}" | grep -Fxq "${FORBIDDEN_PKG}"; then
  echo "ERROR: ${TARGET_PKG} unexpectedly depends on ${FORBIDDEN_PKG}"
  echo "ERROR: the yak engine must not link the aibalance server package; revert the offending import"
  exit 1
fi

echo "OK: ${TARGET_PKG} has no dependency on ${FORBIDDEN_PKG}"
