package syntaxflow_scan

var Exports = map[string]any{
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

	// 进度
	"withProcessCallback": WithProcessCallback,
}
