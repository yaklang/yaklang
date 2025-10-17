package sfreport

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type ReportType string

const (
	SarifReportType ReportType = "sarif"

	// echo file will only show the first 100 characters in the report
	IRifyReportType ReportType = "irify"

	// echo file will show the full content in the report
	IRifyFullReportType ReportType = "irify-full"

	IRifyReactReportType ReportType = "irify-react-report"
)

var (
	log = ssalog.Log
)

func ReportTypeFromString(s string) ReportType {
	switch s {
	case "sarif":
		return SarifReportType
	case "irify":
		return IRifyReportType
	case "irify-full":
		return IRifyFullReportType
	case "irify-react-report":
		return IRifyReactReportType
	default:
		log.Warnf("unsupported report type: %s, use sarif as default, you can set [sarif, irify, irify-full] to set report type", s)
		return SarifReportType
	}
}

func ToReportSeverityLevel(level schema.SyntaxFlowSeverity) string {
	switch level {
	case schema.SFR_SEVERITY_INFO:
		return "note"
	case schema.SFR_SEVERITY_LOW, schema.SFR_SEVERITY_WARNING:
		return "warning"
	case schema.SFR_SEVERITY_CRITICAL, schema.SFR_SEVERITY_HIGH:
		return "error"
	default:
		return "note"
	}
}

func GetValueByRisk(ssarisk *schema.SSARisk) (*ssaapi.Value, error) {
	// get result
	result, err := ssaapi.LoadResultByID(uint(ssarisk.ResultID))
	if err != nil {
		log.Errorf("load result by id %d error: %v", ssarisk.ResultID, err)
		return nil, err
	}

	// get value
	value, err := result.GetValue(ssarisk.Variable, int(ssarisk.Index))
	if err != nil {
		log.Errorf("get value by variable %s and index %d error: %v", ssarisk.Variable, ssarisk.Index, err)
		return nil, err
	}

	return value, nil
}
