package sfreport

import (
	"io"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type IReport interface {
	AddSyntaxFlowResult(result *ssaapi.SyntaxFlowResult) bool
	PrettyWrite(writer io.Writer) error
	AddSyntaxFlowRisks(risks []*schema.SSARisk)
	Save() error
}

func ConvertSyntaxFlowResultToReport(format ReportType) (IReport, error) {
	switch format {
	case SarifReportType:
		return NewSarifReport()
	case IRifyReportType, IRifyFullReportType:
		return NewReport(format), nil
	case IRifyReactReportType:
		return NewReport(format), nil
	default:
		return nil, utils.Errorf("unsupported report format: %s", format)
	}
}
