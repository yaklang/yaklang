package reactloops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type LoopStopReason string

const (
	LoopStopNormal    LoopStopReason = "normal"
	LoopStopToolCalls LoopStopReason = "tool_calls"
	LoopStopAbused    LoopStopReason = "abused"
	LoopStopUnknown   LoopStopReason = "unknown"
	LoopStopCancelled LoopStopReason = "cancelled"
)

func failedLoopStopReason(descriptor *LoopResultDescriptor, err error) LoopStopReason {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return LoopStopCancelled
	}
	if descriptor != nil && descriptor.Snapshot().ProviderFinishReason != "" {
		return LoopStopAbused
	}
	return LoopStopUnknown
}

// LoopCall keeps the native call identity alongside the existing action pair.
// The executor can use ToolCallID to return a matching tool result later.
type LoopCall struct {
	Action        *aicommon.Action
	LoopAction    *LoopAction
	ToolCallID    string
	Index         int
	Description   string
	ArgumentsJSON string
}

// Callbacks own their readers until EOF and must not return while another
// goroutine is still reading them. The two readers may be consumed concurrently.
type LoopGeneralOutputCallback func(outputReader, reasonReader io.Reader)
type LoopFunctionCallOutputCallback func(id, functionName string, descriptionReader, argumentReader io.Reader)

// LoopResultSnapshot is immutable provenance for one selected AI response.
// ResponseJSON is reconstructed from the complete streamed content, reasoning,
// calls and finish reason. RawResponsePreview is only the transport's preview.
type LoopResultSnapshot struct {
	Mode                 string
	Provider             string
	Model                string
	ProviderFinishReason string
	ResponseJSON         string
	RawResponseBody      string
	RawResponsePreview   string
	Error                string
	Complete             bool
}

type LoopResultDescriptor struct {
	mu            sync.RWMutex
	snapshot      LoopResultSnapshot
	contentReady  bool
	providerReady bool
	output        string
	reason        string
	calls         []*aispec.ToolCall
	done          chan struct{}
	once          sync.Once
}

func newLoopResultDescriptor(mode string) *LoopResultDescriptor {
	return &LoopResultDescriptor{snapshot: LoopResultSnapshot{Mode: mode}, done: make(chan struct{})}
}

func (d *LoopResultDescriptor) Snapshot() LoopResultSnapshot {
	if d == nil {
		return LoopResultSnapshot{}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.snapshot
}

func (d *LoopResultDescriptor) WaitComplete(ctx context.Context) bool {
	if d == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-d.done:
		return true
	case <-ctx.Done():
		return false
	}
}

func (d *LoopResultDescriptor) setProviderFinishReason(reason string, body []byte) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.snapshot.ProviderFinishReason = reason
	d.snapshot.RawResponseBody = string(body)
	d.providerReady = true
	d.finalizeLocked()
	d.mu.Unlock()
}

func (d *LoopResultDescriptor) resetProviderCompletion() {
	d.mu.Lock()
	d.snapshot.ProviderFinishReason = ""
	d.snapshot.RawResponseBody = ""
	d.providerReady = false
	d.mu.Unlock()
}

func (d *LoopResultDescriptor) setError(err error) {
	if d == nil || err == nil {
		return
	}
	d.mu.Lock()
	d.snapshot.Error = err.Error()
	// A failed request may never produce a terminal provider frame.
	d.providerReady = true
	d.finalizeLocked()
	d.mu.Unlock()
}

func (d *LoopResultDescriptor) finishFromResponse(resp *aicommon.AIResponse, calls []*aispec.ToolCall) {
	if resp == nil {
		d.finish(nil, "", "", calls)
		return
	}
	d.finish(resp, resp.GetPlainOutput(), resp.GetPlainReason(), calls)
}

