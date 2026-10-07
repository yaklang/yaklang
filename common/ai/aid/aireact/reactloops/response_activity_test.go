package reactloops

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type activityStatusCapture struct {
	mu       sync.Mutex
	statuses []aicommon.StatusPayload
	events   []*schema.AiOutputEvent
	changed  chan struct{}
}

func captureActivityStatus(loop *ReActLoop) *activityStatusCapture {
	c := &activityStatusCapture{changed: make(chan struct{}, 1)}
	loop.emitter = aicommon.NewEmitter("activity-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		copy := *event
		copy.Content = append([]byte(nil), event.Content...)
		copy.StreamDelta = append([]byte(nil), event.StreamDelta...)
		c.mu.Lock()
		c.events = append(c.events, &copy)
		c.mu.Unlock()
		if event.NodeId == "status" {
			var status aicommon.StatusPayload
			if err := json.Unmarshal(event.Content, &status); err != nil {
				return nil, err
			}
			c.mu.Lock()
			c.statuses = append(c.statuses, status)
			c.mu.Unlock()
		}
		select {
		case c.changed <- struct{}{}:
		default:
		}
		return event, nil
	})
	return c
}

func (c *activityStatusCapture) waitStream(node, content string) bool {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		c.mu.Lock()
		var text strings.Builder
		for _, event := range c.events {
			if event.IsStream && event.NodeId == node {
				text.Write(event.StreamDelta)
			}
		}
		found := strings.Contains(text.String(), content)
		c.mu.Unlock()
		if found {
			return true
		}
		select {
		case <-c.changed:
		case <-deadline.C:
			return false
		}
	}
}

func (c *activityStatusCapture) snapshot() []aicommon.StatusPayload {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]aicommon.StatusPayload(nil), c.statuses...)
}

func (c *activityStatusCapture) wait(predicate func(aicommon.StatusPayload) bool) bool {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		for _, status := range c.snapshot() {
			if predicate(status) {
				return true
			}
		}
		select {
		case <-c.changed:
		case <-deadline.C:
			return false
		}
	}
}

func TestResponseActivityStreamsToolNamesBeforeArgumentsFinish(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "native"}[native], func(t *testing.T) {
			var capture *activityStatusCapture
			const partial = `"directly_call_tool_params_group":[{"tool_name":"read_file","params":{"tool_name":"business_value","file":"a"}},{"tool_name":"read_file","params":{"file":"b"}},{"tool_name":"grep","params":{`
			const tail = `"pattern":"needle"}}]}`
			var raw string
			loop := newCallAILoopTransactionTestLoop(t, native, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
				resp := aicommon.NewUnboundAIResponse()
				if native {
					raw = "{" + partial + tail
					cfg.ToolCallCallback([]*aispec.ToolCall{{ID: "group", Type: "function", Function: aispec.FuncReturn{Name: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, Arguments: "{" + partial}}})
					if !capture.wait(func(s aicommon.StatusPayload) bool {
						return s.Value == "正在调用read_file，grep"
					}) {
						resp.Close()
						return resp, io.ErrNoProgress
					}
					cfg.ToolCallCallback([]*aispec.ToolCall{{ID: "group", Function: aispec.FuncReturn{Arguments: tail}}})
					cfg.FinishReasonCallback("tool_calls", nil)
					resp.Close()
				} else {
					raw = `{"@action":"directly_call_tool",` + partial + tail
					reader, writer := io.Pipe()
					resp.EmitOutputStream(reader)
					go func() {
						defer writer.Close()
						defer resp.Close()
						_, _ = io.WriteString(writer, `{"@action":"directly_call_tool",`+partial)
						if !capture.wait(func(s aicommon.StatusPayload) bool {
							return s.Value == "正在调用read_file，grep"
						}) {
							return
						}
						_, _ = io.WriteString(writer, tail)
						cfg.FinishReasonCallback("stop", nil)
					}()
				}
				return resp, nil
			})
			loop.actions.Set(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, &LoopAction{ActionType: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
				ActionVerifier: func(_ *ReActLoop, action *aicommon.Action) error {
					group := action.GetParams()["directly_call_tool_params_group"].([]any)
					require.Len(t, group, 3, "dedup is display-only")
					require.Equal(t, "business_value", group[0].(map[string]any)["params"].(map[string]any)["tool_name"])
					return nil
				}})
			capture = captureActivityStatus(loop)
			calls, _, descriptor, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "unchanged prompt", "nonce", nil,
				func(io.Reader, io.Reader) {}, func(_, _ string, description, arguments io.Reader) {
					_, _ = io.Copy(io.Discard, description)
					_, _ = io.Copy(io.Discard, arguments)
				})
			require.NoError(t, err)
			require.Len(t, calls, 1)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			require.True(t, descriptor.WaitComplete(ctx))
			if native {
				require.Equal(t, raw, calls[0].ArgumentsJSON)
			}
			var preparations []aicommon.StatusPayload
			for _, status := range capture.snapshot() {
				if status.Code == "action.preparing" {
					preparations = append(preparations, status)
				}
				require.NotContains(t, status.Value, "business_value")
				require.NotEqual(t, "reasoning.thinking", status.Code, "no fabricated Reason")
			}
			require.Len(t, preparations, 3, "generic action, first tool, second unique tool; no repeated read_file")
			require.Len(t, preparations[2].Tools, 2)
			require.Equal(t, "Calling: read_file, grep", preparations[2].ValueI18n.En)
		})
	}
}

