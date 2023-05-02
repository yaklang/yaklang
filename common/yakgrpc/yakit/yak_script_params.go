package yakit

import "yaklang.io/yaklang/common/yakgrpc/ypb"

var mitmPluginDefaultPlugins = []*ypb.YakScriptParam{
	{
		Field:        "target",
		TypeVerbose:  "string",
		FieldVerbose: "目标(URL/IP:Port)",
		Help:         "输入插件的测试目标，进行基础爬虫（最多10个请求）",
		Required:     true,
	},
}
