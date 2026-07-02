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
	pinMaxLibraries   = 4    // 最多 PIN 几个库
	pinMaxFuncsPerLib = 26   // 每个库最多 PIN 多少条函数签名
	pinMaxTotalBytes  = 7000 // PIN 段总字节上限(超出则截断, 已 PIN 的足够用)
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

// BuildPinnedAPISection 为给定库生成紧凑的"接口速查卡": 每个库 = 一句话定位(OverviewShort)
// + 该库的函数签名(Decl, 单行)。在 init 阶段一次性算好, 注入反应数据, 让模型上手即有
// 权威签名(参数类型/个数), 从源头减少"参数类型错误/参数个数错误/猜错函数名"。
// 体积受 pinMax* 预算约束; 无任何有效签名时返回 ""(优雅降级)。
// 关键词: PIN 接口, 接口速查卡, 权威签名, 降低语法/类型错误
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
		if short := doc.GetLibOverviewShort(lib); short != "" {
			card.WriteString(" — ")
			card.WriteString(strings.TrimSpace(strings.SplitN(short, "\n", 2)[0]))
		}
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
					card.WriteString(fmt.Sprintf("... (%d more, 用 yakdoc_function_details 查完整签名)\n", remain))
				}
				break
			}
		}
		card.WriteString("```\n")

		if shown == 0 {
			continue
		}
		// 预算控制: 已有内容且再加会超预算就停, 已 PIN 的足够用。
		if total > 0 && total+card.Len() > pinMaxTotalBytes {
			break
		}
		b.WriteString(card.String())
		total += card.Len()
	}
	return strings.TrimSpace(b.String())
}