func TestResponseActivityDedupReasonThrottleAndRetirement(t *testing.T) {
	loop := newCallAILoopTransactionTestLoop(t, true, nil)
	loop.actions.Set("custom", &LoopAction{ActionType: "custom", VerboseNameI18n: &schema.I18n{Zh: "核对结论", En: "checking conclusions"}})
	loop.actions.Set(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, &LoopAction{ActionType: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL})
	capture := captureActivityStatus(loop)
	a := newResponseActivity(loop, nil)
	a.reason(nil)
	a.reason([]byte(" \n"))
	require.Empty(t, capture.snapshot())
	a.reason([]byte("real Reason"))
	a.reason([]byte("more Reason"))
	require.Len(t, capture.snapshot(), 1, "do not refresh per token")
	require.Equal(t, "reasoning.thinking", capture.snapshot()[0].Code)
	a.mu.Lock()
	a.lastReason = time.Now().Add(-7 * time.Second)
	a.mu.Unlock()
	a.reason([]byte("continuing Reason"))
	require.Len(t, capture.snapshot(), 2)
	require.NotEqual(t, capture.snapshot()[0].Value, capture.snapshot()[1].Value)
	a.prepare("first", schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
	a.field("first", "directly_call_tool_name", "read_file", nil)
	a.prepare("second", schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
	a.field("second", "directly_call_tool_name", "read_file", nil)
	a.prepare("third", "custom")
	statuses := capture.snapshot()
	require.Len(t, statuses, 5, "duplicate action/tool names do not flicker")
	require.Equal(t, "正在调用read_file，核对结论", statuses[4].Value)
	require.Equal(t, "Calling: read_file, checking conclusions", statuses[4].ValueI18n.En)
	a.close()
	a.prepare("late", "custom")
	a.field("first", "directly_call_tool_name", "late_tool", nil)
	a.reason([]byte("late Reason"))
	require.Len(t, capture.snapshot(), 5, "late streams cannot overwrite verification/execution")
	fresh := newResponseActivity(loop, nil)
	fresh.prepare("retry", "custom")
	require.Equal(t, "正在调用核对结论", capture.snapshot()[5].Value, "retry does not keep previous names")
}

func TestActionVerboseNamesDoNotChangeModelSchema(t *testing.T) {
	loop := newCallAILoopTransactionTestLoop(t, true, nil)
	loop.loopActions.Set("display_only", func(aicommon.AIInvokeRuntime) (*LoopAction, error) {
		t.Fatal("status rendering must not instantiate a dynamic action")
		return nil, nil
	})
	activity := newResponseActivity(loop, []string{"display_only"})
	capture := captureActivityStatus(loop)
	activity.prepare("display", "display_only")
	require.Len(t, capture.snapshot(), 1)
	activity.close()
	tool := aitool.NewWithoutCallback("external_tool", aitool.WithVerboseNameZh("读取文件"), aitool.WithVerboseName("Read file"))
	action := ConvertAIToolToLoopAction(tool)
	require.Equal(t, schema.I18n{Zh: "读取文件", En: "Read file"}, action.GetVerboseNameI18n())
	beforeText := buildSchema(action)
	beforeNative, err := buildActionTools([]*LoopAction{action}, 8)
	require.NoError(t, err)
	action.VerboseNameI18n = &schema.I18n{Zh: "新展示名", En: "New display name"}
	afterNative, err := buildActionTools([]*LoopAction{action}, 8)
	require.NoError(t, err)
	require.Equal(t, beforeText, buildSchema(action))
	require.Equal(t, beforeNative, afterNative, "UI metadata must not disrupt schema cache prefixes")
	for name := range actionStatusNames {
		label := (&LoopAction{ActionType: name}).GetVerboseNameI18n()
		require.NotEmpty(t, strings.TrimSpace(label.Zh))
		require.NotEmpty(t, strings.TrimSpace(label.En))
	}
}

func TestResponseActivityBeforeRealCallerReturns(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "native"}[native], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var capture *activityStatusCapture
			callback := aicommon.AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
				if prompt != "unchanged prompt" {
					return "", io.ErrUnexpectedEOF
				}
				wire := aispec.NewDefaultAIConfig(opts...)
				reason, writer := io.Pipe()
				defer writer.Close()
				wire.ReasonStreamHandler(reason)
				// Real providers register stream readers before HTTP headers.
				wire.RawHTTPResponseHeaderCallback([]byte("HTTP/1.1 200 OK\r\n\r\n"))
				if _, err := io.WriteString(writer, "分析这次操作的依据"); err != nil {
					return "", err
				}
				if !capture.wait(func(s aicommon.StatusPayload) bool { return s.Code == "reasoning.thinking" }) {
					return "", io.ErrNoProgress
				}
				_ = writer.Close()
				const partial = `"directly_call_tool_name":"read_file","directly_call_tool_params":{`
				const tail = `"file":"sample.txt"}}`
				var outputWriter *io.PipeWriter
				if native {
					wire.ToolCallCallback([]*aispec.ToolCall{{ID: "live", Type: "function", Function: aispec.FuncReturn{Name: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, Arguments: "{" + partial}}})
				} else {
					output, writer := io.Pipe()
					outputWriter = writer
					defer writer.Close()
					wire.StreamHandler(output)
					if _, err := io.WriteString(writer, `{"@action":"directly_call_tool",`+partial); err != nil {
						return "", err
					}
				}
				if !capture.wait(func(s aicommon.StatusPayload) bool { return s.Value == "正在调用读取文件（read_file）" }) {
					return "", io.ErrNoProgress
				}
				if native {
					wire.ToolCallCallback([]*aispec.ToolCall{{ID: "live", Function: aispec.FuncReturn{Arguments: tail}}})
					wire.FinishReasonCallback("tool_calls", nil)
				} else {
					if _, err := io.WriteString(outputWriter, tail); err != nil {
						return "", err
					}
					wire.FinishReasonCallback("stop", nil)
				}
				return "", nil
			})
			cfg := aicommon.NewConfig(ctx, aicommon.WithAICallback(callback), aicommon.WithDebug(false), aicommon.WithAITransactionAutoRetry(1), aicommon.WithAIAutoRetry(1),
				aicommon.WithTools(aitool.NewWithoutCallback("read_file", aitool.WithVerboseNameZh("读取文件"), aitool.WithVerboseName("Read file"))))
			invoker := mock.NewMockInvoker(ctx)
			invoker.SetConfig(cfg)
			loop := NewMinimalReActLoop(cfg, invoker)
			loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
			loop.functionCallMode = native
			loop.actions.Set(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, &LoopAction{ActionType: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL})
			capture = captureActivityStatus(loop)
			calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "unchanged prompt", "nonce", nil,
				func(io.Reader, io.Reader) {}, func(_, _ string, description, arguments io.Reader) {
					_, _ = io.Copy(io.Discard, description)
					_, _ = io.Copy(io.Discard, arguments)
				})
			require.NoError(t, err)
			require.Len(t, calls, 1)
			require.Equal(t, "sample.txt", calls[0].Action.GetParams()["directly_call_tool_params"].(map[string]any)["file"])
			var thinkingCount int
			for _, status := range capture.snapshot() {
				t.Logf("%s | %s | %s", status.Code, status.Value, status.ValueI18n.En)
				require.NotEqual(t, "action.parsing", status.Code, "internal parsing must not interrupt the activity indicator")
				require.NotEqual(t, "action.ready", status.Code, "generic readiness must not replace streamed tool names")
				if status.Code == "reasoning.thinking" {
					thinkingCount++
				}
			}
			require.Equal(t, 1, thinkingCount, "buffered replay must not generate additional thinking statuses")
		})
	}
}

