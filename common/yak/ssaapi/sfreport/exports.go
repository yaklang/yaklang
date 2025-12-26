package sfreport

import "github.com/yaklang/yaklang/common/yak/ssaapi"

var Exports = map[string]interface{}{
	"NewReport": NewReport,

	"IRifyReportType":      IRifyReportType,
	"IRifyFullReportType":  IRifyFullReportType,
	"IRifyReactReportType": IRifyReactReportType,

	"withDataflowPath": WithDataflowPath,
	"withFileContent":  WithFileContent,

	// 单条结果转完整 Report JSON（用于流式上报）
	"ConvertSingleResultToJSON": func(result *ssaapi.SyntaxFlowResult, showDataflow bool) (string, error) {
		return ConvertSingleResultToJSON(result, showDataflow)
	},

	"ImportSSARiskFromJSON": ImportSSARiskFromJSON,
}
