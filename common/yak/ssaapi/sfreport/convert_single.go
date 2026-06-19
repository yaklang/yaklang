package sfreport

import (
	"bytes"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// ConvertSingleResultToJSON 将单个 SyntaxFlow 扫描结果转换为 JSON 报告字符串
// 导出名为 sfreport.ConvertSingleResultToJSON
// 参数:
//   - result: SyntaxFlow 扫描结果对象
//   - showDataflow: 是否在报告中展示数据流路径
//
// 返回值:
//   - JSON 报告字符串（无风险时为空字符串）
//   - 错误信息
//
// Example:
// ```
// // result 来自 ssa/syntaxflow 的扫描结果（示意性示例）
// prog = ssa.Parse(code)~
// result = prog.SyntaxFlowWithError("sink* as $sink")~
// jsonStr = sfreport.ConvertSingleResultToJSON(result, true)~
// println(jsonStr)
// ```
func ConvertSingleResultToJSON(result *ssaapi.SyntaxFlowResult, showDataflowPath bool) (string, error) {
	return ConvertSingleResultToJSONWithOptions(result, IRifyFullReportType, showDataflowPath, true, true)
}

// ConvertSingleResultToJSONWithOptions 将单个 SyntaxFlow 扫描结果按指定选项转换为 JSON 报告
// 导出名为 sfreport.ConvertSingleResultToJSONWithOptions
// 参数:
//   - result: SyntaxFlow 扫描结果对象
//   - reportType: 报告类型
//   - showDataflow: 是否展示数据流路径
//   - showFileContent: 是否展示文件内容
//   - withFile: 是否携带文件数据
//
// 返回值:
//   - JSON 报告字符串（无风险时为空字符串）
//   - 错误信息
//
// Example:
// ```
// // result 来自 ssa/syntaxflow 的扫描结果（示意性示例）
// prog = ssa.Parse(code)~
// result = prog.SyntaxFlowWithError("sink* as $sink")~
// jsonStr = sfreport.ConvertSingleResultToJSONWithOptions(result, sfreport.IRifyFullReportType, true, true, true)~
// println(jsonStr)
// ```
func ConvertSingleResultToJSONWithOptions(result *ssaapi.SyntaxFlowResult, reportType ReportType, showDataflowPath bool, showFileContent bool, withFile bool) (string, error) {
	if result == nil {
		return "", nil
	}

	// NOTE: this helper is legacy and currently unused by IRify's streaming path.
	// Keep the historical behavior: always build full report shape here, and let
	// flags like showDataflowPath/showFileContent/withFile further constrain output.
	_ = reportType
	report := NewReport(IRifyFullReportType)
	if showDataflowPath {
		report.config.showDataflowPath = true
	}
	if showFileContent {
		report.config.showFileContent = true
	}

	report.AddSyntaxFlowResult(result)
	if !withFile {
		report.File = nil
		report.IrSourceHashes = make(map[string]struct{})
		report.FileCount = 0
	}

	if len(report.Risks) == 0 {
		return "", nil
	}
	buf := bytes.NewBuffer(nil)
	if err := report.Write(buf); err != nil {
		return "", err
	}

	return buf.String(), nil
}
