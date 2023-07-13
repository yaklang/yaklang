package sca

import "github.com/yaklang/yaklang/common/sca/analyzer"

var Exports = map[string]interface{}{
	"ScanImageFromContext": LoadDockerImageFromContext,
	"ScanImageFromFile":    LoadDockerImageFromFile,
	"endpoint":             _withEndPoint,
	"scanMode":             _withScanMode,
	"concurrent":           _withConcurrent,

	"ALL_MODE":      analyzer.AllMode,
	"PKG_MODE":      analyzer.PkgMode,
	"LANGUAGE_MODE": analyzer.LanguageMode,
}
