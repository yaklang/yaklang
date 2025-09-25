package ssaproject

var SSAProjectExports = map[string]interface{}{
	"GetSSAProjectByName": LoadSSAProjectBuilderByName,
	"GetSSAProjectByID":   LoadSSAProjectBuilderByID,
	"NewSSAProject":       NewSSAProjectBuilder,
	// 基础信息配置
	"withTags":        WithSSAProjectTags,
	"withLanguage":    WithSSAProjectLanguage,
	"withDescription": WithSSAProjectDescription,

	"withCodeSourceKind":         WithSSAProjectCodeSourceKind,
	"withCodeSourceLocalFile":    WithSSAProjectCodeSourceLocalFile,
	"withCodeSourceURL":          WithSSAProjectCodeSourceURL,
	"withCodeSourceBranch":       WithSSAProjectCodeSourceBranch,
	"withCodeSourcePath":         WithSSAProjectCodeSourcePath,
	"withCodeSourceAuthKind":     WithSSAProjectCodeSourceAuthKind,
	"withCodeSourceAuthUserName": WithSSAProjectCodeSourceAuthUserName,
	"withCodeSourceAuthPassword": WithSSAProjectCodeSourceAuthPassword,
	"withCodeSourceAuthKeyPath":  WithSSAProjectCodeSourceAuthKeyPath,
	"withCodeSourceProxyURL":     WithSSAProjectCodeSourceProxyURL,
	"withCodeSourceProxyAuth":    WithSSAProjectCodeSourceProxyAuth,
}

var SyntaxFlowScanConfigExports = map[string]interface{}{
	// 扫描配置
	"withScanConcurrency": WithSSAProjectScanConcurrency,
	"withMemoryScan":      WithSSAProjectMemoryScan,
	"withIgnoreLanguage":  WithSSAProjectIgnoreLanguage,

	// 进度条
	"withProcessCallback": WithSSAProjectProcessCallback,

	// 规则配置
	"withRuleFilterLanguage":           WithSSAProjectRuleFilterLanguage,
	"withRuleFilterSeverity":           WithSSAProjectRuleFilterSeverity,
	"withRuleFilterKind":               WithSSAProjectRuleFilterKind,
	"withRuleFilterPurpose":            WithSSAProjectRuleFilterPurpose,
	"withRuleFilterKeyword":            WithSSAProjectRuleFilterKeyword,
	"withRuleFilterGroupNames":         WithSSAProjectRuleFilterGroupNames,
	"withRuleFilterRuleNames":          WithSSAProjectRuleFilterRuleNames,
	"withRuleFilterTag":                WithSSAProjectRuleFilterTag,
	"withRuleFilterIncludeLibraryRule": WithSSAProjectRuleFilterIncludeLibraryRule,
}

var SSACompileConfigExports = map[string]interface{}{
	// 编译配置
	"withStrictMode":         WithSSAProjectStrictMode,
	"withPeepholeSize":       WithSSAProjectPeepholeSize,
	"withExcludeFiles":       WithSSAProjectExcludeFiles,
	"withReCompile":          WithSSAProjectReCompile,
	"withMemoryCompile":      WithSSAProjectMemoryCompile,
	"withCompileConcurrency": WithSSAProjectCompileConcurrency,
}
