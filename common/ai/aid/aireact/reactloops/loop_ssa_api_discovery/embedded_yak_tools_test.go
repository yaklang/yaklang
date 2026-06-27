package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
)

func TestEmbeddedRouteCoreHarvestParses(t *testing.T) {
	tool := yakscripttools.LoadYakScriptToAiTools(ToolRouteCoreHarvest, embeddedApiRouteHarvestYak)
	require.NotNil(t, tool, "route_core_harvest must parse for profile DB registration")
	require.Equal(t, ToolRouteCoreHarvest, tool.Name)
}

func TestEmbeddedVulnBatchScanParses(t *testing.T) {
	tool := yakscripttools.LoadYakScriptToAiTools(ToolVulnBatchScan, embeddedVulnBatchScanYak)
	require.NotNil(t, tool, "vuln_batch_scan must parse for profile DB registration")
	require.Equal(t, ToolVulnBatchScan, tool.Name)
	require.NotEmpty(t, tool.Params)
}
