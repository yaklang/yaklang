package syntaxflow_scan

import "github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"

var Exports = map[string]any{
	"StartScan":     StartScan,
	"ResumeScan":    ResumeScan,
	"GetScanStatus": GetScanStatus,
	// 进度
	"withScanProcessCallback":      WithProcessCallback,
	"withProcessRuleDetail":        WithProcessRuleDetail,
	"withScanResultCallback":       WithScanResultCallback,
	"withScanPrograms":             withPrograms,
	"withScanSourceFiles":          WithSourceFiles,
	"withScanSourceDir":            WithSourceDir,
	"withScanQueryTargets":         WithQueryTargets,
	"withScanConcurrency":          ssaconfig.WithScanConcurrency,
	"withScanRuleTimeout":          ssaconfig.WithScanRuleTimeout,
	"withScanRuleWorkLimit":        ssaconfig.WithScanRuleWorkLimit,
	"withScanRuleWorkLimitDefault": ssaconfig.WithScanRuleWorkLimitDefault,
	"withReporter":                 WithReporter,
	// Rule filter (builtin DB rules; prefer Tag=source for source-mode)
	"withRuleFilter":            ssaconfig.WithRuleFilter,
	"withRuleFilterTag":         ssaconfig.WithRuleFilterTag,
	"withRuleFilterMode":        ssaconfig.WithRuleFilterMode,
	"withRuleFilterKeyword":     ssaconfig.WithRuleFilterKeyword,
	"withRuleFilterGroupNames":  ssaconfig.WithRuleFilterGroupNames,
	"withRuleFilterLibRuleKind": ssaconfig.WithRuleFilterLibRuleKind,
}
