package aireact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type guiReplayStep struct {
	Action string              `json:"action"`
	Params aitool.InvokeParams `json:"params"`
}
type guiReplayFixture struct {
	SourceSession      string          `json:"source_session"`
	Title              string          `json:"title"`
	Prompt             string          `json:"prompt"`
	Steps              []guiReplayStep `json:"steps"`
	ExpectedCards      int             `json:"expected_cards"`
	ExpectedExecutions []struct {
		ID          string                 `json:"id"`
		State       asyncexecmanager.State `json:"state"`
		Contains    []string               `json:"stdout_contains"`
		NotContains []string               `json:"stdout_not_contains"`
	} `json:"expected_executions"`
}

func loadGUIReplay(t *testing.T, name string) guiReplayFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "async_gui_sessions", name+".json"))
	require.NoError(t, err)
	var f guiReplayFixture
	require.NoError(t, json.Unmarshal(raw, &f))
	return f
}

// Store the gRPC envelope the GUI consumes, not internal Go event pointers.
// Channel notifications are latches; time is only a generous deadlock watchdog.
type guiReplayEvents struct {
	mu      sync.Mutex
	events  []*ypb.AIOutputEvent
	changed chan struct{}
}

func newGUIReplayEvents() *guiReplayEvents { return &guiReplayEvents{changed: make(chan struct{})} }
func (r *guiReplayEvents) record(e *schema.AiOutputEvent) {
	wire := e.ToGRPC()
	wire.Content = append([]byte(nil), wire.Content...)
	wire.StreamDelta = append([]byte(nil), wire.StreamDelta...)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, wire)
	close(r.changed)
	r.changed = make(chan struct{})
}
func (r *guiReplayEvents) snapshot() []*ypb.AIOutputEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*ypb.AIOutputEvent(nil), r.events...)
}
func (r *guiReplayEvents) wait(ctx context.Context, predicate func([]*ypb.AIOutputEvent) bool) error {
	for {
		r.mu.Lock()
		events := append([]*ypb.AIOutputEvent(nil), r.events...)
		changed := r.changed
		r.mu.Unlock()
		if predicate(events) {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
func guiWaitCompleted(t *testing.T, ctx context.Context, events *guiReplayEvents) {
	t.Helper()
	var status any
	require.NoError(t, events.wait(ctx, func(all []*ypb.AIOutputEvent) bool {
		for _, e := range all {
			if e.NodeId != "react_task_status_changed" || strings.Contains(e.TaskId, "-sub-") {
				continue
			}
			status = guiData(e)["react_task_now_status"]
			if status == "completed" || status == "aborted" {
				return true
			}
		}
		return false
	}))
	// Fail before waiting for streams: an aborted task can still have a held
	// cleanup gate. Test cleanup will release it and join the runtime.
	require.Equal(t, "completed", status)
}

// Runtime lifecycle shutdown joins input/task loops; explicitly join retained
// async resources too, including on a failed assertion with an open tool gate.
func guiShutdownAsyncLoop(t *testing.T, loop *reactloops.ReActLoop) {
	t.Helper()
	if loop == nil {
		return
	}
	manager, err := loop.GetAsyncExecutionManager()
	if errors.Is(err, asyncexecmanager.ErrClosed) {
		return // Released before any execution manager was created.
	}
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, manager.Shutdown(ctx))
}

func guiData(e *ypb.AIOutputEvent) map[string]any {
	var v map[string]any
	_ = json.Unmarshal(e.Content, &v)
	return v
}

// Both transports persist management feedback in timeline items. Native mode
// wraps it in FUNCTION_CALL_ACTION_RESULT.detail; text mode stores it directly.
func guiManagementFeedback(events []*ypb.AIOutputEvent) []map[string]any {
	var feedback []map[string]any
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case string:
			var decoded any
			if json.Unmarshal([]byte(v), &decoded) == nil {
				visit(decoded)
			}
		case map[string]any:
			if _, ok := v["op"]; ok {
				feedback = append(feedback, v)
				return
			}
			for _, key := range []string{"content", "detail"} {
				if nested, ok := v[key]; ok {
					visit(nested)
				}
			}
		}
	}
	for _, e := range events {
		if e.NodeId == "timeline_item" {
			visit(guiData(e))
		}
	}
	return feedback
}

