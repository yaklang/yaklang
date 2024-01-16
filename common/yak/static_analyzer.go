package yak

import "github.com/yaklang/yaklang/common/yak/static_analyzer"

func StaticAnalyzeYaklang(code string, typs ...string) []*static_analyzer.StaticAnalyzeResult {
	pluginType := "yak"
	if len(typs) > 0 {
		pluginType = typs[0]
	}
	return static_analyzer.StaticAnalyzeYaklang(code, pluginType)
}
