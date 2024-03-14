package yakit

import "fmt"

type PluginScanFilter struct {
	ExcludePluginScanURIs []string
	IncludePluginScanURIs []string
}

var GlobalPluginScanFilter = new(PluginScanFilter)

func SetGlobalPluginScanLists(whitelist, blacklist []string) {
	fmt.Printf("!!! set plugin scan filer whitelist: %v, blacklist: %v\n", whitelist, blacklist)
	GlobalPluginScanFilter.IncludePluginScanURIs = whitelist
	GlobalPluginScanFilter.ExcludePluginScanURIs = blacklist
}
