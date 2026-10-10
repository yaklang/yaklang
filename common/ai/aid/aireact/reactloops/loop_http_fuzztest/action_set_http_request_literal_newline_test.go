package loop_http_fuzztest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

// 用户反馈：AI 生成的请求包里换行是字面 "\n" 转义序列，粘成一行不可用。
// set_http_request 后生效/回显的请求包应归一化为真实换行。
func TestSetHTTPRequestAction_NormalizesLiteralNewlinesInPacket(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.CreateLoopByName(
		LoopHTTPFuzztestName,
		invoker,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
			operator.Continue()
		}),
	)
	if err != nil {
		t.Fatalf("create http_fuzztest loop: %v", err)
	}

	// AI 把多行请求包写成单行字面转义序列（真实场景常见输出形态）
	literalPacket := `POST /login HTTP/1.1\nHost: example.com\nContent-Type: application/x-www-form-urlencoded\n\nusername=admin&password=pass`

	handler, err := loop.GetActionHandler("set_http_request")
	if err != nil {
		t.Fatalf("get set_http_request action: %v", err)
	}
	rawAction, err := json.Marshal(map[string]any{
		"@action":      "set_http_request",
		"http_request": literalPacket,
		"reason":       "AI 输出的请求包换行是字面转义序列",
	})
	if err != nil {
		t.Fatalf("marshal action: %v", err)
	}
	action, err := aicommon.ExtractAction(string(rawAction), "set_http_request")
	if err != nil {
		t.Fatalf("extract action: %v", err)
	}
	if err := handler.ActionVerifier(loop, action); err != nil {
		t.Fatalf("verify action: %v", err)
	}
	task := aicommon.NewStatefulTaskBase("set-http-request-test", "set request with literal newlines", context.Background(), loop.GetEmitter())
	operator := reactloops.NewActionHandlerOperator(task)
	handler.ActionHandler(loop, action, operator)
	if _, err := operator.IsTerminated(); err != nil {
		t.Fatalf("set_http_request failed: %v", err)
	}

	current := utils.InterfaceToString(loop.Get("current_request"))
	if strings.Contains(current, `\n`) {
		t.Fatalf("expected literal \\n normalized to real newlines, got:\n%s", current)
	}
	if !strings.Contains(current, "Host: example.com") || !strings.Contains(current, "username=admin&password=pass") {
		t.Fatalf("expected normalized request packet with headers and body, got:\n%s", current)
	}
	// 归一化后请求行必须独立成行，否则请求包不可解析
	if !strings.HasPrefix(current, "POST /login HTTP/1.1") {
		t.Fatalf("expected request line intact, got:\n%s", current)
	}
}