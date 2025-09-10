package syntaxflow_scan

import "github.com/yaklang/yaklang/common/yakgrpc/ypb"

type ProcessCallback func(progress float64)

type RuleProcessCallback func(progName, ruleName string, progress float64)

type ScanConfig struct {
	ScanRequest         *ypb.SyntaxFlowScanRequest
	ProcessCallback     ProcessCallback
	RuleProcessCallback RuleProcessCallback
}

func (sc *ScanConfig) GetScanRequest() *ypb.SyntaxFlowScanRequest {
	return sc.ScanRequest
}

func (sc *ScanConfig) GetProcessCallback() ProcessCallback {
	return sc.ProcessCallback
}

func (sc *ScanConfig) GetRuleProcessCallback() RuleProcessCallback {
	return sc.RuleProcessCallback
}
