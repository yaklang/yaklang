package aiprojection

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestFunctionCallActionResponseBatchInsideTimeline(t *testing.T) {
	payload := `[{"role":"assistant","content":"working","reasoning_content":"consider both","tool_calls":[` +
		`{"id":"call_a","type":"function","function":{"name":"accept","arguments":"{}"}},` +
		`{"id":"call_b","type":"function","function":{"name":"inspect","arguments":"{\"target\":\"x\"}"}}]},` +
		`{"role":"tool","tool_call_id":"call_a","content":"accepted A"},` +
		`{"role":"tool","tool_call_id":"call_b","content":"accepted B"}]`
	marker := actionResponseOpenTag + "\n" + payload + "\n" + actionResponseEndTag
	prompt := strings.Join([]string{
		"<|PROMPT_SECTION_high-static|>system<|PROMPT_SECTION_END_high-static|>",
		"<|PROMPT_SECTION_timeline-open|><|TIMELINE_b1|>\nprior\n" + marker + "\nlater result call_a\n<|TIMELINE_END_b1|><|PROMPT_SECTION_END_timeline-open|>",
		"<|PROMPT_SECTION_dynamic_n|>continue<|PROMPT_SECTION_dynamic_END_n|>",
	}, "\n")
	projected := ProjectAndObserve("test-model", prompt)
	require.NotNil(t, projected)
	var roles []string
	for _, message := range projected.Messages {
		roles = append(roles, message.Role)
	}
	require.Equal(t, []string{"system", "user", "assistant", "tool", "tool", "user"}, roles)
	require.Len(t, projected.Messages[2].ToolCalls, 2)
	require.Equal(t, "consider both", projected.Messages[2].ReasoningContent)
	require.Equal(t, "call_a", projected.Messages[3].ToolCallID)
	require.Equal(t, "call_b", projected.Messages[4].ToolCallID)
	require.Contains(t, projected.Messages[5].Content, "later result call_a")

	for _, invalid := range []string{
		strings.Replace(payload, `"tool_call_id":"call_b"`, `"tool_call_id":"call_a"`, 1),
		strings.Replace(payload, `"id":"call_b"`, `"id":"call_a"`, 1),
		strings.Replace(payload, `,{"role":"tool","tool_call_id":"call_b","content":"accepted B"}`, "", 1),
	} {
		badPrompt := strings.Replace(prompt, payload, invalid, 1)
		bad := ProjectAndObserve("test-model", badPrompt)
		require.NotNil(t, bad)
		for _, message := range bad.Messages {
			require.NotEqual(t, "tool", message.Role)
			require.Empty(t, message.ToolCalls)
		}
	}
	escaped := strings.Replace(prompt, marker, strings.ReplaceAll(marker, "<|", "&lt;|"), 1)
	untrusted := ProjectAndObserve("test-model", escaped)
	require.NotNil(t, untrusted)
	for _, message := range untrusted.Messages {
		require.NotEqual(t, "tool", message.Role)
		require.Empty(t, message.ToolCalls)
	}
}

func TestFunctionCallActionResponseBatchInFrozenTimelineKeepsCacheBoundary(t *testing.T) {
	marker := actionResponseOpenTag + `[{"role":"assistant","content":"","tool_calls":[` +
		`{"id":"call_a","type":"function","function":{"name":"accept","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"call_a","content":"accepted"}]` + actionResponseEndTag
	prompt := strings.Join([]string{
		"<|PROMPT_SECTION_high-static|>system<|PROMPT_SECTION_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|><|TIMELINE_old|>before\n" + marker + "\nafter<|TIMELINE_END_old|><|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|PROMPT_SECTION_timeline-open|><|TIMELINE_new|>later<|TIMELINE_END_new|><|PROMPT_SECTION_END_timeline-open|>",
		"<|PROMPT_SECTION_dynamic_n|>continue<|PROMPT_SECTION_dynamic_END_n|>",
	}, "\n")
	projected := ProjectAndObserve("test-model", prompt)
	require.NotNil(t, projected)
	var roles []string
	for _, message := range projected.Messages {
		roles = append(roles, message.Role)
	}
	require.Equal(t, []string{"system", "user", "assistant", "tool", "user", "user"}, roles)
	require.Equal(t, "call_a", projected.Messages[3].ToolCallID)
	_, firstCached := projected.Messages[1].Content.([]*aispec.ChatContent)
	require.False(t, firstCached)
	last, ok := projected.Messages[4].Content.([]*aispec.ChatContent)
	require.True(t, ok)
	require.Equal(t, map[string]any{"type": "ephemeral"}, last[0].CacheControl)
}