// Text-mode action feedback is carried in the next prompt, rather than a
// FUNCTION_CALL_ACTION_RESULT event. Read its complete JSON objects there.
func guiTextManagementFeedback(prompt string) []map[string]any {
	// Only the newest feedback section matters; scanning all JSON examples and
	// historical prompts is needlessly expensive under the race detector.
	end := strings.LastIndex(prompt, "<|REACTIVE_DATA_END_")
	if end < 0 {
		return nil
	}
	start := strings.LastIndex(prompt[:end], "<|REACTIVE_DATA_")
	if start < 0 {
		return nil
	}
	headerEnd := strings.Index(prompt[start:end], "|>")
	if headerEnd < 0 {
		return nil
	}
	prompt = prompt[start+headerEnd+2 : end]
	var out []map[string]any
	for i := 0; i < len(prompt); i++ {
		if prompt[i] != '{' {
			continue
		}
		var value map[string]any
		if json.NewDecoder(strings.NewReader(prompt[i:])).Decode(&value) == nil {
			if _, ok := value["op"]; ok {
				out = append(out, value)
			}
		}
	}
	return out
}

func guiTerminal(e *ypb.AIOutputEvent) bool {
	return e.Type == schema.EVENT_TOOL_CALL_DONE || e.Type == schema.EVENT_TOOL_CALL_ERROR || e.Type == schema.EVENT_TOOL_CALL_USER_CANCEL
}
func guiAwait(ctx context.Context, ch <-chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type guiReplayCall struct {
	id                                  string
	started, finish, cancelled, cleanup chan struct{}
	finishOnce, cleanupOnce             sync.Once
}

func newGUIReplayCall(id string) *guiReplayCall {
	return &guiReplayCall{id: id, started: make(chan struct{}), finish: make(chan struct{}), cancelled: make(chan struct{}), cleanup: make(chan struct{})}
}
func (c *guiReplayCall) release()        { c.finishOnce.Do(func() { close(c.finish) }) }
func (c *guiReplayCall) releaseCleanup() { c.cleanupOnce.Do(func() { close(c.cleanup) }) }

type guiReplayTools struct {
	mu           sync.Mutex
	calls        map[string]*guiReplayCall
	started      chan struct{}
	changed      chan struct{}
	startOnce    sync.Once
	shutdown     chan struct{}
	shutdownOnce sync.Once
	defaults     atomic.Int32
}

func newGUIReplayTools() *guiReplayTools {
	return &guiReplayTools{calls: map[string]*guiReplayCall{}, started: make(chan struct{}), changed: make(chan struct{}), shutdown: make(chan struct{})}
}
func (g *guiReplayTools) close() { g.shutdownOnce.Do(func() { close(g.shutdown) }) }
func (g *guiReplayTools) all() []*guiReplayCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []*guiReplayCall
	for _, c := range g.calls {
		out = append(out, c)
	}
	return out
}
func (g *guiReplayTools) get(ctx context.Context, id string) (*guiReplayCall, error) {
	for {
		g.mu.Lock()
		c, changed := g.calls[id], g.changed
		g.mu.Unlock()
		if c != nil {
			return c, nil
		}
		if err := guiAwait(ctx, changed); err != nil {
			return nil, fmt.Errorf("callback %s not started: %w", id, err)
		}
	}
}
func (g *guiReplayTools) tool(t *testing.T, name string) *aitool.Tool {
	t.Helper()
	tool, err := aitool.New(name, aitool.WithDangerousNoNeedUserReview(true), aitool.WithDefaultAsync(name == "test_default_async"),
		aitool.WithStringParam("command"), aitool.WithStringParam("shell"), aitool.WithStringParam("accepted-exit-codes"), aitool.WithNumberParam("timeout"),
		aitool.WithCallback(func(ctx context.Context, p aitool.InvokeParams, r *aitool.ToolRuntimeConfig, stdout, stderr io.Writer) (any, error) {
			command := p.GetString("command")
			if strings.HasPrefix(command, "echo ") {
				fmt.Fprintln(stdout, strings.TrimPrefix(command, "echo "))
				return "foreground complete", nil
			}
			c := newGUIReplayCall(r.RuntimeID)
			g.mu.Lock()
			g.calls[r.RuntimeID] = c
			close(g.changed)
			g.changed = make(chan struct{})
			g.mu.Unlock()
			if name == "test_detach" {
				fmt.Fprintln(stdout, "START")
				defer fmt.Fprintln(stdout, "CLEANUP")
				first, err := r.Detach()
				if err != nil {
					return nil, err
				}
				second, err := r.Detach()
				if err != nil {
					return nil, err
				}
				if first != second {
					return nil, errors.New("Detach changed execution ID")
				}
				fmt.Fprintf(stdout, "EXECUTION_ID=%s\nREPEATED_EXECUTION_ID=%s\n", first, second)
			} else {
				fmt.Fprintln(stdout, "START")
			}
			if name == "test_default_async" && g.defaults.Add(1) == 2 {
				c.release()
			}
			close(c.started)
			g.startOnce.Do(func() { close(g.started) })
			select {
			case <-c.finish:
			case <-ctx.Done():
				close(c.cancelled)
				select {
				case <-c.cleanup:
				case <-g.shutdown:
				}
				return nil, ctx.Err()
			}
			switch name {
			case "test_detach":
				fmt.Fprintln(stdout, "DONE")
				return "DETACH_OK", nil
			case "test_default_async":
				fmt.Fprintln(stdout, "DONE")
				return "DEFAULT_ASYNC_OK", nil
			default:
				if strings.HasPrefix(command, "for ") {
					for i := 0; i < 75; i++ {
						fmt.Fprintln(stdout, "TICK")
					}
					fmt.Fprintln(stdout, "DONE")
				} else if strings.Contains(command, "CHILD_DONE") {
					fmt.Fprintln(stdout, "CHILD_DONE")
				} else {
					fmt.Fprintln(stdout, "ASYNC_DONE")
				}
				return map[string]any{"exit_code": 0, "process_completed": true}, nil
			}
		}))
	require.NoError(t, err)
	return tool
}

