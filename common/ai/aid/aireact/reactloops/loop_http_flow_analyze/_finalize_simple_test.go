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
	"github.com/yaklang/yaklang/common/schema"
)

// TestHTTPFlowAnalyze_MaxIterations_Simple 简单测试：验证达到最大迭代次数时的行为
func TestHTTPFlowAnalyze_MaxIterations_Simple(t *testing.T) {
	// 测试变量
	iterationCount := 0
	liteForgeCallCount := 0

	// 创建 ReAct 实例
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			rsp := i.NewAIResponse()

			// 检测 LiteForge 调用（生成最终总结）
			if strings.Contains(prompt, "HTTP 流量分析专家") && strings.Contains(prompt, "生成一个完整的分析报告") {
				liteForgeCallCount++
				t.Logf("LiteForge called: count=%d", liteForgeCallCount)

				summary := "# HTTP 流量分析报告\n\n测试总结内容"
				summaryJSON, _ := json.Marshal(map[string]string{"summary": summary})
				rsp.EmitOutputStream(bytes.NewBuffer(summaryJSON))
				rsp.Close()
				return rsp, nil
			}

			// ReActLoop 迭代
			if strings.Contains(prompt, "filter_and_match_http_flows") {
				iterationCount++
				t.Logf("Iteration: count=%d", iterationCount)

				// 返回一个会继续循环的 action
				actionJSON := `{
					"@action": "filter_and_match_http_flows",
					"human_readable_thought": "继续分析",
					"limit": 10
				}`
				rsp.EmitOutputStream(bytes.NewBufferString(actionJSON))
				rsp.Close()
				return rsp, nil
			}

			// 默认返回 finish
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	// 创建 loop，设置 MaxIterations 为 2
	loop, err := reactloops.NewReActLoop(
		schema.AI_REACT_LOOP_ACTION_HTTP_FLOW_ANALYZE,
		reactIns,
		reactloops.WithMaxIterations(2),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	// 执行 loop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Log("开始执行循环...")
	err = loop.Execute("test-finalize", ctx, "测试流量分析")
	t.Logf("循环执行完成，错误: %v", err)

	// 验证结果
	t.Logf("迭代次数: %d", iterationCount)
	t.Logf("LiteForge 调用次数: %d", liteForgeCallCount)

	// 验证迭代次数
	if iterationCount < 2 {
		t.Errorf("期望至少 2 次迭代，实际: %d", iterationCount)
	}

	// 验证 LiteForge 被调用
	if liteForgeCallCount == 0 {
		t.Error("期望 LiteForge 被调用以生成最终总结")
	} else {
		t.Logf("✓ 测试通过: 达到最大迭代次数后，成功触发 finalize 逻辑")
	}
}