func (d *LoopResultDescriptor) finish(resp *aicommon.AIResponse, output, reason string, calls []*aispec.ToolCall) {
	if d == nil {
		return
	}
	var provider, model, rawPreview string
	if resp != nil {
		provider, model = resp.GetProviderName(), resp.GetModelName()
		rawPreview = resp.GetRawHTTPResponseDump()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.snapshot.Complete {
		return
	}
	d.output, d.reason, d.calls = output, reason, calls
	d.snapshot.Provider = provider
	d.snapshot.Model = model
	d.snapshot.RawResponsePreview = rawPreview
	d.contentReady = true
	d.finalizeLocked()
}

func (d *LoopResultDescriptor) finalizeLocked() {
	if !d.contentReady || !d.providerReady || d.snapshot.Complete {
		return
	}
	responseCalls := make([]map[string]any, 0, len(d.calls))
	for _, call := range d.calls {
		if call == nil {
			continue
		}
		entry := map[string]any{
			"index": call.Index, "id": call.ID, "type": call.Type,
			"function": map[string]any{"name": call.Function.Name, "arguments": call.Function.Arguments},
		}
		if call.Description != "" {
			entry["description"] = call.Description
		}
		responseCalls = append(responseCalls, entry)
	}
	response := map[string]any{
		"content":           d.output,
		"reasoning_content": d.reason,
		"tool_calls":        responseCalls,
		"finish_reason":     d.snapshot.ProviderFinishReason,
	}
	encoded, err := json.Marshal(response)
	if err == nil {
		d.snapshot.ResponseJSON = string(encoded)
	}
	d.snapshot.Complete = true
	d.once.Do(func() { close(d.done) })
}

// callAILoopTransaction only decides and validates the model's proposed calls.
// The caller is responsible for executing them in a later phase.
func (r *ReActLoop) callAILoopTransaction(
	streamWg *sync.WaitGroup, prompt, nonce string, operator *LoopActionHandlerOperator,
	generalOutputCallback LoopGeneralOutputCallback,
	functionCallOutputCallback LoopFunctionCallOutputCallback,
) ([]LoopCall, LoopStopReason, *LoopResultDescriptor, error) {
	if streamWg == nil || generalOutputCallback == nil || functionCallOutputCallback == nil {
		return nil, LoopStopAbused, nil, utils.Error("loop transaction requires a stream waitgroup and both output callbacks")
	}
	if r.functionCallMode {
		return r.callAIFunctionTransaction(prompt, nonce, generalOutputCallback, functionCallOutputCallback)
	}
	descriptor := newLoopResultDescriptor("normal")
	action, handler, err := r.callAINormalTransaction(streamWg, prompt, nonce, operator, descriptor)
	if err != nil {
		descriptor.setError(err)
		descriptor.finish(nil, r.Get("last_ai_decision_response"), "", nil)
		return nil, failedLoopStopReason(descriptor, err), descriptor, err
	}
	return []LoopCall{{Action: action, LoopAction: handler}}, LoopStopNormal, descriptor, nil
}

func (r *ReActLoop) emitLoopGeneralOutput(outputReader, reasonReader io.Reader) {
	var taskIndex string
	if task := r.GetCurrentTask(); task != nil {
		taskIndex = task.GetIndex()
	}
	emitter := r.GetEmitter()
	consume := func(reader io.Reader, node string, system bool) {
		if emitter == nil {
			_, _ = io.Copy(io.Discard, reader)
			return
		}
		prepared, readable, err := waitReadableStream(reader)
		if err != nil || !readable {
			return
		}
		done := make(chan struct{})
		var emitErr error
		if system {
			_, emitErr = emitter.EmitSystemStreamEvent(node, time.Now(), prepared, taskIndex, func() { close(done) })
		} else {
			_, emitErr = emitter.EmitDefaultStreamEvent(node, prepared, taskIndex, func() { close(done) })
		}
		if emitErr != nil {
			_, _ = io.Copy(io.Discard, prepared)
			return
		}
		<-done
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); consume(outputReader, r.loopName, true) }()
	go func() { defer wg.Done(); consume(reasonReader, "thought", false) }()
	wg.Wait()
}

func (r *ReActLoop) emitLoopFunctionCallOutput(_, _ string, descriptionReader, argumentReader io.Reader) {
	// The transaction's collector owns the canonical argument bytes. The
	// current single-action executor has no per-call UI protocol yet; draining
	// both streams keeps producer and consumer lifetimes well-defined.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, descriptionReader) }()
	go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, argumentReader) }()
	wg.Wait()
}

type loopToolCallParts struct {
	index       int
	id          string
	name        string
	typ         string
	description string
	arguments   strings.Builder
	started     bool
	descWriter  io.WriteCloser
	argsWriter  io.WriteCloser
}

type loopToolCallCollector struct {
	mu            sync.Mutex
	parts         []*loopToolCallParts
	byID          map[string]*loopToolCallParts
	activeByIndex map[int]*loopToolCallParts
	callback      LoopFunctionCallOutputCallback
	wg            sync.WaitGroup
	err           error
}

func invokeLoopGeneralOutputCallback(callback LoopGeneralOutputCallback, output, reason io.Reader) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = utils.Errorf("general output callback panicked: %v", recovered)
		}
	}()
	callback(output, reason)
	return nil
}

func newLoopToolCallCollector(callback LoopFunctionCallOutputCallback) *loopToolCallCollector {
	return &loopToolCallCollector{
		byID: make(map[string]*loopToolCallParts), activeByIndex: make(map[int]*loopToolCallParts),
		callback: callback,
	}
}

