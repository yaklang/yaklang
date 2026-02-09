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
	"ConvertSingleResultToJSONWithOptions": func(result *ssaapi.SyntaxFlowResult, reportType ReportType, showDataflow bool, showFileContent bool, withFile bool) (string, error) {
		return ConvertSingleResultToJSONWithOptions(result, reportType, showDataflow, showFileContent, withFile)
	},
	"ConvertSingleResultToStreamJSONWithOptions": func(result *ssaapi.SyntaxFlowResult, streamKey string, reportType ReportType, showDataflow bool, showFileContent bool, withFile bool, dedupFileContent bool) (string, int, error) {
		return ConvertSingleResultToStreamJSONWithOptions(result, streamKey, reportType, showDataflow, showFileContent, withFile, dedupFileContent)
	},
	"ConvertSingleResultToStreamPayload": func(result *ssaapi.SyntaxFlowResult, streamKey string, reportType ReportType, showDataflow bool, showFileContent bool, withFile bool, dedupFileContent bool) (map[string]any, error) {
		return ConvertSingleResultToStreamPayload(result, streamKey, reportType, showDataflow, showFileContent, withFile, dedupFileContent)
	},
	"ResetStreamFileDedup": ResetStreamFileDedup,

	"ImportSSARiskFromJSON": ImportSSARiskFromJSON,
}
