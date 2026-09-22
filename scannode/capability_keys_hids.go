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
		capabilityKeyAIForgeReleaseV1,
		capabilityKeyAIForgeCustomToolsV1,
		capabilityKeyAICodeWorkspaceV1,
		capabilityKeyAIManagedInputV1,
		capabilityKeyAIForgeEvidenceV1,
		capabilityKeyAIForgeDiscoveryV1,
		capabilityKeyAIForgeDiscoveryV2,
		capabilityKeyAIForgeHTTPAssessmentV2,
		capabilityKeyPluginBundleV1,
	}
}