func TestResponseWaitingStopsOnActualOutput(t *testing.T) {
	loop := newCallAILoopTransactionTestLoop(t, true, nil)
	loop.actions.Set("accept", &LoopAction{ActionType: "accept"})
	capture := captureActivityStatus(loop)
	for _, output := range []string{"reason", "content", "function"} {
		a := newResponseActivity(loop, nil)
		a.wait(context.Background())
		statuses := capture.snapshot()
		require.Equal(t, "response.waiting", statuses[len(statuses)-1].Code)
		replacement := newResponseActivity(loop, nil)
		replacement.inheritWaiting(a)
		replacement.wait(context.Background())
		require.Len(t, capture.snapshot(), len(statuses), "header bookkeeping does not flash another waiting phrase")
		replacement.close()
		select {
		case <-a.stopWaiting:
			t.Fatal("waiting stopped before data")
		default:
		}
		switch output {
		case "reason":
			a.reason([]byte("real reason"))
		case "content":
			a.content()
		case "function":
			a.prepare("call", "accept")
		}
		statuses = capture.snapshot()
		require.NotEqual(t, "response.waiting", statuses[len(statuses)-1].Code)
		select {
		case <-a.stopWaiting:
		default:
			t.Fatal("output must stop the waiting timer immediately")
		}
		a.wait(context.Background())
		require.Len(t, capture.snapshot(), len(statuses), "no late waiting over real activity")
		a.close()
	}
}

