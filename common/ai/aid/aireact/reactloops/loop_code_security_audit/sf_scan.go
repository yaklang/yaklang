package loop_code_security_audit

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// SFScanResult SyntaxFlow 扫描结果
type SFScanResult struct {
	RuleName    string      `json:"rule_name"`
	RuleType    string      `json:"rule_type"` // source / sink
	AlertVars   []SFHitVar  `json:"alert_vars"`
	Error       string      `json:"error,omitempty"`
}

// SFHitVar 单个 alert 变量的命中信息
type SFHitVar struct {
	VarName string   `json:"var_name"`
	Count   int      `json:"count"`
	Values  []SFHit  `json:"values"`
}

// SFHit 单个命中点
type SFHit struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function string `json:"function,omitempty"`
	Opcode   string `json:"opcode,omitempty"`
}

// SFScanSummary 扫描汇总
type SFScanSummary struct {
	ProgramName string         `json:"program_name"`
	Language    string         `json:"language"`
	TotalRules  int            `json:"total_rules"`
	HitRules    int            `json:"hit_rules"`
	Results     []*SFScanResult `json:"results"`
	Sources     []SFHit        `json:"sources"` // 所有 source 汇总
	Sinks       []SFHit        `json:"sinks"`   // 所有 sink 汇总
}

// CompileAndScanProject 编译项目并运行 SyntaxFlow lib 规则扫描
func CompileAndScanProject(projectPath, language string) (*SFScanSummary, error) {
	// 1. 编译项目到 SSA IR
	programName := fmt.Sprintf("sfscan-%s-%s", language, filepath.Base(projectPath))
	lang, err := ssaconfig.ValidateLanguage(language)
	if err != nil {
		return nil, fmt.Errorf("invalid language %q: %w", language, err)
	}

	log.Infof("[SFScan] Compiling project: %s (lang=%s)", projectPath, language)
	progs, err := ssaapi.ParseProject(
		ssaapi.WithLanguage(lang),
		ssaapi.WithProgramName(programName),
		ssaapi.WithLocalFs(projectPath),
	)
	if err != nil {
		return nil, fmt.Errorf("SSA compile failed: %w", err)
	}
	if len(progs) == 0 {
		return nil, fmt.Errorf("SSA compile returned no programs")
	}
	prog := progs[0]
	log.Infof("[SFScan] SSA compilation complete: program=%s", programName)

	// 2. 加载该语言的 lib 规则
	libRules, err := loadLibRules(language)
	if err != nil {
		return nil, fmt.Errorf("load lib rules: %w", err)
	}
	log.Infof("[SFScan] Loaded %d lib rules for language %s", len(libRules), language)

	// 3. 运行每条 lib 规则
	summary := &SFScanSummary{
		ProgramName: programName,
		Language:    language,
		TotalRules:  len(libRules),
		Results:     make([]*SFScanResult, 0),
	}

	for _, rule := range libRules {
		result := runSingleRule(prog, rule)
		summary.Results = append(summary.Results, result)

		if len(result.AlertVars) > 0 {
			summary.HitRules++
			// 分类 source 和 sink
			ruleType := classifyRule(rule)
			for _, av := range result.AlertVars {
				for _, v := range av.Values {
					hit := SFHit{
						File:     v.File,
						Line:     v.Line,
						Function: v.Function,
						Opcode:   v.Opcode,
					}
					if ruleType == "source" {
						summary.Sources = append(summary.Sources, hit)
					} else {
						summary.Sinks = append(summary.Sinks, hit)
					}
				}
			}
		}
	}

	log.Infof("[SFScan] Scan complete: %d/%d rules hit, %d sources, %d sinks",
		summary.HitRules, summary.TotalRules, len(summary.Sources), len(summary.Sinks))

	return summary, nil
}

// loadLibRules 加载指定语言的 lib 类型 SyntaxFlow 规则
func loadLibRules(language string) ([]*schema.SyntaxFlowRule, error) {
	db := consts.GetGormProfileDatabase()
	var rules []*schema.SyntaxFlowRule

	// lib 规则的特征：AllowIncluded=true, IsBuildInRule=true, language 匹配
	result := db.Where("language = ? AND allow_included = ? AND is_build_in_rule = ?",
		language, true, true).Find(&rules)
	if result.Error != nil {
		return nil, result.Error
	}

	// 如果数据库中没有 lib 规则，尝试从文件系统加载
	if len(rules) == 0 {
		log.Infof("[SFScan] No lib rules in DB for %s, trying to load from builtin", language)
		rules = loadLibRulesFromBuiltin(language)
	}

	return rules, nil
}

