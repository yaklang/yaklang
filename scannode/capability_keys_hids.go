//go:build hids

package scannode

func compiledScanNodeCapabilityKeys() []string {
	return []string{
		aiTrafficCapabilityV1, aiTrafficAnalysisCapabilityV1,
		"yak.execute",
		"hids",
		capabilityKeySSARuleSyncExport,
		capabilityKeySSARuleSnapshotExecutionV2,
		capabilityKeyAIBindEpochV1,
		capabilityKeyAITurnLifecycleV1,
		capabilityKeyAISkillBundleV1,
		capabilityKeyAIForgeReleaseV1,
		capabilityKeyAIForgeCustomToolsV1,
		capabilityKeyAICodeWorkspaceV1,
		capabilityKeyAIManagedInputV1,
		capabilityKeyAIToolsBuiltinV1,
		capabilityKeyPluginBundleV1,
	}
}