func TestNativeActivityDisplaysOpenReasonContentAndNameOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var capture *activityStatusCapture
	callback := aicommon.AIChatToAICallbackType(func(_ string, opts ...aispec.AIConfigOption) (string, error) {
		wire := aispec.NewDefaultAIConfig(opts...)
		if !capture.wait(func(s aicommon.StatusPayload) bool { return s.Code == "response.waiting" }) {
			return "", io.ErrNoProgress
		}
		reason, reasonWriter := io.Pipe()
		content, contentWriter := io.Pipe()
		defer reasonWriter.Close()
		defer contentWriter.Close()
		// Both readers are registered while HTTP is still waiting.
		wire.ReasonStreamHandler(reason)
		wire.StreamHandler(content)
		wire.RawHTTPResponseHeaderCallback([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		_, _ = io.WriteString(reasonWriter, "正在核对输入依据")
		if !capture.wait(func(s aicommon.StatusPayload) bool { return s.Code == "reasoning.thinking" }) ||
			!capture.waitStream("re-act-loop-thought", "正在核对输入依据") {
			return "", io.ErrNoProgress
		}
		_, _ = io.WriteString(contentWriter, "## 执行进展\n第一段")
		if !capture.waitStream("", "## 执行进展\n第一段") {
			return "", io.ErrNoProgress
		}
		// The ID and argument reader have not arrived yet.
		wire.ToolCallCallback([]*aispec.ToolCall{{Index: 0, Function: aispec.FuncReturn{Name: "accept"}}})
		if !capture.wait(func(s aicommon.StatusPayload) bool {
			return s.Code == "action.preparing" && strings.Contains(s.Value, "核对结果")
		}) {
			return "", io.ErrNoProgress
		}
		// Overlapping Reason must not overwrite the newer call status.
		_, _ = io.WriteString(reasonWriter, "后续依据")
		_, _ = io.WriteString(contentWriter, "，第二段。")
		wire.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "late-id", Type: "function", Function: aispec.FuncReturn{Arguments: `{"value":"complete"}`}}})
		wire.FinishReasonCallback("tool_calls", nil)
		return "", nil
	})
	cfg := aicommon.NewConfig(ctx, aicommon.WithAICallback(callback), aicommon.WithDebug(false), aicommon.WithAITransactionAutoRetry(1), aicommon.WithAIAutoRetry(1))
	invoker := mock.NewMockInvoker(ctx)
	invoker.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, invoker)
	loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
	loop.functionCallMode = true
	loop.actions.Set("accept", &LoopAction{ActionType: "accept", VerboseNameI18n: &schema.I18n{Zh: "核对结果", En: "checking results"}})
	capture = captureActivityStatus(loop)
	calls, _, descriptor, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil, loop.emitLoopGeneralOutput, loop.emitLoopFunctionCallOutput)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	require.Equal(t, "late-id", calls[0].ToolCallID)
	require.Equal(t, "complete", calls[0].Action.GetString("value"))
	require.True(t, descriptor.WaitComplete(ctx))
	var text strings.Builder
	var starts, thoughts int
	for _, event := range capture.events {
		if event.NodeId == "" && event.Type == schema.EVENT_TYPE_STREAM_START {
			starts++
		}
		if event.NodeId == "" && event.IsStream {
			require.Equal(t, aicommon.TypeTextMarkdown, event.ContentType)
			text.Write(event.StreamDelta)
		}
	}
	require.Equal(t, 1, starts, "one live Markdown stream, no buffered replay")
	require.Equal(t, "## 执行进展\n第一段，第二段。", text.String())
	for _, status := range capture.snapshot() {
		if status.Code == "reasoning.thinking" {
			thoughts++
		}
	}
	require.Equal(t, 1, thoughts, "late or replayed Reason cannot hide the call")
	var replay map[string]any
	require.NoError(t, json.Unmarshal([]byte(descriptor.Snapshot().ResponseJSON), &replay))
	require.Equal(t, text.String(), replay["content"], "canonical response capture remains complete")
	require.Equal(t, "正在核对输入依据后续依据", replay["reasoning_content"])
}

