package scannode

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/scannode/inputresolver"
)

const (
	capabilityKeySSARuleSyncExport          = "ssa.rule_sync.export"
	capabilityKeySSARuleSnapshotExecutionV2 = ruleSnapshotExecutionV2
	capabilityKeyAIBindEpochV1              = "ai.session.bind_epoch.v1"
	capabilityKeyAITurnLifecycleV1          = "ai.session.turn_lifecycle.v1"
	capabilityKeyAISkillBundleV1            = "ai.skill_bundle.v1"
	capabilityKeyAIForgeReleaseV1           = "ai.forge_release.v1"
	capabilityKeyAIToolsBuiltinV1           = "ai.tools.builtin.v1"
	capabilityKeyAIForgeCustomToolsV1       = "ai.forge.custom_tools.v1"
	capabilityKeyAICodeWorkspaceV1          = "ai.code_workspace.v1"
	capabilityKeyAIManagedInputV1           = inputresolver.CapabilityV1
	capabilityKeyPluginBundleV1             = "plugin.bundle.v1"
)

func normalizeScanNodeCapabilityKeys(input []string) []string {
	runtimeMode, _ := normalizeAISessionRuntimeMode(os.Getenv("LEGION_AI_RUNTIME"))
	return normalizeScanNodeCapabilityKeysForRuntime(input, runtimeMode)
}

func normalizeScanNodeCapabilityKeysForRuntime(input []string, runtimeMode string) []string {
	result := make([]string, 0, len(input)+len(compiledScanNodeCapabilityKeys()))
	seen := make(map[string]struct{}, len(input)+len(compiledScanNodeCapabilityKeys()))

	appendKey := func(key string) {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			return
		}
		if (trimmed == aiTrafficAnalysisCapabilityV1 || trimmed == capabilityKeyAICodeWorkspaceV1 || trimmed == capabilityKeyAIManagedInputV1 || trimmed == capabilityKeyAIForgeReleaseV1 || trimmed == capabilityKeyAISkillBundleV1) && runtimeMode == aiSessionRuntimeModeStateful {
			return
		}
		// Retired scene-specific keys may arrive from an older enrollment manifest.
		// The current binary advertises the aggregate tool protocol instead.
		switch trimmed {
		case "ai.forge.evidence.v1", "ai.forge.discovery.v1", "ai.forge.discovery.v2", "ai.forge.http_assessment.v2":
			return
		}
		if (trimmed == capabilityKeyAIForgeCustomToolsV1 || trimmed == capabilityKeyAIToolsBuiltinV1) && runtimeMode == aiSessionRuntimeModeStateful {
			return
		}
		if trimmed == capabilityKeyAIManagedInputV1 && !inputresolver.Supported() {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}

	for _, key := range compiledScanNodeCapabilityKeys() {
		appendKey(key)
	}
	for _, key := range input {
		appendKey(key)
	}
	return result
}
