package loop_scan_risk_analysis

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow"
)

var projectNameLineFmt = regexp.MustCompile(`(?is)project_name\s*=\s*([^\r\n]+)`)
var projectNameSlugFmt = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._\-/\\]{0,255}`)

// parseStrictProjectNameLine 仅接受显式行 project_name=<slug>（首条用户输入必须用此格式，禁止仅靠自然语言或 JSON 绕过追问）。
// parseOptionalPlainProjectSlug 当整段用户输入仅为单行 slug（例如 go-sec-code）时，视为隐式 project_name，
// 避免必须手写 project_name= 前缀；含空格、多行或与 slug 清洗结果不一致时不采用。
func parseOptionalPlainProjectSlug(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || strings.ContainsAny(s, "\r\n") {
		return ""
	}
	if parseStrictProjectNameLine(s) != "" {
		return ""
	}
	tok := sanitizeProjectNameToken(s)
	if tok == "" || tok != s {
		return ""
	}
	return tok
}

func parseStrictProjectNameLine(answer string) string {
	s := strings.TrimSpace(answer)
	if s == "" {
		return ""
	}
	m := projectNameLineFmt.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return sanitizeProjectNameToken(m[1])
}

// parseInteractiveProjectNameReply 用于交互回合之后：优先 project_name=，否则兼容前端以 JSON（如 suggestion）提交的回复。
func parseInteractiveProjectNameReply(answer string) string {
	if s := parseStrictProjectNameLine(answer); s != "" {
		return s
	}
	return parseProjectNameFromJSONPayload(strings.TrimSpace(answer))
}

func parseProjectNameFromJSONPayload(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	candidates := []string{s}
	if i := strings.IndexByte(s, '{'); i >= 0 {
		candidates = append(candidates, strings.TrimSpace(s[i:]))
	}
	for _, c := range candidates {
		v := parseProjectNameFromJSONObject(c)
		if v != "" {
			return v
		}
	}
	return ""
}

func parseProjectNameFromJSONObject(raw string) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return ""
	}
	for _, key := range []string{"project_name", "program_hint", "suggestion"} {
		if v := sanitizeProjectNameValue(obj, key); v != "" {
			return v
		}
	}
	if nested, ok := obj["result"].(map[string]any); ok {
		for _, key := range []string{"project_name", "program_hint", "suggestion"} {
			if v := sanitizeProjectNameValue(nested, key); v != "" {
				return v
			}
		}
	}
	return ""
}

func sanitizeProjectNameValue(values map[string]any, key string) string {
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return sanitizeProjectNameToken(s)
	}
	return sanitizeProjectNameToken(fmt.Sprint(raw))
}

func sanitizeProjectNameToken(token string) string {
	s := strings.TrimSpace(token)
	if s == "" {
		return ""
	}
	s = strings.Trim(s, `"'`+"`"+`«»[](){}<>【】`)
	s = strings.TrimFunc(s, func(r rune) bool {
		return !isProjectNameChar(r)
	})
	if s == "" {
		return ""
	}
	return strings.TrimSpace(projectNameSlugFmt.FindString(s))
}

func isProjectNameChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '.' || r == '_' || r == '-' || r == '/' || r == '\\'
}

// buildProjectNameClarificationPrompt 生成「强制固定格式」的追问文案（与前端人工介入 / REQUIRE_USER_INTERACTIVE 配套）。
func buildProjectNameClarificationPrompt(fromUserTextHint string) string {
	const base = `【扫描风险分析 · 必填】

请在下一次回复中**只写一行**固定格式（不要其它说明文字、不要换行多条）：

project_name=<程序或仓库标识>

说明：
- 该标识用于匹配数据库表 syntax_flow_scan_tasks 的 programs 字段（子串），以定位该项目的 SyntaxFlow 扫描任务。
- 提交后系统将**自动执行**与内置 forge「sf_project_scan_check」相同的项目扫描检查（Markdown 报告），再进入风险分析流水线。

示例：project_name=go-sec-code`
	if strings.TrimSpace(fromUserTextHint) == "" {
		return base
	}
	return base + "\n\n（参考：从您上一句中推测可能是 **" + fromUserTextHint + "**，若正确请按上式填写；若不对请直接填写正确标识。）"
}

// resolveScanIDByProjectName 通过项目名先执行项目检查（与内置 sf_project_scan_check 同源），再选择最新 task_id。
func resolveScanIDByProjectName(projectName string, scanOnly bool, limit int) (scanID string, report string, err error) {
	pname := strings.TrimSpace(projectName)
	if pname == "" {
		return "", "", fmt.Errorf("empty project_name")
	}
	checkRes, err := syntaxflow.RunSyntaxFlowProjectScanCheck(pname, scanOnly, normalizeProjectScanLimit(limit))
	if err != nil {
		return "", "", err
	}
	report = checkRes.ReportMarkdown
	scanID = strings.TrimSpace(checkRes.LatestTaskID)
	if strings.TrimSpace(scanID) == "" {
		return "", report, fmt.Errorf("未找到 programs 包含 %q 的 SyntaxFlow 扫描任务", pname)
	}
	return scanID, report, nil
}

func normalizeProjectScanLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 200 {
		return 200
	}
	return limit
}

// extractProjectNameForScanAnalysis pulls a project / repo slug from common Chinese prompts such as
// "分析 go-sec-code 的项目扫描结果". Returns empty if nothing reliable matched.
func extractProjectNameForScanAnalysis(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)分析\s+([A-Za-z0-9][A-Za-z0-9._\-/\\]{0,255})\s+最新`),
		regexp.MustCompile(`(?i)^\s*([A-Za-z0-9][A-Za-z0-9._\-/\\]{0,255})\s+分析\s*最新`),
		regexp.MustCompile(`(?i)^\s*([A-Za-z0-9][A-Za-z0-9._\-/\\]{0,255})\s+.{0,40}最新.{0,20}(?:扫描|结果)`),
		regexp.MustCompile(`(?i)分析\s+([^\s，,。.；;]{1,256})\s+的\s*项目`),
		regexp.MustCompile(`(?i)([A-Za-z0-9][A-Za-z0-9._\-]{0,255})\s+的\s*项目\s*(?:扫描|结果)`),
		regexp.MustCompile(`(?i)([A-Za-z0-9][A-Za-z0-9._\-]{0,255})\s*项目\s*扫描`),
		regexp.MustCompile(`(?i)(?:project|repo)\s+([A-Za-z0-9][A-Za-z0-9._\-]{0,255})\s+(?:scan|syntaxflow)`),
	}
	for _, re := range patterns {
		m := re.FindStringSubmatch(raw)
		if len(m) >= 2 {
			token := strings.TrimSpace(m[1])
			token = strings.Trim(token, `"'«»`)
			if token != "" && len(token) <= 256 {
				return token
			}
		}
	}
	return ""
}
