package aispec

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviderFinishReasonAndOptionalToolDescription(t *testing.T) {
	t.Run("chat completions SSE", func(t *testing.T) {
		body := "data: " + `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","title":"Inspect input","function":{"name":"inspect","arguments":"{}"}}]},"finish_reason":null}]}` + "\n\n" +
			"data: " + `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n" +
			"data: [DONE]\n\n"
		var finish, description string
		err := processAIResponse([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n"),
			io.NopCloser(strings.NewReader(body)), io.Discard, io.Discard, io.Discard,
			func(calls []*ToolCall) { description = calls[0].Description }, nil, nil, nil,
			func(reason string, raw []byte) { finish = reason; require.Equal(t, body, string(raw)) })
		require.NoError(t, err)
		require.Equal(t, "tool_calls", finish)
		require.Equal(t, "Inspect input", description)
	})

	t.Run("chat completions JSON", func(t *testing.T) {
		body := `{"choices":[{"message":{"content":"done"},"finish_reason":"stop"}]}`
		var finish string
		err := processAIResponse([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
			io.NopCloser(strings.NewReader(body)), io.Discard, io.Discard, io.Discard,
			nil, nil, nil, nil, func(reason string, raw []byte) { finish = reason; require.Equal(t, body, string(raw)) })
		require.NoError(t, err)
		require.Equal(t, "stop", finish)
	})

	t.Run("responses SSE", func(t *testing.T) {
		body := "data: " + `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"inspect","arguments":"{}","summary":[{"text":"Inspect input"}]}}` + "\n\n" +
			"data: " + `{"type":"response.completed","response":{"status":"completed"}}` + "\n\n"
		var finish, description string
		err := processAIResponseForResponses([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n"),
			io.NopCloser(strings.NewReader(body)), io.Discard, io.Discard, io.Discard,
			func(calls []*ToolCall) { description = calls[0].Description }, nil, nil, nil,
			func(reason string, raw []byte) { finish = reason; require.Equal(t, body, string(raw)) })
		require.NoError(t, err)
		require.Equal(t, "tool_calls", finish)
		require.Equal(t, "Inspect input", description)
	})
}

func TestResponsesInterleavedToolCallsKeepIndexAndLateMetadata(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_0","call_id":"call_0","name":"alpha"}}`,
		`data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"beta"}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_0","delta":"{\"a\":"}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":1,"item_id":"fc_1","delta":"{\"b\":2}"}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_0","delta":"1}"}`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_0","call_id":"call_0","name":"alpha","title":"Alpha summary","arguments":"{\"a\":1}"}}`,
		`data: {"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"beta","arguments":"{\"b\":2}"}}`,
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
	}, "\n\n") + "\n\n"
	type observed struct{ id, name, description, args string }
	got := map[int]*observed{}
	var finish string
	err := processAIResponseForResponses([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n"),
		io.NopCloser(strings.NewReader(body)), io.Discard, io.Discard, io.Discard,
		func(calls []*ToolCall) {
			for _, call := range calls {
				entry := got[call.Index]
				if entry == nil {
					entry = &observed{}
					got[call.Index] = entry
				}
				if call.ID != "" {
					entry.id = call.ID
				}
				if call.Function.Name != "" {
					entry.name = call.Function.Name
				}
				if call.Description != "" {
					entry.description = call.Description
				}
				entry.args += call.Function.Arguments
			}
		}, nil, nil, nil,
		func(reason string, _ []byte) { finish = reason })
	require.NoError(t, err)
	require.Equal(t, "tool_calls", finish)
	require.Equal(t, map[int]*observed{
		0: {id: "call_0", name: "alpha", description: "Alpha summary", args: `{"a":1}`},
		1: {id: "call_1", name: "beta", args: `{"b":2}`},
	}, got)
}

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
