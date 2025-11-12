package sfweb

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var (
	falsePositiveIssueTemplate *template.Template
	falseNegativeIssueTemplate *template.Template
)

func init() {
	var err error
	falsePositiveIssueTemplate, err = template.New("false_positive_body").Funcs(template.FuncMap{
		"CodeBlock": CodeBlock,
		"Details":   Details,
		"Escape":    func(s schema.SyntaxFlowSeverity) string { return html.EscapeString(string(s)) },
	}).Parse(`## 规则
### 规则名称
{{Details .Rule.Title .Rule.TitleZh }}

## 误报风险
### 风险标题
{{Details .Risk.Title .Risk.TitleVerbose }}

### 风险类型
{{Details .Risk.RiskType .Risk.RiskType }}

### 风险等级
<p>{{.Risk.Severity | Escape }}</p>

## 文件内容
{{CodeBlock .Lang .Content}}

## 额外描述
`)
	if err != nil {
		panic(err)
	}
	falseNegativeIssueTemplate, err = template.New("false_negative_body").Funcs(template.FuncMap{
		"CodeBlock": CodeBlock,
		"Details":   Details,
		"Escape":    html.EscapeString,
	}).Parse(`### 预期存在的规则名称
<p>{{.RuleName | Escape }}</p>

## 文件内容
{{CodeBlock .Lang .Content}}

## 额外描述
`)
	if err != nil {
		panic(err)
	}
}

type ReportFalsePositiveTemplateData struct {
	Rule    *schema.SyntaxFlowRule
	Risk    *schema.SSARisk
	Content string
	Lang    string
}

type ReportFalseNegativeTemplateData struct {
	RuleName string
	Content  string
	Lang     string
}

type ReportFalsePositiveRequest struct {
	// 扫描文件内容
	Content string `json:"content,omitempty"`
	// 语言
	Lang string `json:"lang,omitempty"`
	// 风险hash
	RiskHash string `json:"risk_hash,omitempty"`
}

type ReportFalseNegativeRequest struct {
	// 扫描文件内容
	Content string `json:"content,omitempty"`
	// 语言
	Lang string `json:"lang,omitempty"`
	// 规则名
	RuleName string `json:"rule_name,omitempty"`
}

type ReportMissingParameterError struct {
	param string
}

func (e *ReportMissingParameterError) Error() string {
	return "missing parameter: " + e.param
}

func NewReportMissingParameterError(param string) *ReportMissingParameterError {
	return &ReportMissingParameterError{param: param}
}

type ReportResponse struct {
	Link string `json:"link"`
	Body string `json:"body,omitempty"`
}

func CodeBlock(lang string, content string) string {
	return fmt.Sprintf("<details>\n<summary>click</summary>\n\n~~~%s\n%s\n~~~\n\n</details>", lang, content)
}

func Details(summary, details string) string {
	if details == "" {
		return fmt.Sprintf("<p>%s</p>", html.EscapeString(summary))
	}
	return fmt.Sprintf("<details>\n<summary>%s</summary>\n<p>%s</p>\n</details>", html.EscapeString(summary), html.EscapeString(details))
}

func (s *SyntaxFlowWebServer) registerReportRoute() {
	subRouter := s.router.Name("report").PathPrefix("/report").Subrouter()

	subRouter.HandleFunc("/false_positive", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErrorJson(w, utils.Wrap(err, "read body error"))
			return
		}
		var req ReportFalsePositiveRequest
		if err = json.Unmarshal(body, &req); err != nil {
			writeErrorJson(w, utils.Wrap(err, "unmarshal request error"))
			return
		}
		if req.Content == "" {
			writeErrorJson(w, NewReportMissingParameterError("content"))
			return
		} else if req.Lang == "" {
			writeErrorJson(w, NewReportMissingParameterError("lang"))
			return
		} else if req.RiskHash == "" {
			writeErrorJson(w, NewReportMissingParameterError("risk_hash"))
			return
		}
		lang, err := ssaconfig.ValidateLanguage(req.Lang)
		if err != nil {
			writeErrorJson(w, NewInvalidLangError(req.Lang))
			return
		}

		risk, err := yakit.GetSSARiskByHash(consts.GetGormSSAProjectDataBase(), req.RiskHash)
		if err != nil {
			writeErrorJson(w, utils.Wrap(err, "get risk error"))
			return
		}
		ruleName := risk.FromRule
		rule, err := sfdb.GetRulePure(ruleName)
		if err != nil {
			writeErrorJson(w, utils.Wrap(err, "get rule error"))
			return
		}
		title := fmt.Sprintf("规则 %s 存在误报", rule.Title)

		var issueBodyBuilder strings.Builder
		err = falsePositiveIssueTemplate.Execute(&issueBodyBuilder, ReportFalsePositiveTemplateData{
			Content: req.Content,
			Lang:    string(lang),
			Rule:    rule,
			Risk:    risk,
		})
		if err != nil {
			writeErrorJson(w, utils.Wrap(err, "execute template error"))
			return
		}
		issueBody := strings.TrimSpace(issueBodyBuilder.String())
		writeJson(w, &ReportResponse{
			Link: fmt.Sprintf("https://github.com/yaklang/ssa.to/issues/new?labels=bug&title=%s", url.QueryEscape(title)),
			Body: issueBody,
		})
	}).Name("false positive report").Methods(http.MethodPost, http.MethodOptions)

	subRouter.HandleFunc("/false_negative", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErrorJson(w, utils.Wrap(err, "read body error"))
			return
		}
		var req ReportFalseNegativeRequest
		if err = json.Unmarshal(body, &req); err != nil {
			writeErrorJson(w, utils.Wrap(err, "unmarshal request error"))
			return
		}
		if req.Content == "" {
			writeErrorJson(w, NewReportMissingParameterError("content"))
			return
		} else if req.Lang == "" {
			writeErrorJson(w, NewReportMissingParameterError("lang"))
			return
		} else if req.RuleName == "" {
			writeErrorJson(w, NewReportMissingParameterError("rule_name"))
			return
		}

		lang, err := ssaconfig.ValidateLanguage(req.Lang)
		if err != nil {
			writeErrorJson(w, NewInvalidLangError(req.Lang))
			return
		}

		title := fmt.Sprintf("规则 %s 存在漏报", req.RuleName)

		var issueBodyBuilder strings.Builder
		err = falseNegativeIssueTemplate.Execute(&issueBodyBuilder, ReportFalseNegativeTemplateData{
			RuleName: req.RuleName,
			Content:  req.Content,
			Lang:     string(lang),
		})
		if err != nil {
			writeErrorJson(w, utils.Wrap(err, "execute template error"))
			return
		}
		issueBody := strings.TrimSpace(issueBodyBuilder.String())
		writeJson(w, &ReportResponse{
			Link: fmt.Sprintf("https://github.com/yaklang/ssa.to/issues/new?labels=bug&title=%s", url.QueryEscape(title)),
			Body: issueBody,
		})
	}).Name("false negative report").Methods(http.MethodPost, http.MethodOptions)
}
