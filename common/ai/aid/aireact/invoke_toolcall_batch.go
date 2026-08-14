package aireact

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	_ aicommon.ToolBatchInvokeRuntime = (*ReAct)(nil)

	errToolBatchDirectAnswer = errors.New("tool batch cancelled by direct-answer review")
)

type toolBatchWork struct {
	call            aicommon.ToolBatchCall
	batchID         string
	tool            *aitool.Tool
	params          aitool.InvokeParams
	checkpointSeq   int64
	paramSeq        int64
	reviewSeq       int64
	watcherSeq      int64
	resultID        int64
	artifactOrdinal int
}

// toolBatchBarrier prevents every plugin callback from starting until all
// sibling calls have completed parameter generation/review (or failed before
// that boundary). This is what gives a batch its zero-side-effect direct-answer
// guarantee without rewriting the mature scalar ToolCaller pipeline.
type toolBatchBarrier struct {
	mu       sync.Mutex
	total    int
	arrived  []bool
	count    int
	direct   bool
	released bool
	gate     chan struct{}
}

func newToolBatchBarrier(total int) *toolBatchBarrier {
	return &toolBatchBarrier{
		total:   total,
		arrived: make([]bool, total),
		gate:    make(chan struct{}),
	}
}

func (b *toolBatchBarrier) arrive(index int, directlyAnswer bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if directlyAnswer {
		b.direct = true
	}
	if index >= 0 && index < len(b.arrived) && !b.arrived[index] {
		b.arrived[index] = true
		b.count++
	}
	if b.count == b.total && !b.released {
		b.released = true
		close(b.gate)
	}
}

func (b *toolBatchBarrier) wait(
	ctx context.Context,
	index int,
	invokeGate aicommon.ToolCallerGate,
) (func(), error) {
	b.arrive(index, false)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.gate:
	}
	// Cancellation may race the gate close. A select is allowed to choose the
	// ready gate even when ctx.Done is also ready, so every admission boundary
	// must re-check the context after it acquires permission.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	b.mu.Lock()
	direct := b.direct
	b.mu.Unlock()
	if direct {
		return nil, errToolBatchDirectAnswer
	}
	if invokeGate == nil {
		return func() {}, nil
	}
	release, err := invokeGate(ctx)
	release = aicommonIdempotentRelease(release)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func aicommonIdempotentRelease(release func()) func() {
	if release == nil {
		return func() {}
	}
	var once sync.Once
	return func() { once.Do(release) }
}

// orderedBatchStage makes stateful parameter mutators deterministic. Require
// calls can finish their AI transactions in any order, but each mutator runs in
// model array order exactly once. A call that fails before reaching its mutator
// marks its turn complete from the worker defer, so later calls cannot deadlock.
type orderedBatchStage struct {
	mu        sync.Mutex
	turns     []chan struct{}
	done      []sync.Once
	completed []bool
	next      int
}

func newOrderedBatchStage(total int) *orderedBatchStage {
	s := &orderedBatchStage{
		turns:     make([]chan struct{}, total),
		done:      make([]sync.Once, total),
		completed: make([]bool, total),
	}
	for i := range s.turns {
		s.turns[i] = make(chan struct{})
	}
	if total > 0 {
		close(s.turns[0])
	}
	return s
}

func (s *orderedBatchStage) complete(index int) {
	if index < 0 || index >= len(s.done) {
		return
	}
	s.done[index].Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.completed[index] = true
		// An item can be marked complete before its predecessor (for example a
		// direct child in a mixed defensive request). Only advance the turn when
		// the complete prefix grows, never merely because index+1 finished.
		for s.next < len(s.completed) && s.completed[s.next] {
			s.next++
			if s.next < len(s.turns) {
				close(s.turns[s.next])
			}
		}
	})
}

func (s *orderedBatchStage) run(
	ctx context.Context,
	index int,
	params aitool.InvokeParams,
	mutate func(aitool.InvokeParams) aitool.InvokeParams,
) aitool.InvokeParams {
	if index < 0 || index >= len(s.turns) {
		return params
	}
	select {
	case <-ctx.Done():
		s.complete(index)
		return params
	case <-s.turns[index]:
	}
	if ctx.Err() != nil {
		s.complete(index)
		return params
	}
	defer s.complete(index)
	if mutate == nil {
		return params
	}
	return mutate(params)
}

