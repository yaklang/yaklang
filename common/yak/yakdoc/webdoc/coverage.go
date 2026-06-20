package webdoc

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

// FuncCoverage 记录单个导出函数的文档缺口（缺描述/缺示例/缺参数解释/缺返回解释）。
// 关键词: 文档覆盖率, 文档缺口
type FuncCoverage struct {
	Lib            string
	Method         string
	MissingDesc    bool     // 无描述（首行描述为空）
	MissingExample bool     // 无 Example 段
	ParamsNoExpl   []string // 缺解释的参数名
	ResultsNoExpl  int      // 缺解释的返回值个数
}

// HasGap 该函数是否存在任一文档缺口。
func (c *FuncCoverage) HasGap() bool {
	return c.MissingDesc || c.MissingExample || len(c.ParamsNoExpl) > 0 || c.ResultsNoExpl > 0
}

// CoverageReport 全量文档覆盖率统计结果。
type CoverageReport struct {
	Total    int             // 导出函数总数
	WithGap  int             // 存在缺口的函数数
	Gaps     []*FuncCoverage // 仅包含有缺口的函数明细（按库名+方法名排序）
	libCount map[string]int  // 每库缺口计数
}

// CollectDocCoverage 遍历所有库的导出函数，统计文档缺口。该函数无副作用、可测试。
// 关键词: CollectDocCoverage, 文档覆盖率统计
func CollectDocCoverage(libs map[string]*yakdoc.ScriptLib) *CoverageReport {
	report := &CoverageReport{libCount: make(map[string]int)}
	libNames := lo.Keys(libs)
	sort.Strings(libNames)

	for _, libName := range libNames {
		lib := libs[libName]
		methodNames := lo.Keys(lib.Functions)
		sort.Strings(methodNames)
		for _, name := range methodNames {
			fun := lib.Functions[name]
			report.Total++

			parsed := parseCommentDetails(fun.Document)
			cov := &FuncCoverage{Lib: fun.LibName, Method: fun.MethodName}
			cov.MissingDesc = strings.TrimSpace(parsed.Description) == ""
			cov.MissingExample = extractExampleCode(fun.Document) == ""
			for _, param := range fun.Params {
				if strings.TrimSpace(parsed.Params[param.Name]) == "" {
					cov.ParamsNoExpl = append(cov.ParamsNoExpl, param.Name)
				}
			}
			for i := range fun.Results {
				if i >= len(parsed.Returns) || strings.TrimSpace(parsed.Returns[i]) == "" {
					cov.ResultsNoExpl++
				}
			}

			if cov.HasGap() {
				report.WithGap++
				report.Gaps = append(report.Gaps, cov)
				report.libCount[fun.LibName]++
			}
		}
	}
	return report
}

// LogSummary 以英文 log 打印覆盖率汇总（非阻断）。逐项 Warn 缺口、末尾打印每库与总计。
func (r *CoverageReport) LogSummary() {
	for _, g := range r.Gaps {
		var missing []string
		if g.MissingDesc {
			missing = append(missing, "description")
		}
		if len(g.ParamsNoExpl) > 0 {
			missing = append(missing, fmt.Sprintf("param-explanation(%s)", strings.Join(g.ParamsNoExpl, ",")))
		}
		if g.ResultsNoExpl > 0 {
			missing = append(missing, fmt.Sprintf("return-explanation(%d)", g.ResultsNoExpl))
		}
		if g.MissingExample {
			missing = append(missing, "example")
		}
		log.Warnf("doc coverage gap: %s.%s missing %s", g.Lib, g.Method, strings.Join(missing, ", "))
	}

	libs := lo.Keys(r.libCount)
	sort.Strings(libs)
	for _, name := range libs {
		log.Warnf("doc coverage: lib %s has %d function(s) with gaps", name, r.libCount[name])
	}
	log.Infof("doc coverage summary: %d/%d functions have gaps (%d ok)", r.WithGap, r.Total, r.Total-r.WithGap)
}

// WriteMarkdown 把覆盖率明细写成 markdown 底单，用于驱动 backfill。该文件应写到 docs/api 之外。
func (r *CoverageReport) WriteMarkdown(p string) error {
	if dir := path.Dir(p); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	buf := strings.Builder{}
	buf.WriteString("# API Documentation Coverage Baseline\n\n")
	buf.WriteString(fmt.Sprintf("Total functions: %d; functions with gaps: %d; ok: %d\n\n", r.Total, r.WithGap, r.Total-r.WithGap))

	byLib := make(map[string][]*FuncCoverage)
	for _, g := range r.Gaps {
		byLib[g.Lib] = append(byLib[g.Lib], g)
	}
	libs := lo.Keys(byLib)
	sort.Strings(libs)
	for _, lib := range libs {
		buf.WriteString(fmt.Sprintf("## %s (%d)\n\n", lib, len(byLib[lib])))
		buf.WriteString("|function|missing description|missing param explanation|missing return explanation|missing example|\n")
		buf.WriteString("|:--|:--|:--|:--|:--|\n")
		for _, g := range byLib[lib] {
			descMark := ""
			if g.MissingDesc {
				descMark = "yes"
			}
			paramMark := ""
			if len(g.ParamsNoExpl) > 0 {
				paramMark = strings.Join(g.ParamsNoExpl, ",")
			}
			retMark := ""
			if g.ResultsNoExpl > 0 {
				retMark = fmt.Sprintf("%d", g.ResultsNoExpl)
			}
			exMark := ""
			if g.MissingExample {
				exMark = "yes"
			}
			buf.WriteString(fmt.Sprintf("|%s|%s|%s|%s|%s|\n", g.Method, descMark, paramMark, retMark, exMark))
		}
		buf.WriteString("\n")
	}
	return os.WriteFile(p, []byte(buf.String()), 0o644)
}
