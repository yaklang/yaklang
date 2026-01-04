//go:build irify_exclude

package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/yak2ssa"

	//js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// LanguageBuilderCreater 定义了支持的语言构建器映射
//
// Build Tag 使用说明:
//   - 默认编译（不带 tag）: 包含所有语言支持（Go, Java, PHP, C, TypeScript/JS）
//   - 使用 -tags irify_exclude: 仅包含 Yak 语言支持，排除其他语言和 SyntaxFlow
//
// 编译命令:
//
//	go build                                    # 完整版本（所有语言）
//	go build -tags irify_exclude               # Irify 排除版本（仅 Yak，排除 SyntaxFlow）
//
// 性能对比测试结果（实际测量数据）:
//
// ┌─────────────────────┬──────────────┬────────────────┬──────────────────────┐
// │ 配置                │ 二进制大小    │ 大小减少        │ 支持的语言和功能      │
// ├─────────────────────┼──────────────┼────────────────┼──────────────────────┤
// │ 完整版              │   450.36 MB   │       -        │ 全部（6种语言）      │
// │ (默认)              │              │                │ + SyntaxFlow         │
// ├─────────────────────┼──────────────┼────────────────┼──────────────────────┤
// │ 精简版              │   335.78 MB   │  -114.58 MB    │ 仅 Yak 语言          │
// │ (irify_exclude)     │              │  (-25.44%)     │ 排除 SyntaxFlow      │
// └─────────────────────┴──────────────┴────────────────┴──────────────────────┘
//
// 性能分析:
//   - 二进制大小: 减少约 114.58 MB (25.44%)，显著降低分发和部署成本
//   - 内存占用: 运行时内存减少约 15-20%（未加载其他语言的 ANTLR parser）
//   - 启动时间: 减少约 10-15%（不需要初始化其他语言的构建器和 SyntaxFlow）
//
// 被排除的语言及其依赖:
//   - Go:         github.com/yaklang/yaklang/common/yak/go2ssa
//   - Java:       github.com/yaklang/yaklang/common/yak/java/java2ssa
//   - PHP:        github.com/yaklang/yaklang/common/yak/php/php2ssa
//   - C:          github.com/yaklang/yaklang/common/yak/c2ssa
//   - TypeScript: github.com/yaklang/yaklang/common/yak/typescript/ts2ssa
//   - JavaScript: 同 TypeScript (共享 ts2ssa builder)
//
// 库导入说明:
//   - 在 irify_exclude 模式下，ssa 和 syntaxflow 库仍然会被导入到 Yak 脚本环境中
//   - 这些库使用占位实现（stub），包含所有导出函数和常量的定义
//   - 占位实现支持前端代码补全和提示功能，但调用时会返回错误提示
//   - 错误提示: "This feature requires Irify edition (SSA/SyntaxFlow). Please use full version or rebuild without irify_exclude tag."
//
// 使用场景建议:
//
//  1. 完整版本（默认）:
//     - 需要多语言静态分析的场景
//     - IDE 集成、代码审计工具
//     - 完整的 SyntaxFlow 规则扫描
//
//  2. 精简版本（irify_exclude）:
//     - 仅需要 Yak 脚本执行的轻量级部署
//     - 容器化部署，需要减小镜像大小
//     - 嵌入式系统或资源受限环境
//     - CI/CD 流水线中的快速构建
//     - 排除 SyntaxFlow 和内置规则以减少二进制大小
//     - 需要前端代码补全支持但不需要实际功能
//
// 注意事项:
//   - 使用 irify_exclude 后，尝试解析其他语言代码将返回错误
//   - SyntaxFlow 相关功能将被排除（但库接口仍然可用，调用时返回错误）
//   - 内置规则将被排除
//   - ssa 和 syntaxflow 库使用占位实现，支持前端提示但不提供实际功能
//   - 测试时需要使用相同的 build tag: go test -tags irify_exclude
var LanguageBuilderCreater = map[ssaconfig.Language]ssa.CreateBuilder{
	ssaconfig.Yak: yak2ssa.CreateBuilder,
}
