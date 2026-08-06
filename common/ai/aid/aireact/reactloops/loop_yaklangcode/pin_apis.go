package loop_yaklangcode

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
)

// PIN 接口的预算上限: 控制注入到反应数据里的体积, 避免每轮重复渲染撑爆上下文。
const (
	pinMaxLibraries   = 3    // 最多 PIN 几个库
	pinMaxFuncsPerLib = 12   // 每个库最多 PIN 多少条函数签名
	pinMaxTotalBytes  = 3500 // PIN 段总字节上限(超出则截断)
)

// patternLibPrefixRe 从 search_pattern 里提取库名前缀, 兼容 `poc\.HTTPEx` 与 `poc.HTTPEx` 两种写法。
var patternLibPrefixRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\\?\.`)

// CollectPinnedLibraries 汇总待 PIN 的库: 优先 AI 显式选出的 core_libraries,
// 再用 search_patterns 里的 `lib.` 前缀兜底; 只保留 yakdoc 中真实存在(有函数文档)的库,
// 去重并限制数量。返回的库名顺序: core_libraries 在前, 派生的在后。
// 关键词: 选库, core_libraries, 从搜索关键字派生库名, yakdoc 校验
func CollectPinnedLibraries(coreLibraries, searchPatterns []string) []string {
	seen := map[string]bool{}
	var out []string

	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		// 只 PIN yakdoc 中真实存在(有函数)的库, 否则跳过(优雅降级)。
		if len(doc.GetDocumentFunctions(name)) == 0 {
			return
		}
		seen[name] = true
		out = append(out, name)
	}

	for _, lib := range coreLibraries {
		add(lib)
	}
	for _, pattern := range searchPatterns {
		m := patternLibPrefixRe.FindStringSubmatch(strings.TrimSpace(pattern))
		if len(m) == 2 {
			add(m[1])
		}
	}

	if len(out) > pinMaxLibraries {
		out = out[:pinMaxLibraries]
	}
	return out
}

// BuildPinnedAPISection 为给定库生成紧凑签名卡: 仅库名 + Decl 单行(无 overview)。
// Init 算一次后注入反应数据; 体积受 pinMax* 约束。
func BuildPinnedAPISection(libNames []string) string {
	if len(libNames) == 0 {
		return ""
	}

	var b strings.Builder
	total := 0
	for _, lib := range libNames {
		funcs := doc.GetDocumentFunctions(lib)
		if len(funcs) == 0 {
			continue
		}
		names := make([]string, 0, len(funcs))
		for name := range funcs {
			names = append(names, name)
		}
		sort.Strings(names)

		var card strings.Builder
		card.WriteString("### ")
		card.WriteString(lib)
		card.WriteString("\n```\n")
		shown := 0
		for _, name := range names {
			fn := funcs[name]
			if fn == nil || strings.TrimSpace(fn.Decl) == "" {
				continue
			}
			card.WriteString(lib)
			card.WriteString(".")
			card.WriteString(strings.TrimSpace(fn.Decl))
			card.WriteString("\n")
			shown++
			if shown >= pinMaxFuncsPerLib {
				if remain := len(names) - shown; remain > 0 {
					card.WriteString(fmt.Sprintf("... (%d more → yakdoc_function_details)\n", remain))
				}
				break
			}
		}
		card.WriteString("```\n")

		if shown == 0 {
			continue
		}
		if total > 0 && total+card.Len() > pinMaxTotalBytes {
			break
		}
		b.WriteString(card.String())
		total += card.Len()
	}
	return strings.TrimSpace(b.String())
}

// pinnedDSLRules 写码前注入的 Yaklang DSL 硬规则（短文案，直接 Go 常量，不走 .txt embed）。
// 关键词: PIN DSL, 禁止 Go/Java 语法, 匿名函数, byte 数组, poc 三返回值, YAK_MAIN 自测
const pinnedDSLRules = `### Yaklang DSL 硬规则（写码前必读，禁止 Go/Java 语法）
- **匿名函数**：` + "`name = func(arg) { ... }`" + `；**禁止**参数/返回类型声明
  - 错误：` + "`func(x string) []byte {`" + `、` + "`func(frame []byte) {`" + `
  - 正确：` + "`build = func(gadgetB64) {`" + `、` + "`build = func(frame) {`" + `
- **byte 数组**：` + "`[]byte{0xAC, 0xED, 0x00, 0x05}`" + `；元素必须是 byte 字面量（` + "`0x..`" + `），禁止裸整数
- **[]byte 拼接**：` + "`append(a, b...)`" + `；**禁止** ` + "`append(a, b)`" + `（整段 bytes 不能当单个 T）。单字节：` + "`append(a, 0x70)`" + `
- **poc.Get / poc.Post**：必须三变量接收 ` + "`rsp, req, err := poc.Post(...)`" + `
- **PoC/插件脚本**：末尾加 ` + "`func runSelfTest(){...}`" + ` 与 ` + "`if YAK_MAIN { runSelfTest() }`" + ``

// BuildPinnedDSLSection 返回 Init 阶段注入的 DSL 硬规则，与 BuildPinnedAPISection 成对使用。
func BuildPinnedDSLSection() string {
	return pinnedDSLRules
}
