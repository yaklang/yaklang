package yakit

type PluginScanFilter struct {
	ExcludePluginScanURIs []string
	IncludePluginScanURIs []string
}

var GlobalPluginScanFilter = new(PluginScanFilter)

func SetGlobalPluginScanLists(whitelist, blacklist []string) func() {
	oldInclude := GlobalPluginScanFilter.IncludePluginScanURIs
	oldExclude := GlobalPluginScanFilter.ExcludePluginScanURIs

	GlobalPluginScanFilter.IncludePluginScanURIs = whitelist
	GlobalPluginScanFilter.ExcludePluginScanURIs = blacklist

	return func() {
		GlobalPluginScanFilter.IncludePluginScanURIs = oldInclude
		GlobalPluginScanFilter.ExcludePluginScanURIs = oldExclude
	}
}
