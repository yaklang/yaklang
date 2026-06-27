package loop_ssa_api_discovery

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

//go:embed embedded/api_route_harvest.yak
var embeddedApiRouteHarvestYak string

//go:embed embedded/vuln_batch_scan.yak
var embeddedVulnBatchScanYak string

const (
	ToolRouteCoreHarvest = "route_core_harvest" // static route hints fallback (CollectStaticRouteHints)
	ToolVulnBatchScan    = "vuln_batch_scan"    // Phase4 batch_scan mode greybox
)

func ensureEmbeddedSsaDiscoveryYakTool(name, script string) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		log.Warnf("ssa_api_discovery: profile DB unavailable, skip embedded tool %s", name)
		return
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(name, script)
	if aiTool == nil {
		log.Warnf("ssa_api_discovery: parse embedded %s failed", name)
		return
	}
	aiTool.Author = schema.AIResourceAuthorBuiltin
	aiTool.IsBuiltin = true
	existing, err := yakit.GetAIYakTool(db, name)
	if err == nil && existing.Hash == aiTool.CalcHash() {
		return
	}
	if _, err := yakit.SaveAIYakTool(db, aiTool); err != nil {
		log.Warnf("ssa_api_discovery: register %s: %v", name, err)
		return
	}
	log.Infof("ssa_api_discovery: synced AI Yak tool %s", name)
}

// EnsureSsaDiscoveryEmbeddedYakTools registers live-path embedded Yak tools.
func EnsureSsaDiscoveryEmbeddedYakTools() {
	ensureEmbeddedSsaDiscoveryYakTool(ToolVulnBatchScan, embeddedVulnBatchScanYak)
	ensureEmbeddedSsaDiscoveryYakTool(ToolRouteCoreHarvest, embeddedApiRouteHarvestYak)
}