func (c *loopToolCallCollector) add(deltas []*aispec.ToolCall) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, delta := range deltas {
		if delta == nil {
			continue
		}
		// Call ID is the identity. Some providers omit or reuse index (often 0)
		// for distinct calls. Index only routes fragments that omit their ID.
		var part *loopToolCallParts
		if delta.ID != "" {
			part = c.byID[delta.ID]
			if part == nil {
				if pending := c.activeByIndex[delta.Index]; pending != nil && pending.id == "" &&
					(pending.name == "" || delta.Function.Name == "" || pending.name == delta.Function.Name) {
					part = pending
				}
			}
		} else {
			part = c.activeByIndex[delta.Index]
			if part != nil && delta.Function.Name != "" && part.name != "" && part.name != delta.Function.Name {
				// A new named call may arrive before its ID. Keep its fragments
				// separate even when the provider reused the same index.
				part = nil
			}
		}
		if part == nil {
			part = &loopToolCallParts{index: delta.Index}
			c.parts = append(c.parts, part)
		}
		c.activeByIndex[delta.Index] = part
		if delta.ID != "" {
			part.id = delta.ID
			c.byID[delta.ID] = part
		}
		if part.name != "" && delta.Function.Name != "" && part.name != delta.Function.Name {
			c.err = utils.Errorf("tool call %q changed function from %q to %q", part.id, part.name, delta.Function.Name)
		}
		if delta.Function.Name != "" {
			part.name = delta.Function.Name
		}
		if delta.Type != "" {
			part.typ = delta.Type
		}
		if delta.Description != "" && delta.Description != part.description {
			descriptionDelta := delta.Description
			if strings.HasPrefix(delta.Description, part.description) {
				descriptionDelta = strings.TrimPrefix(delta.Description, part.description)
			}
			part.description += descriptionDelta
			if part.started {
				_, _ = io.WriteString(part.descWriter, descriptionDelta)
			}
		}
		if delta.Function.Arguments != "" {
			part.arguments.WriteString(delta.Function.Arguments)
		}
		if !part.started && part.id != "" && part.name != "" {
			descReader, descWriter := utils.NewBufPipe(nil)
			argsReader, argsWriter := utils.NewBufPipe(nil)
			part.descWriter, part.argsWriter, part.started = descWriter, argsWriter, true
			c.wg.Add(1)
			go func(id, name string) {
				defer c.wg.Done()
				defer func() {
					if recovered := recover(); recovered != nil {
						c.mu.Lock()
						if c.err == nil {
							c.err = utils.Errorf("function-call output callback for %q panicked: %v", id, recovered)
						}
						c.mu.Unlock()
					}
				}()
				c.callback(id, name, descReader, argsReader)
			}(part.id, part.name)
			if part.description != "" {
				_, _ = io.WriteString(part.descWriter, part.description)
			}
			if part.arguments.Len() > 0 {
				_, _ = io.WriteString(part.argsWriter, part.arguments.String())
			}
		} else if part.started && delta.Function.Arguments != "" {
			_, _ = io.WriteString(part.argsWriter, delta.Function.Arguments)
		}
	}
}

func (c *loopToolCallCollector) finish() ([]*aispec.ToolCall, error) {
	c.mu.Lock()
	parts := append([]*loopToolCallParts(nil), c.parts...)
	for _, part := range parts {
		if part.started {
			_ = part.descWriter.Close()
			_ = part.argsWriter.Close()
		}
	}
	c.mu.Unlock()
	c.wg.Wait()
	c.mu.Lock()
	err := c.err
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(parts, func(i, j int) bool { return parts[i].index < parts[j].index })
	calls := make([]*aispec.ToolCall, 0, len(parts))
	seenIDs := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if part.id == "" || part.name == "" {
			return nil, utils.Errorf("incomplete tool call at index %d: id=%q function=%q", part.index, part.id, part.name)
		}
		if _, exists := seenIDs[part.id]; exists {
			return nil, utils.Errorf("duplicate tool call id %q", part.id)
		}
		seenIDs[part.id] = struct{}{}
		if part.typ != "" && part.typ != "function" {
			return nil, utils.Errorf("unsupported tool call type %q at index %d", part.typ, part.index)
		}
		calls = append(calls, &aispec.ToolCall{
			Index: part.index, ID: part.id, Type: "function", Description: part.description,
			Function: aispec.FuncReturn{Name: part.name, Arguments: part.arguments.String()},
		})
	}
	return calls, nil
}

