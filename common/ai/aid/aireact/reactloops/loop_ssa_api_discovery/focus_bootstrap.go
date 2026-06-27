package loop_ssa_api_discovery

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// PrepareSsaApiDiscoveryFocusMode runs at the very start of the focus pipeline (before BootstrapDiscoveryRuntime):
// 1) Registers/updates embedded Yak AI tools into the profile DB (same idea as infosec_recon embeddedInfosecYakTools).
// 2) Ensures SyntaxFlow builtin rules exist in the profile DB — if empty, runs ForceSyncEmbedRule; if embed hash drifted, SyncEmbedRule.
func PrepareSsaApiDiscoveryFocusMode(r aicommon.AIInvokeRuntime) {
	if r == nil {
		return
	}
	EnsureSsaDiscoveryEmbeddedYakTools()
	ensureSyntaxFlowEmbedRulesLoaded(r)
}

func ensureSyntaxFlowEmbedRulesLoaded(r aicommon.AIInvokeRuntime) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		log.Warnf("ssa_api_discovery: GetGormProfileDatabase nil, skip SyntaxFlow rule check")
		r.AddToTimeline("[ssa_focus]", "profile DB unavailable: cannot verify/import SyntaxFlow builtin rules")
		return
	}

	var total int64
	if err := db.Model(&schema.SyntaxFlowRule{}).Count(&total).Error; err != nil {
		log.Warnf("ssa_api_discovery: count syntax_flow_rules: %v", err)
	}
	builtin := yakit.QueryBuildInRule(db)
	nBuiltin := len(builtin)

	if total == 0 || nBuiltin == 0 {
		log.Infof("ssa_api_discovery: SyntaxFlow rules empty (total=%d builtin=%d), ForceSyncEmbedRule", total, nBuiltin)
		if err := sfbuildin.ForceSyncEmbedRule(); err != nil {
			log.Warnf("ssa_api_discovery: ForceSyncEmbedRule: %v", err)
			r.AddToTimeline("[ssa_focus]", fmt.Sprintf("SyntaxFlow ForceSyncEmbedRule failed: %v", err))
			return
		}
		var after int64
		_ = db.Model(&schema.SyntaxFlowRule{}).Count(&after).Error
		nAfterBuiltin := len(yakit.QueryBuildInRule(db))
		r.AddToTimeline("[ssa_focus]", fmt.Sprintf("SyntaxFlow builtin rules imported from engine embed (rules_total=%d builtin=%d).", after, nAfterBuiltin))
		return
	}

	if sfbuildin.NeedSyncEmbedRule() {
		log.Infof("ssa_api_discovery: SyntaxFlow embed hash out of date, SyncEmbedRule")
		if err := sfbuildin.SyncEmbedRule(); err != nil {
			log.Warnf("ssa_api_discovery: SyncEmbedRule: %v", err)
			r.AddToTimeline("[ssa_focus]", fmt.Sprintf("SyntaxFlow SyncEmbedRule failed: %v", err))
			return
		}
		r.AddToTimeline("[ssa_focus]", "SyntaxFlow builtin rules refreshed (embed bundle updated).")
	}
}
