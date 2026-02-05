package loop_http_flow_analyze_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// TestHTTPFlowAnalyze_MaxIterationsExceeded 测试当调用次数超过限制时的行为
func TestHTTPFlowAnalyze_MaxIterationsExceeded(t *testing.T) {
	// 设置测试数据库
	db := consts.GetGormProjectDatabase()
	if db == nil {
		t.Skip("No project database available, skipping test")
		return
	}

	// 清理测试数据
	db.Where("url LIKE ?", "%test-finalize%").Delete(&schema.HTTPFlow{})
	defer db.Where("url LIKE ?", "%test-finalize%").Delete(&schema.HTTPFlow{})

	// 创建测试 HTTP flow 数据
	testFlowConfigs := []struct {
		url        string
		method     string
		statusCode int
		request    string
		response   string
		source     string
	}{
		{
			url:        "http://example.com/test-finalize/api/login",
			method:     "POST",
			statusCode: 200,
			request:    "POST /api/login HTTP/1.1\r\nHost: example.com\r\n\r\nusername=admin&password=secret",
			response:   "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"token\":\"abc123\"}",
			source:     "mitm",
		},
		{
			url:        "http://example.com/test-finalize/api/users",
			method:     "GET",
			statusCode: 200,
			request:    "GET /api/users HTTP/1.1\r\nHost: example.com\r\n\r\n",
			response:   "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n[{\"id\":1,\"name\":\"admin\"}]",
			source:     "mitm",
		},
		{
			url:        "http://example.com/test-finalize/api/data",
			method:     "GET",
			statusCode: 500,
			request:    "GET /api/data HTTP/1.1\r\nHost: example.com\r\n\r\n",
			response:   "HTTP/1.1 500 Internal Server Error\r\n\r\nError: Database connection failed",
			source:     "mitm",
		},
	}

	for _, cfg := range testFlowConfigs {
		_, err := yakit.CreateHTTPFlow(
			yakit.CreateHTTPFlowWithURL(cfg.url),
			yakit.CreateHTTPFlowWithSource(cfg.source),
			yakit.CreateHTTPFlowWithRequestRaw([]byte(cfg.request)),
			yakit.CreateHTTPFlowWithResponseRaw([]byte(cfg.response)),
		)
		if err != nil {
			t.Fatalf("Failed to create test HTTP flow: %v", err)
		}
	}

	// 等待数据库写入
	time.Sleep(100 * time.Millisecond)

	// 测试变量
	iterationCount := 0
	liteForgeCallCount := 0

	// 创建 ReAct 实例
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			rsp := i.NewAIResponse()

			// 检测是否是 LiteForge 调用（用于生成最终总结）
			if strings.Contains(prompt, "HTTP 流量分析专家") && strings.Contains(prompt, "生成一个完整的分析报告") {
				liteForgeCallCount++
				// 模拟 AI 生成的总结
				summary := `# HTTP 流量分析报告

## 概述
在本次分析中，我们发现了以下关键信息：

## 发现的问题
1. **登录接口**: 发现 POST /api/login 接口，可能存在敏感信息泄露
2. **用户列表**: GET /api/users 接口暴露了用户信息
3. **错误响应**: GET /api/data 接口返回 500 错误，表明存在数据库连接问题

## 建议
1. 对登录接口进行安全加固
2. 限制用户列表接口的访问权限
3. 修复数据库连接问题`

				// 返回带有 summary 参数的 JSON
				summaryJSON, _ := json.Marshal(map[string]string{"summary": summary})
				rsp.EmitOutputStream(bytes.NewBuffer(summaryJSON))
				rsp.Close()
				return rsp, nil
			}

			// 检测是否是 ReActLoop 的迭代调用
			if strings.Contains(prompt, "filter_and_match_http_flows") ||
				strings.Contains(prompt, "match_http_flows_with_matcher") ||
				strings.Contains(prompt, "get_http_flow_detail") {
				iterationCount++

				// 前几次迭代：返回一个会继续循环的 action
				if iterationCount <= 5 {
					// 返回 filter_and_match_http_flows action
					actionJSON := `{
						"@action": "filter_and_match_http_flows",
						"human_readable_thought": "搜索包含 test-finalize 的流量",
						"url_contains": "test-finalize",
						"limit": 10
					}`
					rsp.EmitOutputStream(bytes.NewBufferString(actionJSON))
					rsp.Close()
					return rsp, nil
				}

				// 理论上不应该到这里，因为应该在达到 max iterations 前就结束了
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
				rsp.Close()
				return rsp, nil
			}

			// 其他情况：直接返回 finish
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	// 创建 loop，设置 MaxIterations 为 2（较小的值以快速触发限制）
	loop, err := reactloops.NewReActLoop(
		schema.AI_REACT_LOOP_ACTION_HTTP_FLOW_ANALYZE,
		reactIns,
		reactloops.WithMaxIterations(2),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	// 执行 loop，设置较短的超时时间
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Log("Starting loop execution...")
	err = loop.Execute("test-finalize-task", ctx, "分析 test-finalize 相关的流量")
	t.Logf("Loop execution completed with error: %v", err)

	// 验证结果
	t.Logf("Iteration count: %d", iterationCount)
	t.Logf("LiteForge call count: %d", liteForgeCallCount)

	if err != nil {
		// 即使有错误，如果 finalize 逻辑正确执行并忽略了错误，也应该通过测试
		// 检查错误是否与 max iterations 相关
		if !strings.Contains(err.Error(), "max iterations") {
			t.Logf("Loop execution returned error (expected if max iterations exceeded): %v", err)
		}
	}

	// 验证迭代次数 - 应该达到至少 2 次迭代
	if iterationCount < 2 {
		t.Errorf("Expected at least 2 iterations, got %d", iterationCount)
	}

	// 验证 LiteForge 被调用（用于生成最终总结）
	// 这是 finalize 逻辑的核心：当达到 max iterations 时，应该调用 InvokeLiteForge 生成总结
	if liteForgeCallCount == 0 {
		t.Error("Expected LiteForge to be called for generating final summary when max iterations exceeded")
	} else {
		t.Logf("✓ Finalize logic successfully triggered: LiteForge called %d time(s) to generate summary", liteForgeCallCount)
	}

	// 如果 LiteForge 被调用了，说明 finalize 逻辑正常工作
	if liteForgeCallCount > 0 {
		t.Log("✓ Test PASSED: Max iterations exceeded and finalize logic was triggered successfully")
	}
}

// TestHTTPFlowAnalyze_MaxIterations_WithoutData 测试没有数据时的最大迭代行为
func TestHTTPFlowAnalyze_MaxIterations_WithoutData(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		t.Skip("No project database available, skipping test")
		return
	}

	iterationCount := 0
	liteForgeCallCount := 0

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			rsp := i.NewAIResponse()

			// LiteForge 调用
			if strings.Contains(prompt, "HTTP 流量分析专家") {
				liteForgeCallCount++
				summary := "# HTTP 流量分析报告\n\n未找到相关流量数据。"
				summaryJSON, _ := json.Marshal(map[string]string{"summary": summary})
				rsp.EmitOutputStream(bytes.NewBuffer(summaryJSON))
				rsp.Close()
				return rsp, nil
			}

			// ReActLoop 迭代
			if strings.Contains(prompt, "filter_and_match_http_flows") {
				iterationCount++
				// 持续查询不存在的数据
				actionJSON := `{
					"@action": "filter_and_match_http_flows",
					"human_readable_thought": "搜索不存在的流量",
					"url_contains": "nonexistent-url-xyz123",
					"limit": 10
				}`
				rsp.EmitOutputStream(bytes.NewBufferString(actionJSON))
				rsp.Close()
				return rsp, nil
			}

			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop(
		schema.AI_REACT_LOOP_ACTION_HTTP_FLOW_ANALYZE,
		reactIns,
		reactloops.WithMaxIterations(3),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_ = loop.Execute("test-no-data", ctx, "分析不存在的流量")

	t.Logf("Iteration count: %d", iterationCount)
	t.Logf("LiteForge call count: %d", liteForgeCallCount)

	// 验证即使没有数据，finalize 逻辑也会被调用
	if liteForgeCallCount == 0 {
		t.Error("Expected LiteForge to be called even when no data found")
	}

	if iterationCount < 2 {
		t.Errorf("Expected at least 2 iterations, got %d", iterationCount)
	}
}