// wait holds this index's ordered stage until the coordinator explicitly calls
// complete. Unlike run, releasing the returned gate does not advance the turn.
// This lets one child recursively review wrong-tool/wrong-param decisions while
// later children remain blocked; beforeInvoke advances the turn once that child
// has definitively left its review phase.
func (s *orderedBatchStage) wait(ctx context.Context, index int) (func(), error) {
	if index < 0 || index >= len(s.turns) {
		return func() {}, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.turns[index]:
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return func() {}, nil
}

func toolBatchSemaphoreGate(limit int) aicommon.ToolCallerGate {
	if limit <= 0 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	return func(ctx context.Context) (func(), error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case sem <- struct{}{}:
		}
		var once sync.Once
		release := func() {
			once.Do(func() { <-sem })
		}
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		return release, nil
	}
}

func cloneToolBatchParams(params aitool.InvokeParams) aitool.InvokeParams {
	if params == nil {
		return make(aitool.InvokeParams)
	}
	return cloneToolBatchMap(map[string]any(params))
}

func cloneToolBatchMap(input map[string]any) aitool.InvokeParams {
	result := make(aitool.InvokeParams, len(input))
	for key, value := range input {
		result[key] = cloneToolBatchValue(value)
	}
	return result
}

func cloneToolBatchValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneToolBatchMap(typed)
	case aitool.InvokeParams:
		return cloneToolBatchMap(map[string]any(typed))
	case []any:
		result := make([]any, len(typed))
		for i := range typed {
			result[i] = cloneToolBatchValue(typed[i])
		}
		return result
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func stableToolBatchID(prefix string, parts ...any) string {
	identity := fmt.Sprint(parts...)
	digest := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("%s-%x", prefix, digest[:12])
}

func toolBatchLoop(task aicommon.AIStatefulTask) *reactloops.ReActLoop {
	if task == nil || task.GetReActLoop() == nil {
		return nil
	}
	loop, _ := task.GetReActLoop().(*reactloops.ReActLoop)
	return loop
}

func toolBatchStageForError(err error) aicommon.ToolCallStage {
	if err == nil {
		return aicommon.ToolCallStageDone
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, errToolBatchDirectAnswer) {
		return aicommon.ToolCallStageCancelled
	}
	return aicommon.ToolCallStagePrepareFailed
}

func (r *ReAct) batchTaskEmitter(task aicommon.AIStatefulTask) *aicommon.Emitter {
	var base *aicommon.Emitter
	if task != nil {
		base = task.GetEmitter()
	}
	if base == nil && r.config != nil {
		base = r.config.GetEmitter()
	}
	if base == nil {
		return nil
	}
	taskIndex := ""
	if task != nil {
		taskIndex = task.GetIndex()
	}
	return base.PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if event != nil && event.TaskIndex == "" {
			event.TaskIndex = taskIndex
		}
		return event
	})
}

