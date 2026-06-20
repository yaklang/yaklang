package webdoc

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

// 关键词: 选项关联, 可变参数选项, optionType, buildOptionIndex
//
// 选项(option)模式：主函数有一个可变参数 "...T"，库内另有若干"选项函数"——其唯一返回值
// 类型恰为 T。例如 db.SaveHTTPFlowFromRawWithOption(..., exOption ...yakit.CreateHTTPFlowOptions)
// 与 db.saveHTTPFlowWithTags(tags) yakit.CreateHTTPFlowOptions。
//
// 渲染策略(对齐需求：选项只在对应主函数下出现，不在开头单列；多个主函数共享同一选项则重复展示)：
//   - 选项函数从顶层"函数索引/函数详情"中剔除；
//   - 在每个消费该选项类型的主函数详情里，渲染"可选参数 / 选项"小节，逐条列出选项函数
//     (内联签名 + 一句话描述)，多个主函数共享则重复渲染。

// isOptionTypeName 判定一个类型名是否"看起来像选项类型"：以 Option 或 Options 结尾。
// 仅据此从可变参数里识别选项类型，避免把 ...string / ...interface{} 误判为选项。
func isOptionTypeName(typ string) bool {
	return strings.HasSuffix(typ, "Option") || strings.HasSuffix(typ, "Options")
}

// variadicElemType 若 typ 是可变参数(以 ... 开头)则返回其元素类型，否则返回空串。
func variadicElemType(typ string) string {
	if strings.HasPrefix(typ, "...") {
		return strings.TrimPrefix(typ, "...")
	}
	return ""
}

// OptionIndex 记录库内的选项关联关系。
type OptionIndex struct {
	// producers: 选项类型 -> 生产该类型的选项函数(已按方法名排序)
	producers map[string][]*yakdoc.FuncDecl
	// isProducer: 某函数是否为(被消费的)选项函数
	isProducer map[*yakdoc.FuncDecl]bool
}

// isOptionParam 判定某参数是否为"可关联选项"的可变参数(...Option/...Options 且库内有生产者)。
func (oi *OptionIndex) isOptionParam(p *yakdoc.Field) bool {
	elem := variadicElemType(p.Type)
	return elem != "" && isOptionTypeName(elem) && len(oi.producers[elem]) > 0
}

// optionTypesOf 返回某函数作为"主函数"所消费的选项类型列表(其可变参数中以 Option/Options 结尾者)。
func (oi *OptionIndex) optionTypesOf(fun *yakdoc.FuncDecl) []string {
	var types []string
	seen := map[string]bool{}
	for _, p := range fun.Params {
		elem := variadicElemType(p.Type)
		if elem == "" || !isOptionTypeName(elem) {
			continue
		}
		if len(oi.producers[elem]) == 0 {
			continue // 没有任何生产者则不算可关联的选项
		}
		if !seen[elem] {
			seen[elem] = true
			types = append(types, elem)
		}
	}
	return types
}

// buildOptionIndex 扫描库，建立选项类型 -> 选项函数 的索引。
// 关键词: buildOptionIndex, 选项生产者
func buildOptionIndex(funcs []*yakdoc.FuncDecl) *OptionIndex {
	oi := &OptionIndex{
		producers:  map[string][]*yakdoc.FuncDecl{},
		isProducer: map[*yakdoc.FuncDecl]bool{},
	}

	// 第一步：收集所有"被作为可变参数消费、且形如 Option/Options"的选项类型
	consumed := map[string]bool{}
	for _, fun := range funcs {
		for _, p := range fun.Params {
			elem := variadicElemType(p.Type)
			if elem != "" && isOptionTypeName(elem) {
				consumed[elem] = true
			}
		}
	}
	if len(consumed) == 0 {
		return oi
	}

	// 第二步：把"唯一返回值类型属于已消费选项类型"的函数登记为该类型的生产者
	for _, fun := range funcs {
		if len(fun.Results) != 1 {
			continue
		}
		retType := fun.Results[0].Type
		if consumed[retType] {
			oi.producers[retType] = append(oi.producers[retType], fun)
			oi.isProducer[fun] = true
		}
	}
	// 生产者按方法名排序，保证稳定输出
	for typ := range oi.producers {
		fs := oi.producers[typ]
		sort.SliceStable(fs, func(i, j int) bool { return fs[i].MethodName < fs[j].MethodName })
		oi.producers[typ] = fs
	}
	return oi
}

// renderOptionTypeBlock 为某个"选项型可变参数"渲染其全部选项函数(内联展示，无独立锚点，
// 可在多个主函数下重复出现)。调用方负责在外层先写好"**可选参数**"小节标签。
// 关键词: renderOptionTypeBlock, 可选参数, 选项重复渲染
func (oi *OptionIndex) renderOptionTypeBlock(p *yakdoc.Field) string {
	typ := variadicElemType(p.Type)
	producers := oi.producers[typ]
	if len(producers) == 0 {
		return ""
	}
	var b strings.Builder
	name := strings.TrimSpace(p.Name)
	if name != "" {
		b.WriteString(fmt.Sprintf("可作为可变参数 `%s ...%s` 传入以下选项：\n\n", html.EscapeString(name), typ))
	} else {
		b.WriteString(fmt.Sprintf("可作为可变参数 `...%s` 传入以下选项：\n\n", typ))
	}
	for _, opt := range producers {
		parsed := parseCommentDetails(opt.Document)
		desc := stripLeadingFuncName(strings.TrimSpace(parsed.Description), opt.MethodName)
		if desc == "" {
			b.WriteString(fmt.Sprintf("- `%s.%s`\n", opt.LibName, opt.MethodName))
		} else {
			b.WriteString(fmt.Sprintf("- `%s.%s` — %s\n", opt.LibName, opt.MethodName, escapeInlineLabel(stripExportSuffix(desc))))
		}
		// 选项签名用行内代码展示完整签名(围栏内不转义，<&安全)
		b.WriteString(fmt.Sprintf("  - 签名：`%s`\n", inlineSignature(opt.Decl)))
	}
	b.WriteString("\n")
	return b.String()
}

// inlineSignature 把签名压成单行(去多余空白/换行)，用于行内代码展示。
func inlineSignature(decl string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(decl, "\n", " ")), " ")
}

// stripExportSuffix 去掉描述末尾的"（导出名为 xxx）"括注，避免在选项条目里冗长。
func stripExportSuffix(desc string) string {
	for _, marker := range []string{"（导出名为", "(导出名为"} {
		if idx := strings.Index(desc, marker); idx >= 0 {
			return strings.TrimSpace(desc[:idx])
		}
	}
	return desc
}
