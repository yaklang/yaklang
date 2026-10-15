package aispec

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type promptOverflowResponseProcessor func([]byte, io.ReadCloser, io.Writer, io.Writer, io.Writer, func([]*ToolCall), RawHTTPResponseHeaderCallback, func([]byte, []byte, *ChatUsage), func(*ChatUsage), ...func(string, []byte)) error

func TestPromptOverflowResponseProtocols(t *testing.T) {
	for name, process := range map[string]promptOverflowResponseProcessor{
		"chat": processAIResponse, "responses": processAIResponseForResponses,
	} {
		for _, tc := range []struct {
			name        string
			status      int
			contentType string
			body        string
			partial     bool
			chunked     bool
		}{
			{"http_400", 400, "application/json", `{"error":{"code":"context_length_exceeded","message":"prompt is too long"}}`, false, false},
			{"http_413_chunked", 413, "application/json", `{"error":{"message":"prompt is too long"}}`, false, true},
			{"http_422", 422, "application/json", `{"error":{"message":"Prompt 超长"}}`, false, false},
			{"json_error", 200, "application/json", `{"error":{"code":"context_length_exceeded","message":"prompt is too long"}}`, false, false},
			{"sse_error", 200, "text/event-stream", "data: " + `{"error":{"code":"context_length_exceeded","message":"prompt is too long"}}` + "\n\n", false, false},
			{"sse_201_error", 201, "text/event-stream", "data: " + `{"error":{"message":"prompt is too long"}}` + "\n\n", false, false},
			{"sse_flat_error", 200, "text/event-stream", "data: " + `{"type":"error","code":"context_length_exceeded","message":"prompt is too long"}` + "\n\n", false, false},
			{"responses_failed", 200, "text/event-stream", "data: " + `{"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded","message":"prompt is too long"}}}` + "\n\n", false, false},
			{"sse_error_after_partial_output", 200, "text/event-stream", "data: " + `{"error":{"message":"prompt is too long"}}` + "\n\n", true, false},
			{"sse_error_after_preview_limit", 200, "text/event-stream", ": " + strings.Repeat("x", 5000) + "\n\ndata: " + `{"error":{"message":"prompt is too long"}}` + "\n\n", false, false},
			{"sse_error_chunked", 200, "text/event-stream", "data: " + `{"error":{"message":"prompt is too long"}}` + "\n\n", false, true},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				body := tc.body
				if tc.partial {
					prefix := `{"choices":[{"delta":{"content":"partial"}}]}`
					if name == "responses" {
						prefix = `{"type":"response.output_text.delta","delta":"partial"}`
					}
					body = "data: " + prefix + "\n\n" + body
				}
				header := fmt.Sprintf("HTTP/1.1 %d Status\r\nContent-Type: %s\r\n", tc.status, tc.contentType)
				if tc.chunked {
					var wire bytes.Buffer
					writer := httputil.NewChunkedWriter(&wire)
					for pos := 0; pos < len(body); pos += 7 {
						_, err := io.WriteString(writer, body[pos:min(pos+7, len(body))])
						require.NoError(t, err)
					}
					require.NoError(t, writer.Close())
					wire.WriteString("\r\n")
					body = wire.String()
					header += "Transfer-Encoding: chunked\r\n"
				}
				var output, reason bytes.Buffer
				var preview []byte
				err := process([]byte(header+"\r\n"), io.NopCloser(strings.NewReader(body)), &output, &reason,
					nil, nil, nil, func(_, b []byte, _ *ChatUsage) { preview = append([]byte(nil), b...) }, nil)
				require.ErrorIs(t, err, ErrPromptOverflow)
				var overflow *PromptOverflowError
				require.ErrorAs(t, err, &overflow)
				require.Equal(t, tc.status, overflow.HTTPStatus)
				require.NotEmpty(t, overflow.MatchedPattern)
				if tc.name == "http_422" {
					// This fixture intentionally retains the original Chinese text.
					require.Contains(t, overflow.Message, "超长")
				} else {
					require.Contains(t, err.Error(), "long")
				}
				require.LessOrEqual(t, len(preview), 4096)
				if tc.partial {
					require.Equal(t, "partial", output.String())
				} else {
					require.Empty(t, output.String(), "error bodies must not become model output")
				}
			})
		}
	}
}

