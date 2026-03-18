package ssa

import "github.com/yaklang/yaklang/common/utils/diagnostics"

// SSA 相关 TrackKind，用于编译/构建/数据库等场景的测量归类
const (
	TrackKindAST           diagnostics.TrackKind = "AST"           // AST 解析（以文件为单位，含 pre-handler phase）
	TrackKindBuild         diagnostics.TrackKind = "Build"         // 构建（以 LazyBuild/文件为单位，含 parse phase）
	TrackKindDatabase      diagnostics.TrackKind = "Database"      // 数据库读写
	TrackKindScan          diagnostics.TrackKind = "Scan"         // 扫描 / 规则匹配
	TrackKindStaticAnalyze diagnostics.TrackKind = "StaticAnalyze" // 静态分析（Yaklang/Rule/SyntaxFlow）
)