// Fail the test even if a production fallback elects to swallow an AI error.
func guiMockModel(t *testing.T, model aicommon.AICallbackType) aicommon.AICallbackType {
	return func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp, err := model(c, req)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("mock model contract: %v", err)
		}
		return rsp, err
	}
}

var guiResponseSequence atomic.Uint64

func guiRespond(c aicommon.AICallerConfigIf, req *aicommon.AIRequest, native bool, step guiReplayStep) (*aicommon.AIResponse, error) {
	rsp := c.NewAIResponse()
	raw, err := json.Marshal(step.Params)
	if err != nil {
		return nil, err
	}
	if native {
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		if wire.ToolCallCallback == nil {
			return nil, fmt.Errorf("missing native transport: %s", req.GetCallerLabel())
		}
		wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("gui-%d", guiResponseSequence.Add(1)), Type: "function", Function: aispec.FuncReturn{Name: step.Action, Arguments: string(raw)}}})
		wire.FinishReasonCallback("tool_calls", nil)
	} else {
		var p map[string]any
		_ = json.Unmarshal(raw, &p)
		p["@action"] = step.Action
		raw, _ = json.Marshal(p)
		rsp.EmitOutputStream(strings.NewReader(string(raw)))
	}
	rsp.Close()
	return rsp, nil
}

