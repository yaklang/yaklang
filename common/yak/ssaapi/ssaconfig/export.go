package ssaconfig

var Exports = map[string]interface{}{
	"New":                      New,
	"ModeAll":                  ModeAll,
	"ModeProjectCompile":       ModeProjectCompile,
	"withJsonRawConfig":        WithJsonRawConfig,
	"withProjectName":          WithProjectName,
	"withProgramNames":         WithProgramNames,
	"withProgramDescription":   WithProgramDescription,
	"withProjectLanguage":      WithProjectLanguage,
	"withProjectTags":          WithProjectTags,
	"withCodeSourceKind":       WithCodeSourceKind,
	"withCodeSourceLocalFile":  WithCodeSourceLocalFile,
	"withCodeSourceURL":        WithCodeSourceURL,
	"withCodeSourceBranch":     WithCodeSourceBranch,
	"withCodeSourcePath":       WithCodeSourcePath,
	"withCompileStrictMode":    WithCompileStrictMode,
	"withCompilePeepholeSize":  WithCompilePeepholeSize,
	"withCompileExcludeFiles":  WithCompileExcludeFiles,
	"withCompileReCompile":     WithCompileReCompile,
	"withCompileMemoryCompile": WithCompileMemoryCompile,
	"withCompileConcurrency":   WithCompileConcurrency,
}
