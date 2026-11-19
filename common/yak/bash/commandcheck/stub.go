//go:build irify_exclude

package commandcheck

import "context"

// CheckCommand is a no-op in the slim build: bash2ssa and SyntaxFlow are not
// part of that binary, so the guard reports itself unavailable and the caller
// keeps its normal fail-open behavior.
func CheckCommand(_ context.Context, _ string) Result {
	return Result{Available: false, Reason: "not available in irify_exclude build"}
}
