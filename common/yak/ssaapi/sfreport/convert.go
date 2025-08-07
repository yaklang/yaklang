package sfreport

import (
	"github.com/yaklang/yaklang/common/schema"
	"io"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type IReport interface {
	AddSyntaxFlowResult(result *ssaapi.SyntaxFlowResult) bool
	PrettyWrite(writer io.Writer) error
	AddSyntaxFlowRisks(risks []*schema.SSARisk)
}

func ConvertSyntaxFlowResultToReport(format ReportType) (IReport, error) {
	switch format {
	case SarifReportType:
		return NewSarifReport()
	case IRifyReportType, IRifyFullReportType:
		return NewReport(format), nil
	default:
		return nil, utils.Errorf("unsupported report format: %s", format)
	}
}
