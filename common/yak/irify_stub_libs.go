//go:build irify_exclude

package yak

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// irifyExcludedStubFunction 是一个占位函数，用于在 irify_exclude 模式下提示用户
// 该功能需要完整版本
func irifyExcludedStubFunction(args ...interface{}) (interface{}, error) {
	return nil, utils.Errorf("This feature requires Irify edition (SSA/SyntaxFlow). Please use full version or rebuild without irify_exclude tag.")
}

// irifyExcludedStubSSAExports 提供 ssa 库的占位实现
// 用于前端代码补全和提示，但不提供实际功能
var irifyExcludedStubSSAExports = map[string]any{
	"Parse":              irifyExcludedStubFunction,
	"ParseLocalProject":  irifyExcludedStubFunction,
	"ParseProject":       irifyExcludedStubFunction,
	"NewFromProgramName": irifyExcludedStubFunction,
	"NewProgramFromDB":   irifyExcludedStubFunction,

	"withConcurrency":        irifyExcludedStubFunction,
	"withLanguage":           irifyExcludedStubFunction,
	"withConfigInfo":         irifyExcludedStubFunction,
	"withExternLib":          irifyExcludedStubFunction,
	"withExternValue":        irifyExcludedStubFunction,
	"withProgramName":        irifyExcludedStubFunction,
	"withDescription":        irifyExcludedStubFunction,
	"withProcess":            irifyExcludedStubFunction,
	"withEntryFile":          irifyExcludedStubFunction,
	"withReCompile":          irifyExcludedStubFunction,
	"withStrictMode":         irifyExcludedStubFunction,
	"withContext":            irifyExcludedStubFunction,
	"withPeepholeSize":       irifyExcludedStubFunction,
	"withExcludeFile":        irifyExcludedStubFunction,
	"withDefaultExcludeFunc": irifyExcludedStubFunction,
	"withMemory":             irifyExcludedStubFunction,

	// language constants
	"Javascript": irifyExcludedStubFunction,
	"Yak":        irifyExcludedStubFunction,
	"PHP":        irifyExcludedStubFunction,
	"Java":       irifyExcludedStubFunction,

	"YaklangScriptChecking": irifyExcludedStubFunction,
	"NewResultFromDB":       irifyExcludedStubFunction,

	// ssaproject exports
	"GetSSAProjectByNameAndURL": irifyExcludedStubFunction,
	"GetSSAProjectByID":         irifyExcludedStubFunction,
	"NewSSAProject":             irifyExcludedStubFunction,

	// ssaconfig exports
	"NewConfig":                    irifyExcludedStubFunction,
	"ModeAll":                      irifyExcludedStubFunction,
	"ModeProjectCompile":           irifyExcludedStubFunction,
	"withJsonRawConfig":            irifyExcludedStubFunction,
	"withProjectID":                irifyExcludedStubFunction,
	"withProjectName":              irifyExcludedStubFunction,
	"withProjectLanguage":          irifyExcludedStubFunction,
	"withProjectTags":              irifyExcludedStubFunction,
	"withProjectDescription":       irifyExcludedStubFunction,
	"withCodeSourceKind":           irifyExcludedStubFunction,
	"withCodeSourceLocalFile":      irifyExcludedStubFunction,
	"withCodeSourceURL":            irifyExcludedStubFunction,
	"withCodeSourceBranch":         irifyExcludedStubFunction,
	"withCodeSourcePath":           irifyExcludedStubFunction,
	"withCodeSourceAuthKind":       irifyExcludedStubFunction,
	"withCodeSourceAuthUserName":   irifyExcludedStubFunction,
	"withCodeSourceAuthPassword":   irifyExcludedStubFunction,
	"withCodeSourceAuthKeyPath":    irifyExcludedStubFunction,
	"withCodeSourceAuthKeyContent": irifyExcludedStubFunction,
}

// irifyExcludedStubSyntaxFlowExports 提供 syntaxflow 库的占位实现
// 用于前端代码补全和提示，但不提供实际功能
var irifyExcludedStubSyntaxFlowExports = map[string]any{
	"ExecRule":             irifyExcludedStubFunction,
	"withExecTaskID":       irifyExcludedStubFunction,
	"withExecDebug":        irifyExcludedStubFunction,
	"withProcess":          irifyExcludedStubFunction,
	"withContext":          irifyExcludedStubFunction,
	"withCache":            irifyExcludedStubFunction,
	"withSave":             irifyExcludedStubFunction,
	"withSearch":           irifyExcludedStubFunction,
	"QuerySyntaxFlowRules": irifyExcludedStubFunction,
}

// initIrifyLibs 初始化 Irify 相关的库（SSA 和 SyntaxFlow）的占位版本
// 仅在 irify_exclude 模式下调用，用于前端代码补全和提示
func initIrifyLibs() {
	yaklang.Import("ssa", irifyExcludedStubSSAExports)
	yaklang.Import("syntaxflow", irifyExcludedStubSyntaxFlowExports)
	log.Info("irify_exclude mode: SSA and SyntaxFlow libraries are replaced with stubs for frontend hints")
}
