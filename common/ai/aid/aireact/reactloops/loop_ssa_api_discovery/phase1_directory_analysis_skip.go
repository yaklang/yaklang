package loop_ssa_api_discovery

import (
	"os"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// shouldSkipDirectoryAnalysis reports whether directory BFS (D step) should be skipped.
func shouldSkipDirectoryAnalysis(rt *Runtime) bool {
	if os.Getenv("YAK_SSA_SKIP_DIR_ANALYSIS") == "1" {
		return true
	}
	if rt != nil && rt.SkipDirectoryAnalysis {
		return true
	}
	return false
}

func backfillFeatureInventoryAfterSkip(r aicommon.AIInvokeRuntime, rt *Runtime) {
	if rt == nil {
		return
	}
	if err := BackfillFeatureInventoryFromRegistry(rt); err != nil {
		log.Warnf("ssa_api_discovery: feature_inventory registry backfill: %v", err)
		if r != nil {
			r.AddToTimeline("[ssa_pipeline]", "feature_inventory registry backfill failed: "+err.Error())
		}
		return
	}
	log.Infof("ssa_api_discovery: feature_inventory backfilled from code_unit_registry (BFS skipped)")
	if r != nil {
		r.AddToTimeline("[ssa_pipeline]", "feature_inventory backfilled from code_unit_registry (BFS skipped)")
	}
}
