package yakit

type PluginScanFilter struct {
	ExcludePluginScanURIs []string
	IncludePluginScanURIs []string
}

var GlobalPluginScanFilter = new(PluginScanFilter)

func SetGlobalPluginScanLists(whitelist, blacklist []string) {
	GlobalPluginScanFilter.IncludePluginScanURIs = whitelist
	GlobalPluginScanFilter.ExcludePluginScanURIs = blacklist
}
