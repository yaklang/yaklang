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
//   - 在每个消费该选项类型的主函数详情里，渲染"可选参数 / 选项"小节，用与"函数索引"同款
//     表格(函数 | 参数 | 返回值 | 说明)列出全部选项函数，多个主函数共享则重复渲染。

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

// baseTypeName 取类型名的"裸基名"：去掉前导 * 与包限定前缀(pkg.)。
// 用于跨包选项匹配：消费侧形参类型常带包名(如 ...fp.ConfigOption)，而生产侧函数
// 定义在该包内、其返回类型记录为裸名(ConfigOption)，二者本是同一类型，按基名比较才能关联。
func baseTypeName(typ string) string {
	t := strings.TrimSpace(typ)
	t = strings.TrimPrefix(t, "*")
	if i := strings.LastIndex(t, "."); i >= 0 {
		t = t[i+1:]
	}
	return t
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
	return elem != "" && isOptionTypeName(elem) && len(oi.producers[baseTypeName(elem)]) > 0
}

// optionTypesOf 返回某函数作为"主函数"所消费的选项类型基名列表(其可变参数中以 Option/Options 结尾者)。
func (oi *OptionIndex) optionTypesOf(fun *yakdoc.FuncDecl) []string {
	var types []string
	seen := map[string]bool{}
	for _, p := range fun.Params {
		elem := variadicElemType(p.Type)
		if elem == "" || !isOptionTypeName(elem) {
			continue
		}
		base := baseTypeName(elem)
		if len(oi.producers[base]) == 0 {
			continue // 没有任何生产者则不算可关联的选项
		}
		if !seen[base] {
			seen[base] = true
			types = append(types, base)
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

	// 第一步：收集所有"被作为可变参数消费、且形如 Option/Options"的选项类型(按基名归一)
	consumed := map[string]bool{}
	for _, fun := range funcs {
		for _, p := range fun.Params {
			elem := variadicElemType(p.Type)
			if elem != "" && isOptionTypeName(elem) {
				consumed[baseTypeName(elem)] = true
			}
		}
	}
	if len(consumed) == 0 {
		return oi
	}

	// 第二步：把"唯一返回值基名属于已消费选项类型"的函数登记为该类型的生产者。
	// 按基名比较以兼容跨包限定差异(如消费 ...fp.ConfigOption、生产 ConfigOption)。
	for _, fun := range funcs {
		if len(fun.Results) != 1 {
			continue
		}
		base := baseTypeName(fun.Results[0].Type)
		if consumed[base] {
			oi.producers[base] = append(oi.producers[base], fun)
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
	producers := oi.producers[baseTypeName(typ)]
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
	// 与"函数索引"同款表格：函数 | 参数 | 返回值 | 说明。选项函数无独立锚点，故函数名用行内代码而非链接。
	b.WriteString("|选项函数|参数|返回值|说明|\n")
	b.WriteString("|:--|:--|:--|:--|\n")
	for _, opt := range producers {
		parsed := parseCommentDetails(opt.Document)
		desc := stripExportSuffix(stripLeadingFuncName(strings.TrimSpace(parsed.Description), opt.MethodName))
		b.WriteString(fmt.Sprintf("| `%s.%s` | %s | %s | %s |\n",
			html.EscapeString(opt.LibName),
			html.EscapeString(opt.MethodName),
			funcParamCell(opt),
			funcReturnCell(opt),
			escapeTableCell(desc),
		))
	}
	b.WriteString("\n")
	return b.String()
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
