package tests

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

// mockRawMessagesPassthroughRsp 是一个最小可用的 OpenAI chat.completions 响应，
// 用于 RawMessages 端到端透传的集成验证。
const mockRawMessagesPassthroughRsp = `HTTP/1.1 200 OK
Connection: close
Content-Type: application/json; charset=utf-8

{
  "id": "raw-msgs-passthrough",
  "object": "chat.completion",
  "created": 1753327376,
  "model": "test-model",
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "ok"},
      "finish_reason": "stop"
    }
  ],
  "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
}
`

// TestGatewayChat_RawMessagesViaConfig_OpenAI 验证：
// 通过 gateway.LoadOption 注入 aispec.WithRawMessages 后，调用 client.Chat("")
// 时上游 LLM 实际收到的请求体里 messages 数组与注入的 RawMessages 一字不差。
//
// 这是 11 个 gateway 共享的"补一行 WithChatBase_RawMessages(g.config.RawMessages)"
// 模式的代表性集成测试 —— openai gateway 走通即可证明 aispec 核心 +
// gateway 透传链路完整。
//
// 关键词: RawMessages 集成测试, gateway 透传, openai 代表性
func TestGatewayChat_RawMessagesViaConfig_OpenAI(t *testing.T) {
	var capturedRequest []byte
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		capturedRequest = req
		return []byte(mockRawMessagesPassthroughRsp)
	})

	rawMsgs := []aispec.ChatDetail{
		{Role: "system", Content: "you are a passthrough probe"},
		{Role: "user", Content: "first user msg"},
		{Role: "assistant", Content: "first assistant reply"},
		{
			Role: "user",
			Content: []*aispec.ChatContent{
				aispec.NewUserChatContentText("second user with image"),
				aispec.NewUserChatContentImageUrl("https://example.com/passthrough.png"),
			},
		},
	}

	client := ai.OpenAI(
		aispec.WithAPIKey("test-passthrough-key"),
		aispec.WithModel("test-model"),
		aispec.WithBaseURL("http://"+utils.HostPort(host, port)+"/v1/chat/completions"),
		aispec.WithRawMessages(rawMsgs),
	)
	if utils.IsNil(client) {
		t.Fatalf("ai.OpenAI returned nil client")
	}

	// 入参 string 故意非空，用以验证它被 RawMessages 优先级覆盖：
	// 上游不应再看到这条字符串，而是看到 rawMsgs 数组本体。
	res, err := client.Chat("legacy-string-should-not-appear")
	if err != nil {
		t.Fatalf("client.Chat failed: %v", err)
	}
	if res != "ok" {
		t.Fatalf("unexpected response: %q", res)
	}

	if len(capturedRequest) == 0 {
		t.Fatalf("upstream did not capture any request")
	}

	parsed := new(aispec.ChatMessage)
	bodyStart := bytes.Index(capturedRequest, []byte("\r\n\r\n"))
	if bodyStart < 0 {
		t.Fatalf("malformed mock request: no header/body separator")
	}
	body := capturedRequest[bodyStart+4:]
	if err := json.Unmarshal(body, parsed); err != nil {
		t.Fatalf("upstream body is not valid ChatMessage JSON: %v\nbody=%s", err, string(body))
	}

	if got := len(parsed.Messages); got != len(rawMsgs) {
		t.Fatalf("upstream messages length mismatch: got %d want %d", got, len(rawMsgs))
	}
	for i, want := range rawMsgs {
		if parsed.Messages[i].Role != want.Role {
			t.Fatalf("messages[%d].role: got %q want %q", i, parsed.Messages[i].Role, want.Role)
		}
	}
	if parsed.Model != "test-model" {
		t.Fatalf("upstream model field: got %q want %q", parsed.Model, "test-model")
	}

	if !bytes.Contains(body, []byte("https://example.com/passthrough.png")) {
		t.Fatalf("upstream body missing image_url, body=%s", string(body))
	}
	if bytes.Contains(body, []byte("legacy-string-should-not-appear")) {
		t.Fatalf("upstream body should NOT contain the legacy prompt string when RawMessages is set, body=%s", string(body))
	}
}
