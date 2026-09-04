//go:build hids

package scannode

func compiledScanNodeCapabilityKeys() []string {
	return []string{
		"yak.execute",
		"hids",
		capabilityKeySSARuleSyncExport,
		capabilityKeySSARuleSnapshotExecutionV2,
		capabilityKeyAIBindEpochV1,
		capabilityKeyAITurnLifecycleV1,
		capabilityKeyAICodeWorkspaceV1,
		capabilityKeyAIManagedInputV1,
	}
}
