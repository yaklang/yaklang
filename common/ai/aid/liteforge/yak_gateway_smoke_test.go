package liteforge_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

//go:embed smoke_liteforge.yak
var publicLiteForgeSmoke string

//go:embed smoke_functioncall.yak
var functionCallSmoke string

const gatewaySmokeInput = `原样提取这段 JSON 的 summary、payload 和扩展字段，不改变值或类型：{"summary":"新底层交叉验证成功","payload":{"items":[null,true,2.5],"nested":{"source":"yak"}},"business_extension":"保留"}`

func runGatewayYak(t *testing.T, ctx context.Context, script string, opts []aispec.AIConfigOption) {
	t.Helper()
	verified := 0
	engine := yak.NewScriptEngine(1)
	engine.RegisterEngineHooks(func(e *antlr4yak.Engine) error {
		e.SetVars(map[string]any{"INPUT": gatewaySmokeInput, "OPTIONS": opts,
			"VERIFY": func(entry string, native bool, result map[string]any) {
				verified++
				require.Equal(t, "新底层交叉验证成功", result["summary"])
				encoded, err := json.Marshal(result["payload"])
				require.NoError(t, err)
				require.JSONEq(t, `{"items":[null,true,2.5],"nested":{"source":"yak"}}`, string(encoded))
				require.Equal(t, "保留", result["business_extension"])
				t.Logf("%s withFunctionCallMode=%v: summary/payload/extension PASS", entry, native)
			}})
		return nil
	})
	_, err := engine.ExecuteExWithContext(ctx, script, nil)
	require.NoError(t, err)
	require.Equal(t, 2, verified)
}

// Exercise actual Yak exports and real gateway HTTP/SSE, without replacing ai or
// LiteForge functions. The response tail is held until a stream observer reads.
func TestYakLiteForgeAndFunctionCallCrossProtocols(t *testing.T) {
	for _, tc := range []struct{ name, script string }{{"liteforge", publicLiteForgeSmoke}, {"functioncall", functionCallSmoke}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			type captured struct {
				message aispec.ChatMessage
				auth    string
			}
			requests := make(chan captured, 2)
			started := make(chan struct{}, 2)
			var mu sync.Mutex
			var observed []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var sent aispec.ChatMessage
				_ = json.NewDecoder(req.Body).Decode(&sent)
				requests <- captured{sent, req.Header.Get("Authorization")}
				w.Header().Set("Content-Type", "text/event-stream")
				write := func(delta any, finish any) {
					frame, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
					_, _ = fmt.Fprintf(w, "data: %s\n\n", frame)
					w.(http.Flusher).Flush()
				}
				native := len(sent.Tools) == 1
				prefix := `{"summary":"新底层`
				if native {
					write(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_output", "type": "function", "function": map[string]any{"name": sent.Tools[0].Function.Name, "arguments": prefix}}}}, nil)
				} else {
					prefix = `{"@action":"object","summary":"新底层`
					write(map[string]any{"content": prefix}, nil)
				}
				select {
				case <-started:
				case <-ctx.Done():
					return
				}
				tail := `交叉验证成功","payload":{"items":[null,true,2.5],"nested":{"source":"yak"}},"business_extension":"保留"}`
				if native {
					write(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": tail}}}}, nil)
					write(map[string]any{}, "tool_calls")
				} else {
					write(map[string]any{"content": tail}, "stop")
				}
				_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			options := []aispec.AIConfigOption{aispec.WithType("openai"), aispec.WithModel("yak-new-liteforge"),
				aispec.WithAPIKey("local-test-key"), aispec.WithBaseURL(server.URL + "/v1/chat/completions"),
				aispec.WithContext(ctx), aispec.WithFunctionCallRetryTimes(1), aispec.WithDisableProviderFallback(true),
				aispec.WithStreamHandler(func(reader io.Reader) {
					first := make([]byte, 1)
					n, _ := reader.Read(first)
					if n == 0 {
						return
					} // Empty ordinary content in native mode.
					started <- struct{}{}
					tail, _ := io.ReadAll(reader)
					mu.Lock()
					observed = append(observed, string(first[:n])+string(tail))
					mu.Unlock()
				})}
			runGatewayYak(t, ctx, tc.script, options)
			for _, native := range []bool{true, false} {
				sent := <-requests
				require.Equal(t, "Bearer local-test-key", sent.auth)
				require.Equal(t, "yak-new-liteforge", sent.message.Model)
				require.Equal(t, native, len(sent.message.Tools) == 1)
				require.Equal(t, "system", sent.message.Messages[0].Role)
				encoded, _ := json.Marshal(sent.message.Messages)
				require.Contains(t, string(encoded), "LiteForge")
				require.NotContains(t, string(encoded), "FUNCTION_CALL_ACTION_SCHEMA")
				if native {
					require.Equal(t, "object", sent.message.Tools[0].Function.Name)
					parameters := sent.message.Tools[0].Function.Parameters.(map[string]any)
					properties := parameters["properties"].(map[string]any)
					require.Equal(t, "string", properties["summary"].(map[string]any)["type"], "Yak typed maps must retain field schema")
					require.Empty(t, properties["payload"], "arbitrary business values remain unconstrained")
					require.NotContains(t, fmt.Sprint(sent.message.Tools[0].Function.Parameters), "@action")
					require.NotContains(t, string(encoded), `\"properties\"`)
					require.NotNil(t, sent.message.ToolChoice)
				} else {
					require.Nil(t, sent.message.ToolChoice)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			require.Len(t, observed, 2, "both raw text and native arguments must reach the script observer")
			for _, data := range observed {
				require.True(t, json.Valid([]byte(data)), data)
			}
		})
	}
}

// Optional real-model run: no credentials are committed or logged. This executes
// the same two scripts (four requests) against the user's selected provider.
func TestYakGatewayLiveSmoke(t *testing.T) {
	key := os.Getenv("LITEFORGE_SMOKE_API_KEY")
	if key == "" {
		t.Skip("set LITEFORGE_SMOKE_API_KEY to run the live Yak scripts")
	}
	provider, model := os.Getenv("LITEFORGE_SMOKE_PROVIDER"), os.Getenv("LITEFORGE_SMOKE_MODEL")
	if provider == "" {
		provider = "aibalance"
	}
	if model == "" {
		model = "deepseek-v4.1-flash"
	}
	for _, tc := range []struct{ name, script string }{{"liteforge", publicLiteForgeSmoke}, {"functioncall", functionCallSmoke}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			var mu sync.Mutex
			calls := 0
			opts := []aispec.AIConfigOption{aispec.WithType(provider), aispec.WithModel(model), aispec.WithAPIKey(key),
				aispec.WithContext(ctx), aispec.WithFunctionCallRetryTimes(1), aispec.WithDisableProviderFallback(true),
				aispec.WithRawHTTPRequestResponseCallback(func(raw, _, _ []byte, _ *aispec.ChatUsage) {
					body := strings.SplitN(string(raw), "\r\n\r\n", 2)
					if len(body) < 2 {
						return
					}
					var sent aispec.ChatMessage
					if json.Unmarshal([]byte(body[1]), &sent) != nil {
						return
					}
					mu.Lock()
					calls++
					mu.Unlock()
					t.Logf("live HTTP model=%s native=%v projected_messages=%d", sent.Model, len(sent.Tools) == 1, len(sent.Messages))
				})}
			runGatewayYak(t, ctx, tc.script, opts)
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, 2, calls, "one HTTP request per protocol")
		})
	}
}
