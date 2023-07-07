package sca

import "github.com/yaklang/yaklang/common/sca/analyzer"

var Exports = map[string]interface{}{
	"ScanImageFromContext": LoadDockerImageFromContext,
	"ScanImageFromFile":    LoadDockerImageFromFile,
	"endpoint":             _withEndPoint,
	"scanMode":             _withScanMode,
	"concurrent":           _withConcurrent,

	"AllMode":      analyzer.AllMode,
	"PkgMode":      analyzer.PkgMode,
	"LanguageMode": analyzer.LanguageMode,
}
