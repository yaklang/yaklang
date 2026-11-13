package aicommon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// ReviewToolCall prepares an explicit proposal and blocks until the review
// decision is final. It never invokes the tool.
func (t *ToolCaller) ReviewToolCall(tool *aitool.Tool, invokeParams aitool.InvokeParams) (*ToolCallReviewResult, error) {
	if t == nil {
		return nil, fmt.Errorf("tool caller is nil")
	}
	if tool == nil {
		return nil, fmt.Errorf("tool is required")
	}
	if invokeParams != nil {
		invokeParams = cloneEndpointParams(invokeParams)
	}
	if t.emitter == nil {
		emitter := t.config.GetEmitter()
		if emitter == nil {
			return nil, fmt.Errorf("no emitter found in ToolCaller")
		}
		t.emitter = emitter
	}

	t.start.Do(func() { t.emitStart(tool) })
	if strings.TrimSpace(t.reason) != "" {
		t.emitReason(t.reason)
	} else {
		t.generateReasonIfNeeded(tool, nil)
	}
	t.emitter.EmitInfo("start explicit tool[%v] call in task: %v", tool.Name, t.task.GetName())

	var destinationIdentifier string
	if id, ok := invokeParams[ReservedKeyIdentifier]; ok {
		destinationIdentifier = sanitizeIdentifier(utils.InterfaceToString(id))
		delete(invokeParams, ReservedKeyIdentifier)
	}
	if ce, ok := invokeParams[ReservedKeyCallExpectations]; ok {
		t.callExpectations = utils.InterfaceToString(ce)
		delete(invokeParams, ReservedKeyCallExpectations)
	}
	if destinationIdentifier != "" {
		t.emitter.EmitInfo("tool[%v] destination identifier: %v", tool.Name, destinationIdentifier)
	}
	t.emitter.EmitInfo("use explicit params for tool[%v]: %v", tool.Name, invokeParams)
	if t.destinationIdentifier != "" {
		destinationIdentifier = t.destinationIdentifier
	}
	if invokeParams == nil {
		err := &ToolCallRetryError{
			ToolName: tool.Name,
			Reason:   "explicit parameters are missing; use an object, including {} for a parameterless tool",
		}
		t.finishToolCallError(err)
		return nil, err
	}
	if t.paramAugmentForTool != nil {
		invokeParams = t.paramAugmentForTool(tool, invokeParams)
	} else if t.paramAugment != nil {
		invokeParams = t.paramAugment(invokeParams)
	}
	if allow, feedback := CheckAttachedBrowserToolRoute(t.task, tool.Name); !allow {
		t.finishToolCallError(feedback)
		return nil, utils.Error(feedback)
	}
	invokeParams, err := BindAttachedBrowserToolParams(t.task, tool.Name, invokeParams)
	if err != nil {
		t.finishToolCallError(err)
		return nil, err
	}
	if err := toolCallerContextErr(t.ctx); err != nil {
		t.finishToolCallError(err)
		return nil, err
	}

	if tool.NoNeedUserReview {
		t.emitter.EmitInfo("tool[%v] (internal helper tool) no need user review, skip review", tool.Name)
		return &ToolCallReviewResult{
			Tool:                  tool,
			Params:                cloneEndpointParams(invokeParams),
			DestinationIdentifier: destinationIdentifier,
			caller:                t,
		}, nil
	}

	releaseReview := func() {}
	if t.reviewGate != nil {
		var gateErr error
		releaseReview, gateErr = t.reviewGate(t.ctx)
		releaseReview = idempotentToolCallerRelease(releaseReview)
		if gateErr != nil {
			t.finishToolCallError(gateErr)
			return nil, gateErr
		}
		if gateErr = toolCallerContextErr(t.ctx); gateErr != nil {
			releaseReview()
			t.finishToolCallError(gateErr)
			return nil, gateErr
		}
	}
	defer releaseReview()

	t.emitter.EmitInfo("start to require review for tool use: %v", tool.Name)
	reviewCheckpointSeq := t.reviewCheckpointSeq
	t.reviewCheckpointSeq = 0
	ep := t.config.GetEndpointManager().CreateEndpointWithEventTypeAndSeq(
		schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE,
		reviewCheckpointSeq,
	)
	ep.SetDefaultSuggestionContinue()
	reqs := map[string]any{
		"id":               ep.GetId(),
		"selectors":        ToolUseReviewSuggestions,
		"tool":             tool.Name,
		"tool_description": tool.Description,
		"params":           invokeParams,
		"reason":           t.reason,
	}
	if t.batchID != "" {
		reqs["batch_id"] = t.batchID
		reqs["call_index"] = t.batchIndex
		reqs["call_tool_id"] = t.callToolId
	}
	ep.SetReviewMaterials(reqs)

	reviewCheckpoint := ep.GetCheckpoint()
	if reviewCheckpoint.RequestQuotedJson != "" {
		stored := aiddb.AiCheckPointGetRequestParams(reviewCheckpoint)
		storedParams := stored.GetObject("params")
		identityMismatch := stored.GetString("tool") != tool.Name ||
			string(utils.Jsonify(storedParams)) != string(utils.Jsonify(invokeParams))
		if t.batchID != "" {
			storedIndex, hasStoredIndex := stored["call_index"]
			identityMismatch = identityMismatch ||
				stored.GetString("batch_id") != t.batchID ||
				!hasStoredIndex ||
				utils.InterfaceToInt(storedIndex) != t.batchIndex ||
				stored.GetString("call_tool_id") != t.callToolId
		}
		if identityMismatch {
			reviewErr := utils.Errorf(
				"tool review checkpoint identity mismatch: batch=%s index=%d call=%s tool=%s params=%s; stored batch=%s index=%d call=%s tool=%s params=%s",
				t.batchID,
				t.batchIndex,
				t.callToolId,
				tool.Name,
				utils.Jsonify(invokeParams),
				stored.GetString("batch_id"),
				utils.InterfaceToInt(stored["call_index"]),
				stored.GetString("call_tool_id"),
				stored.GetString("tool"),
				utils.Jsonify(storedParams),
			)
			t.finishToolCallError(reviewErr)
			return nil, reviewErr
		}
	} else if reviewCheckpoint.Finished {
		reviewErr := utils.Errorf(
			"finished tool review checkpoint has no request identity: seq=%d",
			reviewCheckpoint.Seq,
		)
		t.finishToolCallError(reviewErr)
		return nil, reviewErr
	}

	reviewQuestion := fmt.Sprintf("determite tool[%v]'s params is proper? what should I do?", tool.Name)
	var reviewParams aitool.InvokeParams
	if reviewCheckpoint.Finished {
		reviewParams = aiddb.AiCheckPointGetResponseParams(reviewCheckpoint)
	} else {
		if err := t.config.SubmitCheckpointRequest(reviewCheckpoint, reqs); err != nil {
			t.finishToolCallError(err)
			return nil, err
		}
		t.emitter.EmitInteractiveJSON(ep.GetId(), schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE, "review-require", reqs)
		t.config.DoWaitAgree(t.ctx, ep)
		if waitErr := toolCallerContextErr(t.ctx); waitErr != nil {
			releaseReview()
			// The worker can stop while its enclosing session stays alive.
			// Close the visible card so the next pending review can proceed.
			t.emitter.EmitInteractiveRelease(ep.GetId(), nil)
			t.finishToolCallError(waitErr)
			return nil, waitErr
		}
		reviewDecidedAt := time.Now()
		reviewParams = ep.GetParams()
		t.emitter.EmitInteractiveRelease(ep.GetId(), reviewParams)
		t.config.CallAfterInteractiveEventReleased(ep.GetId(), reviewParams)
		if !isFastNoopContinueReview(ep, reviewParams, reviewDecidedAt) {
			t.config.CallAfterReview(ep.GetSeq(), reviewQuestion, reviewParams)
		}
		if reviewParams != nil {
			if err := t.config.SubmitCheckpointResponse(reviewCheckpoint, reviewParams); err != nil {
				releaseReview()
				t.finishToolCallError(err)
				return nil, err
			}
		}
	}

	releaseReview()
	if reviewParams == nil {
		err := fmt.Errorf("tool use [%v] review params is nil", tool.Name)
		t.emitter.EmitError("tool use [%v] review params is nil, user may cancel the review", tool.Name)
		t.finishToolCallError(err)
		return nil, err
	}

	reviewedTool, reviewedParams, _, next, err := t.review(
		tool,
		invokeParams,
		reviewParams,
		t.finishToolCallCancelled,
	)
	if err != nil {
		var reconsider *ToolReviewReconsiderError
		if !errors.As(err, &reconsider) {
			t.emitter.EmitError("error handling tool use review: %v", err)
		}
		t.finishToolCallError(err)
		return nil, err
	}
	return &ToolCallReviewResult{
		Tool:                  reviewedTool,
		Params:                cloneEndpointParams(reviewedParams),
		DestinationIdentifier: destinationIdentifier,
		DirectlyAnswer:        next == HandleToolUseNext_DirectlyAnswer,
		caller:                t,
	}, nil
}

