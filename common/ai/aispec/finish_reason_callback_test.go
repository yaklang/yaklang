package aispec

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFinishReasonCallbackFromProviderResponse(t *testing.T) {
	cases := []struct {
		name      string
		responses bool
		body      string
		want      string
	}{
		{
			name: "chat JSON stop", body: `{"choices":[{"message":{"content":"done"},"finish_reason":"stop"}]}`, want: "stop",
		},
		{
			name: "chat SSE tool calls", body: "data: " + `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"inspect","arguments":"{}"}}]},"finish_reason":null}]}` + "\n\n" +
				"data: " + `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n", want: "tool_calls",
		},
		{
			name: "responses JSON stop", responses: true, body: `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`, want: "stop",
		},
		{
			name: "responses SSE tool calls", responses: true, body: "data: " + `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"inspect","arguments":"{}"}}` + "\n\n" +
				"data: " + `{"type":"response.completed","response":{"status":"completed"}}` + "\n\n", want: "tool_calls",
		},
		{
			name: "responses incomplete", responses: true, body: `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}`, want: "max_output_tokens",
		},
		{
			name: "responses failed", responses: true, body: `{"status":"failed"}`, want: "failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			process := processAIResponse
			if tc.responses {
				process = processAIResponseForResponses
			}
			calls := 0
			var got string
			contentType := "application/json"
			if strings.HasPrefix(tc.body, "data: ") {
				contentType = "text/event-stream"
			}
			err := process([]byte("HTTP/1.1 200 OK\r\nContent-Type: "+contentType+"\r\n\r\n"),
				io.NopCloser(strings.NewReader(tc.body)), io.Discard, io.Discard, io.Discard,
				nil, nil, nil, nil, func(reason string, raw []byte) {
					calls++
					got = reason
					require.Equal(t, tc.body, string(raw))
				})
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Equal(t, tc.want, got)
		})
	}
}
