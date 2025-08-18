package sfcompletion

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiforge"
	_ "github.com/yaklang/yaklang/common/aiforge/aibp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestCase 表示一个测试用例
type TestCase struct {
	Filename    string
	Content     string
	Description string
}

// CompleteTestCases 用于给SF规则补全测试用例（仅补全反向测试用例）
func CompleteTestCases(
	fileName, ruleContent string,
	aiConfig ...aispec.AIConfigOption,
) (string, error) {
	// 编译规则以获取正反测试用例信息
	frame, err := sfvm.CompileRule(ruleContent)
	if err != nil {
		return "", utils.Errorf("failed to compile rule: %v", err)
	}

	// 检查规则中已有的测试用例类型
	hasPositive, hasNegative := analyzeTestCases(frame)

	// 如果没有正向测试用例，跳过补全（因为正向测试需要手动指定alert_num）
	if !hasPositive {
		log.Infof("规则 %s 缺少正向测试用例，跳过补全（正向测试需要手动指定alert_num）", fileName)
		return ruleContent, nil
	}

	// 如果反向测试用例已存在，跳过补全
	if hasNegative {
		log.Infof("规则 %s 已包含反向测试用例，跳过补全", fileName)
		return ruleContent, nil
	}

	aiCallback := func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := config.NewAIResponse()
		go func() {
			defer rsp.Close()
			aiConfig = append(aiConfig, aispec.WithStreamHandler(func(c io.Reader) {
				rsp.EmitOutputStream(c)
			}))
			_, err := ai.Chat(req.GetPrompt(), aiConfig...)
			if err != nil {
				log.Errorf("chat error: %v", err)
			}
		}()
		return rsp, nil
	}

	forgeResult, err := aiforge.ExecuteForge(
		"sf_test_cases_completion",
		context.Background(),
		[]*ypb.ExecParamItem{
			{
				Key: "file_name", Value: fileName,
			},
			{
				Key: "file_content", Value: ruleContent,
			},
		},
		aid.WithAgreeYOLO(true),
		aid.WithAICallback(aiCallback),
	)
	if err != nil {
		return "", utils.Errorf("complete test cases failed: %v", err)
	}

	params := forgeResult.GetInvokeParams("params")
	if params == nil {
		return "", utils.Errorf("complete test cases failed: ai response have no params")
	}

	// 解析AI返回的测试用例 - 只使用反向测试用例
	negativeTestCases := parseTestCases(params.GetObjectArray("negative_test_cases"))
	// 只保留最多两个安全用例
	if len(negativeTestCases) > 2 {
		negativeTestCases = negativeTestCases[:2]
	}
	testCaseSummary := params.GetString("test_case_summary")

	// 如果没有需要添加的反向测试用例，直接返回原内容
	if len(negativeTestCases) == 0 {
		log.Infof("规则 %s 无需添加反向测试用例", fileName)
		return ruleContent, nil
	}

	// 使用RuleFormat来处理测试用例补全
	content, err := formatRuleWithTestCases(ruleContent, negativeTestCases)
	if err != nil {
		return "", utils.Errorf("format rule failed: %v", err)
	}

	// 基础语法验证
	if err := validateRuleContent(content); err != nil {
		log.Warnf("AI生成的规则内容无法编译: %v，保持原内容不变", err)
		return ruleContent, nil
	}

	// 严格模式验证：确保测试用例真正有效
	if err := validateRuleWithStrictMode(content, fileName); err != nil {
		log.Warnf("AI生成的规则无法通过严格模式验证: %v，保持原内容不变", err)
		return ruleContent, nil
	}

	log.Infof("为规则 %s 成功添加了反向测试用例: %d 个. %s",
		fileName, len(negativeTestCases), testCaseSummary)

	return content, nil
}

// analyzeTestCases 通过编译后的frame分析规则中的正反测试用例
func analyzeTestCases(frame *sfvm.SFFrame) (hasPositive, hasNegative bool) {
	if frame == nil {
		return false, false
	}

	// 检查正向测试用例
	positiveTests, err := frame.ExtractVerifyFilesystemAndLanguage()
	if err == nil && len(positiveTests) > 0 {
		hasPositive = true
	}

	// 检查反向测试用例
	negativeTests, err := frame.ExtractNegativeFilesystemAndLanguage()
	if err == nil && len(negativeTests) > 0 {
		hasNegative = true
	}

	return hasPositive, hasNegative
}

// formatRuleWithTestCases 使用RuleFormat来补全测试用例
func formatRuleWithTestCases(ruleContent string, negativeTestCases []TestCase) (string, error) {
	// 创建测试用例处理器
	testCaseHandler := func(key, value string) string {
		// 如果是反向测试用例
		if strings.HasPrefix(key, "safefile://") {
			filename := strings.TrimPrefix(key, "safefile://")
			for _, testCase := range negativeTestCases {
				if testCase.Filename == filename {
					return testCase.Content
				}
			}
		}
		return value
	}

	// 构建需要补全的desc键类型
	var requiredDescKeys []sfvm.SFDescKeyType
	for _, testCase := range negativeTestCases {
		requiredDescKeys = append(requiredDescKeys, sfvm.SFDescKeyType("safefile://"+testCase.Filename))
	}

	// 使用格式化选项补全规则
	var opts []sfvm.RuleFormatOption
	opts = append(opts,
		sfvm.RuleFormatWithRequireInfoDescKeyType(requiredDescKeys...),
		sfvm.RuleFormatWithDescHandler(testCaseHandler),
	)

	content, err := sfvm.FormatRule(ruleContent, opts...)
	if err != nil {
		return "", utils.Errorf("format rule failed: %v", err)
	}

	return content, nil
}

// parseTestCases 解析AI返回的测试用例数组
func parseTestCases(testCasesArray []aitool.InvokeParams) []TestCase {
	var testCases []TestCase
	for _, testCaseData := range testCasesArray {
		testCase := TestCase{
			Filename:    testCaseData.GetString("filename"),
			Content:     testCaseData.GetString("content"),
			Description: testCaseData.GetString("description"),
		}
		if testCase.Filename != "" && testCase.Content != "" {
			testCases = append(testCases, testCase)
		}
	}
	return testCases
}

// validateRuleContent 验证规则内容是否能够正确编译
func validateRuleContent(ruleContent string) error {
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	_, err := vm.Compile(ruleContent)
	return err
}

// validateRuleWithStrictMode 使用严格模式验证规则的测试用例是否有效
func validateRuleWithStrictMode(ruleContent, ruleName string) error {
	// 编译规则获取验证信息
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(ruleContent)
	if err != nil {
		return utils.Errorf("compile rule failed: %v", err)
	}

	// 如果没有验证文件系统信息，跳过验证
	if len(frame.VerifyFsInfo) == 0 {
		log.Infof("规则 %s 没有测试用例，跳过严格模式验证", ruleName)
		return nil
	}

	// 创建临时规则对象进行验证
	tempRule := &schema.SyntaxFlowRule{
		RuleName: ruleName,
		Content:  ruleContent,
	}

	// 使用ssatest进行严格模式验证
	err = ssatest.EvaluateVerifyFilesystemWithRule(tempRule, nil, true)
	if err != nil {
		return utils.Errorf("strict mode validation failed: %v", err)
	}

	log.Infof("规则 %s 通过严格模式验证", ruleName)
	return nil
}
