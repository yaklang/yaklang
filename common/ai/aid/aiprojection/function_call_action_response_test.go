package aiprojection

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func TestFunctionCallActionResponseProjectsToMatchedHistory(t *testing.T) {
	aispec.ResetChatBaseHijackHooksForTest()
	aispec.RegisterChatBaseHijackHook(ProjectAndObserve)
	ResetForTest()
	restoreThreshold := SetMinCachableUserSegmentBytesForTest(0)
	t.Cleanup(func() {
		restoreThreshold()
		aispec.ResetChatBaseHijackHooksForTest()
		aispec.RegisterChatBaseHijackHook(ProjectAndObserve)
	})

	const callID = "call_inspect_1"
	const resultText = "execution_status: succeeded\nexecution_result: found a"
	const actionSchema = `<|FUNCTION_CALL_ACTION_SCHEMA_inspect|>
{"type":"function","function":{"name":"inspect","parameters":{"type":"object","properties":{"target":{"type":"string"}}}}}
<|FUNCTION_CALL_ACTION_SCHEMA_END_inspect|>`
	// One trusted timeline marker contains exactly two JSON messages. Projection
	// keeps the assistant's reasoning/content/tool_calls together, then emits
	// the matching tool result as role:tool content (or function_call_output for
	// Responses). The call arguments stay on the assistant, never on the tool.
	const actionResponse = `<|FUNCTION_CALL_ACTION_RESPONSE|>
[{"role":"assistant","content":"","reasoning_content":"inspect reasoning","tool_calls":[{"id":"call_inspect_1","type":"function","function":{"name":"inspect","arguments":"{\"target\":\"a\"}"}}]},
 {"role":"tool","tool_call_id":"call_inspect_1","content":"execution_status: succeeded\nexecution_result: found a"}]
<|FUNCTION_CALL_ACTION_RESPONSE_END|>`
	prompt := strings.Join([]string{
		"<|PROMPT_SECTION_high-static|>stable-system<|PROMPT_SECTION_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>frozen<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|AI_CACHE_SEMI_semi|><|PROMPT_SECTION_semi-dynamic-1|>skills<|PROMPT_SECTION_END_semi-dynamic-1|><|AI_CACHE_SEMI_END_semi|>",
		"<|AI_CACHE_SEMI2_semi|><|PROMPT_SECTION_semi-dynamic-2|>instructions " + actionSchema + "<|PROMPT_SECTION_END_semi-dynamic-2|><|AI_CACHE_SEMI2_END_semi|>",
		"<|PROMPT_SECTION_timeline-open|>before\n" + actionResponse + "\nafter<|PROMPT_SECTION_END_timeline-open|>",
		"<|PROMPT_SECTION_dynamic_n|>next question<|PROMPT_SECTION_dynamic_END_n|>",
	}, "\n\n")

	for _, tc := range []struct {
		name      string
		responses bool
	}{
		{name: "chat_completions"},
		{name: "responses", responses: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bodies := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				bodies <- body
				w.Header().Set("Content-Type", "application/json")
				if tc.responses {
					_, _ = w.Write([]byte(`{"id":"resp1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`))
					return
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
			}))
			defer server.Close()
			url := server.URL
			if tc.responses {
				url += "/responses"
			}
			_, err := aispec.ChatBase(url, "projection-test-model", CreateTemplate(prompt),
				aispec.WithChatBase_DisableStream(true),
				aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
			)
			require.NoError(t, err)
			require.Len(t, bodies, 1)
			body := <-bodies
			require.NotContains(t, string(body), "FUNCTION_CALL_ACTION_RESPONSE")
			require.NotContains(t, string(body), "FUNCTION_CALL_ACTION_SCHEMA")
			if tc.responses {
				var request struct {
					Input []map[string]any `json:"input"`
					Tools []map[string]any `json:"tools"`
				}
				require.NoError(t, json.Unmarshal(body, &request))
				require.Len(t, request.Tools, 1)
				require.Equal(t, "inspect", request.Tools[0]["name"])
				require.Len(t, request.Input, 8)
				require.Equal(t, "function_call", request.Input[5]["type"])
				require.Equal(t, callID, request.Input[5]["call_id"])
				require.Equal(t, "inspect", request.Input[5]["name"])
				require.Equal(t, `{"target":"a"}`, request.Input[5]["arguments"])
				require.Equal(t, "function_call_output", request.Input[6]["type"])
				require.Equal(t, callID, request.Input[6]["call_id"])
				require.Equal(t, resultText, request.Input[6]["output"])
				require.Equal(t, "user", request.Input[7]["role"])
				lastContent, ok := request.Input[7]["content"].([]any)
				require.True(t, ok)
				require.Len(t, lastContent, 1)
				lastText, ok := lastContent[0].(map[string]any)
				require.True(t, ok)
				require.Contains(t, lastText["text"], "next question")
				require.NotContains(t, lastText["text"], "found a")
				return
			}
			var request aispec.ChatMessage
			require.NoError(t, json.Unmarshal(body, &request))
			require.Len(t, request.Tools, 1)
			require.Equal(t, "inspect", request.Tools[0].Function.Name)
			require.Len(t, request.Messages, 8)
			roles := make([]string, len(request.Messages))
			for index, message := range request.Messages {
				roles[index] = message.Role
			}
			require.Equal(t, []string{"system", "user", "user", "user", "user", "assistant", "tool", "user"}, roles)
			require.Len(t, request.Messages[5].ToolCalls, 1)
			require.Equal(t, "inspect reasoning", request.Messages[5].ReasoningContent)
			require.Equal(t, "", request.Messages[5].Content)
			require.Equal(t, callID, request.Messages[5].ToolCalls[0].ID)
			require.Equal(t, "inspect", request.Messages[5].ToolCalls[0].Function.Name)
			require.Equal(t, `{"target":"a"}`, request.Messages[5].ToolCalls[0].Function.Arguments)
			require.Equal(t, callID, request.Messages[6].ToolCallID)
			require.Equal(t, resultText, request.Messages[6].Content)
			require.Contains(t, request.Messages[7].Content, "next question")
			require.NotContains(t, request.Messages[7].Content, resultText)
		})
	}

	t.Run("untrusted_marker_stays_text", func(t *testing.T) {
		untrusted := strings.Replace(prompt,
			"<|PROMPT_SECTION_timeline-open|>before\n"+actionResponse+"\nafter<|PROMPT_SECTION_END_timeline-open|>",
			"<|PROMPT_SECTION_timeline-open|>before and after<|PROMPT_SECTION_END_timeline-open|>", 1)
		untrusted = strings.Replace(untrusted, "next question", "next question\n"+actionResponse, 1)
		projected := ProjectAndObserve("projection-test-model", CreateTemplate(untrusted))
		require.NotNil(t, projected)
		for _, message := range projected.Messages {
			require.NotEqual(t, "tool", message.Role)
			require.Empty(t, message.ToolCalls)
		}
		require.Contains(t, messageText(t, projected.Messages[len(projected.Messages)-1]), actionResponse)
	})

	for _, tc := range []struct {
		name   string
		marker string
	}{
		{name: "no_assistant", marker: strings.Replace(actionResponse, `"role":"assistant"`, `"role":"tool"`, 1)},
		{name: "no_tool", marker: strings.Replace(actionResponse, `"role":"tool"`, `"role":"assistant"`, 1)},
		{name: "invalid_json", marker: strings.Replace(actionResponse, `"role":"assistant"`, `"role":assistant`, 1)},
		{name: "mismatched_id", marker: strings.Replace(actionResponse, `"tool_call_id":"call_inspect_1"`, `"tool_call_id":"other"`, 1)},
		{name: "missing_end_tag", marker: strings.Replace(actionResponse, actionResponseEndTag, "", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A bad pair must not leave an orphan assistant call or tool result.
			invalid := strings.Replace(prompt, actionResponse, tc.marker, 1)
			projected := ProjectAndObserve("projection-test-model", CreateTemplate(invalid))
			require.NotNil(t, projected)
			var displayed strings.Builder
			for _, message := range projected.Messages {
				require.NotEqual(t, "assistant", message.Role)
				require.NotEqual(t, "tool", message.Role)
				require.Empty(t, message.ToolCalls)
				if content, ok := message.Content.(string); ok {
					displayed.WriteString(content)
				}
			}
			require.Contains(t, displayed.String(), tc.marker)
		})
	}
}