func TestPromptOverflowQuotedErrorInOutputIsNotProtocolError(t *testing.T) {
	for name, process := range map[string]promptOverflowResponseProcessor{
		"chat": processAIResponse, "responses": processAIResponseForResponses,
	} {
		t.Run(name, func(t *testing.T) {
			const content = `{"error":{"code":"context_length_exceeded","message":"prompt is too long"}}`
			body := fmt.Sprintf(`{"choices":[{"delta":{"content":%q},"finish_reason":"length"}]}`, content)
			if name == "responses" {
				body = fmt.Sprintf(`{"type":"response.output_text.delta","delta":%q}`, content)
			}
			var output bytes.Buffer
			err := process([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n"),
				io.NopCloser(strings.NewReader("data: "+body+"\n\n")), &output, io.Discard, nil, nil, nil, nil, nil)
			require.NoError(t, err)
			require.Equal(t, content, output.String())
		})
	}
}

func TestPromptOverflowStreamErrorIsNotAlwaysOverflow(t *testing.T) {
	for name, process := range map[string]promptOverflowResponseProcessor{
		"chat": processAIResponse, "responses": processAIResponseForResponses,
	} {
		for _, body := range []string{
			`{"error":{"code":"rate_limit_exceeded","message":"rate limit exceeded"}}`,
			`{"type":"error","code":"invalid_api_key","message":"invalid api key"}`,
			`{"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"upstream failed"}}}`,
			`{"type":"response.failed","response":{"status":"failed"}}`,
		} {
			t.Run(name+"/"+body, func(t *testing.T) {
				err := process([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n"),
					io.NopCloser(strings.NewReader("data: "+body+"\n\n")), io.Discard, io.Discard, nil, nil, nil, nil, nil)
				require.NoError(t, err)
				require.NotErrorIs(t, err, ErrPromptOverflow)
			})
		}
	}
}

func TestPromptOverflowHTTPErrorBodyRemainsBounded(t *testing.T) {
	body := `{"error":{"message":"prompt is too long ` + strings.Repeat("x", 8000)
	var captured bytes.Buffer
	err := readAIHTTPError(400, strings.NewReader(body), false, &captured)
	require.ErrorIs(t, err, ErrPromptOverflow)
	require.Equal(t, 4096, captured.Len())
}

func TestPromptOverflowChatBaseErrorChain(t *testing.T) {
	for _, protocol := range []ChatBaseInterfaceType{ChatBaseInterfaceTypeChatCompletions, ChatBaseInterfaceTypeResponses} {
		for _, streaming := range []bool{false, true} {
			for _, status := range []int{200, 400} {
				t.Run(fmt.Sprintf("%s/stream=%t/status=%d", protocol, streaming, status), func(t *testing.T) {
					const payload = `{"error":{"code":"context_length_exceeded","message":"prompt is too long"}}`
					body, contentType := payload, "application/json"
					if streaming && status == 200 {
						body, contentType = "data: "+payload+"\n\n", "text/event-stream"
					}
					var requests atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests.Add(1)
						_, _ = io.Copy(io.Discard, r.Body)
						w.Header().Set("Content-Type", contentType)
						w.WriteHeader(status)
						_, _ = io.WriteString(w, body)
					}))
					defer server.Close()

					output, err := ChatBase(server.URL, "overflow-test", "test",
						WithChatBase_InterfaceType(protocol), WithChatBase_DisableStream(!streaming),
						WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }))
					require.ErrorIs(t, err, ErrPromptOverflow)
					var overflow *PromptOverflowError
					require.ErrorAs(t, err, &overflow)
					require.Equal(t, "context_length_exceeded", overflow.Code)
					require.Empty(t, output)
					require.EqualValues(t, 1, requests.Load())
				})
			}
		}
	}
}
