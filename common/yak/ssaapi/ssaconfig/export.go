package ssaconfig

var Exports = map[string]interface{}{
	"NewConfig":               New,
	"ModeAll":                 ModeAll,
	"ModeProjectCompile":      ModeProjectCompile,
	"withJsonRawConfig":       WithJsonRawConfig,
	"withProjectID":           WithProjectID,
	"withProjectName":         WithProjectName,
	"withProjectLanguage":     WithProjectLanguage,
	"withProjectTags":         WithProjectTags,
	"withProjectDescription":  WithProjectDescription,
	"withCodeSourceKind":      WithCodeSourceKind,
	"withCodeSourceLocalFile": WithCodeSourceLocalFile,
	"withCodeSourceURL":       WithCodeSourceURL,
	"withCodeSourceBranch":    WithCodeSourceBranch,
	"withCodeSourcePath":      WithCodeSourcePath,
}