// The prompts and core model actions are extracted from the seven GUI sessions.
// Opaque execution IDs are rebound, long-running tools are channel-controlled,
// and UI event invariants replace unstable snapshots of timestamps/UUIDs/chunks.
func TestAsyncGUISessionReplay(t *testing.T) {
	for _, name := range []string{"explicit_async", "script_detach", "default_async", "interval_detach", "stop_cleanup", "batch_isolation"} {
		for _, native := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/native=%t", name, native), func(t *testing.T) {
				fixture := loadGUIReplay(t, name)
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
				events := newGUIReplayEvents()
				g := newGUIReplayTools()
				defer g.close()
				defer func() {
					for _, c := range g.all() {
						c.release()
						c.releaseCleanup()
					}
				}()
				var runtime *ReAct
				var stepIndex int
				var reviews atomic.Int32
				reviewInFlight := make(chan struct{})
				var reviewInFlightOnce sync.Once
				var manager *asyncexecmanager.Manager
				ids := map[string]string{}
				waits := 0
				var textFeedback []map[string]any
				answer := "GUI_REPLAY_COMPLETE " + name
				model := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (response *aicommon.AIResponse, modelErr error) {
					defer func() {
						if modelErr != nil {
							g.close()
						}
					}()
					if strings.Contains(req.GetPrompt(), "Runtime supports detachment:") {
						wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						if (wire.ToolCallCallback != nil) != native {
							return nil, errors.New("interval review did not inherit the session protocol")
						}
						if reviews.Add(1) > 1 {
							reviewInFlightOnce.Do(func() { close(reviewInFlight) })
							<-req.GetContext().Done()
							return nil, req.GetContext().Err()
						}
						if err := guiAwait(ctx, g.started); err != nil {
							return nil, err
						}
						return guiRespond(c, req, native, guiReplayStep{"interval-toolcall-review", aitool.InvokeParams{"decision": "detach", "reason": "healthy independent background work", "progress_summary": "started", "estimated_remaining_time": "unknown"}})
					}
					if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
						return guiRespond(c, req, false, guiReplayStep{"verify-satisfaction", aitool.InvokeParams{"user_satisfied": false, "reasoning": "Continue the recorded scenario"}})
					}
					if name == "interval_detach" && req.GetCallerLabel() == "directly-answer" && stepIndex > len(fixture.Steps) {
						return guiRespond(c, req, false, guiReplayStep{"directly_answer", aitool.InvokeParams{"answer_payload": answer}})
					}
					if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
						return nil, fmt.Errorf("unexpected GUI replay model request: %s", req.GetCallerLabel())
					}
					if !native {
						textFeedback = append(textFeedback, guiTextManagementFeedback(req.GetPrompt())...)
					}
					if !strings.Contains(req.GetPrompt(), fixture.Prompt) {
						return nil, errors.New("original GUI prompt missing from model context")
					}
					task := runtime.GetCurrentTask()
					if task == nil {
						return nil, errors.New("missing GUI task")
					}
					loop, ok := task.GetReActLoop().(*reactloops.ReActLoop)
					if !ok {
						return nil, errors.New("missing owner loop")
					}
					var err error
					manager, err = loop.GetAsyncExecutionManager()
					if err != nil {
						return nil, err
					}
					if stepIndex >= len(fixture.Steps) {
						if stepIndex == len(fixture.Steps) && name != "interval_detach" {
							stepIndex++
							return guiRespond(c, req, native, guiReplayStep{"directly_answer", aitool.InvokeParams{"answer_payload": answer}})
						}
						stepIndex++
						return guiRespond(c, req, native, guiReplayStep{"finish", aitool.InvokeParams{}})
					}
					step := fixture.Steps[stepIndex]
					stepIndex++
					if step.Action == "directly_call_tool" {
						if !step.Params.Has("directly_call_tool_params_group") && !step.Params.Has("directly_call_reason") {
							step.Params["directly_call_reason"] = "Replay GUI tool call"
						}
					}
					if step.Action == "manage_async_execution_runtime" {
						entries := manager.List()
						if len(entries) == 0 {
							return nil, errors.New("missing submitted execution")
						}
						if len(ids) == 0 {
							if name == "batch_isolation" {
								pattern := regexp.MustCompile(`execute_id: "([^"]+)"\s+identifier: "([^"]+)"`)
								// Bind from the receipt already delivered to the model. The GUI
								// timeline can be emitted later on another goroutine.
								for _, m := range pattern.FindAllStringSubmatch(req.GetPrompt(), -1) {
									if m[2] == "detach_first_sync" {
										ids["$exec1"] = m[1]
									}
									if m[2] == "detach_second_sync" {
										ids["$exec2"] = m[1]
									}
								}
							} else {
								ids["$exec1"] = entries[0].ID
							}
						}
						alias := step.Params.GetString("execute_id")
						id := ids[alias]
						if id == "" {
							return nil, fmt.Errorf("unbound execution %s", alias)
						}
						step.Params["execute_id"] = id
						entry, err := manager.Get(id)
						if err != nil {
							return nil, err
						}
						callID := entry.Metadata.Attributes["call_tool_id"]
						if callID == "" {
							callID = entry.Metadata.Attributes["call_id"]
						}
						call, err := g.get(ctx, callID)
						if err != nil {
							return nil, err
						}
						if err := guiAwait(ctx, call.started); err != nil {
							return nil, err
						}
						if step.Params.GetString("op") == "inspect" {
							if _, err := manager.Wait(ctx, id); err != nil {
								return nil, err
							}
						}
						if step.Params.GetString("op") == "wait" {
							waits++
							blocked := name == "interval_detach" || name == "stop_cleanup" || name == "batch_isolation"
							if name == "stop_cleanup" || name == "batch_isolation" {
								if err := guiAwait(ctx, call.cancelled); err != nil {
									return nil, err
								}
								for _, e := range events.snapshot() {
									if e.NodeId == callID && guiTerminal(e) {
										return nil, fmt.Errorf("card terminated before cleanup: %s", e.Type)
									}
								}
							}
							if blocked && waits == 1 {
								step.Params["timeout_seconds"] = 0
							} else {
								if name == "interval_detach" {
									if err := guiAwait(ctx, reviewInFlight); err != nil {
										return nil, err
									}
								}
								for _, work := range g.all() {
									work.releaseCleanup()
									work.release()
								}
							}
							if blocked && waits == 2 {
								expected := asyncexecmanager.StateRunning
								if name != "interval_detach" {
									expected = asyncexecmanager.StateStopping
								}
								if entry.State != expected {
									return nil, fmt.Errorf("wait timeout changed worker state: %s", entry.State)
								}
								if !strings.Contains(req.GetPrompt(), "wait_timed_out") {
									return nil, errors.New("timeout feedback missing")
								}
							}
						}
					}
					return guiRespond(c, req, native, step)
				}
				tools := []*aitool.Tool{g.tool(t, "bash"), g.tool(t, "test_detach"), g.tool(t, "test_default_async")}
				var err error
				runtime, err = NewTestReAct(
					aicommon.WithContext(ctx),
					aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tools...)),
					aicommon.WithEnableFunctionCallMode(native),
					aicommon.WithAgreeYOLO(),
					aicommon.WithWorkdir(t.TempDir()),
					aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithNoOpMemoryTriage(),
					aicommon.WithDisableToolCallerIntervalReview(name != "interval_detach"),
					aicommon.WithToolCallerIntervalReviewDuration(time.Millisecond),
					aicommon.WithAICallback(guiMockModel(t, model)),
					aicommon.WithEventHandler(events.record),
					aicommon.WithAIAutoRetry(1),
					aicommon.WithAITransactionAutoRetry(1))
				require.NoError(t, err)
				defer func() {
					g.close()
					cancel()
					for _, work := range g.all() {
						work.release()
						work.releaseCleanup()
					}
					runtime.WaitLifecycleStopped()
				}()
				runtime.config.SetWorkDir(runtime.config.Workdir)
				require.NoError(t, runtime.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: fixture.Prompt}))
				guiWaitCompleted(t, ctx, events)
				runtime.Emitter.WaitForStream()
				require.NotNil(t, manager)
				entries := manager.List()
				require.Len(t, entries, len(fixture.ExpectedExecutions), "unexpected execution created or tool restarted")
				for _, expected := range fixture.ExpectedExecutions {
					snapshot, err := manager.Inspect(ctx, ids[expected.ID])
					require.NoError(t, err)
					require.Equal(t, expected.State, snapshot.State)
					for _, marker := range expected.Contains {
						require.Contains(t, snapshot.Observation.Stdout, marker)
					}
					for _, marker := range expected.NotContains {
						require.NotContains(t, snapshot.Observation.Stdout, marker)
					}
					require.NotEmpty(t, snapshot.Observation.Result, "inspect must return the archived result")
				}
				cards := 0
				for _, e := range events.snapshot() {
					if e.Type == schema.EVENT_TOOL_CALL_START {
						cards++
					}
				}
				require.Equal(t, fixture.ExpectedCards, cards)
				for _, e := range entries {
					require.True(t, e.State.IsTerminal(), "task finished with live execution: %s", e.ID)
				}
				validateGUIReplayCards(t, events.snapshot(), name, answer)
				if name == "interval_detach" || name == "stop_cleanup" || name == "batch_isolation" {
					expectedState := "running"
					if name != "interval_detach" {
						expectedState = "stopping"
					}
					var timedOut, settled bool
					feedback := guiManagementFeedback(events.snapshot())
					if !native {
						feedback = textFeedback
					}
					for _, f := range feedback {
						if f["op"] != "wait" {
							continue
						}
						if f["wait_timed_out"] == true {
							execution, ok := f["execution"].(map[string]any)
							require.True(t, ok)
							require.Equal(t, expectedState, execution["state"])
							require.Equal(t, ids["$exec1"], execution["execute_id"])
							timedOut = true
						} else if f["wait_timed_out"] == false {
							settled = true
						}
					}
					require.True(t, timedOut, "missing wait timeout feedback")
					require.True(t, settled, "missing settled wait feedback")
				}
				if name == "interval_detach" {
					require.GreaterOrEqual(t, reviews.Load(), int32(1))
				}
			})
		}
	}
}

