//go:build !hids

package scannode

func compiledScanNodeCapabilityKeys() []string {
	return []string{"yak.execute", capabilityKeySSARuleSyncExport}
}
