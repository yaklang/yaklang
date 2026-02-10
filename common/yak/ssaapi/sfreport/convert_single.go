package sfreport

import (
	"bytes"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func ConvertSingleResultToJSON(result *ssaapi.SyntaxFlowResult, showDataflowPath bool) (string, error) {
	return ConvertSingleResultToJSONWithOptions(result, IRifyFullReportType, showDataflowPath, true, true)
}

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