func (r *ReAct) newToolCallerForBatchCall(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	work *toolBatchWork,
	baseEmitter *aicommon.Emitter,
	paramGate aicommon.ToolCallerGate,
	reviewGate aicommon.ToolCallerGate,
	beforeInvoke aicommon.ToolCallerBeforeInvoke,
	paramMutator func(*aitool.Tool, aitool.InvokeParams) aitool.InvokeParams,
	promptMu *sync.Mutex,
) (*aicommon.ToolCaller, error) {
	childEmitter := baseEmitter
	if childEmitter != nil {
		childEmitter = childEmitter.AssociativeAIProcess(&schema.AiProcess{
			ProcessId:   work.call.ExecutionCallID,
			ProcessType: schema.AI_Call_Tool,
		})
	}

	statsSource := aicommon.StatsSourceToolDirect
	if work.call.Mode == aicommon.ToolCallModeRequire {
		statsSource = aicommon.StatsSourceToolRequested
	}
	opts := []aicommon.ToolCallerOption{
		aicommon.WithToolCaller_AICallerConfig(r.config),
		aicommon.WithToolCaller_AICaller(r.config),
		aicommon.WithToolCaller_InvokeRuntime(r),
		aicommon.WithToolCaller_RuntimeId(r.config.Id),
		aicommon.WithToolCaller_Emitter(childEmitter),
		aicommon.WithToolCaller_Task(task),
		aicommon.WithToolCaller_CallToolID(work.call.ExecutionCallID),
		aicommon.WithToolCaller_Reason(work.call.Reason),
		aicommon.WithToolCaller_CallExpectations(work.call.Expectations),
		aicommon.WithToolCaller_DestinationIdentifier(work.call.Identifier),
		aicommon.WithToolCaller_ParamGenerationGate(paramGate),
		aicommon.WithToolCaller_ReviewGate(reviewGate),
		aicommon.WithToolCaller_BeforeInvoke(beforeInvoke),
		aicommon.WithToolCaller_CheckpointSeq(work.checkpointSeq),
		aicommon.WithToolCaller_ParamTransactionSeq(work.paramSeq),
		aicommon.WithToolCaller_ReviewCheckpointSeq(work.reviewSeq),
		aicommon.WithToolCaller_WatcherCheckpointSeq(work.watcherSeq),
		aicommon.WithToolCaller_ResultID(work.resultID),
		aicommon.WithToolCaller_ArtifactOrdinal(work.artifactOrdinal),
		aicommon.WithToolCaller_BatchMetadata(work.batchID, work.call.Index),
		aicommon.WithToolCaller_StatsSource(statsSource),
		aicommon.WithToolCaller_ReviewWrongTool(func(
			callCtx context.Context,
			tool *aitool.Tool,
			newToolName string,
			keyword string,
		) (*aitool.Tool, bool, error) {
			return r._invokeToolCall_ReviewWrongToolForTask(callCtx, task, tool, newToolName, keyword)
		}),
		aicommon.WithToolCaller_ReviewWrongParam(func(
			callCtx context.Context,
			tool *aitool.Tool,
			old aitool.InvokeParams,
			extraPrompt string,
		) (aitool.InvokeParams, error) {
			return r._invokeToolCall_ReviewWrongParamForTask(callCtx, task, tool, old, extraPrompt)
		}),
	}
	if paramMutator != nil {
		opts = append(opts, aicommon.WithToolCaller_ParamAugmentForTool(paramMutator))
	}
	if !r.config.DisableIntervalReview {
		if intervalHandler := r.CreateIntervalReviewHandlerForTaskAndEmitter(task, childEmitter); intervalHandler != nil {
			opts = append(opts, aicommon.WithToolCaller_IntervalReviewHandler(intervalHandler))
			if r.config.IntervalReviewDuration > 0 {
				opts = append(opts, aicommon.WithToolCaller_IntervalReviewDuration(r.config.IntervalReviewDuration))
			}
		}
	}
	// Direct proposals normally arrive with preset params, but a manual
	// wrong_tool decision recursively enters CallTool for the replacement. Keep
	// the real builder on every batch child so that path generates parameters
	// exactly like the scalar runtime instead of failing after review.
	opts = append(opts,
		aicommon.WithToolCaller_GenerateToolParamsBuilderWithMeta(
			func(tool *aitool.Tool, toolName string) (*aicommon.ToolParamsPromptMeta, error) {
				// PromptManager owns mutable dynamic rendering state. Only prompt
				// construction is serialized; the expensive AI transactions run
				// under the independent parameter-generation semaphore.
				promptMu.Lock()
				defer promptMu.Unlock()
				return r.generateToolParamsPromptWithMetaForTask(task, tool, toolName)
			},
		),
	)
	return aicommon.NewToolCaller(ctx, opts...)
}

