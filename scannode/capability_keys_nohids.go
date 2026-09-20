//go:build !hids

package scannode

func compiledScanNodeCapabilityKeys() []string {
	return []string{
		"yak.execute",
		capabilityKeySSARuleSyncExport,
		capabilityKeySSARuleSnapshotExecutionV2,
		capabilityKeyAIBindEpochV1,
		capabilityKeyAITurnLifecycleV1,
		capabilityKeyAIForgeReleaseV1,
		capabilityKeyAICodeWorkspaceV1,
		capabilityKeyAIManagedInputV1,
		capabilityKeyPluginBundleV1,
	}
}
