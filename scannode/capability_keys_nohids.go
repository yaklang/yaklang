//go:build !hids

package scannode

func compiledScanNodeCapabilityKeys() []string {
	return []string{
		"yak.execute",
		capabilityKeySSARuleSyncExport,
		capabilityKeySSARuleSnapshotExecutionV2,
		capabilityKeyAIBindEpochV1,
		capabilityKeyAITurnLifecycleV1,
		capabilityKeyAICodeWorkspaceV1,
		capabilityKeyAISyntaxFlowRuleV1,
	}
}
