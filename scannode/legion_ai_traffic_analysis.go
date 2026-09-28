package scannode

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiengine"
)

const aiTrafficAnalysisCapabilityV1 = "ai.traffic.analysis.readonly.v1"
const aiTrafficAnalysisMaxEvidenceBytes = 1 << 20

func validateTrafficAnalysisBinding(binding aiSessionBinding, options yakRuntimeOptions) error {
	analysis := binding.TrafficAnalysis
	if analysis == nil {
		return nil
	}
	if !analysis.ReadOnly || analysis.SourceSessionId == "" || analysis.SourceSessionId == binding.Ref.SessionID || len(analysis.FlowIds) == 0 || len(analysis.FlowIds) > 50 {
		return fmt.Errorf("traffic analysis requires an independent session and bounded evidence references")
	}
	if options.SourceWorkspace != nil || binding.InputWorkspace != nil || binding.LegionResultRuntime != nil || binding.AuthorizedFocusReleaseID != "" || options.ForgeName != "" || options.Focus != "" || options.FocusModeLoop != "" || options.FocusReleaseID != "" || len(options.SessionMCPServers) > 0 || len(binding.Attachments) > 0 || len(binding.CredentialRefs) > 0 {
		return fmt.Errorf("traffic evidence analysis forbids executable resources and external capabilities")
	}
	return nil
}
func applyTrafficAnalysisRuntimeOptions(options *yakRuntimeOptions) {
	yes, no := true, false
	options.DisableToolUse = &yes
	options.EnableSystemFileSystemOperator = &no
	options.EnableAISearchTool = &no
	options.EnableAISearchInternet = &no
	options.EnabledCapabilities = nil
}
func trafficAnalysisEnginePolicy() aiengine.AIEngineConfigOption {
	return func(config *aiengine.AIEngineConfig) {
		config.DisableToolUse = true
		config.DisableAIForge = true
		config.DisableMCPServers = true
		config.EnableAISearchTool = false
		config.EnableForgeSearchTool = false
		config.ExtraMCPServers = nil
		config.IncludeToolNames = nil
		config.Focus = ""
		config.ExtOptions = append(config.ExtOptions, aicommon.WithReadOnlyEvidence(), aicommon.WithDisallowMCPServers(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableWebSearch(true), aicommon.WithEnablePlanAndExec(false), aicommon.WithDisableDynamicPlanning(true), aicommon.WithDisablePerception(true), aicommon.WithDisableIntentRecognition(true), aicommon.WithDisableMemoryTriage(true), aicommon.WithAllowSyncInitContext(false))
	}
}
func validateTrafficAnalysisInput(binding aiSessionBinding, input aiSessionInput) error {
	if strings.EqualFold(input.InputType, "hotpatch") || isSyncAISessionInput(input.InputType) || isInteractiveAISessionInput(input.InputType) {
		return fmt.Errorf("traffic analysis permits evidence-backed message turns only")
	}
	analysis := input.ContextPackage.GetTrafficAnalysis()
	if analysis == nil || !analysis.ReadOnly || analysis.SourceSessionId != binding.TrafficAnalysis.SourceSessionId || analysis.EvidenceText == "" || len(analysis.EvidenceText) > aiTrafficAnalysisMaxEvidenceBytes || input.ContextPackage.GetFocusRelease() != nil || len(input.ContextPackage.Tools) > 0 {
		return fmt.Errorf("traffic analysis requires server-assembled read-only evidence context")
	}
	allowed := make(map[string]bool, len(binding.TrafficAnalysis.FlowIds))
	for _, id := range binding.TrafficAnalysis.FlowIds {
		allowed[id] = true
	}
	if len(analysis.FlowIds) == 0 {
		return fmt.Errorf("traffic analysis evidence references are required")
	}
	for _, id := range analysis.FlowIds {
		if !allowed[id] {
			return fmt.Errorf("traffic analysis flow is outside the bound evidence selection")
		}
	}
	return nil
}