func validateGUIReplayCards(t *testing.T, events []*ypb.AIOutputEvent, name, answer string) {
	t.Helper()
	type card struct {
		start, terminal, result int
		kind, tool, task        string
		stdout                  strings.Builder
		params                  bool
		running                 bool
	}
	cards := map[string]*card{}
	answered := false
	reviewDetach := false
	for i, e := range events {
		data := guiData(e)
		if e.NodeId == "react_task_status_changed" {
			require.NotEqual(t, "aborted", data["react_task_now_status"])
		}
		if strings.Contains(string(e.Content), "user did not select a valid action") {
			t.Errorf("watcher cancellation became a user error: %s", e.Content)
		}
		if e.Type == "result" && strings.Contains(fmt.Sprint(data["result"]), answer) {
			answered = true
		}
		if e.Type == schema.EVENT_TOOL_CALL_PROGRESS_REVIEW {
			require.NotEqual(t, schema.TOOL_CALL_PROGRESS_REVIEW_PHASE_FAILED, data["phase"], "normal completion must not report review failure")
		}
		if e.Type == schema.EVENT_TOOL_CALL_PROGRESS_REVIEW && data["decision"] == "detach" {
			reviewDetach = true
		}
		id, _ := data["call_tool_id"].(string)
		if id == "" {
			id = e.CallToolID
		}
		if e.Type == schema.EVENT_TOOL_CALL_START {
			require.NotEmpty(t, id)
			require.Nil(t, cards[id], "duplicate tool card")
			tool := data["tool"].(map[string]any)
			cards[id] = &card{start: i, terminal: -1, result: -1, tool: fmt.Sprint(tool["name"]), task: e.TaskId}
			continue
		}
		c := cards[id]
		if c == nil {
			continue
		}
		if e.TaskId != "" {
			require.Equal(t, c.task, e.TaskId, "card event escaped its owning task")
		}
		if e.Type == schema.EVENT_TOOL_CALL_STATUS && data["status"] == schema.TOOL_CALL_STATUS_RUNNING {
			c.running = true
		}
		if strings.HasPrefix(e.NodeId, "tool-") && strings.HasSuffix(e.NodeId, "-stdout") && e.Type == "stream" {
			require.Equal(t, -1, c.terminal, "output after card ended")
			c.stdout.Write(e.StreamDelta)
		}
		if e.Type == schema.EVENT_TOOL_CALL_PARAM {
			c.params = true
		}
		if e.Type == schema.EVENT_TOOL_CALL_RESULT {
			require.Equal(t, -1, c.terminal, "result after card ended")
			c.result = i
		}
		if guiTerminal(e) {
			require.Equal(t, -1, c.terminal, "duplicate terminal event")
			c.terminal = i
			c.kind = e.Type
		}
	}
	require.True(t, answered, "missing user-facing answer bubble")
	require.NotEmpty(t, cards)
	cancelled := 0
	var allStdout strings.Builder
	for id, c := range cards {
		allStdout.WriteString(c.stdout.String())
		require.Greater(t, c.terminal, c.start, "unfinished card %s", id)
		require.True(t, c.params, "card missing parameters")
		require.True(t, c.running, "card never entered running status")
		require.NotEmpty(t, c.task, "card missing task identity")
		if c.kind == schema.EVENT_TOOL_CALL_USER_CANCEL {
			cancelled++
			require.Contains(t, c.stdout.String(), "CLEANUP")
			require.NotContains(t, c.stdout.String(), "DONE")
			require.Equal(t, -1, c.result)
		} else {
			require.Equal(t, schema.EVENT_TOOL_CALL_DONE, c.kind)
			require.Greater(t, c.result, c.start)
			if c.tool == "test_detach" {
				require.Contains(t, c.stdout.String(), "DONE\nCLEANUP")
			}
		}
	}
	if name == "stop_cleanup" || name == "batch_isolation" {
		require.Equal(t, 1, cancelled)
	} else {
		require.Zero(t, cancelled)
	}
	switch name {
	case "explicit_async":
		require.Contains(t, allStdout.String(), "FOREGROUND_DONE")
		require.Contains(t, allStdout.String(), "ASYNC_DONE")
	case "script_detach":
		require.Contains(t, allStdout.String(), "AGENT_CONTINUES")
	case "default_async":
		require.Equal(t, 2, strings.Count(allStdout.String(), "DONE\n"))
	case "subagent_isolation":
		require.Equal(t, 2, strings.Count(allStdout.String(), "CHILD_DONE\n"))
	}
	if name == "interval_detach" {
		require.True(t, reviewDetach, "GUI did not receive detach review")
		for _, c := range cards {
			require.Equal(t, 75, strings.Count(c.stdout.String(), "TICK\n"))
		}
	}
}
