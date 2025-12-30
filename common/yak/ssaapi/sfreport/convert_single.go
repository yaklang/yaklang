package sfreport

import (
	"bytes"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func ConvertSingleResultToJSON(result *ssaapi.SyntaxFlowResult, showDataflowPath bool) (string, error) {
	if result == nil {
		return "", nil
	}

	report := NewReport(IRifyFullReportType)
	if showDataflowPath {
		report.config.showDataflowPath = true
	}

	report.AddSyntaxFlowResult(result)

	if len(report.Risks) == 0 {
		return "", nil
	}
	buf := bytes.NewBuffer(nil)
	if err := report.PrettyWrite(buf); err != nil {
		return "", err
	}

	return buf.String(), nil
}
