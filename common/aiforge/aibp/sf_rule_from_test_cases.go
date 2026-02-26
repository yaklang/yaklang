package aibp

import (
	_ "embed"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed sf_rule_from_test_cases_prompts/rule_generation_init.txt
var sfRuleFromTestCasesPrompt string

func init() {
	err := aiforge.RegisterLiteForge("sf_rule_from_test_cases",
		aiforge.WithLiteForge_Prompt(sfRuleFromTestCasesPrompt),
		aiforge.WithLiteForge_OutputSchema(
			aitool.WithStringParam("rule_content",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("完整的 .sf 规则内容，含 desc、规则体、file:///safefile:// 测试用例")),
			aitool.WithStringParam("title",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("规则英文标题")),
			aitool.WithStringParam("title_zh",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("规则中文标题")),
			aitool.WithNumberParam("cwe",
				aitool.WithParam_Description("CWE 编号（纯数字，如 89）"),
				aitool.WithParam_Min(1),
				aitool.WithParam_Max(2000)),
			aitool.WithStringParam("summary",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("规则生成说明：检测逻辑及适用场景")),
		))
	if err != nil {
		log.Errorf("register sf_rule_from_test_cases failed: %v", err)
		return
	}
}

// ValidateSFRuleResult 验证结果
type ValidateSFRuleResult struct {
	Passed bool     `json:"passed"`
	Errors []string `json:"errors,omitempty"`
}

// ValidateSFRule 验证 SyntaxFlow 规则：编译 + 可选严格模式（测试用例）验证。
// 输入 ruleContent 为完整 .sf 规则文本，isStrict 为 true 时会执行 EvaluateVerifyFilesystemWithRule。
func ValidateSFRule(ruleContent string, isStrict bool) ValidateSFRuleResult {
	var errs []string

	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(ruleContent)
	if err != nil {
		return ValidateSFRuleResult{
			Passed: false,
			Errors: []string{fmt.Sprintf("compile failed: %v", err)},
		}
	}

	if !isStrict {
		return ValidateSFRuleResult{Passed: true}
	}

	if len(frame.VerifyFsInfo) == 0 {
		return ValidateSFRuleResult{Passed: true}
	}

	rule := &schema.SyntaxFlowRule{
		RuleName: "sf_rule_from_test_cases_validate",
		Content:  ruleContent,
	}
	err = ssatest.EvaluateVerifyFilesystemWithRule(rule, nil, true)
	if err != nil {
		errs = append(errs, fmt.Sprintf("%v", err))
		return ValidateSFRuleResult{
			Passed: false,
			Errors: errs,
		}
	}

	return ValidateSFRuleResult{Passed: true}
}
