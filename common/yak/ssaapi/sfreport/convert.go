package sfreport

import (
	"io"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type IReport interface {
	AddSyntaxFlowResult(result *ssaapi.SyntaxFlowResult) bool
	AddSyntaxFlowRisks(risks ...*schema.SSARisk)
	SetWriter(writer io.Writer) error
	Save() error
}

func ConvertSyntaxFlowResultToReport(format ReportType, opt ...Option) (IReport, error) {
	switch format {
	case SarifReportType:
		return NewSarifReport()
	case IRifyReportType, IRifyFullReportType:
		return NewReport(format, opt...), nil
	case IRifyReactReportType:
		return NewReport(format, opt...), nil
	default:
		return nil, utils.Errorf("unsupported report format: %s", format)
	}
}