func (t *ToolCaller) finishToolCallCancelled(reason any) {
	if t == nil || t.m == nil || t.done == nil {
		return
	}
	t.done.Do(func() {
		t.m.Lock()
		defer t.m.Unlock()
		if t.emitter != nil {
			endTime := time.Now()
			t.emitter.EmitToolCallStatus(
				t.callToolId,
				fmt.Sprintf("%s by reason: %v", schema.TOOL_CALL_STATUS_CANCELLED, reason),
			)
			t.emitter.EmitToolCallUserCancel(t.callToolId, endTime, t.startTime, 0)
		}
		if t.onCallToolEnd != nil {
			t.onCallToolEnd(t.callToolId)
		}
	})
	t.finalizeToolCall(nil)
}

func (t *ToolCaller) finishToolCallError(err any) {
	if t == nil || t.m == nil || t.done == nil {
		return
	}
	t.done.Do(func() {
		t.m.Lock()
		defer t.m.Unlock()
		if t.emitter != nil {
			t.emitter.EmitToolCallError(t.callToolId, err, time.Now(), t.startTime, 0)
		}
		if t.onCallToolEnd != nil {
			t.onCallToolEnd(t.callToolId)
		}
	})
	t.finalizeToolCall(nil)
}

// ExecuteReviewedToolCall owns the invocation phase only. Review, parameter
// preparation and checkpoints have already completed.
func (t *ToolCaller) ExecuteReviewedToolCall(review *ToolCallReviewResult) (result *aitool.ToolResult, directlyAnswer bool, err error) {
	if t == nil || review == nil || review.caller != t || review.Tool == nil || review.Params == nil {
		return nil, false, fmt.Errorf("an approved tool review from this caller is required")
	}
	if review.DirectlyAnswer {
		return nil, true, nil
	}
	t.m.Lock()
	if t.executionStarted {
		t.m.Unlock()
		return nil, false, fmt.Errorf("tool caller has already started execution")
	}
	t.executionStarted = true
	t.m.Unlock()

	defer func() { t.finalizeToolCall(result) }()
	tool := review.Tool
	invokeParams := cloneEndpointParams(review.Params)
	destinationIdentifier := review.DestinationIdentifier
	callToolId := t.callToolId

	var pluginInvokeStartTime time.Time
	var pluginInvokeDuration time.Duration
	getPurePluginDuration := func(endTime time.Time) time.Duration {
		if pluginInvokeStartTime.IsZero() {
			return 0
		}
		if pluginInvokeDuration > 0 {
			return pluginInvokeDuration
		}
		if endTime.IsZero() {
			endTime = time.Now()
		}
		if endTime.Before(pluginInvokeStartTime) {
			return 0
		}
		return endTime.Sub(pluginInvokeStartTime)
	}
	handleDone := func() {
		t.done.Do(func() {
			t.m.Lock()
			defer t.m.Unlock()
			endTime := time.Now()
			t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_DONE)
			t.emitter.EmitToolCallDone(callToolId, endTime, t.startTime, getPurePluginDuration(endTime))
			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}
	handleUserCancel := func(reason any) {
		t.done.Do(func() {
			t.m.Lock()
			defer t.m.Unlock()
			endTime := time.Now()
			t.emitter.EmitToolCallStatus(
				t.callToolId,
				fmt.Sprintf("%s by reason: %v", schema.TOOL_CALL_STATUS_CANCELLED, reason),
			)
			t.emitter.EmitToolCallUserCancel(callToolId, endTime, t.startTime, getPurePluginDuration(endTime))
			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}
	handleError := func(err any) {
		t.done.Do(func() {
			t.m.Lock()
			defer t.m.Unlock()
			endTime := time.Now()
			t.emitter.EmitToolCallError(callToolId, err, endTime, t.startTime, getPurePluginDuration(endTime))
			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}
	defer handleDone()

	releaseInvoke := func() {}
	if t.beforeInvoke != nil {
		var hookErr error
		releaseInvoke, hookErr = t.beforeInvoke(t.ctx, tool, invokeParams)
		releaseInvoke = idempotentToolCallerRelease(releaseInvoke)
		if hookErr != nil {
			handleError(hookErr)
			return nil, false, hookErr
		}
		if hookErr = toolCallerContextErr(t.ctx); hookErr != nil {
			releaseInvoke()
			handleError(hookErr)
			return nil, false, hookErr
		}
	}
	defer releaseInvoke()
	if ctxErr := toolCallerContextErr(t.ctx); ctxErr != nil {
		handleError(ctxErr)
		return nil, false, ctxErr
	}

	stdoutReader, stdoutWriter := utils.NewPipe()
	defer stdoutWriter.Close()
	stderrReader, stderrWriter := utils.NewPipe()
	defer stderrWriter.Close()

	stdoutBuffer := &toolOutputBuffer{}
	stderrBuffer := &toolOutputBuffer{}
	t.bindInspectionOutput(stdoutBuffer, stderrBuffer)

	artifactBundle := t.newToolCallArtifactBundle(tool, callToolId, destinationIdentifier)
	defer artifactBundle.discardIfUnfinished()

	stdoutUIWriter := newBoundedToolUIWriter(stdoutWriter)
	stderrUIWriter := newBoundedToolUIWriter(stderrWriter)
	stdoutMultiWriter := io.MultiWriter(artifactBundle.Writer(artifactStdout), stdoutUIWriter, stdoutBuffer)
	stderrMultiWriter := io.MultiWriter(artifactBundle.Writer(artifactStderr), stderrUIWriter, stderrBuffer)

	waitToolStdFlush := t.emitter.EmitToolCallStd(tool.Name, stdoutReader, stderrReader, t.task.GetIndex())

	if buildinaitools.IsMCPToolName(tool.Name) {
		if mgr := t.config.GetAiToolManager(); mgr != nil {
			waitCtx := t.ctx
			if waitCtx == nil {
				waitCtx = t.config.GetContext()
			}
			liveTool, waitErr := buildinaitools.WaitForMCPLiveTool(
				waitCtx,
				mgr,
				tool.Name,
				buildinaitools.MCPToolInitWaitTimeout,
				buildinaitools.MCPToolInitPollInterval,
				func(elapsed time.Duration) {
					t.emitter.EmitInfo(
						"MCP tool %q still connecting (elapsed %v), waiting for remote server before invoke...",
						tool.Name,
						elapsed.Round(time.Second),
					)
				},
			)
			if waitErr != nil {
				handleError(waitErr)
				return nil, false, waitErr
			}
			if liveTool != nil {
				tool = liveTool
			}
		}
	}

	t.emitter.EmitInfo("start to invoke tool: %v", tool.Name)
	t.emitter.EmitToolCallStatus(callToolId, schema.TOOL_CALL_STATUS_RUNNING)
	if ctxErr := toolCallerContextErr(t.ctx); ctxErr != nil {
		handleError(ctxErr)
		return nil, false, ctxErr
	}
	t.m.Lock()
	pluginInvokeStartTime = time.Now()
	pluginInvokeDuration = 0
	t.m.Unlock()

	t.emitter.EmitToolCallParam(callToolId, invokeParams)
	if ctxErr := toolCallerContextErr(t.ctx); ctxErr != nil {
		handleError(ctxErr)
		return nil, false, ctxErr
	}
	var outcomeMu sync.Mutex
	var cancellationReason, invocationError any
	toolResult, err := t.invoke(
		tool,
		invokeParams,
		func(reason any) {
			outcomeMu.Lock()
			cancellationReason = reason
			outcomeMu.Unlock()
		},
		func(err any) {
			outcomeMu.Lock()
			invocationError = err
			outcomeMu.Unlock()
		},
		stdoutMultiWriter,
		stderrMultiWriter,
		stdoutBuffer,
		stderrBuffer,
		func(result *aitool.ToolResult) error {
			return artifactBundle.finalize(t, tool, callToolId, destinationIdentifier, invokeParams, result)
		},
	)
	t.m.Lock()
	if !pluginInvokeStartTime.IsZero() && pluginInvokeDuration <= 0 {
		pluginInvokeDuration = time.Since(pluginInvokeStartTime)
	}
	t.m.Unlock()

	if err != nil {
		if toolResult == nil {
			toolResult = &aitool.ToolResult{
				Param:       invokeParams,
				Name:        tool.Name,
				Description: tool.Description,
				ToolCallID:  callToolId,
			}
		}
		if toolResult.Error == "" {
			toolResult.Error = fmt.Sprintf("error invoking tool[%v]: %v", tool.Name, err)
		}
		toolResult.Success = false
		enforceCanonicalToolResultLimit(toolResult)
	}
	if toolResult != nil {
		toolResult.OmitParamsInTimeline = t.omitResultParamsInTimeline
		if t.resultID > 0 {
			toolResult.ID = t.resultID
		}
	}

	if finishErr := stdoutUIWriter.Finish(); finishErr != nil {
		log.Warnf("failed to flush bounded stdout tail: %v", finishErr)
	}
	if finishErr := stderrUIWriter.Finish(); finishErr != nil {
		log.Warnf("failed to flush bounded stderr tail: %v", finishErr)
	}
	stdoutWriter.Close()
	stderrWriter.Close()
	waitToolStdFlush()

	t.emitter.EmitInfo("start to generate and feedback tool[%v] result in task: %#v", tool.Name, t.task.GetName())
	if toolResult != nil && toolResult.Data != nil {
		t.emitter.EmitToolCallResult(callToolId, toolResult.Data)
	}

	outcomeMu.Lock()
	cancelReason, reportedError := cancellationReason, invocationError
	outcomeMu.Unlock()
	if cancelReason != nil || errors.Is(err, context.Canceled) {
		if cancelReason == nil {
			cancelReason = err
		}
		handleUserCancel(cancelReason)
	} else if err != nil {
		handleError(err)
	} else if reportedError != nil {
		handleError(reportedError)
	}

	if !t.checkpointReplayed && (t.ctx == nil || t.ctx.Err() == nil) {
		NotifySessionSnapshotToolCall(t.config, toolResult, t.statsSource)
	}
	return toolResult, false, nil
}
