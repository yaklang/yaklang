package sfreport

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

var Exports = map[string]interface{}{
	"NewReport": NewReport,

	"IRifyReportType":      IRifyReportType,
	"IRifyFullReportType":  IRifyFullReportType,
	"IRifyReactReportType": IRifyReactReportType,

	"withDataflowPath": WithDataflowPath,
	"withFileContent":  WithFileContent,

	"withStreamKey":              WithStreamKey,
	"withStreamReportType":       WithStreamReportType,
	"withStreamShowDataflowPath": WithStreamShowDataflowPath,
	"withStreamShowFileContent":  WithStreamShowFileContent,
	"withStreamWithFile":         WithStreamWithFile,
	"withStreamDedupFileContent": WithStreamDedupFileContent,
	"withStreamDedupDataflow":    WithStreamDedupDataflow,

	// Legacy JSON export helpers (not used by the current IRify streaming path),
	// keep for backward compatibility of external yak scripts/tools.
	"ConvertSingleResultToJSON": func(result *ssaapi.SyntaxFlowResult, showDataflow bool) (string, error) {
		return ConvertSingleResultToJSON(result, showDataflow)
	},
	"ConvertSingleResultToJSONWithOptions": func(result *ssaapi.SyntaxFlowResult, reportType ReportType, showDataflow bool, showFileContent bool, withFile bool) (string, error) {
		return ConvertSingleResultToJSONWithOptions(result, reportType, showDataflow, showFileContent, withFile)
	},

	"ConvertSingleResultToStreamPartsJSONPayload": func(result *ssaapi.SyntaxFlowResult, opts ...StreamPartsOption) (map[string]any, error) {
		o := NewStreamPartsOptions(opts...)
		j, stats, err := ConvertSingleResultToStreamPartsJSON(result, o)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"json":        j,
			"stats":       stats,
			"ok":          err == nil,
			"has_payload": stats != nil && stats["has_payload"] == true,
		}, nil
	},
	"ResetStreamFileDedup": ResetStreamFileDedup,

	"ImportSSARiskFromJSON": ImportSSARiskFromJSON,
}