func (r *ReActLoop) callAIFunctionTransaction(
	prompt, nonce string, generalOutputCallback LoopGeneralOutputCallback,
	functionCallOutputCallback LoopFunctionCallOutputCallback,
) ([]LoopCall, LoopStopReason, *LoopResultDescriptor, error) {
	descriptor := newLoopResultDescriptor("functioncall")
	activeTaskCtx := r.config.GetContext()
	if task := r.GetCurrentTask(); task != nil && !utils.IsNil(task.GetContext()) {
		activeTaskCtx = task.GetContext()
	}
	r.resetModelThinkingBuffer()
	r.Set("last_ai_decision_response", "")
	var currentCollector *loopToolCallCollector
	var collectorMu sync.Mutex
	var acceptedCalls []LoopCall
	var acceptedResp *aicommon.AIResponse
	var lastOutput, lastReason string
	var lastRawCalls []*aispec.ToolCall
	// AIRequestOption is applied for each retry. Keeping the collector scoped to
	// that request prevents fragments from a rejected attempt entering the next.
	captureOption := aicommon.AIRequestOption(func(req *aicommon.AIRequest) {
		descriptor.resetProviderCompletion()
		collectorMu.Lock()
		currentCollector = newLoopToolCallCollector(functionCallOutputCallback)
		collectorMu.Unlock()
		aicommon.WithAIRequest_ExtraSpecOpts(
			// A tiered provider may retry/fall back inside one AIRequest. Each
			// response needs its own collector or calls from a discarded response
			// can be executed together with the accepted response.
			aispec.AIConfigOption(func(cfg *aispec.AIConfig) {
				previous := cfg.RawHTTPResponseHeaderCallback
				cfg.RawHTTPResponseHeaderCallback = func(header []byte) {
					if previous != nil {
						previous(header)
					}
					collectorMu.Lock()
					old := currentCollector
					currentCollector = newLoopToolCallCollector(functionCallOutputCallback)
					collectorMu.Unlock()
					if old != nil {
						_, _ = old.finish() // close readers from the discarded response
					}
					descriptor.resetProviderCompletion()
				}
			}),
			aispec.WithToolCallCallback(func(deltas []*aispec.ToolCall) {
				collectorMu.Lock()
				collector := currentCollector
				collectorMu.Unlock()
				if collector != nil {
					collector.add(deltas)
				}
			}),
			aispec.WithFinishReasonCallback(descriptor.setProviderFinishReason),
		)(req)
	})
	requestOpts := []aicommon.AIRequestOption{
		aicommon.WithAIRequest_CallerLabel(fmt.Sprintf("react-loop:%s", r.loopName)),
		aicommon.WithAIRequest_Context(activeTaskCtx),
		captureOption,
	}
	postHandler := func(resp *aicommon.AIResponse) error {
		r.resetModelThinkingBuffer()
		r.Set("last_ai_decision_response", "")
		acceptedResp = resp
		collectorMu.Lock()
		collector := currentCollector
		collectorMu.Unlock()
		if collector == nil {
			return utils.Error("function-call collector missing for AI response")
		}
		// Content and reasoning are display/capture streams only. Neither stream
		// is passed through ExtractActionFromStream in function-call mode.
		reasonReader, outputReader := resp.GetUnboundStreamReaderEx(nil, nil, nil)
		var output, reason synchronizedResponseCapture
		capturedOutput := io.TeeReader(outputReader, &output)
		capturedReason := io.TeeReader(reasonReader, &reason)
		callbackErr := invokeLoopGeneralOutputCallback(generalOutputCallback, capturedOutput, capturedReason)
		var drain sync.WaitGroup
		drain.Add(2)
		go func() { defer drain.Done(); _, _ = io.Copy(io.Discard, capturedOutput) }()
		go func() { defer drain.Done(); _, _ = io.Copy(io.Discard, capturedReason) }()
		drain.Wait()
		r.appendModelThinkingChunk([]byte(reason.String()))
		r.Set("last_ai_decision_response", output.String())
		if r.config != nil {
			r.config.CallAIResponseOutputFinishedCallback(output.String())
		}
		rawCalls, err := collector.finish()
		lastOutput, lastReason, lastRawCalls = output.String(), reason.String(), rawCalls
		if callbackErr != nil {
			return callbackErr
		}
		if err != nil {
			return err
		}
		if descriptor.Snapshot().ProviderFinishReason != "tool_calls" {
			return utils.Errorf("function-call response ended with %q, expected tool_calls", descriptor.Snapshot().ProviderFinishReason)
		}
		if len(rawCalls) == 0 {
			return utils.Error("function-call response contained no tool calls")
		}
		r.Delete(loopVarNativeTodoBatchAdjusted)
		defer r.Delete(loopVarNativeTodoBatchAdjusted)
		calls := make([]LoopCall, 0, len(rawCalls))
		for _, rawCall := range rawCalls {
			if !validActionToolName(rawCall.Function.Name) {
				return utils.Errorf("invalid function-call action name %q", rawCall.Function.Name)
			}
			handler, err := r.GetActionHandler(rawCall.Function.Name)
			if err != nil || utils.IsNil(handler) {
				return actionTypeResolutionError(rawCall.Function.Name, r.GetAllActionNames(), "native function has no registered loop action")
			}
			params := make(map[string]any)
			dec := json.NewDecoder(strings.NewReader(rawCall.Function.Arguments))
			if err := dec.Decode(&params); err != nil || params == nil {
				return utils.Errorf("invalid JSON arguments for tool call %q: %v", rawCall.ID, err)
			}
			var extra any
			if err := dec.Decode(&extra); err != io.EOF {
				return utils.Errorf("trailing JSON arguments for tool call %q", rawCall.ID)
			}
			if proposed, exists := params["@action"]; exists && proposed != rawCall.Function.Name {
				return utils.Errorf("tool call %q has conflicting @action %v", rawCall.ID, proposed)
			}
			params["@action"] = rawCall.Function.Name
			action := aicommon.NewSimpleAction(rawCall.Function.Name, aitool.InvokeParams(params))
			calls = append(calls, LoopCall{
				Action: action, LoopAction: handler, ToolCallID: rawCall.ID,
				Index: rawCall.Index, Description: rawCall.Description,
				ArgumentsJSON: rawCall.Function.Arguments,
			})
		}
		// Normalize all deltas before action-specific verification. An adjustment
		// may be independent or appear anywhere in a provider's tool-call batch;
		// its presence lets an answer in that batch avoid premature auto-finish.
		for _, call := range calls {
			validateTodoDeltaBeforeActionVerifier(r, call.Action)
			if call.Action.Name() == nativeAdjustTodolistActionName {
				if delta, err := aicommon.NormalizeTodoDelta(call.Action); err == nil && delta != nil {
					r.Set(loopVarNativeTodoBatchAdjusted, true)
				}
			}
		}
		for _, call := range calls {
			if call.LoopAction.ActionVerifier != nil {
				if err := call.LoopAction.ActionVerifier(r, call.Action); err != nil {
					return utils.Wrapf(err, "verify tool call %q", call.ToolCallID)
				}
			}
		}
		acceptedCalls = calls
		r.Set("last_ai_decision_nonce", nonce)
		return nil
	}
	var transactionErr error
	if r.useSpeedPriorityAI {
		r.config.ScheduleAuxiliaryTask(activeTaskCtx, fmt.Sprintf("react-loop:%s", r.loopName),
			func() string { return prompt },
			func(*aicommon.Action) {},
			aicommon.WithAuxiliaryResponseHandler(func(resp *aicommon.AIResponse) (*aicommon.Action, error) {
				acceptedCalls = nil
				if err := postHandler(resp); err != nil {
					return nil, err
				}
				return acceptedCalls[0].Action, nil
			}),
			aicommon.WithAuxiliaryEmitter(r.GetEmitter()),
			aicommon.WithAuxiliaryOpts(aicommon.WithGeneralConfigExtraRequestOpts(requestOpts...)),
			aicommon.WithAuxiliaryOnError(func(err error) { transactionErr = err }),
		)
	} else {
		transactionErr = aicommon.CallAITransaction(r.config, prompt, r.config.CallAI, postHandler, requestOpts...)
	}
	if transactionErr != nil {
		descriptor.setError(transactionErr)
		descriptor.finish(acceptedResp, lastOutput, lastReason, lastRawCalls)
		return nil, failedLoopStopReason(descriptor, transactionErr), descriptor, transactionErr
	}
	if len(acceptedCalls) == 0 || acceptedResp == nil {
		err := utils.Error("function-call transaction returned no accepted calls")
		descriptor.setError(err)
		descriptor.finish(acceptedResp, lastOutput, lastReason, lastRawCalls)
		return nil, failedLoopStopReason(descriptor, err), descriptor, err
	}
	if activeTaskCtx != nil && activeTaskCtx.Err() != nil {
		err := activeTaskCtx.Err()
		descriptor.setError(err)
		descriptor.finish(acceptedResp, lastOutput, lastReason, lastRawCalls)
		return nil, failedLoopStopReason(descriptor, err), descriptor, err
	}
	descriptor.finish(acceptedResp, lastOutput, lastReason, lastRawCalls)
	return acceptedCalls, LoopStopToolCalls, descriptor, nil
}