func TestNativeActivityHTTPFirstDeltas(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var capture *activityStatusCapture
	stages := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flush := w.(http.Flusher)
		flush.Flush()
		send := func(delta string) { _, _ = io.WriteString(w, "data: "+delta+"\n\n"); flush.Flush() }
		send(`{"choices":[{"delta":{"reasoning_content":"核对真实流"}}]}`)
		if !capture.waitStream("re-act-loop-thought", "核对真实流") ||
			!capture.wait(func(s aicommon.StatusPayload) bool { return s.Code == "reasoning.thinking" }) {
			stages <- "reason"
			return
		}
		send(`{"choices":[{"delta":{"content":"## 正文首段"}}]}`)
		if !capture.waitStream("", "## 正文首段") {
			stages <- "content"
			return
		}
		send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"accept"}}]}}]}`)
		if !capture.wait(func(s aicommon.StatusPayload) bool {
			return s.Code == "action.preparing" && strings.Contains(s.Value, "核对结果")
		}) {
			stages <- "function name"
			return
		}
		send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"http-call","type":"function","function":{"arguments":"{\"value\":\"ok\"}"}}]},"finish_reason":"tool_calls"}]}`)
		send(`[DONE]`)
		stages <- "complete"
	}))
	defer server.Close()
	callback := aicommon.AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		wire := aispec.NewDefaultAIConfig(opts...)
		return aispec.ChatBase(server.URL+"/v1/chat/completions", "stream-fixture", prompt,
			aispec.WithChatBase_StreamHandler(wire.StreamHandler), aispec.WithChatBase_ReasonStreamHandler(wire.ReasonStreamHandler),
			aispec.WithChatBase_ToolCallCallback(wire.ToolCallCallback), aispec.WithChatBase_FinishReasonCallback(wire.FinishReasonCallback),
			aispec.WithChatBase_RawHTTPResponseHeaderCallback(wire.RawHTTPResponseHeaderCallback),
			aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
				return []poc.PocConfigOption{poc.WithContext(ctx), poc.WithTimeout(5), poc.WithSave(false)}, nil
			}))
	})
	cfg := aicommon.NewConfig(ctx, aicommon.WithAICallback(callback), aicommon.WithDebug(false), aicommon.WithAITransactionAutoRetry(1), aicommon.WithAIAutoRetry(1))
	invoker := mock.NewMockInvoker(ctx)
	invoker.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, invoker)
	loop.functionCallMode = true
	loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
	loop.actions.Set("accept", &LoopAction{ActionType: "accept", VerboseNameI18n: &schema.I18n{Zh: "核对结果", En: "checking results"}})
	capture = captureActivityStatus(loop)
	calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil, loop.emitLoopGeneralOutput, loop.emitLoopFunctionCallOutput)
	require.Equal(t, "complete", <-stages, "the HTTP producer waits for UI delivery before sending the next frame")
	require.NoError(t, err)
	require.Len(t, calls, 1)
	require.Equal(t, "http-call", calls[0].ToolCallID)
	require.Equal(t, "ok", calls[0].Action.GetString("value"))
}

func TestActivityReaderFromDiscardedResponseCannotBindLaterHeaders(t *testing.T) {
	loop := newCallAILoopTransactionTestLoop(t, true, nil)
	capture := captureActivityStatus(loop)
	first := newResponseActivity(loop, nil)
	current := first
	var mu sync.Mutex
	reader, writer := io.Pipe()
	observed := pumpActivityStream(reader, func() *responseActivity { mu.Lock(); defer mu.Unlock(); return current }, true, true)
	second, third := newResponseActivity(loop, nil), newResponseActivity(loop, nil)
	first.headerSuccessor = second
	second.headerSuccessor = third
	first.close()
	second.close()
	mu.Lock()
	current = third
	mu.Unlock()
	go func() { _, _ = io.WriteString(writer, "late discarded reasoning"); _ = writer.Close() }()
	bytes, err := io.ReadAll(observed)
	require.NoError(t, err)
	require.Equal(t, "late discarded reasoning", string(bytes), "UI retirement never changes canonical bytes")
	require.Empty(t, capture.snapshot(), "old readers must not create reasoning for the latest response")
	third.close()
}