// loadLibRulesFromBuiltin 从内置规则文件加载 lib 规则（回退方案）
func loadLibRulesFromBuiltin(language string) []*schema.SyntaxFlowRule {
	// 直接使用 sfdb 的内置规则加载
	db := consts.GetGormProfileDatabase()
	var rules []*schema.SyntaxFlowRule

	// 尝试加载所有内置规则中 lib 类型的
	db.Model(&schema.SyntaxFlowRule{}).
		Where("is_build_in_rule = ? AND allow_included = ?", true, true).
		Find(&rules)

	// 按语言过滤
	langMap := map[string]string{
		"go":         "golang",
		"golang":     "golang",
		"java":       "java",
		"python":     "python",
		"javascript": "javascript",
		"ecmascript": "javascript",
		"php":        "php",
		"ruby":       "ruby",
		"rust":       "rust",
		"c":          "c",
		"cpp":        "c",
	}

	targetLang := langMap[language]
	if targetLang == "" {
		targetLang = language
	}

	var filtered []*schema.SyntaxFlowRule
	for _, r := range rules {
		if strings.EqualFold(string(r.Language), targetLang) || strings.EqualFold(string(r.Language), language) {
			filtered = append(filtered, r)
		}
	}

	return filtered
}

// runSingleRule 对编译后的程序运行单条 SyntaxFlow 规则
func runSingleRule(prog *ssaapi.Program, rule *schema.SyntaxFlowRule) *SFScanResult {
	result := &SFScanResult{
		RuleName: rule.RuleName,
	}

	sfResult, err := prog.SyntaxFlowWithError(rule.Content)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// 提取 alert 变量
	alertVars := sfResult.GetAlertVariables()
	for _, varName := range alertVars {
		vals := sfResult.GetValues(varName)
		if vals == nil {
			continue
		}

		hitVar := SFHitVar{
			VarName: varName,
			Count:   len(vals),
		}

		for _, val := range vals {
			hit := SFHit{}
			r := val.GetRange()
			if r != nil {
				if r.GetEditor() != nil {
					hit.File = r.GetEditor().GetFilename()
				}
				if r.GetStart() != nil {
					hit.Line = r.GetStart().GetLine()
				}
			}
			hit.Opcode = string(val.GetOpcode())
			hitVar.Values = append(hitVar.Values, hit)
		}

		result.AlertVars = append(result.AlertVars, hitVar)
	}

	return result
}

// classifyRule 根据规则名称和内容判断是 source 还是 sink
func classifyRule(rule *schema.SyntaxFlowRule) string {
	name := strings.ToLower(rule.RuleName)
	title := strings.ToLower(rule.Title)
	content := strings.ToLower(rule.Content)

	// source 规则特征（用户输入入口）
	sourceKeywords := []string{
		"source", "input", "param", "user-input", "http-source",
		"handlefunc", "gin-context", "query", "form", "cookie",
		"header", "url-param", "request-param", "env-source",
	}
	for _, kw := range sourceKeywords {
		if strings.Contains(name, kw) || strings.Contains(title, kw) {
			return "source"
		}
	}

	// sink 规则特征（危险函数调用）
	sinkKeywords := []string{
		"sink", "exec", "command", "sql", "database", "file-read",
		"file-write", "file-path", "http-sink", "request-execute",
		"xml", "ldap", "ftp", "os-sink", "os-exec", "fmt-print",
	}
	for _, kw := range sinkKeywords {
		if strings.Contains(name, kw) || strings.Contains(title, kw) {
			return "sink"
		}
	}

	// 通过内容判断
	if strings.Contains(content, "as $output") || strings.Contains(content, "as $sink") {
		return "sink"
	}
	if strings.Contains(content, "as $input") || strings.Contains(content, "as $source") {
		return "source"
	}

	return "unknown"
}

// FormatSFScanSummaryForPrompt 将扫描汇总格式化为 prompt 文本
func FormatSFScanSummaryForPrompt(summary *SFScanSummary) string {
	if summary == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## SyntaxFlow 自动扫描结果\n\n")
	sb.WriteString(fmt.Sprintf("- **编译程序**: %s\n", summary.ProgramName))
	sb.WriteString(fmt.Sprintf("- **语言**: %s\n", summary.Language))
	sb.WriteString(fmt.Sprintf("- **规则数**: %d (命中 %d)\n", summary.TotalRules, summary.HitRules))
	sb.WriteString(fmt.Sprintf("- **发现 Source**: %d 个\n", summary.Sources))
	sb.WriteString(fmt.Sprintf("- **发现 Sink**: %d 个\n", len(summary.Sinks)))

	if len(summary.Sources) > 0 {
		sb.WriteString("\n### Source 位置（用户输入入口）\n\n")
		for _, s := range summary.Sources {
			sb.WriteString(fmt.Sprintf("- `%s:%d`", s.File, s.Line))
			if s.Function != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", s.Function))
			}
			sb.WriteString("\n")
		}
	}

	if len(summary.Sinks) > 0 {
		sb.WriteString("\n### Sink 位置（危险函数调用）\n\n")
		for _, s := range summary.Sinks {
			sb.WriteString(fmt.Sprintf("- `%s:%d`", s.File, s.Line))
			if s.Function != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", s.Function))
			}
			sb.WriteString("\n")
		}
	}

	// 按规则分类显示命中详情
	sb.WriteString("\n### 命中规则详情\n\n")
	for _, r := range summary.Results {
		if len(r.AlertVars) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("**%s** (%s): ", r.RuleName, r.RuleType))
		total := 0
		for _, av := range r.AlertVars {
			total += av.Count
		}
		sb.WriteString(fmt.Sprintf("%d 处命中\n", total))
	}

	return sb.String()
}