func (r *ReActLoop) callAINormalTransaction(streamWg *sync.WaitGroup, prompt string, nonce string, operator *LoopActionHandlerOperator, descriptor *LoopResultDescriptor) (*aicommon.Action, *LoopAction, error) {
	var action *aicommon.Action
	var actionNames = r.GetAllActionNames()
	// Bind the provider request to the immutable task that owns this
	// transaction. ReAct.currentTask may temporarily point at a nested loop,
	// while Stop cancels the queue-owned task context. If the request only uses
	// the session config context, the runtime can emit a successful cancellation
	// receipt yet continue streaming model output until the provider finishes.
	activeTask := r.GetCurrentTask()
	activeTaskCtx := r.config.GetContext()
	if activeTask != nil && !utils.IsNil(activeTask.GetContext()) {
		activeTaskCtx = activeTask.GetContext()
	}

	getNextActionType := func(a *aicommon.Action) string { //legacy support
		return inferActionTypeFromPayload(a, r.Get("tag_final_answer"))
	}

	ctxCanceled := utils.NewBool(false)
	currentCtxCanceled := func() bool {
		if !utils.IsNil(activeTaskCtx) {
			select {
			case <-activeTaskCtx.Done():
				ctxCanceled.SetTo(true)
				return true
			default:
				return false
			}
		}
		return false
	}
	currentCtxCanceled()

	log.Infof("start to call aicommon.CallAITransaction in ReActLoop[%v]", r.loopName)
	r.resetModelThinkingBuffer()
	r.Set("last_ai_decision_response", "")
	r.UserStatus(
		"正在理解你的需求",
		"Understanding your request",
		aicommon.WithStatusCode("reasoning.understanding"),
	)
	requestOpts := []aicommon.AIRequestOption{
		aicommon.WithAIRequest_CallerLabel(fmt.Sprintf("react-loop:%s", r.loopName)),
		aicommon.WithAIRequest_Context(activeTaskCtx),
		func(req *aicommon.AIRequest) {
			descriptor.resetProviderCompletion()
			aicommon.WithAIRequest_ExtraSpecOpts(aispec.WithFinishReasonCallback(descriptor.setProviderFinishReason))(req)
		},
	}
	var acceptedResp *aicommon.AIResponse

	postHandler := func(resp *aicommon.AIResponse) error {
		acceptedResp = resp
		if ctxCanceled.IsSet() {
			return nil
		}
		// The action parser can return after it has enough fields while the
		// output stream is still draining. Capture the exact action response
		// only after the stream finishes; assigning buf.String() immediately
		// after ExtractActionFromStream can otherwise persist an empty or
		// truncated action and make the replay record unusable.
		// This also resets reasoning per concrete response, so rejected retry
		// attempts cannot leak into the accepted replay record.
		r.bindDecisionResponseCapture(resp)
		boundEmitter := resp.BindEmitter(r.GetEmitter())
		stream := resp.GetOutputStreamReader(
			r.loopName,
			true,
			r.GetEmitter(),
		)

		buf := new(synchronizedResponseCapture)
		stream = io.TeeReader(stream, buf)
		tagOptions := r.buildActionTagOption(boundEmitter, streamWg, resp.GetTaskIndex(), nonce)
		// The immediate assignment below is intentionally only a snapshot. Once
		// the parser consumes EOF, replace it with the full response for
		// diagnostics and action recovery.
		tagOptions = append(tagOptions, aicommon.WithActionOnReaderFinished(func() {
			r.Set("last_ai_decision_response", buf.String())
		}))
		streamFields := r.streamFields.Copy()

		for _, i := range r.GetAllActions() {
			for _, field := range i.StreamFields {
				streamFields.Set(field.FieldName, field)
			}
		}
		var actionErr error
		options := append(tagOptions, aicommon.WithActionAlias(actionNames...),
			aicommon.WithActionFieldStreamHandler(
				streamFields.Keys(),
				func(key string, reader io.Reader) {
					streamWg.Add(1)
					doneOnce := utils.NewOnce()
					done := func() {
						doneOnce.Do(func() {
							log.Debugf("stream handler for field [%s] done, streamWg.Done() called", key)
							streamWg.Done()
						})
					}

					// Ensure done is always called even if something goes wrong
					defer func() {
						if rec := recover(); rec != nil {
							log.Errorf("stream handler for field [%s] panic recovered: %v", key, rec)
							done()
						}
					}()

					log.Debugf("stream handler started for field [%s]", key)
					jsonReader := utils.JSONStringReader(reader)

					fieldIns, ok := streamFields.Get(key)
					if !ok {
						log.Warnf("stream field [%s] not found in streamFields, skipping", key)
						done()
						return
					}

					pr, pw := utils.NewPipe()
					copyStartTime := time.Now()
					go func(field *LoopStreamField) {
						defer func() {
							pw.Close()
							log.Debugf("stream copy goroutine for field [%s] completed, took %v", key, time.Since(copyStartTime))
						}()
						if field.StreamHandler != nil {
							field.StreamHandler(jsonReader, pw)
							return
						}
						if field.Prefix != "" {
							pw.WriteString(field.Prefix + ": ")
						}
						n, copyErr := io.Copy(pw, jsonReader)
						if copyErr != nil {
							log.Warnf("stream copy for field [%s] error: %v (copied %d bytes)", key, copyErr, n)
						} else {
							log.Debugf("stream copy for field [%s] success, copied %d bytes", key, n)
						}
					}(fieldIns)

					defaultNodeId := "re-act-loop-thought"
					if fieldIns.AINodeId != "" {
						defaultNodeId = fieldIns.AINodeId
					}
					// 把字段名作为流来源记录到 VizSource，让 viz 前端能区分
					// 这条 think/assistant 流来自 AI 响应中的哪个字段（如 human_readable_thought
					// 还是 modify_code_reason）。不污染 ContentType，避免破坏前端按 MIME 主类型解析。
					contentType := fieldIns.ContentType
					preparedReader, readable, readableErr := waitReadableStream(pr)
					if readableErr != nil {
						log.Warnf("stream handler for field [%s] failed waiting first byte: %v", key, readableErr)
						done()
						return
					}
					if !readable {
						log.Debugf("stream handler for field [%s] got empty stream, skipping empty emit", key)
						done()
						return
					}

					_, emitErr := boundEmitter.EmitStreamEventWithVizSource(
						defaultNodeId,
						preparedReader,
						resp.GetTaskIndex(),
						contentType,
						fieldIns.FieldName,
						fieldIns.IsSystem,
						func() {
							log.Debugf("stream emit callback for field [%s] triggered", key)
							done()
						},
					)
					if emitErr != nil {
						log.Errorf("EmitStreamEvent for field [%s] failed: %v", key, emitErr)
						done() // Ensure done is called even on error
						return
					}
				}),
		)

		r.UserStatus(
			"正在梳理思路",
			"Organizing the next steps",
			aicommon.WithStatusCode("reasoning.organizing"),
		)
		extractStart := time.Now()
		action, actionErr = aicommon.ExtractActionFromStream(
			activeTaskCtx,
			stream,
			"object",
			options...,
		)
		log.Debugf("ExtractActionFromStream completed, took %v, error: %v", time.Since(extractStart), actionErr)
		r.Set("last_ai_decision_nonce", nonce)

		if actionErr != nil {
			r.UserStatus(
				"刚才的信息不够完整，正在重新整理",
				"The previous response was incomplete; reorganizing it",
				aicommon.WithStatusCode("reasoning.recovering"),
				aicommon.WithStatusState(aicommon.StatusStateRecovering),
			)
			log.Errorf("ai response stream content before error: %s", buf.String())
			if currentCtxCanceled() {
				actionErr = utils.Wrap(actionErr, "task context canceled while parsing action")
			}
			return utils.Wrap(actionErr, "failed to parse action")
		}
		observedActionType := ""
		admittedActionType := ""
		if action != nil {
			// ActionType waits until @action is admitted or parsing finishes.
			// Read the raw observation afterwards so an unsupported value cannot
			// race with the asynchronous parser and be mislabeled as missing.
			admittedActionType = strings.TrimSpace(action.ActionType())
			observedActionType = strings.TrimSpace(action.ObservedActionType())
		}
		actionType := getNextActionType(action)
		if observedActionType != "" && admittedActionType == "" {
			r.UserStatus(
				"当前思路还不够合适，正在重新整理",
				"The current approach needs adjustment; reorganizing it",
				aicommon.WithStatusCode("reasoning.adjusting"),
				aicommon.WithStatusState(aicommon.StatusStateRecovering),
			)
			log.Errorf("ai response stream content before error: %s", buf.String())
			unsupportedErr := actionTypeResolutionError(
				observedActionType,
				actionNames,
				"a non-empty @action or action value was parsed, but it did not exactly match any action registered in this loop",
			)
			if currentCtxCanceled() {
				unsupportedErr = utils.Wrap(unsupportedErr, "task context canceled while parsing action")
			}
			return unsupportedErr
		}
		if actionType == "" {
			r.UserStatus(
				"正在重新确认下一步",
				"Reconsidering the next step",
				aicommon.WithStatusCode("reasoning.reconsidering"),
				aicommon.WithStatusState(aicommon.StatusStateRecovering),
			)
			log.Errorf("ai response stream content before error: %s", buf.String())
			missingErr := actionTypeResolutionError(
				"",
				actionNames,
				"no non-empty @action or action value was found and legacy payload inference found no known action",
			)
			if currentCtxCanceled() {
				missingErr = utils.Wrap(missingErr, "task context canceled while parsing action")
			}
			return missingErr
		}
		if !utils.StringArrayContains(actionNames, actionType) {
			r.UserStatus(
				"正在换一种方式继续",
				"Switching to another approach",
				aicommon.WithStatusCode("reasoning.fallback"),
				aicommon.WithStatusState(aicommon.StatusStateRecovering),
			)
			return actionTypeResolutionError(
				actionType,
				actionNames,
				"legacy payload inference produced an action type that has no handler in this loop",
			)
		}

		r.UserStatus(
			"已经找到下一步，正在准备执行",
			"The next step is ready and being prepared",
			aicommon.WithStatusCode("action.preparing"),
		)
		log.Infof("action type extracted: %s", actionType)

		verifier, err := r.GetActionHandler(actionType)
		if err != nil {
			resolutionErr := actionTypeResolutionError(
				actionType,
				actionNames,
				fmt.Sprintf("the action name was admitted but handler lookup failed: %v", err),
			)
			r.GetInvoker().AddToTimeline("error", resolutionErr.Error())
			return resolutionErr
		}
		if utils.IsNil(verifier) {
			return utils.Errorf("action[%s] verifier is nil", actionType)
		}
		// TODO validation must run first. Otherwise an invalid delta can
		// masquerade as progress while an action verifier runs (notably the
		// duplicate directly_answer guard), then be removed afterwards.
		validateTodoDeltaBeforeActionVerifier(r, action)
		if verifier.ActionVerifier != nil {
			r.UserStatus(
				"正在确认关键细节",
				"Checking the important details",
				aicommon.WithStatusCode("action.verifying"),
			)
			if err := verifier.ActionVerifier(r, action); err != nil {
				return err
			}
		}
		return nil
	}
	var transactionErr error
	if r.useSpeedPriorityAI {
		// Config owns execution, not just policy lookup. LiteForge runs each
		// response through the existing streaming parser and verifier within its
		// retry transaction; the loop never supplies an AI caller override.
		r.config.ScheduleAuxiliaryTask(activeTaskCtx, fmt.Sprintf("react-loop:%s", r.loopName),
			func() string { return prompt },
			func(accepted *aicommon.Action) { action = accepted },
			aicommon.WithAuxiliaryResponseHandler(func(resp *aicommon.AIResponse) (*aicommon.Action, error) {
				action = nil
				if err := postHandler(resp); err != nil {
					return nil, err
				}
				return action, nil
			}),
			aicommon.WithAuxiliaryEmitter(r.GetEmitter()),
			aicommon.WithAuxiliaryOpts(aicommon.WithGeneralConfigExtraRequestOpts(requestOpts...)),
			aicommon.WithAuxiliaryOnError(func(err error) { transactionErr = err }),
		)
	} else {
		transactionErr = aicommon.CallAITransaction(r.config, prompt, r.config.CallAI, postHandler, requestOpts...)
	}
	if transactionErr != nil {
		r.UserStatus(
			"暂时没能完成这一步",
			"This step could not be completed",
			aicommon.WithStatusCode("action.failed"),
			aicommon.WithStatusState(aicommon.StatusStateError),
		)
		log.Errorf("AI transaction failed: %v", transactionErr)
		return nil, nil, transactionErr
	}

	if ctxCanceled.IsSet() {
		r.UserStatus(
			"任务已停止",
			"Task stopped",
			aicommon.WithStatusCode("task.stopped"),
			aicommon.WithStatusState(aicommon.StatusStateWarning),
		)
		return nil, nil, utils.Error("task context canceled before execute ReActLoop")
	}

	if utils.IsNil(action) {
		r.UserStatus(
			"没有得到完整的下一步信息",
			"The next-step information was incomplete",
			aicommon.WithStatusCode("action.empty"),
			aicommon.WithStatusState(aicommon.StatusStateError),
		)
		return nil, nil, utils.Error("action is nil in ReActLoop")
	}

	r.UserStatus(
		"正在推进下一步",
		"Moving on to the next step",
		aicommon.WithStatusCode("action.ready"),
	)

	handler, err := r.GetActionHandler(getNextActionType(action))
	if err != nil {
		return nil, nil, utils.Wrap(err, "GetActionHandler failed")
	}
	if utils.IsNil(handler) {
		return nil, nil, utils.Errorf("action[%s] 's handler is nil in ReActLoop.actions", action.Name())
	}

	// Wait for all streams to complete with timeout (max 3 seconds)
	// Don't block forever if streams are stuck
	log.Infof("action.WaitStream starting for action [%s] with 3s timeout", action.Name())
	waitStart := time.Now()

	// Create a timeout context for stream waiting
	streamWaitCtx, streamWaitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer streamWaitCancel()

	// Wait with timeout
	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		action.WaitStream(activeTaskCtx)
	}()

	select {
	case <-waitDone:
		log.Infof("action.WaitStream completed normally for action [%s], took %v", action.Name(), time.Since(waitStart))
	case <-streamWaitCtx.Done():
		log.Warnf("action.WaitStream timeout (3s) for action [%s], continuing execution", action.Name())
		r.UserStatus(
			"这一步比预期久一些，仍在继续",
			"This step is taking longer than expected, but is still progressing",
			aicommon.WithStatusCode("action.slow"),
			aicommon.WithStatusState(aicommon.StatusStateWarning),
		)
	}
	if acceptedResp != nil {
		go func() {
			action.WaitStream(activeTaskCtx)
			descriptor.finishFromResponse(acceptedResp, nil)
		}()
	}

	return action, handler, nil
}
