package sfreport

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

var Exports = map[string]interface{}{
	"NewReport": NewReport,

	"IRifyReportType":      IRifyReportType,
	"IRifyFullReportType":  IRifyFullReportType,
	"IRifyReactReportType": IRifyReactReportType,

	"withDataflowPath": WithDataflowPath,
	"withFileContent":  WithFileContent,

	"ConvertSingleResultToJSON": func(result *ssaapi.SyntaxFlowResult, showDataflow bool) (string, error) {
		return ConvertSingleResultToJSON(result, showDataflow)
	},
	"ConvertSingleResultToJSONWithOptions": func(result *ssaapi.SyntaxFlowResult, reportType ReportType, showDataflow bool, showFileContent bool, withFile bool) (string, error) {
		return ConvertSingleResultToJSONWithOptions(result, reportType, showDataflow, showFileContent, withFile)
	},
	"ConvertSingleResultToStreamPartsJSONPayload": func(result *ssaapi.SyntaxFlowResult, opts map[string]any) (map[string]any, error) {
		o := StreamPartsOptions{
			StreamKey:        utils.InterfaceToString(utils.MapGetFirstRaw(opts, "stream_key", "streamKey")),
			ReportType:       ReportType(utils.InterfaceToString(utils.MapGetFirstRaw(opts, "report_type", "reportType"))),
			ShowDataflowPath: utils.InterfaceToBoolean(utils.MapGetFirstRaw(opts, "show_dataflow_path", "showDataflowPath", "show_dataflow")),
			ShowFileContent:  utils.InterfaceToBoolean(utils.MapGetFirstRaw(opts, "show_file_content", "showFileContent")),
			WithFile:         utils.InterfaceToBoolean(utils.MapGetFirstRaw(opts, "with_file", "withFile")),
			DedupFileContent: utils.InterfaceToBoolean(utils.MapGetFirstRaw(opts, "dedup_file_content", "dedupFileContent", "dedup_file")),
			DedupDataflow:    utils.InterfaceToBoolean(utils.MapGetFirstRaw(opts, "dedup_dataflow", "dedupDataflow")),
		}
		if o.ReportType == "" {
			o.ReportType = IRifyFullReportType
		}
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
