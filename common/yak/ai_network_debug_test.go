package yak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/yakscript"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
)

func TestAIGlobalNetworkDebugToggle(t *testing.T) {
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	for _, provider := range []string{"openai", "yak-script"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(httptrace.WithDirectory(context.Background(), dir), 10*time.Second)
			defer cancel()
			// This already-running session has neither a session nor model debug opt-in.
			cfg := aicommon.NewConfig(ctx, aicommon.WithPersistentSessionId(t.Name()), aicommon.WithDisableMemoryTriage(true))
			base := []aispec.AIConfigOption{aispec.WithType(provider), aispec.WithModel("debug-fixture"), aispec.WithEndpoint(server.URL), aispec.WithEnableEndpoint(true), aispec.WithBaseURL(server.URL), aispec.WithAPIKey("fixture-key"), aispec.WithDisableProviderFallback(true), aispec.WithTimeout(2)}
			if provider == "yak-script" {
				base = append(base, aispec.WithScript(strings.NewReplacer(`mode = "json"`, `mode = "sse"`, "https://model.example/v1/chat/completions", server.URL).Replace(yakscript.ExampleScript)))
			}
			callback := aicommon.AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
				return ai.Chat(prompt, append(base, opts...)...)
			})
			for index, enabled := range []bool{false, true, false} {
				consts.SetTieredAIConfig(&consts.TieredAIConfig{NetworkDebug: enabled})
				rsp, err := callback(cfg, aicommon.NewAIRequest("local network debug fixture"))
				require.NoError(t, err)
				reason, output := rsp.GetUnboundStreamReaderEx(nil, nil, nil)
				done := make(chan struct{})
				go func() { io.Copy(io.Discard, reason); close(done) }()
				body, err := io.ReadAll(output)
				require.NoError(t, err)
				<-done
				require.True(t, rsp.WaitForCallbackDone(ctx))
				require.NoError(t, rsp.GetError())
				require.Equal(t, "answer", string(body))
				files, err := filepath.Glob(filepath.Join(dir, "*", "*", "metadata.json"))
				require.NoError(t, err)
				if index == 0 {
					require.Empty(t, files)
					continue
				}
				require.Len(t, files, 1, "turning global debug off must stop recording subsequent calls")
				data, err := os.ReadFile(files[0])
				require.NoError(t, err)
				var meta httptrace.Metadata
				require.NoError(t, json.Unmarshal(data, &meta))
				require.Equal(t, t.Name(), meta.SessionID)
				require.Equal(t, provider, meta.Provider)
				require.Equal(t, 200, meta.Status)
				require.Greater(t, meta.DurationMS, float64(0))
				require.NotNil(t, meta.FirstByteMS)
				request, err := os.ReadFile(filepath.Join(filepath.Dir(files[0]), "request.http"))
				require.NoError(t, err)
				require.Contains(t, string(request), "local network debug fixture")
				response, err := os.ReadFile(filepath.Join(filepath.Dir(files[0]), "response.http"))
				require.NoError(t, err)
				require.Contains(t, string(response), "answer")
				require.Contains(t, string(response), "[DONE]")
			}
		})
	}
}

func TestAINetworkDebugCallbackAndScriptRecovery(t *testing.T) {
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{NetworkDebug: true, RetryForever: true})
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	for _, provider := range []string{"openai", "yak-script"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if provider == "yak-script" && call == 1 {
					io.WriteString(w, "data: {\"error\":{\"message\":\"raw-upstream-diagnostic\"}}\n\ndata: [DONE]\n\n")
					return
				}
				fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":%q}}]}\n\n", strings.Repeat("reason", 2000))
				io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(httptrace.WithDirectory(context.Background(), dir), 8*time.Second)
			defer cancel()
			cfg := aicommon.NewConfig(ctx, aicommon.WithPersistentSessionId(t.Name()), aicommon.WithDisableMemoryTriage(true))
			base := []aispec.AIConfigOption{aispec.WithType(provider), aispec.WithModel("local-fixture"), aispec.WithEndpoint(server.URL), aispec.WithEnableEndpoint(true), aispec.WithBaseURL(server.URL), aispec.WithAPIKey("fixture-key"), aispec.WithDisableProviderFallback(true), aispec.WithTimeout(2)}
			if provider == "yak-script" {
				base = append(base, aispec.WithScript(strings.NewReplacer(`mode = "json"`, `mode = "sse"`, "https://model.example/v1/chat/completions", server.URL).Replace(yakscript.ExampleScript)))
			}
			callback := aicommon.AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
				return ai.Chat(prompt, append(base, opts...)...)
			})
			rsp, err := callback(cfg, aicommon.NewAIRequest("local diagnostic fixture"))
			require.NoError(t, err)
			reasoning, reader := rsp.GetUnboundStreamReaderEx(nil, nil, nil)
			done := make(chan struct{})
			go func() { io.Copy(io.Discard, reasoning); close(done) }()
			output, err := io.ReadAll(reader)
			require.NoError(t, err)
			<-done
			require.True(t, rsp.WaitForCallbackDone(ctx))
			require.NoError(t, rsp.GetError())
			require.Equal(t, "answer", string(output))
			files, err := filepath.Glob(filepath.Join(dir, "*", "*", "metadata.json"))
			require.NoError(t, err)
			want := 1
			if provider == "yak-script" {
				want = 2
			}
			require.Len(t, files, want)
			require.EqualValues(t, want, calls.Load())
			ids := map[string]bool{}
			attempts := map[int]bool{}
			bodies := ""
			for _, file := range files {
				data, err := os.ReadFile(file)
				require.NoError(t, err)
				var meta httptrace.Metadata
				require.NoError(t, json.Unmarshal(data, &meta))
				require.Equal(t, t.Name(), meta.SessionID)
				require.Equal(t, cfg.GetRuntimeId(), meta.RuntimeID)
				require.Equal(t, provider, meta.Provider)
				require.Equal(t, "local-fixture", meta.Model)
				ids[meta.CallID] = true
				attempts[meta.Attempt] = true
				data, err = os.ReadFile(filepath.Join(filepath.Dir(file), "response.http"))
				require.NoError(t, err)
				bodies += string(data)
			}
			require.Len(t, ids, 1)
			if provider == "yak-script" {
				require.True(t, attempts[1] && attempts[2])
				require.Contains(t, bodies, "raw-upstream-diagnostic")
			} else {
				require.Contains(t, bodies, strings.Repeat("reason", 2000))
			}
		})
	}
}
