package sfverify

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func init() {
	ssaapi.RegisterExport("VerifySFRuleMatchesSample", func(ruleContent, sampleCode, filename, language string) map[string]any {
		res := VerifySFRuleMatchesSample(ruleContent, sampleCode, filename, language)
		out := map[string]any{
			"matched": res.Matched,
			"message": res.Message,
		}
		if res.Error != "" {
			out["error"] = res.Error
		}
		if res.AlertCount > 0 {
			out["alert_count"] = res.AlertCount
			out["alert_details"] = res.AlertDetails
		}
		if res.Suggestion != "" {
			out["suggestion"] = res.Suggestion
		}
		return out
	})
}

// VerifySFRuleMatchesSampleResult 规则与样例匹配验证结果
type VerifySFRuleMatchesSampleResult struct {
	Matched      bool         `json:"matched"`        // 规则是否在样例上触发告警（即正确匹配漏洞）
	Message      string       `json:"message"`        // 人类可读的验证结果描述
	Error        string       `json:"error,omitempty"` // 错误类型或详细信息
	AlertCount   int          `json:"alert_count,omitempty"`   // 触发告警的数量（matched 时有效）
	AlertDetails map[string]int `json:"alert_details,omitempty"` // 各 alert 变量及其匹配数量，便于调试
	Suggestion   string       `json:"suggestion,omitempty"`   // 未匹配时的修复建议
}

// VerifySFRuleMatchesSample 静态检测：SyntaxFlow 规则是否能正确匹配用户提供的漏洞样例。
// 将 sample 作为虚拟项目解析，对该项目执行规则扫描，若规则产生告警则判定为匹配。
// 参数: ruleContent 完整 .sf 规则文本；sampleCode 漏洞样例代码；filename 样例文件名（如 vuln.go）；language 语言（如 golang、java、php、c）。
func VerifySFRuleMatchesSample(ruleContent, sampleCode, filename, language string) VerifySFRuleMatchesSampleResult {
	if ruleContent == "" || sampleCode == "" {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "缺少必要参数：rule_content、sample_code 均不能为空",
			Error:   "invalid_args",
		}
	}
	lang, err := ssaconfig.ValidateLanguage(language)
	if err != nil {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "不支持的语言: " + language + "。支持: golang, java, php, c, javascript, yak, python",
			Error:   err.Error(),
		}
	}
	// 若 filename 为空，根据语言推断默认文件名
	if filename == "" {
		ext := lang.GetFileExt()
		if ext != "" {
			filename = "sample" + ext
		} else {
			filename = "sample"
		}
	}
	// 若 filename 无扩展名或扩展名与语言不匹配，尝试补充
	if !strings.Contains(filename, ".") && lang.GetFileExt() != "" {
		filename = strings.TrimSuffix(filename, "/") + lang.GetFileExt()
	}
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, sampleCode)
	progs, err := ssaapi.ParseProjectWithFS(vfs, ssaapi.WithLanguage(lang))
	if err != nil {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "样例代码解析失败: " + err.Error(),
			Error:   err.Error(),
			Suggestion: "请确认 sample_code 为有效的 " + string(lang) + " 代码，且 filename 扩展名正确（如 .go/.java/.php/.c）",
		}
	}
	if len(progs) == 0 {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "样例代码未能生成有效程序",
			Error:   "empty_program",
			Suggestion: "请检查 sample_code 是否包含可解析的入口（如 package main、class 等）",
		}
	}
	opts := []ssaapi.QueryOption{ssaapi.QueryWithInitInputVar(progs[0])}
	result, err := progs.SyntaxFlowWithError(ruleContent, opts...)
	if err != nil {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "规则执行失败: " + err.Error(),
			Error:   err.Error(),
			Suggestion: "请先调用 check-syntaxflow-syntax 检查规则语法，并确认规则中的 include/lib 引用是否可用",
		}
	}
	if len(result.GetErrors()) > 0 {
		return VerifySFRuleMatchesSampleResult{
			Matched: false,
			Message: "规则执行有错误: " + strings.Join(result.GetErrors(), "; "),
			Error:   strings.Join(result.GetErrors(), "; "),
			Suggestion: "检查规则逻辑是否与样例中的 API/调用方式一致，如 source/sink 方法名、数据流路径等",
		}
	}
	alertVars := result.GetAlertVariables()
	alertDetails := make(map[string]int)
	totalAlert := 0
	for _, name := range alertVars {
		vals := result.GetValues(name)
		n := len(vals)
		if n > 0 {
			alertDetails[name] = n
			totalAlert += n
		}
	}
	if totalAlert > 0 {
		return VerifySFRuleMatchesSampleResult{
			Matched:      true,
			Message:      fmt.Sprintf("规则已正确匹配样例中的漏洞，触发 %d 处告警", totalAlert),
			AlertCount:   totalAlert,
			AlertDetails: alertDetails,
		}
	}
	return VerifySFRuleMatchesSampleResult{
		Matched: false,
		Message: "规则未在样例上触发告警，可能未正确匹配漏洞",
		Suggestion: "请检查：1) 规则的 source/sink 模式是否覆盖样例中的调用链；2) 数据流 #-> 是否与样例实际路径一致；3) 条件过滤 ?{} 是否过于严格",
	}
}
