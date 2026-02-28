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

// VerifySFRuleAgainstSample 工具名：静态检测生成的规则是否能正确匹配用户提供的漏洞样例
const SyntaxFlowToolName_VerifyAgainstSample = "verify-syntaxflow-rule-against-sample"

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

	return strings.TrimSpace(buf.String()), true
}

// CreateSyntaxFlowTools returns built-in SyntaxFlow tools
func CreateSyntaxFlowTools() ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()
	err := factory.RegisterTool(SyntaxFlowToolName_SyntaxCheck,
		aitool.WithDescription("Performs a syntax check on SyntaxFlow rule. The rule can be provided directly as a string ('syntaxflow-code') or by specifying a .sf file path ('path'). Use this for .sf files; do NOT use check-yaklang-syntax for SyntaxFlow rules."),
		aitool.WithStringParam("syntaxflow-code", aitool.WithParam_Description("SyntaxFlow rule content string to check")),
		aitool.WithStringParam("path", aitool.WithParam_Description("Local file path of .sf SyntaxFlow rule file to check")),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			codeContent := params.GetString("syntaxflow-code")
			if codeContent == "" {
				path := params.GetString("path")
				if path == "" {
					return nil, utils.Error("syntaxflow-code content or path is required")
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return nil, utils.Errorf("read file %s failed: %s", path, err)
				}
				codeContent = string(content)
			}
			errMsg, hasErrors := checkSyntaxFlowRule(codeContent)
			if !hasErrors {
				return map[string]any{
					"passed":  true,
					"message": "SyntaxFlow 规则语法检查通过",
				}, nil
			}
			return map[string]any{
				"passed":  false,
				"errors":  errMsg,
				"message": "SyntaxFlow 规则存在语法错误",
			}, nil
		}),
	)
	if err != nil {
		log.Errorf("register check-syntaxflow-syntax tool: %v", err)
	}

	err = factory.RegisterTool(SyntaxFlowToolName_VerifyAgainstSample,
		aitool.WithDescription("静态检测 SyntaxFlow 规则能否正确匹配漏洞样例。将 sample_code 作为虚拟项目解析并执行规则扫描；若产生告警则 matched=true，表示规则已覆盖样例中的漏洞。生成规则后必须调用此工具验证，matched=false 时需根据 suggestion 修改规则后再次验证。"),
		aitool.WithStringParam("syntaxflow-code", aitool.WithParam_Description("SyntaxFlow 规则完整内容，与 path 二选一")),
		aitool.WithStringParam("path", aitool.WithParam_Description("规则 .sf 文件路径，与 syntaxflow-code 二选一")),
		aitool.WithStringParam("sample_code", aitool.WithParam_Description("漏洞样例代码（需为完整可解析的源代码）"), aitool.WithParam_Required(true)),
		aitool.WithStringParam("filename", aitool.WithParam_Description("样例虚拟文件名，如 vuln.go、Main.java。可选，为空则按 language 自动推断")),
		aitool.WithStringParam("language", aitool.WithParam_Description("语言：golang/go、java、php、c、javascript、yak、python"), aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			ruleContent := params.GetString("syntaxflow-code")
			if ruleContent == "" {
				path := params.GetString("path")
				if path != "" {
					content, err := os.ReadFile(path)
					if err != nil {
						return nil, utils.Errorf("read rule file %s failed: %s", path, err)
					}
					ruleContent = string(content)
				}
			}
			if ruleContent == "" {
				return nil, utils.Error("syntaxflow-code 或 path 必须提供其一")
			}
			sampleCode := params.GetString("sample_code")
			filename := params.GetString("filename")
			language := params.GetString("language")
			res := sfverify.VerifySFRuleMatchesSample(ruleContent, sampleCode, filename, language)
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
			return out, nil
		}),
	)
	if err != nil {
		log.Errorf("register verify-syntaxflow-rule-against-sample tool: %v", err)
	}

	return factory.Tools(), nil
}
