//go:build !hids

package scannode

func compiledScanNodeCapabilityKeys() []string {
	return []string{
		"yak.execute",
		capabilityKeySSARuleSyncExport,
		capabilityKeyAIBindEpochV1,
		capabilityKeyAITurnLifecycleV1,
		capabilityKeyPluginBundleV1,
	}
}
