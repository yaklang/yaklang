package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalyzer"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_LANGUAGE_SMOKING_EVALUATE_PLUGIN_BATCH(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	type code struct {
		src string
		typ string
	}

	test := func(codes []code) {
		names := make([]string, 0, len(codes))
		clearFuncs := make([]func(), 0, len(codes))
		for _, c := range codes {
			typ := c.typ
			if typ == "" {
				typ = "port-scan"
			}
			name, clearFunc, err := yakit.CreateTemporaryYakScriptEx(typ, c.src)
			require.NoError(t, err)
			clearFuncs = append(clearFuncs, clearFunc)
			names = append(names, name)
		}
		if len(clearFuncs) > 0 {
			defer func() {
				for _, f := range clearFuncs {
					f()
				}
			}()
		}

		fmt.Println("names:", names)
		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: names,
			PluginType:  schema.SCRIPT_TYPE_YAK,
		})
		require.NoError(t, err)
		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}
			t.Log(res)
		}
	}

	t.Run("Basic Batch Evaluation", func(t *testing.T) {
		test([]code{
			{
				src: `
yakit.AutoInitYakit()
handle = result => {
	yakit.Info("HELLO")
	// risk.NewRisk("http://baidu.com")
}
			`,
				typ: "port-scan",
			},
			{
				src: `
print(aa) // undefine variable
			`,
				typ: "port-scan",
			},
		})
	})

	t.Run("Batch Evaluate Multiple Valid Yak Scripts", func(t *testing.T) {
		testScripts := []code{
			{
				src: `
yakit.AutoInitYakit()

# Basic MITM Plugin
handle = func(req, rsp) {
	yakit.Info("Processing request")
	return
}
`,
				typ: "mitm",
			},
			{
				src: `
yakit.AutoInitYakit()

# Port Scan Plugin with Parameters
handle = func(result) {
	yakit.Info(sprintf("Port scan result: %v", result))
}
`,
				typ: "port-scan",
			},
			{
				src: `
yakit.AutoInitYakit()

# Codec Plugin
handle = func(data) {
	return codec.EncodeBase64(data)
}
`,
				typ: "codec",
			},
		}

		names := make([]string, 0, len(testScripts))
		clearFuncs := make([]func(), 0, len(testScripts))
		for _, c := range testScripts {
			name, clearFunc, err := yakit.CreateTemporaryYakScriptEx(c.typ, c.src)
			require.NoError(t, err)
			clearFuncs = append(clearFuncs, clearFunc)
			names = append(names, name)
		}
		defer func() {
			for _, f := range clearFuncs {
				f()
			}
		}()

		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: names,
			PluginType:  schema.SCRIPT_TYPE_YAK,
		})
		require.NoError(t, err)

		successCount := 0
		errorCount := 0
		var finalResult string

		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}

			t.Logf("Progress: %.2f, Message: %s, Type: %s", res.Progress, res.Message, res.MessageType)

			if res.Progress > 0 && res.Progress <= 1 {
				if res.MessageType == "success" {
					if len(res.Message) > 0 && strings.Contains(res.Message, "插件得分") {
						successCount++
					}
				} else if res.MessageType == "error" {
					errorCount++
				}
			}

			if res.MessageType == "success-again" {
				finalResult = res.Message
			}
		}

		assert.Greater(t, successCount, 0, "Should have successful evaluations")

		var resultNames []string
		err = json.Unmarshal([]byte(finalResult), &resultNames)
		require.NoError(t, err)
		t.Logf("Successfully evaluated %d scripts", len(resultNames))
	})

	t.Run("Batch Evaluate Mixed Valid and Invalid Scripts", func(t *testing.T) {
		testScripts := []code{
			{
				src: `
yakit.AutoInitYakit()
handle = func(result) {
	yakit.Info("Valid script")
}
`,
				typ: "port-scan",
			},
			{
				src: `
undefined_variable_error
`,
				typ: "port-scan",
			},
		}

		names := make([]string, 0, len(testScripts))
		clearFuncs := make([]func(), 0, len(testScripts))
		for _, c := range testScripts {
			name, clearFunc, err := yakit.CreateTemporaryYakScriptEx(c.typ, c.src)
			require.NoError(t, err)
			clearFuncs = append(clearFuncs, clearFunc)
			names = append(names, name)
		}
		defer func() {
			for _, f := range clearFuncs {
				f()
			}
		}()

		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: names,
			PluginType:  schema.SCRIPT_TYPE_YAK,
		})
		require.NoError(t, err)

		successCount := 0
		errorCount := 0

		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}

			t.Logf("Progress: %.2f, Message: %s, Type: %s", res.Progress, res.Message, res.MessageType)

			if res.Progress > 0 && res.Progress <= 1 {
				if res.MessageType == "success" && strings.Contains(res.Message, "插件得分") {
					successCount++
				} else if res.MessageType == "error" {
					errorCount++
				}
			}
		}

		t.Logf("Success: %d, Error: %d", successCount, errorCount)
		assert.Greater(t, successCount, 0, "Should have some successful scripts")
		assert.Greater(t, errorCount, 0, "Should have some failed scripts")
	})

	t.Run("Batch Evaluate Different Plugin Types", func(t *testing.T) {
		testScripts := []struct {
			src  string
			typ  string
			name string
		}{
			{
				src: `
yakit.AutoInitYakit()
handle = func(result) {
	yakit.Info("Port scan plugin")
}
`,
				typ:  "port-scan",
				name: "portscan_test",
			},
			{
				src: `
yakit.AutoInitYakit()
handle = func(data) {
	return codec.EncodeBase64(data)
}
`,
				typ:  "codec",
				name: "codec_test",
			},
		}

		names := make([]string, 0, len(testScripts))
		clearFuncs := make([]func(), 0, len(testScripts))

		for _, script := range testScripts {
			name, clearFunc, err := yakit.CreateTemporaryYakScriptEx(script.typ, script.src)
			require.NoError(t, err)
			clearFuncs = append(clearFuncs, clearFunc)
			names = append(names, name)
		}
		defer func() {
			for _, f := range clearFuncs {
				f()
			}
		}()

		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: names,
			PluginType:  schema.SCRIPT_TYPE_YAK,
		})
		require.NoError(t, err)

		messageCount := 0
		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}
			messageCount++
			t.Logf("Progress: %.2f, Message: %s, Type: %s", res.Progress, res.Message, res.MessageType)
		}

		assert.Greater(t, messageCount, 0, "Should receive messages")
	})

	t.Run("Batch Evaluate Non-existent Scripts", func(t *testing.T) {
		nonExistentNames := []string{
			"non_existent_script_1",
			"non_existent_script_2",
		}

		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: nonExistentNames,
			PluginType:  schema.SCRIPT_TYPE_YAK,
		})
		require.NoError(t, err)

		errorCount := 0
		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}

			t.Logf("Progress: %.2f, Message: %s, Type: %s", res.Progress, res.Message, res.MessageType)

			if res.MessageType == "error" {
				errorCount++
			}
		}

		assert.GreaterOrEqual(t, errorCount, len(nonExistentNames), "All non-existent scripts should report errors")
	})

	t.Run("Verify Progress and Message Flow", func(t *testing.T) {
		scriptCount := 2
		scripts := make([]code, scriptCount)
		for i := 0; i < scriptCount; i++ {
			scripts[i] = code{
				src: fmt.Sprintf(`
yakit.AutoInitYakit()
handle = func(result) {
	yakit.Info("Script %d")
}
`, i),
				typ: "port-scan",
			}
		}

		names := make([]string, 0, scriptCount)
		clearFuncs := make([]func(), 0, scriptCount)
		for _, c := range scripts {
			name, clearFunc, err := yakit.CreateTemporaryYakScriptEx(c.typ, c.src)
			require.NoError(t, err)
			clearFuncs = append(clearFuncs, clearFunc)
			names = append(names, name)
		}
		defer func() {
			for _, f := range clearFuncs {
				f()
			}
		}()

		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: names,
			PluginType:  schema.SCRIPT_TYPE_YAK,
		})
		require.NoError(t, err)

		var progressList []float64
		messageTypes := make(map[string]int)

		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}

			progressList = append(progressList, res.Progress)
			messageTypes[res.MessageType]++

			t.Logf("Progress: %.2f, Message: %s, Type: %s", res.Progress, res.Message, res.MessageType)
		}

		// Verify we have different message types
		assert.Greater(t, messageTypes["success"], 0, "Should have success messages")

		// Verify progress goes from 0 to at least 1
		if len(progressList) > 0 {
			assert.Equal(t, 0.0, progressList[0], "Should start with progress 0")
			assert.GreaterOrEqual(t, progressList[len(progressList)-1], 1.0, "Should end with progress >= 1")
		}
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SMOKING_EVALUATE_SYNTAXFLOW_BATCH(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("Batch Evaluate Multiple Valid SyntaxFlow Rules", func(t *testing.T) {
		// 创建多个测试规则
		testRules := []struct {
			name    string
			content string
			score   int64
		}{
			{
				name: "test_batch_rule_1",
				content: `desc(
	title: "Runtime命令执行检测",
	description: "检测使用Runtime.getRuntime().exec()方法执行系统命令的安全风险",
	solution: "使用参数化的方式执行命令，避免直接传入用户输入",
	severity: "high"
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
				score: int64(sfanalyzer.MaxScore - sfanalyzer.MissingPositiveTestPenalty - sfanalyzer.MissingNegativeTestPenalty), // 80
			},
			{
				name: "test_batch_rule_2",
				content: `desc(
	title: "SQL注入检测",
	description: "检测SQL注入漏洞",
	solution: "使用预编译语句"
)

executeQuery(* as $sql) as $result;
alert $result;`,
				score: int64(sfanalyzer.MaxScore - sfanalyzer.MissingSolutionPenalty - sfanalyzer.MissingPositiveTestPenalty - sfanalyzer.MissingNegativeTestPenalty), // 70
			},
			{
				name: "test_batch_rule_3",
				content: `desc(
	title: "完整测试规则",
	description: "这是一个完整的测试规则",
	solution: "参考安全最佳实践",
	language: java,
	'file://positive.java': <<<EOF
public class Test {
    public void test() {
        Runtime.getRuntime().exec("ls");
    }
}
EOF,
	'safefile://negative.java': <<<SAFE
public class Safe {
    public void safe() {
        System.out.println("safe");
    }
}
SAFE
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
				score: int64(sfanalyzer.MaxScore), // 100
			},
		}

		// 创建规则
		ruleNames := make([]string, 0, len(testRules))
		for _, rule := range testRules {
			_, err := client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
				SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
					RuleName: rule.name,
					Content:  rule.content,
					Language: "java",
				},
			})
			require.NoError(t, err)
			ruleNames = append(ruleNames, rule.name)
		}

		// 清理资源
		defer func() {
			for _, name := range ruleNames {
				sfdb.DeleteRuleByRuleName(name)
			}
		}()

		// 批量评估
		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: ruleNames,
			PluginType:  schema.SCRIPT_TYPE_SYNTAXFLOW,
		})
		require.NoError(t, err)

		successCount := 0
		messageTypes := make(map[string]int)
		var finalResult string

		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}

			t.Logf("Progress: %.2f, Message: %s, Type: %s", res.Progress, res.Message, res.MessageType)
			messageTypes[res.MessageType]++

			// 检查成功消息（排除开始和结束的总结消息）
			if res.MessageType == "success" && res.Progress > 0 && res.Progress <= 1 {
				// 判断是否是插件得分消息（包含"插件得分"关键字）
				if len(res.Message) > 0 && !strings.Contains(res.Message, "检测通过") && !strings.Contains(res.Message, "检测失败") && !strings.Contains(res.Message, "检测结束") && res.Message != "开始检测" {
					successCount++
				}
			}

			// 保存最终结果
			if res.MessageType == "success-again" {
				finalResult = res.Message
			}
		}

		// 验证所有规则都通过了
		assert.Equal(t, len(testRules), successCount, "应该有 %d 个规则评估成功", len(testRules))
		assert.Greater(t, messageTypes["success"], 0, "应该有成功消息")

		// 验证最终结果包含所有规则名称
		var resultNames []string
		err = json.Unmarshal([]byte(finalResult), &resultNames)
		require.NoError(t, err)
		assert.Equal(t, len(testRules), len(resultNames), "返回的规则数量应该匹配")
	})

	t.Run("Batch Evaluate Mixed Valid and Invalid SyntaxFlow Rules", func(t *testing.T) {
		testRules := []struct {
			name        string
			content     string
			shouldPass  bool
			description string
		}{
			{
				name: "test_batch_valid",
				content: `desc(
	title: "有效规则",
	description: "这是一个有效的规则",
	solution: "修复建议"
)

test.method(* as $arg) as $result;
alert $result;`,
				shouldPass:  true,
				description: "有效规则应该通过",
			},
			{
				name:        "test_batch_syntax_error",
				content:     `invalid syntax here $$$`,
				shouldPass:  false,
				description: "语法错误的规则应该失败",
			},
			{
				name: "test_batch_no_alert",
				content: `desc(
	title: "缺少alert",
	description: "这个规则缺少alert语句",
	solution: "修复建议"
)

test.method(* as $arg) as $result;`,
				shouldPass:  false,
				description: "缺少alert的规则应该失败（得分<60）",
			},
		}

		// 创建规则
		ruleNames := make([]string, 0, len(testRules))
		for _, rule := range testRules {
			_, err := client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
				SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
					RuleName: rule.name,
					Content:  rule.content,
					Language: "java",
				},
			})
			require.NoError(t, err)
			ruleNames = append(ruleNames, rule.name)
		}

		// 清理资源
		defer func() {
			for _, name := range ruleNames {
				sfdb.DeleteRuleByRuleName(name)
			}
		}()

		// 批量评估
		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: ruleNames,
			PluginType:  schema.SCRIPT_TYPE_SYNTAXFLOW,
		})
		require.NoError(t, err)

		successCount := 0
		errorCount := 0

		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}

			t.Logf("Progress: %.2f, Message: %s, Type: %s", res.Progress, res.Message, res.MessageType)

			if res.Progress > 0 && res.Progress <= 1 {
				if res.MessageType == "success" {
					successCount++
				} else if res.MessageType == "error" {
					errorCount++
				}
			}
		}

		// 验证结果
		expectedSuccess := 0
		expectedError := 0
		for _, rule := range testRules {
			if rule.shouldPass {
				expectedSuccess++
			} else {
				expectedError++
			}
		}

		t.Logf("成功: %d (期望: %d), 失败: %d (期望: %d)", successCount, expectedSuccess, errorCount, expectedError)
		// 至少应该有一些成功和失败的情况
		assert.Greater(t, successCount, 0, "应该有成功的规则")
		assert.Greater(t, errorCount, 0, "应该有失败的规则")
	})

	t.Run("Batch Evaluate Non-existent Rules", func(t *testing.T) {
		// 使用不存在的规则名称
		nonExistentNames := []string{
			"non_existent_rule_1",
			"non_existent_rule_2",
			"non_existent_rule_3",
		}

		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: nonExistentNames,
			PluginType:  schema.SCRIPT_TYPE_SYNTAXFLOW,
		})
		require.NoError(t, err)

		errorCount := 0
		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}

			t.Logf("Progress: %.2f, Message: %s, Type: %s", res.Progress, res.Message, res.MessageType)

			if res.MessageType == "error" {
				errorCount++
			}
		}

		// 所有规则都应该报错
		assert.GreaterOrEqual(t, errorCount, len(nonExistentNames), "所有不存在的规则都应该报错")
	})

	t.Run("Batch Evaluate Single Rule", func(t *testing.T) {
		// 创建单个规则
		ruleName := "test_batch_single_rule"
		ruleContent := `desc(
	title: "单个测试规则",
	description: "用于测试批量接口处理单个规则的情况",
	solution: "解决方案"
)

test.method(* as $arg) as $result;
alert $result;`

		_, err := client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
			SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
				RuleName: ruleName,
				Content:  ruleContent,
				Language: "java",
			},
		})
		require.NoError(t, err)

		defer func() {
			sfdb.DeleteRuleByRuleName(ruleName)
		}()

		// 批量评估单个规则
		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: []string{ruleName},
			PluginType:  schema.SCRIPT_TYPE_SYNTAXFLOW,
		})
		require.NoError(t, err)

		found := false
		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}

			t.Logf("Progress: %.2f, Message: %s, Type: %s", res.Progress, res.Message, res.MessageType)

			// 检查是否是插件得分的成功消息
			if res.MessageType == "success" && res.Progress > 0 && res.Progress <= 1 && strings.Contains(res.Message, "插件得分") {
				found = true
			}
		}

		assert.True(t, found, "应该成功评估单个规则")
	})

	t.Run("Verify Progress Increment", func(t *testing.T) {
		// 创建多个规则用于测试进度
		ruleCount := 5
		ruleNames := make([]string, 0, ruleCount)

		for i := 0; i < ruleCount; i++ {
			ruleName := fmt.Sprintf("test_batch_progress_%d", i)
			ruleContent := fmt.Sprintf(`desc(
	title: "进度测试规则 %d",
	description: "用于测试进度递增",
	solution: "解决方案"
)

test.method%d(* as $arg) as $result;
alert $result;`, i, i)

			_, err := client.CreateSyntaxFlowRule(context.Background(), &ypb.CreateSyntaxFlowRuleRequest{
				SyntaxFlowInput: &ypb.SyntaxFlowRuleInput{
					RuleName: ruleName,
					Content:  ruleContent,
					Language: "java",
				},
			})
			require.NoError(t, err)
			ruleNames = append(ruleNames, ruleName)
		}

		defer func() {
			for _, name := range ruleNames {
				sfdb.DeleteRuleByRuleName(name)
			}
		}()

		// 批量评估
		streamClient, err := client.SmokingEvaluatePluginBatch(context.Background(), &ypb.SmokingEvaluatePluginBatchRequest{
			ScriptNames: ruleNames,
			PluginType:  schema.SCRIPT_TYPE_SYNTAXFLOW,
		})
		require.NoError(t, err)

		var progressList []float64
		for {
			res, err := streamClient.Recv()
			if err != nil {
				break
			}

			if res.Progress > 0 && res.Progress <= 1 {
				progressList = append(progressList, res.Progress)
			}

			t.Logf("Progress: %.2f, Message: %s, Type: %s", res.Progress, res.Message, res.MessageType)
		}

		// 验证进度是递增的（允许相同值）
		for i := 1; i < len(progressList); i++ {
			assert.GreaterOrEqual(t, progressList[i], progressList[i-1],
				"进度应该是递增的: %.2f >= %.2f", progressList[i], progressList[i-1])
		}

		// 验证最后的进度应该是1.0
		if len(progressList) > 0 {
			assert.Equal(t, 1.0, progressList[len(progressList)-1], "最后的进度应该是1.0")
		}
	})
}
