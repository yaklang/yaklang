package ssa

import "os"

// SkeletonTopLevelEnabled reports whether language builders use the pass1 skeleton +
// detached lazy hooks model (docs/ssa-ast-to-ssa-skeleton-plan.md). Set
// YAK_SSA_LEGACY_TOPLEVEL=1 to restore the legacy whole-file-AST pass2 closure
// for A/B measurement or per-language revert.
func SkeletonTopLevelEnabled() bool {
	return os.Getenv("YAK_SSA_LEGACY_TOPLEVEL") == ""
}
