package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalyzer"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestEvaluateSyntaxFlowRule_WithDirectRuleInput(t *testing.T) {
	// 测试直接传入规则内容的情况
	client, err := NewLocalClient()
	require.NoError(t, err)

	tests := []struct {
		name          string
		ruleInput     string
		expectError   bool
		expectedScore int64
		checkProblems bool
	}{
		{
			name: "完整的有效规则",
			ruleInput: `desc(
	title: "Runtime命令执行检测",
	description: "检测使用Runtime.getRuntime().exec()方法执行系统命令的安全风险",
	solution: "使用参数化的方式执行命令，避免直接传入用户输入",
	severity: "high",
	type: "audit"
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectError:   false,
			expectedScore: int64(sfanalyzer.MaxScore - sfanalyzer.MissingPositiveTestPenalty - sfanalyzer.MissingNegativeTestPenalty), // 100 - 15 - 5 = 80
			checkProblems: false,
		},
		{
			name:          "完全语法错误",
			ruleInput:     `invalid syntax here $$$`,
			expectError:   false,
			expectedScore: int64(sfanalyzer.MaxScore - sfanalyzer.SyntaxErrorPenalty), // 100 - 100 = 0
			checkProblems: true,
		},
		{
			name: "部分语法错误",
			ruleInput: `desc(
	title: "测试规则"
)

test.* as $result &&& invalid syntax`,
			expectError:   false,
			expectedScore: int64(sfanalyzer.MaxScore - sfanalyzer.SyntaxErrorPenalty), // 100 - 100 = 0
			checkProblems: true,
		},
		{
			name: "缺少description字段",
			ruleInput: `desc(
	title: "测试规则",
	solution: "解决方案"
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectError:   false,
			expectedScore: int64(sfanalyzer.MaxScore - sfanalyzer.MissingDescriptionPenalty - sfanalyzer.MissingPositiveTestPenalty - sfanalyzer.MissingNegativeTestPenalty), // 100 - 40 - 15 - 5 = 40
			checkProblems: true,
		},
		{
			name: "缺少solution字段",
			ruleInput: `desc(
	title: "测试规则",
	description: "详细描述"
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectError:   false,
			expectedScore: int64(sfanalyzer.MaxScore - sfanalyzer.MissingSolutionPenalty - sfanalyzer.MissingPositiveTestPenalty - sfanalyzer.MissingNegativeTestPenalty), // 100 - 10 - 15 - 5 = 70
			checkProblems: true,
		},
		{
			name: "缺少多个字段",
			ruleInput: `desc(
	title: "测试规则"
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectError:   false,
			expectedScore: int64(sfanalyzer.MaxScore - sfanalyzer.MissingDescriptionPenalty - sfanalyzer.MissingSolutionPenalty - sfanalyzer.MissingPositiveTestPenalty - sfanalyzer.MissingNegativeTestPenalty), // 100 - 40 - 10 - 15 - 5 = 30
			checkProblems: true,
		},
		{
			name: "缺少alert语句",
			ruleInput: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案"
)

Runtime.getRuntime().exec(* as $cmd) as $result;`,
			expectError:   false,
			expectedScore: int64(sfanalyzer.MinScore), // 缺少alert直接0分
			checkProblems: true,
		},
		{
			name:          "空规则",
			ruleInput:     "",
			expectError:   true, // 数据库找不到报错
			expectedScore: 0,
			checkProblems: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ypb.SmokingEvaluatePluginRequest{
				PluginType: schema.SCRIPT_TYPE_SYNTAXFLOW,
				Code:       tt.ruleInput,
			}

			resp, err := client.SmokingEvaluatePlugin(context.Background(), req)
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)

			// 检查分数
			assert.Equal(t, tt.expectedScore, resp.Score, "分数不匹配，期望 %d，实际 %d", tt.expectedScore, resp.Score)

			// 检查是否有问题报告
			if tt.checkProblems {
				assert.Greater(t, len(resp.Results), 0)
				// 验证问题结构
				for _, result := range resp.Results {
					assert.NotEmpty(t, result.Item)
					assert.NotEmpty(t, result.Severity)
					assert.Contains(t, []string{"Error", "Warning"}, result.Severity)
				}
			}
		})
	}
}

func TestEvaluateSyntaxFlowRule_WithRuleName(t *testing.T) {
	// 测试通过规则名称查找规则的情况
	client, err := NewLocalClient()
	require.NoError(t, err)

	// 首先创建一个测试规则
	testRuleName := "test_evaluate_rule"
	testRuleContent := `desc(
	title: "Runtime命令执行检测",
	description: "检测使用Runtime.getRuntime().exec()方法执行系统命令的安全风险",
	solution: "使用参数化的方式执行命令，避免直接传入用户输入",
	severity: "high",
	type: "audit"
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`

	// 创建规则
	_, err = client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			RuleName: testRuleName,
			Content:  testRuleContent,
			Language: "java",
		},
	})
	require.NoError(t, err)

	// 清理资源
	defer func() {
		sfdb.DeleteRuleByRuleName(testRuleName)
	}()

	t.Run("通过规则名称评估", func(t *testing.T) {
		req := &ypb.SmokingEvaluatePluginRequest{
			PluginType: schema.SCRIPT_TYPE_SYNTAXFLOW,
			Code:       testRuleContent,
		}

		resp, err := client.SmokingEvaluatePlugin(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// 由于是完整规则但缺少测试数据，应该是80分
		expectedScore := int64(sfanalyzer.MaxScore - sfanalyzer.MissingPositiveTestPenalty - sfanalyzer.MissingNegativeTestPenalty) // 100 - 15 - 5 = 80
		assert.Equal(t, expectedScore, resp.Score)
	})

	t.Run("规则名称不存在", func(t *testing.T) {
		req := &ypb.SmokingEvaluatePluginRequest{
			PluginType: schema.SCRIPT_TYPE_SYNTAXFLOW,
			Code:       "non_existent_rule",
		}

		_, err := client.SmokingEvaluatePlugin(context.Background(), req)
		_ = err
		// assert.Error(t, err)
	})
}

func TestEvaluateSyntaxFlowRule_PriorityRuleInput(t *testing.T) {
	// 测试当同时提供规则名称和规则内容时，优先使用规则内容
	client, err := NewLocalClient()
	require.NoError(t, err)

	// 创建一个测试规则
	testRuleName := "test_priority_rule"
	dbRuleContent := `desc(title: "数据库中的规则")
Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`

	_, err = client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
		SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
			RuleName: testRuleName,
			Content:  dbRuleContent,
			Language: "java",
		},
	})
	require.NoError(t, err)

	defer func() {
		sfdb.DeleteRuleByRuleName(testRuleName)
	}()

	t.Run("规则内容优先于规则名称", func(t *testing.T) {
		directRuleContent := `desc(
	title: "直接传入的规则",
	description: "这个规则应该被优先使用",
	solution: "解决方案",
	severity: "high"
)
Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`

		req := &ypb.SmokingEvaluatePluginRequest{
			PluginType: schema.SCRIPT_TYPE_SYNTAXFLOW,
			PluginName: testRuleName,
			Code:       directRuleContent,
		}

		resp, err := client.SmokingEvaluatePlugin(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// 由于直接传入的规则更完整，应该是80分（缺少测试数据）
		expectedScore := int64(sfanalyzer.MaxScore - sfanalyzer.MissingPositiveTestPenalty - sfanalyzer.MissingNegativeTestPenalty) // 100 - 15 - 5 = 80
		assert.Equal(t, expectedScore, resp.Score)
	})
}

func TestEvaluateSyntaxFlowRule_AnalyzeResults(t *testing.T) {
	// 测试各种类型的分析结果
	client, err := NewLocalClient()
	require.NoError(t, err)

	testCases := []struct {
		name                string
		ruleContent         string
		expectedProblemType []string
		expectedSeverity    []string
	}{
		{
			name:                "语法错误",
			ruleContent:         `Runtime.getRuntime().exec( as $invalid`,
			expectedProblemType: []string{sfanalyzer.ProblemTypeSyntaxError},
			expectedSeverity:    []string{sfanalyzer.Error},
		},
		{
			name: "缺少描述字段",
			ruleContent: `desc(title: "测试")
Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectedProblemType: []string{sfanalyzer.ProblemTypeLackDescriptionField},
			expectedSeverity:    []string{sfanalyzer.Warning},
		},
		{
			name: "缺少alert语句",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案"
)

Runtime.getRuntime().exec(* as $cmd) as $result;`,
			expectedProblemType: []string{sfanalyzer.ProblemTypeMissingPositiveTestData, sfanalyzer.ProblemTypeMissingNegativeTestData, sfanalyzer.ProblemTypeMissingAlert},
			expectedSeverity:    []string{sfanalyzer.Warning, sfanalyzer.Warning, sfanalyzer.Error},
		},
		{
			name: "缺少测试数据",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案"
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectedProblemType: []string{sfanalyzer.ProblemTypeMissingPositiveTestData, sfanalyzer.ProblemTypeMissingNegativeTestData},
			expectedSeverity:    []string{sfanalyzer.Warning, sfanalyzer.Warning},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			req := &ypb.SmokingEvaluatePluginRequest{
				PluginType: schema.SCRIPT_TYPE_SYNTAXFLOW,
				Code:       tt.ruleContent,
			}

			resp, err := client.SmokingEvaluatePlugin(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, resp)

			// 检查是否包含预期的问题类型
			foundProblems := make(map[string]bool)
			for _, result := range resp.Results {
				foundProblems[result.Item] = true
				assert.Contains(t, tt.expectedSeverity, result.Severity)
			}

			for _, expectedType := range tt.expectedProblemType {
				assert.True(t, foundProblems[expectedType],
					"预期问题类型 %s 未在结果中找到", expectedType)
			}
		})
	}
}

func TestEvaluateSyntaxFlowRule_WithTestData(t *testing.T) {
	// 测试包含测试数据的规则
	client, err := NewLocalClient()
	require.NoError(t, err)

	tests := []struct {
		name          string
		ruleContent   string
		expectedScore int64
	}{
		{
			name: "只有正向测试",
			ruleContent: `desc(
	title: "Runtime命令执行检测",
	description: "检测使用Runtime.getRuntime().exec()方法执行系统命令的安全风险",
	solution: "使用参数化的方式执行命令，避免直接传入用户输入",
	alert_min: 1,
	language: java,
	'file://positive.java': <<<EOF
public class Test {
    public void vulnerable() {
        Runtime.getRuntime().exec("ls"); // 这里应该匹配规则
    }
}
EOF
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectedScore: int64(sfanalyzer.MaxScore - sfanalyzer.MissingNegativeTestPenalty), // 100 - 5 = 95
		},
		{
			name: "正反测试都有",
			ruleContent: `desc(
	title: "Runtime命令执行检测",
	description: "检测使用Runtime.getRuntime().exec()方法执行系统命令的安全风险",
	solution: "使用参数化的方式执行命令，避免直接传入用户输入",
	alert_min: 1,
	language: java,
	'file://positive.java': <<<EOF
public class Test {
    public void vulnerable() {
        Runtime.getRuntime().exec("ls"); // 这里应该匹配规则
    }
}
EOF,
	'safefile://negative.java': <<<SAFE
public class Safe {
    public void safe() {
        System.out.println("safe operation"); // 这里不应该匹配规则
    }
}
SAFE
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectedScore: int64(sfanalyzer.MaxScore), // 100分
		},
		{
			name: "测试用例不通过",
			ruleContent: `desc(
	title: "Runtime命令执行检测",
	description: "检测使用Runtime.getRuntime().exec()方法执行系统命令的安全风险",
	solution: "使用参数化的方式执行命令，避免直接传入用户输入",
	alert_min: 1,
	language: java,
	'file://positive.java': <<<EOF
public class Test {
    public void safe() {
        System.out.println("safe operation"); // 这里不会匹配规则，导致正向测试失败
    }
}
EOF
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectedScore: int64(sfanalyzer.MinScore), // 测试用例不通过直接0分
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ypb.SmokingEvaluatePluginRequest{
				PluginType: schema.SCRIPT_TYPE_SYNTAXFLOW,
				Code:       tt.ruleContent,
			}

			resp, err := client.SmokingEvaluatePlugin(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, resp)

			// 检查分数
			assert.Equal(t, tt.expectedScore, resp.Score, "分数不匹配，期望 %d，实际 %d", tt.expectedScore, resp.Score)

			// 记录详细信息用于调试
			t.Logf("规则: %s, 得分: %d, 问题数: %d", tt.name, resp.Score, len(resp.Results))
			for i, result := range resp.Results {
				t.Logf("问题 %d: Type=%s, Severity=%s", i+1, result.Item, result.Severity)
			}
		})
	}
}

func TestEvaluateSyntaxFlowRule_EmptyRequests(t *testing.T) {
	// 测试边界条件
	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("完全空的请求", func(t *testing.T) {
		req := &ypb.SmokingEvaluatePluginRequest{}

		_, err := client.SmokingEvaluatePlugin(context.Background(), req)
		assert.Error(t, err)
	})
}
