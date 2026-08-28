package scannode

import (
	"os"
	"strings"
)

const (
	capabilityKeySSARuleSyncExport          = "ssa.rule_sync.export"
	capabilityKeySSARuleSnapshotExecutionV2 = ruleSnapshotExecutionV2
	capabilityKeyAIBindEpochV1              = "ai.session.bind_epoch.v1"
	capabilityKeyAITurnLifecycleV1          = "ai.session.turn_lifecycle.v1"
	capabilityKeyAICodeWorkspaceV1          = "ai.code_workspace.v1"
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
		if trimmed == capabilityKeyAICodeWorkspaceV1 && runtimeMode == aiSessionRuntimeModeStateful {
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
