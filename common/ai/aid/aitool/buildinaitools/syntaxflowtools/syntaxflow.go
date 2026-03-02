package syntaxflowtools

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfverify"
)

const SyntaxFlowToolName_SyntaxCheck = "check-syntaxflow-syntax"

// checkSyntaxFlowRule performs SyntaxFlow rule syntax validation via sfvm.Compile
// Returns: errorMessages string, hasErrors bool
func checkSyntaxFlowRule(content string) (string, bool) {
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	_, err := vm.Compile(content)
	if err == nil {
		return "", false
	}

	me := memedit.NewMemEditor(content)
	var buf bytes.Buffer

	errs := vm.GetCompileErrors()
	if len(errs) == 0 {
		buf.WriteString(fmt.Sprintf("SyntaxFlow 编译错误: %s\n", err.Error()))
		buf.WriteString("------------------------")
		return buf.String(), true
	}

	sort.Slice(errs, func(i, j int) bool {
		si, sj := errs[i], errs[j]
		if si == nil || sj == nil {
			return false
		}
		lineI, lineJ := 0, 0
		if si.StartPos != nil {
			lineI = si.StartPos.GetLine()
		}
		if sj.StartPos != nil {
			lineJ = sj.StartPos.GetLine()
		}
		if lineI != lineJ {
			return lineI < lineJ
		}
		colI, colJ := 0, 0
		if si.StartPos != nil {
			colI = si.StartPos.GetColumn()
		}
		if sj.StartPos != nil {
			colJ = sj.StartPos.GetColumn()
		}
		return colI < colJ
	})

	// 识别 heredoc 结束符错误，输出明确错误类型（避免空洞的 mismatched input ':' expecting <EOF>）
	errTextPreview := ""
	if len(errs) > 0 && errs[0] != nil {
		errTextPreview = errs[0].Error()
	}
	if strings.Contains(content, "<<<") && strings.Contains(errTextPreview, "mismatched input ':'") && strings.Contains(errTextPreview, "expecting <EOF>") {
		buf.WriteString("【错误类型】heredoc 结束符格式错误\n")
		buf.WriteString("【原因】heredoc（如 <<<TEXT ... TEXT）的结束标识符有前导空格，未被识别，导致解析异常。\n")
		buf.WriteString("【修复】结束标识符必须单独占一行且行首无空格。错误：`    TEXT`。正确：换行后紧跟 `TEXT`。\n")
		buf.WriteString("------------------------\n")
	}

	maxShow := 3
	if len(errs) < maxShow {
		maxShow = len(errs)
	}
	for i := 0; i < maxShow; i++ {
		e := errs[i]
		if e == nil {
			continue
		}
		buf.WriteString(e.Error() + "\n")
		if e.StartPos != nil && e.EndPos != nil {
			startLine := e.StartPos.GetLine()
			startCol := e.StartPos.GetColumn()
			endLine := e.EndPos.GetLine()
			endCol := e.EndPos.GetColumn()
			if startLine >= 0 && endLine >= 0 {
				markedErr := me.GetTextContextWithPrompt(
					memedit.NewRange(
						memedit.NewPosition(startLine, startCol),
						memedit.NewPosition(endLine, endCol),
					),
					3, e.Error(),
				)
				if markedErr != "" {
					buf.WriteString(markedErr)
				}
			}
		}
		buf.WriteString("------------------------\n")
	}

	if len(errs) > maxShow {
		buf.WriteString("------------------------\n")
		buf.WriteString(fmt.Sprintf("还有 %d 个错误，建议先修复以上关键问题\n", len(errs)-maxShow))
	}

	// 当错误疑似特定格式问题时，附加可操作建议
	errText := buf.String()
	if strings.Contains(content, "desc(") && (strings.Contains(errText, "missing ')'") || strings.Contains(errText, "mismatched input ','")) {
		buf.WriteString("------------------------\n")
		buf.WriteString("【desc 格式提示】若错误位于 desc 块内：字段必须为 fieldName: value（冒号不可省略），字段间用换行分隔、禁止用逗号。参考 golang-template-ssti.sf 的 desc 写法。\n")
	}
	// heredoc 结束符错误：mismatched input ':' expecting <EOF> 常因 heredoc 未正确闭合导致
	if strings.Contains(content, "<<<") && strings.Contains(errText, "mismatched input ':'") && strings.Contains(errText, "expecting <EOF>") {
		buf.WriteString("------------------------\n")
		buf.WriteString("【heredoc 结束符错误】heredoc（如 desc: <<<TEXT ... TEXT）的结束标识符必须**单独占一行且行首无空格**。错误：`    TEXT`（有前导空格，不会被识别）。正确：换行后紧跟 `TEXT` 或 `DESC`，无任何前导空格或制表符。参考 golang-reflected-xss-gin-context.sf。\n")
	}

	return strings.TrimSpace(buf.String()), true
}

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

			// Step 1: 语法检查
			errMsg, hasErrors := checkSyntaxFlowRule(codeContent)
			if hasErrors {
				return map[string]any{
					"passed":       false,
					"errors":       errMsg,
					"message":      "【语法错误】SyntaxFlow 规则存在语法错误，请根据下方 errors 逐行修复后再验证。禁止在语法未通过时进行正例自检。",
					"syntax_error": true,
				}, nil
			}

			out := map[string]any{
				"passed":  true,
				"message": "【语法检查通过】SyntaxFlow 规则语法正确。",
			}

			// Step 2: 若有正例（用户提供的漏洞样例，对应 file://、UNSAFE）则进行正例自检
			if !hasSample {
				return out, nil
			}

			res := sfverify.VerifySFRuleMatchesSample(codeContent, sampleCode, filename, language)
			out["sample_verified"] = true
			out["matched"] = res.Matched

			if res.Matched {
				out["message"] = "【语法检查通过】【正例自检通过】规则已正确匹配正例（file://、UNSAFE）中的漏洞，可进行 directly_answer。"
				if res.AlertCount > 0 {
					out["alert_count"] = res.AlertCount
					out["alert_details"] = res.AlertDetails
				}
				if res.QueryResultsFull != "" {
					out["query_results_full"] = res.QueryResultsFull
				}
				return out, nil
			}

			// 正例自检未通过
			if res.Error != "" {
				out["message"] = fmt.Sprintf("【语法检查通过】【正例自检失败】%s", res.Message)
				out["error"] = res.Error
			} else {
				out["message"] = fmt.Sprintf("【语法检查通过】【正例自检未通过】%s", res.Message)
			}
			if res.Suggestion != "" {
				out["suggestion"] = res.Suggestion
			}
			if len(res.ResultVarsDiagnostic) > 0 {
				out["result_vars_diagnostic"] = res.ResultVarsDiagnostic
				if res.Suggestion == "" {
					out["suggestion"] = "根据 result_vars_diagnostic 中各变量匹配数量（0 表示数据流未贯通）修改规则，修改后再次调用本工具验证。"
				}
			}
			if res.DiagnosticHint != "" {
				out["diagnostic_hint"] = res.DiagnosticHint
			}
			return out, nil
		}),
	)
	if err != nil {
		log.Errorf("register check-syntaxflow-syntax tool: %v", err)
	}

	return factory.Tools(), nil
}
