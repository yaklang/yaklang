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
