package loop_ssa_api_discovery

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func allowedVerify() []string {
	return []string{"verify-satisfaction"}
}

func TestExtractValidActionFromReaderWithLooseFallback_MarkdownFence(t *testing.T) {
	blob := "好的，结果如下：\n```json\n{\n  \"@action\": \"verify-satisfaction\",\n  \"user_satisfied\": false,\n  \"reasoning\": \"任务未完成\"\n}\n```\n"
	act, err := ExtractValidActionFromReaderWithLooseFallback(context.Background(), strings.NewReader(blob), "verify-satisfaction", allowedVerify())
	require.NoError(t, err)
	require.Equal(t, "verify-satisfaction", act.ActionType())
	require.False(t, act.GetBool("user_satisfied"))
	require.Contains(t, act.GetString("reasoning"), "任务")
}

func TestExtractValidActionFromReaderWithLooseFallback_ActionAliasKey(t *testing.T) {
	blob := `以下是JSON：{"action":"verify-satisfaction","user_satisfied":true,"reasoning":"done"}`
	act, err := ExtractValidActionFromReaderWithLooseFallback(context.Background(), strings.NewReader(blob), "verify-satisfaction", allowedVerify())
	require.NoError(t, err)
	require.True(t, act.GetBool("user_satisfied"))
}

func TestExtractValidActionFromReaderWithLooseFallback_Preamble(t *testing.T) {
	blob := `我分析一下。
{"@action":"directly_answer","answer_payload":"hello","brief":"ok"}
希望对你有帮助。`
	allowed := []string{"directly_answer"}
	act, err := ExtractValidActionFromReaderWithLooseFallback(context.Background(), strings.NewReader(blob), "directly_answer", allowed)
	require.NoError(t, err)
	require.Equal(t, "directly_answer", act.ActionType())
	require.Contains(t, act.GetString("answer_payload"), "hello")
}

func TestExtractValidActionFromReaderWithLooseFallback_CallToolFence(t *testing.T) {
	blob := "```\n{\"@action\":\"call-tool\",\"tool_name\":\"abc\"}\n```"
	allowed := []string{"call-tool"}
	act, err := ExtractValidActionFromReaderWithLooseFallback(context.Background(), strings.NewReader(blob), "call-tool", allowed)
	require.NoError(t, err)
	require.Equal(t, "call-tool", act.ActionType())
	require.Equal(t, "abc", act.GetString("tool_name"))
}
