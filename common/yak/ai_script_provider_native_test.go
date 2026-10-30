package yak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/yakscript"
)

type scriptProtocolInvoker struct {
	*mock.MockInvoker
	timeline *aicommon.Timeline
}

func (i *scriptProtocolInvoker) AddToTimelineWithPromptProjection(kind, display, prompt string) {
	i.timeline.PushTextWithPromptProjection(i.GetConfig().AcquireId(), "["+kind+"]:\n"+display, "["+kind+"]:\n"+prompt)
}

func TestYakScriptProviderAgentProtocols(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			script := `chat = func(req, emit) {
				if len(req.Tools) == 0 { return "{\"@action\":\"accept_script\"}" }
				return {"tool_calls":[{"id":"call_accept", "type":"function", "function":{"name":"accept_script", "arguments":"{}"}}], "finish_reason":"tool_calls"}
			}`
			chat, err := ai.LoadChater(yakscript.ProviderType, aispec.WithScript(script), aispec.WithNativeToolCalling(native))
			require.NoError(t, err)
			cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithNoOpMemoryTriage(),
				aicommon.WithEnableFunctionCallMode(native), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithAICallback(aicommon.AIChatToAICallbackType(chat)))
			invoker := &scriptProtocolInvoker{MockInvoker: mock.NewMockInvoker(ctx), timeline: cfg.Timeline}
			invoker.SetConfig(cfg)
			var executed atomic.Int32
			loop, err := reactloops.NewReActLoop("script-provider-protocol", invoker, reactloops.WithFunctionCallMode(native),
				reactloops.WithAllowToolCall(false), reactloops.WithAllowRAG(false), reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
				reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableIncreaseIteration(true),
				reactloops.WithRegisterLoopAction("require_tool", "unused routing", nil, nil, nil),
				reactloops.WithRegisterLoopAction("accept_script", "Finish the test", nil, nil,
					func(_ *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
						executed.Add(1)
						op.Exit()
					}))
			require.NoError(t, err)
			defer loop.Release()
			require.NoError(t, loop.Execute("script-protocol", ctx, "finish the test"))
			require.EqualValues(t, 1, executed.Load(), "script result must execute exactly one Agent action")
		})
	}
}

func TestYakScriptProviderNativeCalls(t *testing.T) {
	tools := []aispec.Tool{{Type: "function", Function: aispec.ToolFunction{Name: "directly_answer", Parameters: map[string]any{"type": "object"}}}}
	const call = `{"id":"call_1","type":"function","function":{"name":"directly_answer","arguments":"{\"answer\":\"42\"}"}}`
	for _, tc := range []struct {
		name, value    string
		enabled, valid bool
	}{
		{"default off", `{"tool_calls":[` + call + `],"finish_reason":"tool_calls"}`, false, false},
		{"enabled", `{"content":"ready","tool_calls":[` + call + `],"finish_reason":"tool_calls"}`, true, true},
		{"truncated", `{"tool_calls":[` + call + `],"finish_reason":"length"}`, true, false},
		{"invalid arguments", `{"tool_calls":[` + strings.Replace(call, `42\"}`, `42\"`, 1) + `],"finish_reason":"tool_calls"}`, true, false},
		{"unadvertised", `{"tool_calls":[` + strings.Replace(call, "directly_answer", "unknown", 1) + `],"finish_reason":"tool_calls"}`, true, false},
		{"missing id", `{"tool_calls":[` + strings.Replace(call, "call_1", "", 1) + `],"finish_reason":"tool_calls"}`, true, false},
		{"duplicate calls", `{"tool_calls":[` + call + `,` + call + `],"finish_reason":"tool_calls"}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := fmt.Sprintf(`chat = func(req, emit) {
				assert req.Tools[0].Function.Name == "directly_answer"
				assert req.ToolChoice == "required"
				return json.loads(%q)
			}`, tc.value)
			chat, err := ai.LoadChater(yakscript.ProviderType, aispec.WithScript(script), aispec.WithNativeToolCalling(tc.enabled))
			require.NoError(t, err)
			var calls []*aispec.ToolCall
			var output, arguments strings.Builder
			var finish string
			text, err := chat("6 * 7", aispec.WithTools(tools), aispec.WithToolChoice("required"),
				aispec.WithStreamHandler(func(r io.Reader) { _, _ = io.Copy(&output, r) }),
				aispec.WithToolCallArgumentsStreamHandler(func(r io.Reader) { _, _ = io.Copy(&arguments, r) }),
				aispec.WithToolCallCallback(func(c []*aispec.ToolCall) { calls = append(calls, c...) }),
				aispec.WithFinishReasonCallback(func(s string, _ []byte) { finish = s }))
			if !tc.valid {
				require.Error(t, err)
				require.Empty(t, calls, "never commit a valid prefix of an invalid call array")
				require.Empty(t, output.String())
				require.Empty(t, arguments.String())
				require.Empty(t, finish)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "ready", text)
			require.Equal(t, text, output.String())
			require.Equal(t, `{"answer":"42"}`, arguments.String())
			require.Len(t, calls, 1)
			require.Equal(t, "call_1", calls[0].ID)
			require.Equal(t, "directly_answer", calls[0].Function.Name)
			require.Equal(t, "tool_calls", finish)
		})
	}
}

func TestYakScriptProviderNativeExample(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Tools      []aispec.Tool `json:"tools"`
			ToolChoice string        `json:"tool_choice"`
			Stream     bool          `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if len(payload.Tools) != 1 || payload.Tools[0].Function.Name != "directly_answer" || payload.ToolChoice != "auto" || payload.Stream {
			t.Errorf("unexpected native request: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call_answer","type":"function","function":{"name":"directly_answer","arguments":"{\"answer_payload\":\"42\"}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	defer server.Close()
	// Even a template set to SSE uses complete JSON for native calls.
	script := strings.Replace(yakscript.ExampleScript, "https://model.example/v1/chat/completions", server.URL, 1)
	script = strings.Replace(script, `mode = "json"`, `mode = "sse"`, 1)
	chat, err := ai.LoadChater(yakscript.ProviderType, aispec.WithScript(script), aispec.WithNativeToolCalling(true))
	require.NoError(t, err)
	var calls []*aispec.ToolCall
	var finish string
	text, err := chat("6 * 7", aispec.WithTools([]aispec.Tool{{Type: "function", Function: aispec.ToolFunction{Name: "directly_answer"}}}),
		aispec.WithToolChoice("auto"), aispec.WithToolCallCallback(func(c []*aispec.ToolCall) { calls = c }),
		aispec.WithFinishReasonCallback(func(s string, _ []byte) { finish = s }))
	require.NoError(t, err)
	require.Empty(t, text, "native arguments are not model content")
	require.Len(t, calls, 1)
	require.Equal(t, `{"answer_payload":"42"}`, calls[0].Function.Arguments)
	require.Equal(t, "tool_calls", finish)
}
