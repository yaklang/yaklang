package syntaxflow_scan

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
)

type ProcessCallback func(progress float64)

type RuleProcessCallback func(progName, ruleName string, progress float64)

type scanInputConfig struct {
	ScanRequest         *ypb.SyntaxFlowScanRequest
	ProcessCallback     ProcessCallback
	RuleProcessCallback RuleProcessCallback
	Reporter            sfreport.IReport
	ReporterWriter      io.Writer
}

func (sc *scanInputConfig) GetScanRequest() *ypb.SyntaxFlowScanRequest {
	return sc.ScanRequest
}

func (sc *scanInputConfig) GetProcessCallback() ProcessCallback {
	if sc == nil {
		return nil
	}
	return sc.ProcessCallback
}

func (sc *scanInputConfig) GetRuleProcessCallback() RuleProcessCallback {
	if sc == nil {
		return nil
	}
	return sc.RuleProcessCallback
}
