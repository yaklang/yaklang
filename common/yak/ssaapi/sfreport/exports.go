package sfreport

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// convertSingleResultToSSAResultPartsJSONPayload 将单个 SyntaxFlow 扫描结果转换为分片 JSON 载荷（导出名为 sfreport.ConvertSingleResultToSSAResultPartsJSONPayload）
// 返回一个字典，包含 json(报告内容)、stats(统计信息)、ok(是否成功)、has_payload(是否含有效载荷)
//
// 参数:
//   - result: SyntaxFlow 扫描结果对象
//   - opts: 分片选项，如 sfreport.withStreamReportType / sfreport.withStreamShowDataflowPath 等
//
// 返回值:
//   - 包含 json/stats/ok/has_payload 的字典
//   - 错误信息（转换失败时返回）
//
// Example:
// ```
// // result 来自 ssa/syntaxflow 的扫描结果（示意性示例，需要可用的 SSA 程序与规则）
// prog = ssa.Parse(code)~
// result = prog.SyntaxFlowWithError("sink* as $sink")~
// payload = sfreport.ConvertSingleResultToSSAResultPartsJSONPayload(result)~
// println(payload["ok"])
// ```
func convertSingleResultToSSAResultPartsJSONPayload(result *ssaapi.SyntaxFlowResult, opts ...StreamPartsOption) (map[string]any, error) {
	o := NewStreamPartsOptions(opts...)
	j, stats, err := ConvertSingleResultToSSAResultPartsJSON(result, o)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"json":        j,
		"stats":       stats,
		"ok":          err == nil,
		"has_payload": stats != nil && stats["has_payload"] == true,
	}, nil
}

var Exports = map[string]interface{}{
	"NewReport": NewReport,

	"IRifyReportType":      IRifyReportType,
	"IRifyFullReportType":  IRifyFullReportType,
	"IRifyReactReportType": IRifyReactReportType,

	"withDataflowPath": WithDataflowPath,
	"withFileContent":  WithFileContent,

	"withStreamReportType":       WithStreamReportType,
	"withStreamShowDataflowPath": WithStreamShowDataflowPath,
	"withStreamShowFileContent":  WithStreamShowFileContent,
	"withStreamWithFile":         WithStreamWithFile,

	// Legacy JSON export helpers (not used by the current IRify streaming path),
	// keep for backward compatibility of external yak scripts/tools.
	"ConvertSingleResultToJSON":                      ConvertSingleResultToJSON,
	"ConvertSingleResultToJSONWithOptions":           ConvertSingleResultToJSONWithOptions,
	"ConvertSingleResultToSSAResultPartsJSONPayload": convertSingleResultToSSAResultPartsJSONPayload,
	"ImportSSARiskFromJSON":                          ImportSSARiskFromJSON,

	"GenerateSSAReportMarkdownForTask": GenerateSSAReportMarkdownForTask,
}
