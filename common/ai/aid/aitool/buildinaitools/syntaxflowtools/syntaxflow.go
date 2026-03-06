package syntaxflowtools

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

const SyntaxFlowToolName_SyntaxCheck = "check-syntaxflow-syntax"

// CreateSyntaxFlowTools returns built-in SyntaxFlow tools
func CreateSyntaxFlowTools() ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()
	err := factory.RegisterTool(SyntaxFlowToolName_SyntaxCheck,
		aitool.WithDescription("SyntaxFlow 规则语法检查与正例自检（合并）。1) 语法检查：验证 .sf 规则是否符合 SyntaxFlow 语法。2) 正例自检（可选）：当提供 sample_code+language 时，将用户提供的漏洞样例作为正例（file://、UNSAFE）执行规则，若产生告警则 matched=true。有漏洞样例时必须传入 path、sample_code、filename、language 完成正例自检；无样例时仅传 path 或 syntaxflow-code 做语法检查。"),
		aitool.WithKeywords([]string{
			"include 必须 as $gin", "正确 include as $gin",
			"<include('golang-gin-context')> as $gin", "include 漏写 as",
			"$gin 未定义", "$source 没有查到", "include 未匹配",
		}),
		aitool.WithStringParam("syntaxflow-code", aitool.WithParam_Description("SyntaxFlow 规则内容字符串，与 path 二选一")),
		aitool.WithStringParam("path", aitool.WithParam_Description(".sf 规则文件路径，与 syntaxflow-code 二选一。有样例自检时推荐传 path。")),
		aitool.WithStringParam("sample_code", aitool.WithParam_Description("【有正例时必传】正例（file://、UNSAFE）漏洞样例完整源代码，即用户提供的漏洞代码，用于自检规则能否正确匹配")),
		aitool.WithStringParam("filename", aitool.WithParam_Description("【有正例时推荐】正例虚拟文件名，如 vuln.go、Main.java，对应规则中 file:// 的文件名。为空则按 language 推断")),
		aitool.WithStringParam("language", aitool.WithParam_Description("【有正例时必传】语言：golang、java、php、c、javascript、yak、python")),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			codeContent := params.GetString("syntaxflow-code")
			if codeContent == "" {
				path := params.GetString("path")
				if path == "" {
					return nil, utils.Error("【参数错误】syntaxflow-code 或 path 必须提供其一")
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return nil, utils.Errorf("【读取失败】无法读取规则文件 %s: %s", path, err)
				}
				codeContent = string(content)
			}

			sampleCode := strings.TrimSpace(params.GetString("sample_code"))
			filename := params.GetString("filename")
			language := params.GetString("language")
			hasSample := sampleCode != "" && language != ""

			// 使用 SyntaxFlowRuleCheckingWithSample：语法检查 + 正例自检（若有样例），统一入口无重复编译
			res := static_analyzer.SyntaxFlowRuleCheckingWithSample(codeContent, sampleCode, filename, language)

			// 语法错误：直接使用 FormattedErrors 富格式输出
			if len(res.SyntaxErrors) > 0 {
				return map[string]any{
					"passed":       false,
					"errors":       res.FormattedErrors,
					"message":      "【语法错误】SyntaxFlow 规则存在语法错误，请根据下方 errors 逐行修复后再验证。禁止在语法未通过时进行正例自检。",
					"syntax_error": true,
				}, nil
			}

			out := map[string]any{
				"passed":  true,
				"message": "【语法检查通过】SyntaxFlow 规则语法正确。",
			}

			if !hasSample || res.Sample == nil {
				return out, nil
			}

			sample := res.Sample
			out["sample_verified"] = true
			out["matched"] = sample.Matched

			if sample.Matched {
				out["message"] = "【语法检查通过】【正例自检通过】规则已正确匹配正例（file://、UNSAFE）中的漏洞，可进行 directly_answer。"
				if sample.AlertCount > 0 {
					out["alert_count"] = sample.AlertCount
					out["alert_details"] = sample.AlertDetails
				}
				if sample.QueryResultsFull != "" {
					out["query_results_full"] = sample.QueryResultsFull
				}
				return out, nil
			}

			if sample.Error != "" {
				out["message"] = fmt.Sprintf("【语法检查通过】【正例自检失败】%s", sample.Message)
				out["error"] = sample.Error
			} else {
				out["message"] = fmt.Sprintf("【语法检查通过】【正例自检未通过】%s", sample.Message)
			}
			if sample.Suggestion != "" {
				out["suggestion"] = sample.Suggestion
			}
			if len(sample.ResultVarsDiagnostic) > 0 {
				out["result_vars_diagnostic"] = sample.ResultVarsDiagnostic
				if sample.Suggestion == "" {
					out["suggestion"] = "根据 result_vars_diagnostic 中各变量匹配数量（0 表示数据流未贯通）修改规则，修改后再次调用本工具验证。"
				}
			}
			if sample.DiagnosticHint != "" {
				out["diagnostic_hint"] = sample.DiagnosticHint
			}
			return out, nil
		}),
	)
	if err != nil {
		log.Errorf("register check-syntaxflow-syntax tool: %v", err)
	}

	return factory.Tools(), nil
}
