package syntaxflow_scan

// Exports 用于导出到yak脚本引擎的函数映射
var Exports = map[string]any{
	// 主要扫描API
	"StartScan":     StartScan,
	"ResumeScan":    ResumeScan,
	"GetScanStatus": GetScanStatus,

	// 基础配置选项
	"withProgramNames":   WithProgramNames,
	"withIgnoreLanguage": WithIgnoreLanguage,
	"withConcurrency":    WithConcurrency,
	"withMemory":         WithMemory,
	"withResumeTaskId":   WithResumeTaskId,

	// 规则过滤选项
	"withRuleNames":          WithRuleNames,
	"withLanguages":          WithLanguages,
	"withGroupNames":         WithGroupNames,
	"withSeverity":           WithSeverity,
	"withPurpose":            WithPurpose,
	"withTags":               WithTags,
	"withKeyword":            WithKeyword,
	"withIncludeLibraryRule": WithIncludeLibraryRule,
}
