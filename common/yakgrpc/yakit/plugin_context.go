package yakit

type YakitPluginInfo struct {
	PluginName string
	PluginUUID string
	RuntimeId  string
}

func CreateYakitPluginContext(runtimeID string) YakitPluginInfo {
	return YakitPluginInfo{
		RuntimeId: runtimeID,
	}
}
