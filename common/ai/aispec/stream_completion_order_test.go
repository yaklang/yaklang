package aispec

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// HTTP chunks can split any JSON token. Tool deltas use the decoded reader;
// completion metadata must use that same entity body, not the chunk framing.
func TestProviderCompletionAcrossHTTPChunkBoundaries(t *testing.T) {
	for name, process := range map[string]func([]byte, io.ReadCloser, io.Writer, io.Writer, io.Writer, func([]*ToolCall), RawHTTPResponseHeaderCallback, func([]byte, []byte, *ChatUsage), func(*ChatUsage), ...func(string, []byte)) error{
		"chat": processAIResponse, "responses": processAIResponseForResponses,
	} {
		t.Run(name, func(t *testing.T) {
			body := "data: " + `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"inspect","arguments":"{}"}}]},"finish_reason":null}]}` + "\n\n" +
				"data: " + `{"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}` + "\n\ndata: [DONE]\n\n"
			if name == "responses" {
				body = "data: " + `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"inspect","arguments":"{}"}}` + "\n\n" +
					"data: " + `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15}}}` + "\n\n"
			}
			var wire bytes.Buffer
			chunked := httputil.NewChunkedWriter(&wire)
			for start := 0; start < len(body); start += 7 {
				_, err := chunked.Write([]byte(body[start:min(start+7, len(body))]))
				require.NoError(t, err)
			}
			require.NoError(t, chunked.Close())
			wire.WriteString("\r\n")
			var finish, arguments string
			var raw []byte
			var usage *ChatUsage
			err := process([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n"),
				io.NopCloser(bytes.NewReader(wire.Bytes())), io.Discard, io.Discard, io.Discard,
				func(calls []*ToolCall) {
					for _, call := range calls {
						arguments += call.Function.Arguments
					}
				}, nil, nil, func(u *ChatUsage) { usage = u },
				func(reason string, response []byte) { finish, raw = reason, response })
			require.NoError(t, err)
			require.Equal(t, "{}", arguments)
			assert.Equal(t, "tool_calls", finish)
			assert.Equal(t, wire.String(), string(raw), "retain raw transport bytes for diagnostics")
			require.NotNil(t, usage)
			assert.Equal(t, 12, usage.PromptTokens)
		})
	}
}

type completionObservedWriter struct {
	bytes.Buffer
	onClose func()
}

func (w *completionObservedWriter) Close() error { w.onClose(); return nil }

// A consumer can act as soon as any pipe closes. Observe completion at that
// exact boundary, not after processAIResponse returns: the old defer order
// deterministically exposes an empty finish reason here (production request 17).
func TestProviderCompletionPublishedBeforeStreamEOF(t *testing.T) {
	for name, process := range map[string]func([]byte, io.ReadCloser, io.Writer, io.Writer, io.Writer, func([]*ToolCall), RawHTTPResponseHeaderCallback, func([]byte, []byte, *ChatUsage), func(*ChatUsage), ...func(string, []byte)) error{
		"chat": processAIResponse, "responses": processAIResponseForResponses,
	} {
		t.Run(name, func(t *testing.T) {
			body := "data: " + `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"inspect","arguments":"{}"}}]},"finish_reason":null}]}` + "\n\n" +
				"data: " + `{"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}` + "\n\ndata: [DONE]\n\n"
			if name == "responses" {
				body = "data: " + `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"inspect","arguments":"{}"}}` + "\n\n" +
					"data: " + `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15}}}` + "\n\n"
			}
			var finish string
			var raw []byte
			var usageReady bool
			var closes, completions int
			writer := func() *completionObservedWriter {
				return &completionObservedWriter{onClose: func() {
					closes++
					assert.Equal(t, "tool_calls", finish, "EOF must not precede terminal metadata")
					assert.Equal(t, body, string(raw))
				}}
			}
			err := process([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n"),
				io.NopCloser(strings.NewReader(body)), writer(), writer(), writer(), nil, nil, nil,
				func(*ChatUsage) {
					assert.Equal(t, 3, closes, "usage callbacks retain their after-EOF timing")
					usageReady = true
				},
				func(reason string, response []byte) { finish, raw = reason, response; completions++ })
			require.NoError(t, err)
			require.Equal(t, 3, closes)
			require.Equal(t, 1, completions)
			require.True(t, usageReady)
		})
	}
}

func TestProviderCompletionPanicStillClosesStreams(t *testing.T) {
	for name, process := range map[string]func([]byte, io.ReadCloser, io.Writer, io.Writer, io.Writer, func([]*ToolCall), RawHTTPResponseHeaderCallback, func([]byte, []byte, *ChatUsage), func(*ChatUsage), ...func(string, []byte)) error{
		"chat": processAIResponse, "responses": processAIResponseForResponses,
	} {
		t.Run(name, func(t *testing.T) {
			var closes int
			var usageCalled bool
			writer := func() *completionObservedWriter {
				return &completionObservedWriter{onClose: func() { closes++ }}
			}
			require.Panics(t, func() {
				_ = process([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
					io.NopCloser(strings.NewReader(`{}`)), writer(), writer(), writer(), nil, nil, nil,
					func(*ChatUsage) { usageCalled = true }, func(string, []byte) { panic("callback failure") })
			})
			require.Equal(t, 3, closes, "moving completion before EOF must not strand readers on panic")
			require.True(t, usageCalled)
		})
	}
}

// Exercise the real HTTP reader and concurrent content/reason consumers. A
// usage callback deliberately waits for EOF observations, exposing the old
// close -> usage -> finish ordering without scheduler-dependent sleeps.
func TestChatBaseConsumerEOFSeesProviderCompletion(t *testing.T) {
	for _, protocol := range []ChatBaseInterfaceType{ChatBaseInterfaceTypeChatCompletions, ChatBaseInterfaceTypeResponses} {
		t.Run(string(protocol), func(t *testing.T) {
			body := "data: " + `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"inspect","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}` + "\n\ndata: [DONE]\n\n"
			if protocol == ChatBaseInterfaceTypeResponses {
				body = "data: " + `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"inspect","arguments":"{}"}}` + "\n\n" +
					"data: " + `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15}}}` + "\n\n"
			}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			var finish atomic.Value
			finish.Store("")
			observed := make(chan string, 2)
			usageDone := make(chan struct{})
			consumer := func(reader io.Reader) {
				_, _ = io.Copy(io.Discard, reader)
				observed <- finish.Load().(string)
			}
			_, err := ChatBase(server.URL, "completion-order-test", "inspect", WithChatBase_InterfaceType(protocol),
				WithChatBase_StreamHandler(consumer), WithChatBase_ReasonStreamHandler(consumer),
				WithChatBase_ToolCallCallback(func([]*ToolCall) {}),
				WithChatBase_FinishReasonCallback(func(reason string, _ []byte) { finish.Store(reason) }),
				WithChatBase_UsageCallback(func(*ChatUsage) {
					defer close(usageDone)
					for i := 0; i < 2; i++ {
						select {
						case reason := <-observed:
							assert.Equal(t, "tool_calls", reason, "consumer must see completion at EOF")
						case <-time.After(3 * time.Second):
							t.Error("usage callback could not observe closed output readers")
							return
						}
					}
				}),
				WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }))
			require.NoError(t, err)
			select {
			case <-usageDone:
			case <-time.After(3 * time.Second):
				t.Fatal("completion callbacks did not finish")
			}
			require.EqualValues(t, 1, requests.Load())
		})
	}
}
