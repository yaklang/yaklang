package yak

import (
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

func StaticAnalyzeYaklang(code string, typs ...string) []*result.StaticAnalyzeResult {
	pluginType := "yak"
	if len(typs) > 0 {
		pluginType = typs[0]
	}
	return static_analyzer.StaticAnalyzeYaklang(code, pluginType)
}