// ExecuteToolBatch executes one directly_call_tool/require_tool array. It never
// reads or swaps ReAct.currentTask and never mutates task.Emitter. Child events
// use immutable derived emitters, while task/timeline results are committed by
// this coordinator in model order after every child has settled.
func (r *ReAct) ExecuteToolBatch(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	request *aicommon.ToolBatchRequest,
) (*aicommon.ToolBatchResult, error) {
	if r == nil || r.config == nil {
		return nil, fmt.Errorf("tool batch runtime is not initialized")
	}
	if request == nil {
		return nil, fmt.Errorf("tool batch request is nil")
	}
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}
	if task == nil {
		task = r.config.DefaultTask
	}
	if task == nil {
		return nil, fmt.Errorf("tool batch task is nil")
	}
	if task.GetContext() != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		stopTaskCancel := context.AfterFunc(task.GetContext(), cancel)
		defer func() {
			stopTaskCancel()
			cancel()
		}()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	maxCalls := r.config.GetConfigInt(
		aicommon.ConfigKeyToolBatchMaxCalls,
		aicommon.DefaultToolBatchMaxCalls,
	)
	if maxCalls < 2 {
		maxCalls = 2
	}
	if maxCalls > aicommon.DefaultToolBatchMaxCalls {
		maxCalls = aicommon.DefaultToolBatchMaxCalls
	}
	if len(request.Calls) < 2 {
		return nil, fmt.Errorf("tool batch calls must contain at least 2 items")
	}
	if len(request.Calls) > maxCalls {
		return nil, fmt.Errorf("tool batch contains %d calls, maximum is %d", len(request.Calls), maxCalls)
	}

	// Always reserve the batch seed, even when a caller supplies BatchID, so all
	// later preallocated sequence IDs retain the same positions. On checkpoint
	// recovery a freshly parsed request starts at the same runtime sequence and
	// derives byte-identical batch/call identities.
	batchSeedSeq := r.config.AcquireId()
	if request.BatchID == "" {
		request.BatchID = stableToolBatchID(
			"tool-batch",
			r.config.GetRuntimeId(), ":", batchSeedSeq,
		)
	}
	result := &aicommon.ToolBatchResult{
		BatchID:  request.BatchID,
		Outcomes: make([]aicommon.ToolCallOutcome, len(request.Calls)),
	}
	works := make([]toolBatchWork, len(request.Calls))
	baseArtifactOrdinal := len(task.GetAllToolCallResults())
	loop := toolBatchLoop(task)
	directAdmissionFailed := false

	// Resolve and preflight the complete direct batch before emitting any card
	// or starting any callback. Mutators receive private copies and run in model
	// order, so a failure guarantees zero tool execution for the entire batch.
	for i := range request.Calls {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		call := request.Calls[i]
		call.Index = i
		if call.ExecutionCallID == "" {
			call.ExecutionCallID = stableToolBatchID(
				"tool-call",
				request.BatchID, ":", i,
			)
		}
		request.Calls[i].Index = i
		request.Calls[i].ExecutionCallID = call.ExecutionCallID
		work := toolBatchWork{
			call:            call,
			batchID:         request.BatchID,
			paramSeq:        r.config.AcquireId(),
			reviewSeq:       r.config.AcquireId(),
			checkpointSeq:   r.config.AcquireId(),
			watcherSeq:      r.config.AcquireId(),
			resultID:        r.config.AcquireId(),
			artifactOrdinal: baseArtifactOrdinal + i + 1,
		}
		result.Outcomes[i] = aicommon.ToolCallOutcome{
			Index:         i,
			CallID:        call.ExecutionCallID,
			RequestedTool: call.ToolName,
			Stage:         aicommon.ToolCallStageQueued,
		}
		if call.Mode != aicommon.ToolCallModeDirect && call.Mode != aicommon.ToolCallModeRequire {
			result.Outcomes[i].Stage = aicommon.ToolCallStageValidationFailed
			result.Outcomes[i].Err = fmt.Errorf("unsupported tool call mode %q", call.Mode)
			directAdmissionFailed = true
			works[i] = work
			continue
		}
		tool, resolveErr := r.resolveToolForCall(ctx, call.ToolName)
		if resolveErr != nil {
			result.Outcomes[i].Stage = aicommon.ToolCallStagePrepareFailed
			result.Outcomes[i].Err = resolveErr
			if call.Mode == aicommon.ToolCallModeDirect {
				directAdmissionFailed = true
			}
			works[i] = work
			continue
		}
		work.tool = tool
		if loop != nil {
			guardParams := aitool.InvokeParams(nil)
			if call.Mode == aicommon.ToolCallModeDirect {
				guardParams = call.Params
			}
			if allow, guardMessage := reactloops.CheckToolInvokeGuard(loop, call.ToolName, guardParams); !allow {
				result.Outcomes[i].Stage = aicommon.ToolCallStageValidationFailed
				result.Outcomes[i].Err = utils.Error(guardMessage)
				if call.Mode == aicommon.ToolCallModeDirect {
					directAdmissionFailed = true
				}
				works[i] = work
				continue
			}
		}
		if call.Mode == aicommon.ToolCallModeDirect {
			work.params = cloneToolBatchParams(call.Params)
			if loop != nil {
				if err := ctx.Err(); err != nil {
					return result, err
				}
				work.params = reactloops.ApplyToolInvokeParamsMutators(loop, call.ToolName, work.params)
			}
			valid, validationErrors := tool.ValidateParams(work.params)
			if !valid {
				result.Outcomes[i].Stage = aicommon.ToolCallStageValidationFailed
				result.Outcomes[i].Err = fmt.Errorf(
					"tool %q params failed validation: %v",
					call.ToolName,
					validationErrors,
				)
				directAdmissionFailed = true
			}
		}
		works[i] = work
	}

	if directAdmissionFailed {
		for i := range result.Outcomes {
			if result.Outcomes[i].Err == nil {
				result.Outcomes[i].Stage = aicommon.ToolCallStageCancelled
				result.Outcomes[i].Err = fmt.Errorf("tool batch admission failed before execution")
			}
		}
		return result, nil
	}

	paramConcurrency := r.config.GetConfigInt(
		aicommon.ConfigKeyToolBatchParamConcurrency,
		aicommon.DefaultToolBatchParamConcurrency,
	)
	invokeConcurrency := r.config.GetConfigInt(
		aicommon.ConfigKeyToolBatchInvokeConcurrency,
		aicommon.DefaultToolBatchInvokeConcurrency,
	)
	paramGate := toolBatchSemaphoreGate(paramConcurrency)
	invokeGate := toolBatchSemaphoreGate(invokeConcurrency)
	barrier := newToolBatchBarrier(len(works))
	orderedMutators := newOrderedBatchStage(len(works))
	orderedReviews := newOrderedBatchStage(len(works))
	baseEmitter := r.batchTaskEmitter(task)
	promptMu := new(sync.Mutex)
	// A direct-answer review is a batch-wide terminal decision. Keep this
	// cancellation separate from the caller/task context: it stops siblings
	// immediately, but ExecuteToolBatch still returns the settled batch result
	// rather than an external context error.
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()

	if r.config != nil {
		r.config.RunVerificationWatchdogToolBlockingStart()
		defer r.config.RunVerificationWatchdogToolBlockingEnd()
	}

	var wg sync.WaitGroup
	for i := range works {
		i := i
		work := &works[i]
		if work.call.Mode == aicommon.ToolCallModeDirect {
			orderedMutators.complete(i)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				// Review can terminate through direct_answer before beforeInvoke.
				// Always release this child's ordered mutator turn on every exit.
				orderedMutators.complete(i)
				// Failures and direct-answer returns can terminate before the final
				// invoke boundary; they must still hand the ordered review turn to
				// the next model-array child.
				orderedReviews.complete(i)
				if result.Outcomes[i].DirectlyAnswer {
					// Publish the direct-answer bit before cancellation so children
					// already waiting at the final barrier cannot mistake this for an
					// ordinary cancellation and enter the invoke gate.
					barrier.arrive(i, true)
					cancelWorkers()
					return
				}
				barrier.arrive(i, false)
			}()

			// Preflight may reject a require child while still having resolved its
			// Tool (for example a loop ToolInvokeGuard veto). Preserve that settled
			// outcome and do not let the worker generate params, request approval,
			// invoke the plugin, or overwrite the rejection with a later result.
			if result.Outcomes[i].Err != nil {
				return
			}
			if work.tool == nil {
				// A require-tool manager race is all-settled: a missing child does
				// not stop valid siblings, but it still participates in the barrier.
				return
			}
			var paramMutator func(*aitool.Tool, aitool.InvokeParams) aitool.InvokeParams
			if work.call.Mode == aicommon.ToolCallModeRequire {
				paramMutator = func(currentTool *aitool.Tool, params aitool.InvokeParams) aitool.InvokeParams {
					if _, err := orderedMutators.wait(workerCtx, i); err != nil || loop == nil || currentTool == nil {
						return params
					}
					return reactloops.ApplyToolInvokeParamsMutators(loop, currentTool.Name, params)
				}
			} else if loop != nil {
				// Direct calls were already admission-mutated before any card or
				// callback was allowed. Skip that first application; recursive review
				// proposals must be mutated once for their *current* (possibly changed)
				// tool rather than inheriting the originally requested tool's mutator.
				firstProposal := true
				paramMutator = func(currentTool *aitool.Tool, params aitool.InvokeParams) aitool.InvokeParams {
					if firstProposal {
						firstProposal = false
						return params
					}
					if workerCtx.Err() != nil || currentTool == nil {
						return params
					}
					return reactloops.ApplyToolInvokeParamsMutators(loop, currentTool.Name, params)
				}
			}
			beforeInvoke := func(
				callCtx context.Context,
				_ *aitool.Tool,
				_ aitool.InvokeParams,
			) (func(), error) {
				// This child has finished every possible review (including recursive
				// wrong-tool/wrong-param review) and can safely expose the next
				// array index's approval card.
				orderedMutators.complete(i)
				orderedReviews.complete(i)
				if err := callCtx.Err(); err != nil {
					return nil, err
				}
				return barrier.wait(callCtx, i, invokeGate)
			}
			reviewGate := func(callCtx context.Context) (func(), error) {
				return orderedReviews.wait(callCtx, i)
			}
			caller, callerErr := r.newToolCallerForBatchCall(
				workerCtx,
				task,
				work,
				baseEmitter,
				paramGate,
				reviewGate,
				beforeInvoke,
				paramMutator,
				promptMu,
			)
			if callerErr != nil {
				result.Outcomes[i].Stage = aicommon.ToolCallStagePrepareFailed
				result.Outcomes[i].Err = callerErr
				return
			}

			var toolResult *aitool.ToolResult
			var directlyAnswer bool
			var callErr error
			if work.call.Mode == aicommon.ToolCallModeDirect {
				toolResult, directlyAnswer, callErr = caller.CallToolWithExistedParams(
					work.tool,
					true,
					work.params,
				)
			} else {
				toolResult, directlyAnswer, callErr = caller.CallTool(work.tool)
			}

			outcome := &result.Outcomes[i]
			outcome.Result = toolResult
			outcome.DirectlyAnswer = directlyAnswer
			outcome.Err = callErr
			if toolResult != nil {
				outcome.FinalTool = toolResult.Name
				if toolResult.GetID() <= 0 {
					toolResult.ID = work.resultID
				}
			}
			switch {
			case directlyAnswer:
				outcome.Stage = aicommon.ToolCallStageCancelled
			case workerCtx.Err() != nil:
				outcome.Stage = aicommon.ToolCallStageCancelled
				outcome.Err = workerCtx.Err()
			case callErr != nil:
				outcome.Stage = toolBatchStageForError(callErr)
			case toolResult == nil:
				outcome.Stage = aicommon.ToolCallStageInvokeFailed
				outcome.Err = fmt.Errorf("tool %q returned no result", work.call.ToolName)
			case !toolResult.Success:
				outcome.Stage = aicommon.ToolCallStageInvokeFailed
				if toolResult.Error != "" {
					outcome.Err = errors.New(toolResult.Error)
				}
			default:
				outcome.Stage = aicommon.ToolCallStageDone
			}
		}()
	}
	wg.Wait()

	// Commit shared state once, in input order. Workers only own their indexed
	// outcome slot and immutable child emitter, avoiding task/timeline races.
	for i := range result.Outcomes {
		outcome := &result.Outcomes[i]
		if outcome.DirectlyAnswer {
			result.DirectlyAnswer = true
		}
		if outcome.Result == nil || outcome.Stage == aicommon.ToolCallStageCancelled {
			continue
		}
		task.PushToolCallResult(outcome.Result)
		r.config.Timeline.PushToolResult(outcome.Result)
		r.EmitInfo("Tool batch child completed: %s", outcome.Result.Name)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, nil
}
